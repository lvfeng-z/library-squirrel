package database

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	pkglogger "github.com/library-squirrel/backend/base/logger"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	gormDB *gorm.DB
	once   sync.Once
)

// Init 初始化数据库连接（GORM + mattn/go-sqlite3）
func Init(path string) error {
	// 确保数据库文件所在目录存在
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	var err error
	once.Do(func() {
		// 打开 SQLite 连接（GORM 自动使用 mattn/go-sqlite3）
		gormDB, err = gorm.Open(sqlite.Open(path+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000"), &gorm.Config{
			Logger: pkglogger.NewGormLogger(), // GORM SQL 日志输出到 zap
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
		sqlDB.SetMaxOpenConns(1)  // SQLite 单文件数据库仅支持单写者，MaxOpenConns=1 让 Go 连接池做排队
		sqlDB.SetMaxIdleConns(1)
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
