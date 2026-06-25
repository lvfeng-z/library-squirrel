package base

import (
	"encoding/json"

	pkgmodel "github.com/library-squirrel/backend/base/model"
)

// SlotType 插槽类型
type SlotType string

const (
	// SlotTypeEmbed 嵌入插槽
	SlotTypeEmbed SlotType = "embed"
	// SlotTypePanel 面板插槽
	SlotTypePanel SlotType = "panel"
	// SlotTypeView 视图插槽
	SlotTypeView SlotType = "view"
	// SlotTypeMenu 菜单插槽
	SlotTypeMenu SlotType = "menu"
	// SlotTypeSiteBrowserList 站点浏览器列表插槽
	SlotTypeSiteBrowserList SlotType = "siteBrowserList"
)

// ContentType 插槽内容类型
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

// SlotConfig 插槽配置
type SlotConfig struct {
	Metadata       *pkgmodel.ExtensionMetadata // 组合元数据
	SlotType       SlotType                    // 插槽类型
	Content        json.RawMessage             // 插槽内容（JSON 结构，前端根据 ContentType 解析）
	ContentType    ContentType                 // 内容类型
	Title          string                      // 插槽标题
	Icon           string                      // 插槽图标（解析后的完整 URL）
	Order          int                         // 排序
	Position       string                      // embed: topbar|toolbar|statusbar|dialog; panel: left-sidebar|right-sidebar|bottom
	Width          *int                        // panel: 宽度
	Height         *int                        // panel: 高度
	ViewId         string                      // menu: 关联的 view slot ID
	ExtensionId string                      // siteBrowserList: 关联的 siteBrowser extension ID
	Props          json.RawMessage             // 传递给组件的额外属性
	Children       []SlotConfig                // menu: 子菜单项
}

// NewSlotConfig 创建插槽配置
func NewSlotConfig() *SlotConfig {
	return &SlotConfig{
		Metadata: &pkgmodel.ExtensionMetadata{
			Type: pkgmodel.ExtensionTypeSlot,
		},
	}
}
