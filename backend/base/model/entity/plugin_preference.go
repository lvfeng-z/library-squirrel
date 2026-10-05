package entity

import "github.com/library-squirrel/backend/base/model"

// PluginPreference 插件偏好记忆行：插件经问答获得用户决策后代存的宿主侧记忆。
// 用户在管理面可见可删，删除后插件读取即无记录、下次问答重新发起。
// (plugin_id, pref_key) 复合唯一——一个插件一个键只保留一条现值，重写为整值覆写。
// 域边界：与 plugin_storage（插件配置 KV，数据主人=插件）正交，本表数据主人=用户的决策
type PluginPreference struct {
	*model.BaseEntity
	// PluginID 归属插件 DB id（外键 plugin 表）
	PluginID int64 `gorm:"column:plugin_id;uniqueIndex:idx_plugin_preference_plugin_key;not null" json:"pluginId"`
	// PrefKey 插件自定义键（键粒度由插件自决：按内容类型、按目录模式或全局均可）
	PrefKey string `gorm:"column:pref_key;uniqueIndex:idx_plugin_preference_plugin_key;not null" json:"prefKey"`
	// Value 偏好值 JSON 文本，信封结构 {schemaVersion, title, description, data}
	//（title/description 供管理面展示决策内容，data 为插件自定义负载、宿主不解释）
	Value string `gorm:"column:value;not null" json:"value"`
}

// NewPluginPreference 创建插件偏好记忆行（工厂方法，禁止 &PluginPreference{}）
func NewPluginPreference() *PluginPreference {
	return &PluginPreference{BaseEntity: &model.BaseEntity{}}
}

// TableName 指定表名
func (PluginPreference) TableName() string {
	return "plugin_preference"
}
