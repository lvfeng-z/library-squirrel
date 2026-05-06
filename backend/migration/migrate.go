package migration

import (
	entity2 "github.com/library-squirrel/backend/base/model/entity"

	"gorm.io/gorm"
)

// AutoMigrate 执行数据库自动迁移
func AutoMigrate(db *gorm.DB) error {
	// 定义所有需要迁移的模型（按依赖顺序排列）
	models := []interface{}{
		// 基础表（无外键依赖）
		&entity2.Backup{},
		entity2.NewLocalAuthor(),
		&entity2.Site{},
		entity2.NewSiteTag(),
		&entity2.WorkSet{},
		entity2.NewWork(),
		entity2.NewSiteAuthor(),
		&entity2.Poi{},
		&entity2.SecureStorage{},
		&entity2.Plugin{},
		&entity2.Task{},
		&entity2.Resource{},

		// 关联表（有外键依赖）
		&entity2.ReWorkAuthor{},
		&entity2.ReWorkTag{},
		&entity2.ReWorkWorkSet{},
		&entity2.RePoiTarget{},

		// 本地标签（独立表）
		entity2.NewLocalTag(),
	}

	// 执行自动迁移
	if err := db.AutoMigrate(models...); err != nil {
		return err
	}

	return nil
}
