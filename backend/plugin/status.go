package plugin

// PluginStatusDTO 插件状态
type PluginStatusDTO struct {
	// 运行时状态
	IsRunning   bool  `json:"isRunning"`
	PID         int   `json:"pid"`
	ActivatedAt int64 `json:"activatedAt"` // Unix 毫秒，0 表示未激活

	// 扩展点列表
	TaskHandlers []ExtensionInfo `json:"taskHandlers"`
	SiteBrowsers []ExtensionInfo `json:"siteBrowsers"`
	Slots        []SlotInfo      `json:"slots"`

	// 存储状态
	PluginDataSize int `json:"pluginDataSize"` // 字节数

	// URL 监听规则
	UrlPatterns []string `json:"urlPatterns"`
}

// ExtensionInfo 扩展点信息
type ExtensionInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SlotInfo 插槽信息
type SlotInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SlotType string `json:"slotType"`
}
