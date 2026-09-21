package dto

// SiteAuthorFetchConflict 站点作者信息拉取的候选冲突载荷：该站点的候选多于一个且本次触发对该
// 站点未显选时随响应交回调用方（此时未调用任何插件），调用方发起选择后带显选重发。
// 候选按站点收窄，故一组冲突恒对应一个站点键（SiteKey）；Candidates 为该站点的候选清单，
// 按插件标识字典序，首位即默认选中项。
type SiteAuthorFetchConflict struct {
	Conflict   bool               `json:"conflict"`
	SiteKey    string             `json:"siteKey"`
	Candidates []*PluginCandidate `json:"candidates"`
}

// SiteAuthorFetchChoice 交互面显选：某站点键下用户点名的插件。站点不同则候选集不同，
// 故显选按站点键逐站给出。
type SiteAuthorFetchChoice struct {
	SiteKey        string `json:"siteKey"`
	PluginPublicId string `json:"pluginPublicId"`
}
