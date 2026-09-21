package dto

// SiteAuthorFetchConflict 站点作者信息拉取的多候选冲突载荷：候选多于一个且本次触发未显选时
// 随响应交回调用方（此时未调用任何插件），调用方发起选择后带显选插件重发。
// Candidates 为候选清单，按插件标识字典序，首位即默认选中项。
type SiteAuthorFetchConflict struct {
	Conflict   bool               `json:"conflict"`
	Candidates []*PluginCandidate `json:"candidates"`
}
