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
	Source            sql.NullString `gorm:"column:source" json:"source"`               // 来源枚举 bundled/local/url/marketplace，由主程序按安装入口判定（不由插件声明）
	SourceDetail      sql.NullString `gorm:"column:source_detail" json:"sourceDetail"`  // 来源详情（安装包路径或 URL），供追溯
	IntegrityHash     sql.NullString `gorm:"column:integrity_hash" json:"integrityHash"` // 包级 SHA256(hex)，安装时对原始 zip 计算，纯追溯存档
	Trusted           sql.NullBool   `gorm:"column:trusted" json:"trusted"`             // 信任标记：bundled=true，第三方经用户确认后 true；false 则不运行（运行门控）
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
