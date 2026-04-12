package database

import (
	"fmt"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	gormDB *gorm.DB
	once   sync.Once
)

// Init 初始化数据库连接（GORM + mattn/go-sqlite3）
func Init(path string) error {
	var err error
	once.Do(func() {
		// 打开 SQLite 连接（GORM 自动使用 mattn/go-sqlite3）
		gormDB, err = gorm.Open(sqlite.Open(path+"?_journal_mode=WAL&_synchronous=NORMAL"), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent), // 关闭 GORM 日志
		})
		if err != nil {
			err = fmt.Errorf("failed to open database: %w", err)
			return
		}

		// 获取底层 sql.DB 设置连接池
		sqlDB, err := gormDB.DB()
		if err != nil {
			err = fmt.Errorf("failed to get underlying DB: %w", err)
			return
		}

		// 设置连接池参数
		sqlDB.SetMaxOpenConns(100) // GORM 建议的最大连接数
		sqlDB.SetMaxIdleConns(5)
	})
	return err
}

// GetDB 获取 GORM 数据库实例
func GetDB() *gorm.DB {
	return gormDB
}

// Close 关闭数据库连接
func Close() error {
	if gormDB != nil {
		sqlDB, err := gormDB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
