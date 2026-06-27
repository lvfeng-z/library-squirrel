package migration

import (
	"fmt"

	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
)

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

	// 数据搬入:现有 Resource 的 WorkStoreID/ThumbnailStoreID 迁入 resource_store(幂等;旧列保留,供现有消费端继续使用)
	if err := migrateResourceStores(db); err != nil {
		return err
	}

	return nil
}

// migrateResourceStores 将现有 Resource 的固定列(WorkStoreID/ThumbnailStoreID)
// 迁入 resource_store 关联表。幂等:resource_store 已有数据则跳过。
func migrateResourceStores(db *gorm.DB) error {
	var count int64
	if err := db.Model(&entity2.ResourceStore{}).Count(&count).Error; err != nil {
		return fmt.Errorf("查询 resource_store 数量失败: %w", err)
	}
	if count > 0 {
		return nil
	}

	var resources []*entity2.Resource
	if err := db.Find(&resources).Error; err != nil {
		return fmt.Errorf("查询 resource 失败: %w", err)
	}

	now := util.GetCurrentTimestamp()
	var stores []*entity2.ResourceStore
	for _, r := range resources {
		if r.WorkStoreID.Valid {
			s := entity2.NewResourceStore()
			s.BaseEntity.CreateTime = now
			s.BaseEntity.UpdateTime = now
			s.ResourceID = r.GetID()
			s.StoreType = entity2.StoreTypeMain
			s.Generation = entity2.GenerationDownloaded
			s.StoreID = r.WorkStoreID.Int64
			stores = append(stores, s)
		}
		if r.ThumbnailStoreID.Valid {
			s := entity2.NewResourceStore()
			s.BaseEntity.CreateTime = now
			s.BaseEntity.UpdateTime = now
			s.ResourceID = r.GetID()
			s.StoreType = entity2.StoreTypeThumbnail
			s.Generation = entity2.GenerationDerived
			s.StoreID = r.ThumbnailStoreID.Int64
			stores = append(stores, s)
		}
	}

	if len(stores) > 0 {
		if err := db.Create(&stores).Error; err != nil {
			return fmt.Errorf("写入 resource_store 失败: %w", err)
		}
	}
	return nil
}
