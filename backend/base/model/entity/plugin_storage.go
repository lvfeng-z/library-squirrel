package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// PluginStorage 插件自存信息（统一 KV 存储，取代临时 plugin_data）
// 明文项与加密项共存于单表：加密项 Value 存密文，Encrypted 标记为 true
type PluginStorage struct {
	*model.BaseEntity
	PluginID      int64          `gorm:"column:plugin_id;uniqueIndex:idx_plugin_key" json:"pluginId"`
	Key           string         `gorm:"column:key;uniqueIndex:idx_plugin_key" json:"key"`
	Value         sql.NullString `gorm:"column:value" json:"value"`                  // 明文值 或 密文(base64)；空串是合法取值，用 NullString 区分未设置
	Encrypted     sql.NullBool   `gorm:"column:encrypted" json:"encrypted"`          // true=密文（读取需解密）；false 是合法取值，用 NullBool 区分未设置
	SchemaVersion sql.NullInt64  `gorm:"column:schema_version" json:"schemaVersion"` // 该值写入时的插件配置 schema 版本（host 盖戳；0=legacy/未管理）
}

// NewPluginStorage 创建插件自存信息
func NewPluginStorage() *PluginStorage {
	return &PluginStorage{
		BaseEntity: &model.BaseEntity{},
	}
}

// TableName 指定表名
func (PluginStorage) TableName() string {
	return "plugin_storage"
}
