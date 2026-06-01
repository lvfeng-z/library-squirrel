package database

import (
	"context"

	"gorm.io/gorm"
)

// TxFunc 事务执行函数
type TxFunc func(tx *gorm.DB) error

// WithTransaction 执行事务
func WithTransaction(db *gorm.DB, fn TxFunc) error {
	return db.Transaction(fn)
}

// WithTransactionContext 带context的事务执行
func WithTransactionContext(ctx context.Context, db *gorm.DB, fn TxFunc) error {
	return db.WithContext(ctx).Transaction(fn)
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
	return t.db.WithContext(ctx).Transaction(fn)
}
