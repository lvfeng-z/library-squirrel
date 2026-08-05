package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// Plugin 插件
type Plugin struct {
	*model.BaseEntity                // 嵌入基础实体
	PublicID          sql.NullString `gorm:"column:public_id;uniqueIndex" json:"publicId"`
	Author            sql.NullString `gorm:"column:author" json:"author"`
	Name              sql.NullString `gorm:"column:name" json:"name"`
	Version           sql.NullString `gorm:"column:version" json:"version"`
	ContractVersion   sql.NullInt64  `gorm:"column:contract_version" json:"contractVersion"`
	ConfigSchemaVersion sql.NullInt64 `gorm:"column:config_schema_version" json:"configSchemaVersion"` // 插件声明的配置 schema 版本（plugin.json configSchemaVersion；0=legacy/未管理）
	Capabilities      sql.NullString `gorm:"column:capabilities" json:"capabilities"`
	Description       sql.NullString `gorm:"column:description" json:"description"`
	Changelog         sql.NullString `gorm:"column:changelog" json:"changelog"`
	EntryPath         sql.NullString `gorm:"column:entry_path" json:"entryPath"`
	RootPath          sql.NullString `gorm:"column:root_path" json:"rootPath"`
	BackupID          sql.NullInt64  `gorm:"column:backup_id" json:"backupId"`
	SortNum           sql.NullInt64  `gorm:"column:sort_num" json:"sortNum"`
	Uninstalled       sql.NullBool   `gorm:"column:uninstalled" json:"uninstalled"`
	ActivationType    sql.NullString `gorm:"column:activation_type" json:"activationType"`
}

// NewPlugin 创建插件
func NewPlugin() *Plugin {
	return &Plugin{
		BaseEntity: &model.BaseEntity{},
	}
}

// TableName 指定表名
func (Plugin) TableName() string {
	return "plugin"
}
