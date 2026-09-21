package dto

// PluginCandidate 插件触发式功能的候选路由原子：一个扩展点（插件 × 扩展点 ID）。
// ExtensionId 为空串表示该消费面为插件级单服务（插件内单扩展点特例）。
// 两个消费面（任务创建、站点作者拉取）同形同义共用本类型。
type PluginCandidate struct {
	PluginPublicId string `json:"pluginPublicId"`
	PluginName     string `json:"pluginName"`
	ExtensionId    string `json:"extensionId"`
}
