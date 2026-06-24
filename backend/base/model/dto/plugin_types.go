package dto

import (
	"encoding/json"
)

// InstallType 安装类型
type InstallType int

const (
	// InstallTypeManual 手动安装
	InstallTypeManual InstallType = 0
	// InstallTypeAuto 自动安装
	InstallTypeAuto InstallType = 1
)

// ActivationType 激活类型
type ActivationType int

const (
	// ActivationTypeManual 手动激活
	ActivationTypeManual ActivationType = 0
	// ActivationTypeStartup 启动时激活
	ActivationTypeStartup ActivationType = 1
)

// PluginActivation 插件激活配置
type PluginActivation struct {
	Type ActivationType `json:"type"`
}

// PluginManifest 插件清单（从 plugin.json 解析）
type PluginManifest struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Author      string            `json:"author"`
	Description string            `json:"description,omitempty"`
	Extensions  *PluginExtensions `json:"extensions"`
	Activation  PluginActivation  `json:"activation"`
	EntryFile   string            `json:"entryFile"`
}

// PluginInstallDTO 插件安装数据传输对象
type PluginInstallDTO struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Author      string            `json:"author"`
	Description string            `json:"description,omitempty"`
	Extensions  *PluginExtensions `json:"extensions"`
	Activation  PluginActivation  `json:"activation"`
	EntryFile   string            `json:"entryFile"`
	PackagePath string            `json:"packagePath,omitempty"`
	PublicID    string            `json:"publicId,omitempty"`
}

// GetPublicID 获取插件公开ID（格式：作者/名称）
func (p *PluginManifest) GetPublicID() string {
	return p.Author + "/" + p.ID
}

// ToPluginInstallDTO 转换为安装DTO
func (p *PluginManifest) ToPluginInstallDTO(packagePath string) *PluginInstallDTO {
	return &PluginInstallDTO{
		ID:          p.ID,
		Name:        p.Name,
		Version:     p.Version,
		Author:      p.Author,
		Description: p.Description,
		Extensions:  p.Extensions,
		Activation:  p.Activation,
		EntryFile:   p.EntryFile,
		PackagePath: packagePath,
		PublicID:    p.GetPublicID(),
	}
}

// NewPluginManifest 创建插件清单
func NewPluginManifest() *PluginManifest {
	return &PluginManifest{}
}

// PluginExtensions 插件扩展点集合（plugin.json 的 extensions 段）
type PluginExtensions struct {
	TaskHandlers    []TaskHandlerDeclaration `json:"taskHandlers,omitempty"`
	SiteBrowsers    []SiteBrowserDeclaration `json:"siteBrowsers,omitempty"`
	Slots           []SlotDeclaration        `json:"slots,omitempty"`
	StaticResources *StaticResourcesConfig   `json:"staticResources,omitempty"`
	Settings        []SettingDeclaration     `json:"settings,omitempty"`
}

// SettingDeclaration 用户设置项声明（plugin.json extensions.settings）
type SettingDeclaration struct {
	Key         string          `json:"key"`
	Type        string          `json:"type"` // string | integer | boolean | select
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Default     string          `json:"default,omitempty"`
	Encrypted   bool            `json:"encrypted,omitempty"`
	Group       string          `json:"group,omitempty"`
	Order       int             `json:"order,omitempty"`
	Options     []SettingOption `json:"options,omitempty"`
	Min         *int            `json:"min,omitempty"`
	Max         *int            `json:"max,omitempty"`
}

// SettingOption select 类型的选项
type SettingOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// TaskHandlerDeclaration 任务处理器声明
type TaskHandlerDeclaration struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SiteBrowserDeclaration 站点浏览器声明
type SiteBrowserDeclaration struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SlotDeclaration 插槽声明（plugin.json 中每个 slot 条目）
type SlotDeclaration struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	SlotType    string          `json:"slotType"`
	Order       int             `json:"order,omitempty"`
	Content     json.RawMessage `json:"content"`
}

// EmbedSlotContent embed 类型插槽配置
type EmbedSlotContent struct {
	ContentType    string          `json:"contentType"`
	Source         json.RawMessage `json:"source"`
	Position       string          `json:"position"`
	ContributionId string          `json:"contributionId,omitempty"`
	Props          json.RawMessage `json:"props,omitempty"`
}

// PanelSlotContent panel 类型插槽配置
type PanelSlotContent struct {
	ContentType string          `json:"contentType"`
	Source      json.RawMessage `json:"source"`
	Position    string          `json:"position"`
	Width       *int            `json:"width,omitempty"`
	Height      *int            `json:"height,omitempty"`
	Props       json.RawMessage `json:"props,omitempty"`
}

// ViewSlotContent view 类型插槽配置
type ViewSlotContent struct {
	ContentType string          `json:"contentType"`
	Source      json.RawMessage `json:"source"`
	Title       string          `json:"title,omitempty"`
	Props       json.RawMessage `json:"props,omitempty"`
}

// MenuSlotContent menu 类型插槽配置
type MenuSlotContent struct {
	Icon     string            `json:"icon,omitempty"`
	ViewId   string            `json:"viewId,omitempty"`
	Children []SlotDeclaration `json:"children,omitempty"`
}

// SiteBrowserListSlotContent siteBrowserList 类型插槽配置
type SiteBrowserListSlotContent struct {
	Icon           string `json:"icon,omitempty"`
	ContributionId string `json:"contributionId"`
}

// StaticResourcesConfig 静态资源配置
type StaticResourcesConfig struct {
	Directories []string `json:"directories"`
}
