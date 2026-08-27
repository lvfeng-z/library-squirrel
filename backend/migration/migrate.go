package migration

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
	entity2 "github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/gorm"
)

// AutoMigrate 执行数据库自动迁移
func AutoMigrate(db *gorm.DB) error {
	// 命名迁移(前置)：persistent_store 软删机制引入——status 改名改型 completed_at（0/1 → 完成时刻毫秒，1 转 create_time 近似）、
	// invalid_at 退役并入 deleted_at（外部裁决不复从改走软删；deleted_at 先手工加列，AutoMigrate 见列已存在不重复建）
	var pcStatusCol int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('persistent_store') WHERE name = 'status'").Scan(&pcStatusCol).Error; err != nil {
		return fmt.Errorf("迁移检查 persistent_store.status 列失败: %w", err)
	}
	if pcStatusCol > 0 {
		if err := db.Exec(`ALTER TABLE persistent_store ADD COLUMN completed_at INTEGER DEFAULT 0`).Error; err != nil {
			return fmt.Errorf("迁移 persistent_store 加 completed_at 列失败: %w", err)
		}
		if err := db.Exec(`UPDATE persistent_store SET completed_at = CASE WHEN status = 1 THEN create_time ELSE 0 END`).Error; err != nil {
			return fmt.Errorf("迁移 persistent_store status 值转换失败: %w", err)
		}
		if err := db.Exec(`ALTER TABLE persistent_store DROP COLUMN status`).Error; err != nil {
			return fmt.Errorf("迁移 persistent_store 删 status 列失败: %w", err)
		}
	}
	var pcInvalidCol int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('persistent_store') WHERE name = 'invalid_at'").Scan(&pcInvalidCol).Error; err != nil {
		return fmt.Errorf("迁移检查 persistent_store.invalid_at 列失败: %w", err)
	}
	if pcInvalidCol > 0 {
		if err := db.Exec(`ALTER TABLE persistent_store ADD COLUMN deleted_at INTEGER DEFAULT 0`).Error; err != nil {
			return fmt.Errorf("迁移 persistent_store 加 deleted_at 列失败: %w", err)
		}
		if err := db.Exec(`UPDATE persistent_store SET deleted_at = invalid_at WHERE invalid_at > 0`).Error; err != nil {
			return fmt.Errorf("迁移 persistent_store invalid_at 并入 deleted_at 失败: %w", err)
		}
		if err := db.Exec(`ALTER TABLE persistent_store DROP COLUMN invalid_at`).Error; err != nil {
			return fmt.Errorf("迁移 persistent_store 删 invalid_at 列失败: %w", err)
		}
	}

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
		&entity2.ShareRecord{},
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

		// 本地标签(独立表)
		entity2.NewLocalTag(),
	}

	// 执行自动迁移
	if err := db.AutoMigrate(models...); err != nil {
		return err
	}

	// 数据迁移：share-host 任务行退役清理（分享方发布去任务化——发布直跑不经任务模块，
	// 分享参数与生命周期改由 share_record 承载）。存量 share-host 任务行一次性物理删除
	// （原生 DELETE，无软删改写；task 表无软删列；幂等——二次启动无命中行）
	if err := db.Exec(`DELETE FROM task WHERE task_type = 'share-host'`).Error; err != nil {
		return fmt.Errorf("迁移清理 share-host 任务行失败: %w", err)
	}

	// 命名迁移(后置)：persistent_store file_path 唯一索引升级为部分唯一索引（软删行释放路径，
	// 重新下载同路径合法；drop 用 IF EXISTS 幂等——部分库的历史全量索引可能从未建成或已不在，
	// 缺失即跳过直接建新；新索引名存在性做二次启动幂等标记）
	if !db.Migrator().HasIndex(&entity2.PersistentStore{}, "idx_persistent_store_file_path_active") {
		if err := db.Exec(`DROP INDEX IF EXISTS idx_persistent_store_file_path`).Error; err != nil {
			return fmt.Errorf("迁移 persistent_store 旧 file_path 唯一索引删除失败: %w", err)
		}
		if err := db.Exec(`CREATE UNIQUE INDEX idx_persistent_store_file_path_active ON persistent_store(file_path) WHERE deleted_at = 0`).Error; err != nil {
			return fmt.Errorf("迁移 persistent_store file_path 部分唯一索引创建失败: %w", err)
		}
	}

	// 命名迁移(后置)：work 软删列存量回填——AutoMigrate 加列无默认值，存量行 deleted_at 为 NULL，
	// 而软删过滤条件 deleted_at = 0 对 NULL 不命中（NULL=0 为 UNKNOWN），不回填则存量作品全部从查询中消失
	if err := db.Exec(`UPDATE work SET deleted_at = 0 WHERE deleted_at IS NULL`).Error; err != nil {
		return fmt.Errorf("迁移 work.deleted_at 存量回填失败: %w", err)
	}

	// 命名迁移(后置)：persistent_store.deleted_at 存量 NULL 回填（AutoMigrate 加列无默认值遗留 NULL，
	// NULL × deleted_at=0 过滤不命中会让全表从查询中消失）。置于后置段：全新库此处表才存在
	//（前置段时 AutoMigrate 未跑、表未建）
	if err := db.Exec(`UPDATE persistent_store SET deleted_at = 0 WHERE deleted_at IS NULL`).Error; err != nil {
		return fmt.Errorf("迁移 persistent_store.deleted_at 存量回填失败: %w", err)
	}

	// 命名迁移(后置)：persistent_store.backup_id 无引用哨兵由 0 迁移为 NULL
	//（外键对 NULL 豁免对 0 不豁免；含历史加列遗留 NULL 归一，幂等）
	if err := db.Exec(`UPDATE persistent_store SET backup_id = NULL WHERE backup_id = 0`).Error; err != nil {
		return fmt.Errorf("迁移 persistent_store.backup_id 哨兵 NULL 化失败: %w", err)
	}

	// 命名迁移(后置)：backup.file_path 存量分隔符规范化（历史 filepath.Join 构造产反斜杠入库，
	// 与 relPath 域正斜杠基准不符；幂等：规范化后无反斜杠不再命中）
	if err := db.Exec(`UPDATE backup SET file_path = REPLACE(file_path, '\\', '/') WHERE file_path LIKE '%\\%'`).Error; err != nil {
		return fmt.Errorf("迁移 backup.file_path 分隔符规范化失败: %w", err)
	}

	// 命名迁移(后置)：backup.work_id 归属列退役（P0 违规修复：能力包表结构不得有业务实体键，
	// 备份归属改经 original_file_path 反查表达；未发布口径直接删列）
	var backupWorkIdCol int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('backup') WHERE name = 'work_id'").Scan(&backupWorkIdCol).Error; err != nil {
		return fmt.Errorf("迁移检查 backup.work_id 列失败: %w", err)
	}
	if backupWorkIdCol > 0 {
		if err := db.Exec(`DROP INDEX IF EXISTS idx_backup_work_id`).Error; err != nil {
			return fmt.Errorf("迁移删除 backup.work_id 索引失败: %w", err)
		}
		if err := db.Exec(`ALTER TABLE backup DROP COLUMN work_id`).Error; err != nil {
			return fmt.Errorf("迁移删除 backup.work_id 列失败: %w", err)
		}
	}

	// 命名迁移(后置)：backup 能力包纯化——来源三元组（source_type/source_id/original_*）退役，
	// 来源关联内嵌发起方业务行（persistent_store.backup_id / plugin.backup_id 引用保管清单行）。
	// 整段以 source_type 列存在性为幂等标记：搬迁与无主清理须先于 drop 列完成（判别依赖 source_type）
	var backupSourceTypeCol int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('backup') WHERE name = 'source_type'").Scan(&backupSourceTypeCol).Error; err != nil {
		return fmt.Errorf("迁移检查 backup.source_type 列失败: %w", err)
	}
	if backupSourceTypeCol > 0 {
		// 1. 存量搬迁：store 类备份（source_type=3）的清单行 ID 写入对应 persistent_store 行的 backup_id
		//    （仅已软删行承接引用；行已物理删的清单行留在原地，由下方无主清理处置）
		if err := db.Exec(`UPDATE persistent_store SET backup_id = (
			SELECT b.id FROM backup b WHERE b.source_type = 3 AND b.source_id = persistent_store.id LIMIT 1
		) WHERE backup_id = 0 AND deleted_at > 0`).Error; err != nil {
			return fmt.Errorf("迁移 backup_id 存量搬迁失败: %w", err)
		}

		// 2. 无主存量清理（用户裁决直接清理）：清单行不被任何业务列引用——store 类行已物理删、
		//    资源类全链死代码无引用方、插件类未被 plugin.backup_id 引用；文件与记录一并删除
		type orphanBackupRow struct {
			ID       int64
			Workdir  string
			FilePath string
		}
		var orphans []orphanBackupRow
		if err := db.Raw(`
			SELECT id, COALESCE(workdir, '') AS workdir, COALESCE(file_path, '') AS file_path FROM backup
			WHERE NOT EXISTS (SELECT 1 FROM persistent_store ps WHERE ps.backup_id = backup.id)
			  AND NOT EXISTS (SELECT 1 FROM plugin p WHERE p.backup_id = backup.id)
		`).Scan(&orphans).Error; err != nil {
			return fmt.Errorf("迁移查询无主备份失败: %w", err)
		}
		for _, o := range orphans {
			if o.Workdir != "" && o.FilePath != "" {
				if err := os.Remove(filepath.Join(o.Workdir, o.FilePath)); err != nil && !os.IsNotExist(err) {
					logger.Log.Warnf("迁移清理无主备份文件失败（仅删记录）: %s, %v", o.FilePath, err)
				}
			}
		}
		if err := db.Exec(`DELETE FROM backup
			WHERE NOT EXISTS (SELECT 1 FROM persistent_store ps WHERE ps.backup_id = backup.id)
			  AND NOT EXISTS (SELECT 1 FROM plugin p WHERE p.backup_id = backup.id)`).Error; err != nil {
			return fmt.Errorf("迁移清理无主备份记录失败: %w", err)
		}

		// 3. drop 来源列与索引（backup 瘦身为纯保管清单：file_name/file_path/workdir + 时间戳）
		if err := db.Exec(`DROP INDEX IF EXISTS idx_backup_original_file_path`).Error; err != nil {
			return fmt.Errorf("迁移删除 backup.original_file_path 索引失败: %w", err)
		}
		for _, col := range []string{"source_type", "source_id", "original_file_path", "original_file_name", "original_filename_extension"} {
			if err := db.Exec(fmt.Sprintf("ALTER TABLE backup DROP COLUMN %s", col)).Error; err != nil {
				return fmt.Errorf("迁移删除 backup.%s 列失败: %w", col, err)
			}
		}
	}

	// 命名迁移(后置)：work 业务键唯一索引升级为部分唯一索引（软删标志引入后，已删行释放业务键，
	// 删除后可重新下载同作品；AutoMigrate 不管部分索引，drop 旧全量索引 + 建新索引全手写。
	// 以新索引名存在性做幂等标记，二次启动直接跳过）
	if !db.Migrator().HasIndex(&entity2.Work{}, "idx_work_site_site_work_active") {
		if err := db.Exec(`DROP INDEX IF EXISTS idx_work_site_site_work`).Error; err != nil {
			return fmt.Errorf("迁移 work 旧全量唯一索引删除失败: %w", err)
		}
		if err := db.Exec(`CREATE UNIQUE INDEX idx_work_site_site_work_active ON work(site_id, site_work_id) WHERE deleted_at = 0`).Error; err != nil {
			return fmt.Errorf("迁移 work 部分唯一索引创建失败: %w", err)
		}
	}

	// 命名迁移(后置)：work_set 软删列存量回填——AutoMigrate 加列无默认值，存量行 deleted_at 为 NULL，
	// 而软删过滤条件 deleted_at = 0 对 NULL 不命中（NULL=0 为 UNKNOWN），不回填则存量作品集全部从查询中消失
	if err := db.Exec(`UPDATE work_set SET deleted_at = 0 WHERE deleted_at IS NULL`).Error; err != nil {
		return fmt.Errorf("迁移 work_set.deleted_at 存量回填失败: %w", err)
	}

	// 命名迁移(后置)：work_set 业务键唯一索引升级为三列唯一索引（软删行按删除时刻互异释放业务键，
	// 删除后可重新下载同键作品集）。取三列全量形态而非部分索引：ON CONFLICT 带列目标的 upsert
	// 只能匹配无 WHERE 的唯一索引（SQLite 限制），三列形态使 BatchUpsert/Upsert 的单语句原子 upsert 得以保留
	// （冲突目标补 deleted_at 列）。drop 用 IF EXISTS——全新库旧索引从未存在时静默跳过；
	// 以新索引名存在性做幂等标记，二次启动直接跳过
	if !db.Migrator().HasIndex(&entity2.WorkSet{}, "idx_work_set_site_site_set_gen") {
		if err := db.Exec(`DROP INDEX IF EXISTS idx_work_set_site_site_set`).Error; err != nil {
			return fmt.Errorf("迁移 work_set 旧全量唯一索引删除失败: %w", err)
		}
		if err := db.Exec(`CREATE UNIQUE INDEX idx_work_set_site_site_set_gen ON work_set(site_id, site_work_set_id, deleted_at)`).Error; err != nil {
			return fmt.Errorf("迁移 work_set 三列唯一索引创建失败: %w", err)
		}
	}

	// 命名迁移(后置)：封面存储从成员关联行的 is_cover 列迁至作品集自身的 cover_work_id 引用列
	//（封面升格为作品集属性，可指向传递包含内任意作品）。以 is_cover 列存在性为幂等标记：
	// 回填（一集历史多封面行取其一）→ drop 封面唯一索引（SQLite DROP COLUMN 不允许列被索引引用，
	// 须先删索引）→ drop 列
	var rwwsCoverCol int
	if err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('re_work_work_set') WHERE name = 'is_cover'").Scan(&rwwsCoverCol).Error; err != nil {
		return fmt.Errorf("迁移检查 re_work_work_set.is_cover 列失败: %w", err)
	}
	if rwwsCoverCol > 0 {
		if err := db.Exec(`UPDATE work_set SET cover_work_id = (
			SELECT work_id FROM re_work_work_set WHERE work_set_id = work_set.id AND is_cover = 1 LIMIT 1
		) WHERE cover_work_id IS NULL`).Error; err != nil {
			return fmt.Errorf("迁移封面引用回填失败: %w", err)
		}
		if err := db.Exec(`DROP INDEX IF EXISTS idx_re_work_work_set_set_cover`).Error; err != nil {
			return fmt.Errorf("迁移删除封面唯一索引失败: %w", err)
		}
		if err := db.Exec(`ALTER TABLE re_work_work_set DROP COLUMN is_cover`).Error; err != nil {
			return fmt.Errorf("迁移删除 is_cover 列失败: %w", err)
		}
	}

	// 外键声明批次：关联表悬空行清理 + 表重建挂外键。置于全部命名迁移后——
	// 表结构已为最终形态，重建舞步以此为基准复原列与索引
	if err := cleanDanglingAssociations(db); err != nil {
		return err
	}
	if err := ApplyForeignKeys(db); err != nil {
		return err
	}

	return nil
}
