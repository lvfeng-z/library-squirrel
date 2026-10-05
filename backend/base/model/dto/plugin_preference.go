package dto

// PluginPreferenceEntryDTO 插件偏好管理条目：记忆管理页（按插件分组）与插件设置区
// （只读列表）共用的展示形态——值信封拆出的标题/描述 + 条目身份与归属插件显示信息。
// 管理面只读展示 + 删除，无编辑
type PluginPreferenceEntryDTO struct {
	ID       int64  `json:"id"`
	PluginID int64  `json:"pluginId"`
	// PluginPublicId 归属插件公开 ID（前端定位插件/分组的稳定标识）
	PluginPublicId string `json:"pluginPublicId"`
	// PluginName 归属插件显示名（plugin 表 name 列）
	PluginName string `json:"pluginName"`
	// PrefKey 插件自定义键
	PrefKey string `json:"prefKey"`
	// Title 偏好标题（值信封携带；空值时组装方回落偏好键，管理面恒有可读文本）
	Title string `json:"title"`
	// Description 偏好描述（值信封携带，管理面次级说明文本）
	Description string `json:"description"`
	UpdateTime  int64  `json:"updateTime"`
}
