package model

// ExtensionType 扩展点类型
type ExtensionType string

const (
	// ExtensionTypeTaskHandler 任务处理器
	ExtensionTypeTaskHandler ExtensionType = "taskHandler"
	// ExtensionTypeSiteBrowser 站点浏览器
	ExtensionTypeSiteBrowser ExtensionType = "siteBrowser"
	// ExtensionTypeSlot 插槽
	ExtensionTypeSlot ExtensionType = "slot"
)

// ExtensionMetadata 扩展点元数据
type ExtensionMetadata struct {
	Type           ExtensionType // 扩展点类型
	ID             string        // 扩展点ID
	PluginID       int64         // 插件数据库ID
	PluginPublicID string        // 插件公开ID
	Name           string        // 扩展点名称
	Description    string        // 扩展点描述
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
