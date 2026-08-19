package model

// ExtensionType 扩展点类型（顶层笼统抽象：taskHandler/siteBrowser/frontendExtension 三类并列）
type ExtensionType string

const (
	// ExtensionTypeTaskHandler 任务处理器
	ExtensionTypeTaskHandler ExtensionType = "taskHandler"
	// ExtensionTypeSiteBrowser 站点浏览器
	ExtensionTypeSiteBrowser ExtensionType = "siteBrowser"
	// ExtensionTypeFrontendExtension 前端扩展（面向前端 UI 的扩展点：embed/view/replaceView/menu/siteBrowserList/dialog/resourceViewer 七种平级）
	ExtensionTypeFrontendExtension ExtensionType = "frontendExtension"
)

// ExtensionMetadata 扩展点元数据（三类共用）
type ExtensionMetadata struct {
	Type           ExtensionType `json:"type"`                  // 扩展点类型
	ID             string        `json:"extensionId"`           // 扩展点ID
	PluginID       int64         `json:"pluginId"`              // 插件数据库ID
	PluginPublicID string        `json:"pluginPublicId"`        // 插件公开ID
	Name           string        `json:"name"`                  // 扩展点名称
	Description    string        `json:"description,omitempty"` // 扩展点描述
}

// Extension 扩展点（实例 + 元数据组合）
type Extension[T any] struct {
	Metadata ExtensionMetadata
	Instance T
}

// NewExtension 创建扩展点
func NewExtension[T any](metadata ExtensionMetadata, instance T) *Extension[T] {
	return &Extension[T]{
		Metadata: metadata,
		Instance: instance,
	}
}
