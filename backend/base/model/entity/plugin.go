package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// Plugin 插件
type Plugin struct {
	*model.BaseEntity                     // 嵌入基础实体
	PublicID               sql.NullString `gorm:"column:public_id;uniqueIndex" json:"publicId"`
	Author                 sql.NullString `gorm:"column:author" json:"author"`
	Name                   sql.NullString `gorm:"column:name" json:"name"`
	Version                sql.NullString `gorm:"column:version" json:"version"`
	ContractVersion        sql.NullInt64  `gorm:"column:contract_version" json:"contractVersion"`
	ConfigSchemaVersion    sql.NullInt64  `gorm:"column:config_schema_version" json:"configSchemaVersion"` // 插件声明的配置 schema 版本（plugin.json configSchemaVersion；0=legacy/未管理）
	Capabilities           sql.NullString `gorm:"column:capabilities" json:"capabilities"`
	ResourceTypes          sql.NullString `gorm:"column:resource_types" json:"resourceTypes"` // 插件自定义资源类型声明(JSON,来自 manifest resourceTypes 段;声明 resourceTypeProvider 通行证时注册进 Registry)
	Description            sql.NullString `gorm:"column:description" json:"description"`
	Changelog              sql.NullString `gorm:"column:changelog" json:"changelog"`
	EntryPath              sql.NullString `gorm:"column:entry_path" json:"entryPath"`
	RootPath               sql.NullString `gorm:"column:root_path" json:"rootPath"`
	BackupID               sql.NullInt64  `gorm:"column:backup_id" json:"backupId"`
	SortNum                sql.NullInt64  `gorm:"column:sort_num" json:"sortNum"`
	Uninstalled            sql.NullBool   `gorm:"column:uninstalled" json:"uninstalled"`
	ActivationType         sql.NullString `gorm:"column:activation_type" json:"activationType"`
	Source                 sql.NullString `gorm:"column:source" json:"source"`                                    // 来源枚举 bundled/local/url/marketplace，由主程序按安装入口判定（不由插件声明）
	SourceDetail           sql.NullString `gorm:"column:source_detail" json:"sourceDetail"`                       // 来源详情（安装包路径或 URL），供追溯
	BuildID                sql.NullString `gorm:"column:build_id" json:"buildId"`                                 // 构建身份标识（构建管线注入 git describe 输出；同源码状态永远同值，升级判据与静态资产缓存键以此判同；历史记录为 NULL）
	UpgradeDeclinedBuildID sql.NullString `gorm:"column:upgrade_declined_build_id" json:"upgradeDeclinedBuildId"` // 用户拒绝升级的目标 buildId（「跳过此构建」持久化；与捆绑包 buildId 等值时检测静默跳过，新 buildId 到来自动失效；重装全字段覆盖自然清零）
	Trusted                sql.NullBool   `gorm:"column:trusted" json:"trusted"`                                  // 信任标记：bundled=true，第三方经用户确认后 true；false 则不运行（运行门控）
	Official               sql.NullBool   `gorm:"column:official" json:"official"`                                // 官方身份：内容摘要命中主程序携带的官方指纹名单（locked_config.yaml）为 true；NULL/false=未证实。与渠道（source）和信任（trusted）均正交，恒不接入信任/运行门控
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
