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
	Description       sql.NullString `gorm:"column:description" json:"description"`
	Changelog         sql.NullString `gorm:"column:changelog" json:"changelog"`
	EntryPath         sql.NullString `gorm:"column:entry_path" json:"entryPath"`
	RootPath          sql.NullString `gorm:"column:root_path" json:"rootPath"`
	BackupID          sql.NullInt64  `gorm:"column:backup_id" json:"backupId"`
	SortNum           sql.NullInt64  `gorm:"column:sort_num" json:"sortNum"`
	PluginData        sql.NullString `gorm:"column:plugin_data" json:"pluginData"`
	Uninstalled       sql.NullInt64  `gorm:"column:uninstalled" json:"uninstalled"`
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
