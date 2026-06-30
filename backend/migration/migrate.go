package migration

import (
	"fmt"

	entity2 "github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/gorm"
)

// resource 旧固定列(work_store_id / thumbnail_store_id)的原始列定义,用于数据搬入与删除。
// 实体已移除这两列,无法用 ORM 结构映射,故用 raw 结构描述。
type resourceLegacyColumns struct {
	ID               int64  `gorm:"column:id"`
	WorkStoreID      *int64 `gorm:"column:work_store_id"`
	ThumbnailStoreID *int64 `gorm:"column:thumbnail_store_id"`
}

func (resourceLegacyColumns) TableName() string { return "resource" }

// AutoMigrate 执行数据库自动迁移
func AutoMigrate(db *gorm.DB) error {
	// 命名迁移:task.plugin_contribution_id → plugin_extension_id(contribution→extension 命名统一)
	// 须在 AutoMigrate 前执行,否则 AutoMigrate 会先新建 plugin_extension_id 列导致 RenameColumn 冲突
	if db.Migrator().HasColumn(&entity2.Task{}, "plugin_contribution_id") {
		if !db.Migrator().HasColumn(&entity2.Task{}, "plugin_extension_id") {
			if err := db.Migrator().RenameColumn(&entity2.Task{}, "plugin_contribution_id", "plugin_extension_id"); err != nil {
				return fmt.Errorf("迁移 task.plugin_contribution_id → plugin_extension_id 失败: %w", err)
			}
		}
	}

	// 定义所有需要迁移的模型(按依赖顺序排列)
	models := []interface{}{
		// 基础表(无外键依赖)
		&entity2.Backup{},
		entity2.NewLocalAuthor(),
		&entity2.Site{},
		entity2.NewSiteTag(),
		&entity2.WorkSet{},
		entity2.NewWork(),
		entity2.NewSiteAuthor(),
		&entity2.Poi{},
		&entity2.Plugin{},
		&entity2.PluginStorage{},
		&entity2.Task{},
		&entity2.Resource{},
		&entity2.ResourceStore{},
		entity2.NewPersistentStore(),

		// 关联表(有外键依赖)
		&entity2.ReWorkAuthor{},
		&entity2.ReWorkTag{},
		&entity2.ReWorkWorkSet{},
		&entity2.RePoiTarget{},

		// 回收站(作品逻辑删除快照,独立表)
		entity2.NewRecycleItem(),

		// 本地标签(独立表)
		entity2.NewLocalTag(),
	}

	// 执行自动迁移
	if err := db.AutoMigrate(models...); err != nil {
		return err
	}

	// 数据搬入:resource 旧固定列(work_store_id/thumbnail_store_id)迁入 resource_store,然后删除旧列。
	// 必须在 AutoMigrate 之后(此时旧列若存在仍保留在表中,实体已不含这两列)。
	if err := migrateResourceLegacyColumns(db); err != nil {
		return err
	}

	return nil
}

// migrateResourceLegacyColumns 处理 resource 表的旧固定列:
// 1. 若旧列存在且有数据,先把数据搬入 resource_store(幂等:resource_store 已存在该 resourceId+storeType 则跳过)
// 2. 搬入后删除旧列(DROP COLUMN,SQLite 3.35+ 支持)
// 实体已不含这两列,故用 raw SQL / Migrator 列名操作。
func migrateResourceLegacyColumns(db *gorm.DB) error {
	if !db.Migrator().HasTable(&resourceLegacyColumns{}) {
		// 表不存在(全新库),无需迁移
		return nil
	}
	if !db.Migrator().HasColumn(&resourceLegacyColumns{}, "work_store_id") &&
		!db.Migrator().HasColumn(&resourceLegacyColumns{}, "thumbnail_store_id") {
		// 旧列已删除,无需迁移
		return nil
	}

	// 1. 读旧列数据
	var legacies []resourceLegacyColumns
	if err := db.Select("id, work_store_id, thumbnail_store_id").Find(&legacies).Error; err != nil {
		return fmt.Errorf("查询 resource 旧固定列失败: %w", err)
	}

	// 2. 搬入 resource_store(按 resourceId 分组,已有同 resourceId 行则补齐缺失的 store_type)
	// 先查现有 resource_store 行,避免重复插入
	type existingKey struct {
		ResourceID int64
		StoreType  string
	}
	existingSet := make(map[existingKey]struct{})
	var existingStores []*entity2.ResourceStore
	if err := db.Find(&existingStores).Error; err != nil {
		return fmt.Errorf("查询现有 resource_store 失败: %w", err)
	}
	for _, s := range existingStores {
		existingSet[existingKey{ResourceID: s.ResourceID, StoreType: s.StoreType}] = struct{}{}
	}

	var toCreate []entity2.ResourceStore
	for _, lg := range legacies {
		if lg.WorkStoreID != nil && *lg.WorkStoreID > 0 {
			k := existingKey{ResourceID: lg.ID, StoreType: entity2.StoreTypeMain}
			if _, ok := existingSet[k]; !ok {
				toCreate = append(toCreate, entity2.ResourceStore{
					BaseEntity: nil, // 让 repository 的 Save 填充时间戳;直接 Create 需手动
					ResourceID: lg.ID,
					StoreType:  entity2.StoreTypeMain,
					Generation: entity2.GenerationDownloaded,
					StoreID:    *lg.WorkStoreID,
				})
				existingSet[k] = struct{}{}
			}
		}
		if lg.ThumbnailStoreID != nil && *lg.ThumbnailStoreID > 0 {
			k := existingKey{ResourceID: lg.ID, StoreType: entity2.StoreTypeThumbnail}
			if _, ok := existingSet[k]; !ok {
				toCreate = append(toCreate, entity2.ResourceStore{
					ResourceID: lg.ID,
					StoreType:  entity2.StoreTypeThumbnail,
					Generation: entity2.GenerationDerived,
					StoreID:    *lg.ThumbnailStoreID,
				})
				existingSet[k] = struct{}{}
			}
		}
	}

	if len(toCreate) > 0 {
		if err := db.Create(&toCreate).Error; err != nil {
			return fmt.Errorf("写入 resource_store 失败: %w", err)
		}
	}

	// 3. 删除旧列(DROP COLUMN)
	if db.Migrator().HasColumn(&resourceLegacyColumns{}, "work_store_id") {
		if err := db.Migrator().DropColumn(&resourceLegacyColumns{}, "work_store_id"); err != nil {
			return fmt.Errorf("删除 resource.work_store_id 列失败: %w", err)
		}
	}
	if db.Migrator().HasColumn(&resourceLegacyColumns{}, "thumbnail_store_id") {
		if err := db.Migrator().DropColumn(&resourceLegacyColumns{}, "thumbnail_store_id"); err != nil {
			return fmt.Errorf("删除 resource.thumbnail_store_id 列失败: %w", err)
		}
	}

	return nil
}
