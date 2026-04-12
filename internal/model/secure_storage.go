package model

import (
	"database/sql"

	"github.com/library-squirrel/wails/pkg/model"
)

// SecureStorage 安全存储
type SecureStorage struct {
	*model.BaseEntity                // 嵌入基础实体
	ID                int64          `gorm:"primaryKey;column:id" json:"id"`
	StorageKey        sql.NullString `gorm:"column:storage_key;uniqueIndex" json:"storageKey"`
	EncryptedValue    sql.NullString `gorm:"column:encrypted_value" json:"encryptedValue"`
	Description       sql.NullString `gorm:"column:description" json:"description"`
	CreateTime        int64          `gorm:"column:create_time" json:"createTime"`
	UpdateTime        int64          `gorm:"column:update_time" json:"updateTime"`
}

func (SecureStorage) TableName() string {
	return "secure_storage"
}
