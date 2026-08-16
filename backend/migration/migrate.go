package migration

import (
	"fmt"

	entity2 "github.com/library-squirrel/backend/base/model/entity"

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

	// 命名迁移:resource_store.order_idx → store_seq(列语义由排序提示正名为 store 稳定身份)
	// 须在 AutoMigrate 前执行,否则 AutoMigrate 会先新建 store_seq 列导致 RenameColumn 冲突
	if db.Migrator().HasColumn(&entity2.ResourceStore{}, "order_idx") {
		if !db.Migrator().HasColumn(&entity2.ResourceStore{}, "store_seq") {
			if err := db.Migrator().RenameColumn(&entity2.ResourceStore{}, "order_idx", "store_seq"); err != nil {
				return fmt.Errorf("迁移 resource_store.order_idx → store_seq 失败: %w", err)
			}
		}
	}

	// 数据迁移：re_work_author 去重并升级索引（普通联合 index → 唯一索引）
	// 旧 schema 用普通联合 index（idx_re_work_author_local_author_id / idx_re_work_author_site_author_id），
	// 升级为 (work_id, local_author_id) / (work_id, site_author_id) 唯一索引，使 LOCAL 关联增量入库可借 OnConflict DoNothing 去重。
	// 须在 AutoMigrate 前执行：清理历史重复行（否则建唯一索引失败），并删除旧普通索引（AutoMigrate 不删旧索引，改名后会与新唯一索引冗余共存）。
	// SQLite 唯一索引中 NULL 不参与唯一性：LOCAL 关联 site_author_id=NULL、SITE 关联 local_author_id=NULL，两类互不冲突。
	if db.Migrator().HasIndex(&entity2.ReWorkAuthor{}, "idx_re_work_author_local_author_id") {
		// 清理 LOCAL 重复行（每组 work_id+local_author_id 保留最早 id）
		if err := db.Exec(`DELETE FROM re_work_author WHERE local_author_id IS NOT NULL AND id NOT IN (SELECT MIN(id) FROM re_work_author WHERE local_author_id IS NOT NULL GROUP BY work_id, local_author_id)`).Error; err != nil {
			return fmt.Errorf("迁移 re_work_author LOCAL 去重失败: %w", err)
		}
		// 清理 SITE 重复行（每组 work_id+site_author_id 保留最早 id）
		if err := db.Exec(`DELETE FROM re_work_author WHERE site_author_id IS NOT NULL AND id NOT IN (SELECT MIN(id) FROM re_work_author WHERE site_author_id IS NOT NULL GROUP BY work_id, site_author_id)`).Error; err != nil {
			return fmt.Errorf("迁移 re_work_author SITE 去重失败: %w", err)
		}
		// 删除旧普通索引（改名后 AutoMigrate 不会自动删除）
		if err := db.Migrator().DropIndex(&entity2.ReWorkAuthor{}, "idx_re_work_author_local_author_id"); err != nil {
			return fmt.Errorf("迁移 re_work_author 删除旧 LOCAL 索引失败: %w", err)
		}
		if err := db.Migrator().DropIndex(&entity2.ReWorkAuthor{}, "idx_re_work_author_site_author_id"); err != nil {
			return fmt.Errorf("迁移 re_work_author 删除旧 SITE 索引失败: %w", err)
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
		entity2.NewFsmonitorCursor(),

		// 关联表(有外键依赖)
		&entity2.ReWorkAuthor{},
		&entity2.ReWorkTag{},
		&entity2.ReWorkWorkSet{},
		entity2.NewReWorkSetWorkSet(),
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

	return nil
}
