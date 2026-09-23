package dto

// PluginCandidate 插件触发式功能的候选路由原子：一个扩展点（插件 × 扩展点 ID）。
// ExtensionId 恒为清单条目 id（两个消费面均按 (插件, 条目) 全键取用：任务创建面取
// workFetch 条目 id、站点作者拉取面取 siteAuthorFetch 条目 id），空串不再表示
// 「插件级单服务」特例——该特例随契约 v11 的扩展点实例化一并终结。
// 两个消费面（任务创建、站点作者拉取）同形同义共用本类型。
type PluginCandidate struct {
	PluginPublicId string `json:"pluginPublicId"`
	PluginName     string `json:"pluginName"`
	ExtensionId    string `json:"extensionId"`
}
