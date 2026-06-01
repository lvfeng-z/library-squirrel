package database

import (
	"context"

	"gorm.io/gorm"
)

// txKeyType 事务 context key 类型（不导出，防止外部直接操作）
type txKeyType struct{}

// TxKey 用于在 context 中存取 *gorm.DB 事务实例的 key
var TxKey = txKeyType{}

// DBFromContext 从 context 中获取事务 DB，无事务时返回 defaultDB
func DBFromContext(ctx context.Context, defaultDB *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(TxKey).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return defaultDB
}
