package dto

// StickyMemoryCandidateDTO 粘性记忆条目里的候选展示项：记忆键内候选全键拆解出的
// 插件/条目身份加上解析出的显示名。条目显示名唯一来源 = 清单条目声明（workFetch /
// siteAuthorFetch 段条目的 name 字段），插件未加载（停用/卸载）取不到时回落原始条目 id，
// 不因显示名缺失阻塞管理列表
type StickyMemoryCandidateDTO struct {
	PluginPublicId string `json:"pluginPublicId"`
	PluginName     string `json:"pluginName"`
	ExtensionId    string `json:"extensionId"`
	ExtensionName  string `json:"extensionName"`
	// DisplayName 条目展示名：清单条目显示名优先，取不到回落条目 id
	DisplayName string `json:"displayName"`
}

// StickyMemoryEntryDTO 粘性记忆管理列表条目：记忆行的上下文键与显选值拆解为站点域、
// 候选名单与当前选中者后的富化展示形态（管理面只读展示 + 删除，无编辑）
type StickyMemoryEntryDTO struct {
	ID          int64                       `json:"id"`
	Domain      string                      `json:"domain"`
	DomainLabel string                      `json:"domainLabel"`
	// SiteDomain 记忆归属的站点域（交互面判定粒度：清单声明的站点键或回落任务 URL host）
	SiteDomain string                      `json:"siteDomain"`
	// Candidates 该条记忆对应的候选组合（触发冲突时的全部候选，按记忆键内序）
	Candidates []*StickyMemoryCandidateDTO `json:"candidates"`
	// Selected 用户显选并记住的候选（显示名解析规则同候选名单；候选不在名单内时仅含身份字段）
	Selected   *StickyMemoryCandidateDTO  `json:"selected"`
	CreateTime int64                       `json:"createTime"`
	UpdateTime int64                       `json:"updateTime"`
}
