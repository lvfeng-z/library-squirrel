package model

import pkgmodel "github.com/library-squirrel/wails/pkg/model"

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
	// ContentTypeHTML HTML 内容
	ContentTypeHTML ContentType = "html"
	// ContentTypeComponent 组件
	ContentTypeComponent ContentType = "component"
)

// SlotConfig 插槽配置
type SlotConfig struct {
	*pkgmodel.ExtensionMetadata             // 嵌入元数据
	SlotType                    SlotType    // 插槽类型
	Content                     string      // 插槽内容
	ContentType                 ContentType // 内容类型
	Title                       string      // 插槽标题
	Icon                        string      // 插槽图标
	Order                       int         // 排序
}

// NewSlotConfig 创建插槽配置
func NewSlotConfig() *SlotConfig {
	return &SlotConfig{
		ExtensionMetadata: &pkgmodel.ExtensionMetadata{
			Type: pkgmodel.ExtensionTypeSlot,
		},
	}
}
