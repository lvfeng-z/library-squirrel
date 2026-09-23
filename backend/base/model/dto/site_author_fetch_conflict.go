package dto

// SiteAuthorFetchConflict 站点作者信息拉取的候选冲突载荷：该站点的候选多于一个且本次触发对该
// 站点未显选时随响应交回调用方（此时未调用任何插件），调用方发起选择后带显选重发。
// 候选按站点收窄，故一组冲突恒对应一个站点键（SiteKey）；Candidates 为该站点的候选清单
// （插件 × siteAuthorFetch 条目对，ExtensionId 携带条目 id），按候选全键（插件公开 ID 与
// 条目 id 拼接）字典序，首位即默认选中项。
type SiteAuthorFetchConflict struct {
	Conflict   bool               `json:"conflict"`
	SiteKey    string             `json:"siteKey"`
	Candidates []*PluginCandidate `json:"candidates"`
}

// SiteAuthorFetchChoice 交互面显选：某站点键下用户点名的拉取来源。站点不同则候选集不同，
// 故显选按站点键逐站给出。PluginPublicId 与 ExtensionId 两键联合定位候选集内的一个条目
// （ExtensionId 缺省时该插件在该站点候选集内须恰有一个条目，由编排侧解析补全为该条目）
type SiteAuthorFetchChoice struct {
	SiteKey        string `json:"siteKey"`
	PluginPublicId string `json:"pluginPublicId"`
	ExtensionId    string `json:"extensionId"`
	// Remember 记住此选择（冲突弹窗勾选态随显选带回）：该站点本次拉取成功后把显选落
	// 粘性记忆，同站点同候选组合的后续冲突按记忆直接路由不再询问；缺省 false 不落表。
	// 仅手动交互面携带本字段——自动触发面不经冲突显选，恒不写记忆
	Remember bool `json:"remember"`
}
