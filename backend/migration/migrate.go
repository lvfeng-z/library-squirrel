package migration

import (
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/dto"
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

	// 数据迁移：publicId 身份键收敛为纯插件 id（旧格式 author/id，id 可含 UUID 后缀）。
	// 捆绑插件安装（InstallBundledPlugins）按 publicId 查已装记录判升级，新键先于安装落库
	// 才能命中存量记录、复用原记录 ID——plugin_storage 按 plugin_id 关联用户数据，记录换 ID 即孤儿
	if err := migratePluginPublicId(db); err != nil {
		return fmt.Errorf("迁移 plugin.public_id 身份简化失败: %w", err)
	}

	return nil
}

// migratePluginPublicId 一次性数据迁移：把旧格式 publicId（author/id[_uuid]）改写为纯插件 id，
// 同步改写 task.plugin_public_id（任务按 publicId 引用插件、解析执行器）。
// root_path/entry_path 不动：bundled 记录随后的升级重装会写为新路径，local 记录沿旧目录激活（激活不比对 manifest id）
func migratePluginPublicId(db *gorm.DB) error {
	var plugins []entity2.Plugin
	if err := db.Where("public_id LIKE ?", "%/%").Find(&plugins).Error; err != nil {
		return err
	}
	if len(plugins) == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, p := range plugins {
			old := p.PublicID.String
			fresh := deriveNewPublicId(old)
			// 派生失败（id 段为空或整体被后缀剥离耗尽）的异常记录不处理
			if fresh == "" {
				logger.Log.Warnf("publicId 迁移跳过（无法派生新身份键）: %s", old)
				continue
			}
			// 目标身份键已被占用（旧主程序配新插件包回滚后再升级会造出双记录）：跳过并告警，交由人工处置
			var conflict int64
			if err := tx.Model(&entity2.Plugin{}).Where("public_id = ?", fresh).Count(&conflict).Error; err != nil {
				return err
			}
			if conflict > 0 {
				logger.Log.Warnf("publicId 迁移跳过（目标身份键已存在）: %s -> %s", old, fresh)
				continue
			}
			if err := tx.Model(&entity2.Plugin{}).Where("id = ?", p.GetID()).Update("public_id", fresh).Error; err != nil {
				return err
			}
			if err := tx.Model(&entity2.Task{}).Where("plugin_public_id = ?", old).Update("plugin_public_id", fresh).Error; err != nil {
				return err
			}
			logger.Log.Infof("publicId 已迁移: %s -> %s", old, fresh)
		}
		return nil
	})
}

// deriveNewPublicId 从旧格式 publicId 派生新身份键：去掉 author 段，再剥离 id 末尾的旧身份 UUID 后缀。
// 派生结果须为纯 id（反向域名不含 "/"）；空或残留 "/" 视为记录数据异常，返回空串交由迁移跳过并告警
func deriveNewPublicId(old string) string {
	idx := strings.Index(old, "/")
	if idx < 0 {
		return ""
	}
	id := domain.StripLegacyUUIDSuffix(old[idx+1:])
	if id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}
