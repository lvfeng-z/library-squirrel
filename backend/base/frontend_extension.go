package base

import (
	"encoding/json"

	pkgmodel "github.com/library-squirrel/backend/base/model"
)

// FrontendExtensionKind 前端扩展类型（7 种平级：主动注入型 embed/view/replaceView/menu/siteBrowserList/dialog + 被动响应型 resourceViewer）
type FrontendExtensionKind string

const (
	// FrontendExtensionKindEmbed 嵌入（插入主程序具名插槽位，position 为插槽位标识）
	FrontendExtensionKindEmbed FrontendExtensionKind = "embed"
	// FrontendExtensionKindView 视图（新增页面）
	FrontendExtensionKindView FrontendExtensionKind = "view"
	// FrontendExtensionKindReplaceView 替换视图（覆盖主程序已有路由）
	FrontendExtensionKindReplaceView FrontendExtensionKind = "replaceView"
	// FrontendExtensionKindMenu 菜单
	FrontendExtensionKindMenu FrontendExtensionKind = "menu"
	// FrontendExtensionKindSiteBrowserList 站点浏览器列表
	FrontendExtensionKindSiteBrowserList FrontendExtensionKind = "siteBrowserList"
	// FrontendExtensionKindDialog 弹窗（模态层）
	FrontendExtensionKindDialog FrontendExtensionKind = "dialog"
	// FrontendExtensionKindResourceViewer 资源渲染器（被动响应型：插件为某 resourceType 提供自定义渲染器，主程序渲染该类型资源时按 resourceType 查找命中）
	FrontendExtensionKindResourceViewer FrontendExtensionKind = "resourceViewer"
)

// ContentType 前端扩展内容类型
type ContentType string

const (
	// ContentTypeVueSource Vue 源码
	ContentTypeVueSource ContentType = "vueSource"
	// ContentTypePrecompiled 预编译 JS/CSS
	ContentTypePrecompiled ContentType = "precompiled"
	// ContentTypeCode JavaScript 代码字符串
	ContentTypeCode ContentType = "code"
	// ContentTypeHTML HTML 文件
	ContentTypeHTML ContentType = "html"
)

// FrontendExtensionConfig 前端扩展配置（单一 flat 结构，承载 7 种 kind 的全部字段；不拆主动注入型/被动响应型两结构体——被动响应型独有字段仅 ResourceType）
type FrontendExtensionConfig struct {
	Metadata     *pkgmodel.ExtensionMetadata // 组合元数据
	Kind         FrontendExtensionKind       // 前端扩展类型
	Content      json.RawMessage             // 内容（JSON 结构，前端根据 ContentType 解析）
	ContentType  ContentType                 // 内容类型
	Title        string                      // view: 标题
	Icon         string                      // 图标（解析后的完整 URL）
	Order        int                         // 排序
	Position     string                      // embed: 主程序具名插槽位标识（如 work.toolbar）
	Target       string                      // replaceView: 主程序路由 name（覆盖目标）
	ViewId       string                      // menu: 关联的 view 前端扩展 ID
	ExtensionId  string                      // siteBrowserList: 关联的 siteBrowser extension ID
	ResourceType string                      // resourceViewer: 资源类型查找键（前端按此匹配 resource.resource_type）
	Props        json.RawMessage             // 传递给组件的额外属性
	Children     []FrontendExtensionConfig   // menu: 子菜单项
}

// NewFrontendExtensionConfig 创建前端扩展配置
func NewFrontendExtensionConfig() *FrontendExtensionConfig {
	return &FrontendExtensionConfig{
		Metadata: &pkgmodel.ExtensionMetadata{
			Type: pkgmodel.ExtensionTypeFrontendExtension,
		},
	}
}
