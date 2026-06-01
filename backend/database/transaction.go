package database

import (
	"context"
	"runtime"
	"time"

	pkglogger "github.com/library-squirrel/backend/base/logger"
	"gorm.io/gorm"
)

// TxFunc 事务执行函数
type TxFunc func(tx *gorm.DB) error

// WithTransaction 执行事务
func WithTransaction(db *gorm.DB, fn TxFunc) error {
	return withTransactionLog(db, fn)
}

// WithTransactionContext 带context的事务执行
func WithTransactionContext(ctx context.Context, db *gorm.DB, fn TxFunc) error {
	return withTransactionLog(db.WithContext(ctx), fn)
}

// Transaction 事务封装
type Transaction struct {
	db *gorm.DB
}

// NewTransaction 创建事务封装
func NewTransaction(db *gorm.DB) *Transaction {
	return &Transaction{db: db}
}

// Exec 执行事务
func (t *Transaction) Exec(ctx context.Context, fn TxFunc) error {
	return withTransactionLog(t.db.WithContext(ctx), fn)
}

// withTransactionLog 包装事务执行，记录开始/结束日志和耗时
func withTransactionLog(db *gorm.DB, fn TxFunc) error {
	// 跳过 getStackTrace → withTransactionLog → WithTransaction(等) → 实际调用方
	caller := getCaller(3)
	pkglogger.Log.Infof("[DB] 事务开始: caller=%s", caller)
	start := time.Now()
	err := db.Transaction(fn)
	elapsed := time.Since(start)
	if err != nil {
		pkglogger.Log.Errorf("[DB] 事务失败: caller=%s, elapsed=%v, err=%v", caller, elapsed, err)
	} else {
		pkglogger.Log.Infof("[DB] 事务完成: caller=%s, elapsed=%v", caller, elapsed)
	}
	return err
}

// getCaller 获取调用方函数名，skip 为栈帧偏移
func getCaller(skip int) string {
	pc, _, _, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}
	return runtime.FuncForPC(pc).Name()
}
