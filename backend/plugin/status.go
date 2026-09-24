package plugin

import "github.com/library-squirrel/backend/plugin/participation"

// PluginStatusDTO 插件状态
type PluginStatusDTO struct {
	// 生命周期状态（inactive=未激活、activating=激活中、active=运行中、stopping=停用中）
	LifecycleState string `json:"lifecycleState"`
	// 最近一次激活失败原因（空=无；下次激活成功时清空）
	ActivateError string `json:"activateError"`

	// 运行时状态
	IsRunning   bool  `json:"isRunning"`
	PID         int   `json:"pid"`
	ActivatedAt int64 `json:"activatedAt"` // Unix 毫秒，0 表示未激活

	// 扩展点列表
	WorkFetch          []ExtensionInfo         `json:"workFetch"`
	SiteBrowsers       []ExtensionInfo         `json:"siteBrowsers"`
	FrontendExtensions []FrontendExtensionInfo `json:"frontendExtensions"`

	// URL 监听规则
	UrlPatterns []string `json:"urlPatterns"`

	// 参与度概要（真相层序列化快照，每次查询现读内存表；nil = 插件未激活无会话）
	Participation *ParticipationOverview `json:"participation"`
}

// ParticipationOverview 管理页「声明 × 状态」数据面：全部声明条目的当前参与态
// （声明集 ⊕ 覆盖表）与 resolver 求值状态（降级态）
type ParticipationOverview struct {
	// 全部声明条目的当前参与态（point、id 字典序）
	Entries []ParticipationEntryState `json:"entries"`
	// 求值状态（降级态数据面）
	Status ParticipationEvalStatus `json:"status"`
}

// ParticipationEntryState 单个声明条目的当前参与态
type ParticipationEntryState struct {
	// 条目所属派生面（workFetch/siteAuthorFetch/siteBrowsers/resourceTypes/frontendExtensions）
	Point string `json:"point"`
	// 条目 id（资源类型条目 = 类型串）
	ID string `json:"id"`
	// 参与中；false = 被覆盖停用
	Active bool `json:"active"`
	// 覆盖停用时 resolver 给出的理由（参与态或无覆盖为空）
	Reason string `json:"reason"`
}

// ParticipationEvalStatus resolver 求值状态快照（降级态数据面）
type ParticipationEvalStatus struct {
	// 清单是否声明 settingsResolver（false = 该插件无参与度求值，条目全为基线参与）
	HasResolver bool `json:"hasResolver"`
	// 最近一次求值完成时间（Unix 毫秒，0 = 从未求值）
	LastEvalAt int64 `json:"lastEvalAt"`
	// 最近一次求值失败分类（script_load/settings_read/timeout/runtime/output/canceled/internal；空 = 最近一次成功或从未求值）
	LastFailure string `json:"lastFailure"`
	// 最近一次失败的人读信息
	LastFailureMsg string `json:"lastFailureMsg"`
	// 最近一次求值输出被单条拒收的条目数（含声明集外条目）
	LastRejected int `json:"lastRejected"`
}

// ParticipationStatusProvider 参与度概要提供者接口（由 plugin service 定义需要的真相层
// 查询面；*participation.Manager 结构性实现——查询语义与该包 Entries/StatusOf 一致）
type ParticipationStatusProvider interface {
	// Entries 列出插件全部声明条目的当前参与态（point、id 字典序）。nil = 无会话（插件未激活/已停用）
	Entries(pluginPublicId string) []participation.EntryState
	// StatusOf 查询插件求值状态快照（降级态数据面）。nil = 无会话
	StatusOf(pluginPublicId string) *participation.Status
}

// ExtensionInfo 扩展点信息
type ExtensionInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// FrontendExtensionInfo 前端扩展信息
type FrontendExtensionInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}
