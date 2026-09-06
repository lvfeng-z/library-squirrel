package plugin

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
	TaskHandlers       []ExtensionInfo         `json:"taskHandlers"`
	SiteBrowsers       []ExtensionInfo         `json:"siteBrowsers"`
	FrontendExtensions []FrontendExtensionInfo `json:"frontendExtensions"`

	// URL 监听规则
	UrlPatterns []string `json:"urlPatterns"`
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
