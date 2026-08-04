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
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	ContractVersion int               `json:"contractVersion"` // 插件编译时锁定的契约版本（主程序加载时与 currentContractVersion/minSupportedContractVersion 比对；缺字段=0 视为当前契约放行）
	Author          string            `json:"author"`
	Description     string            `json:"description,omitempty"`
	Extensions      *PluginExtensions `json:"extensions"`
	Activation      PluginActivation  `json:"activation"`
	EntryFile       string            `json:"entryFile"`
	Capabilities    []string          `json:"capabilities,omitempty"` // 声明的可选能力（封闭枚举；主程序加载时读取，未声明者跳过对应能力调用）
}

// PluginInstallDTO 插件安装数据传输对象
type PluginInstallDTO struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	ContractVersion int               `json:"contractVersion"` // 插件编译时锁定的契约版本（主程序加载时与 currentContractVersion/minSupportedContractVersion 比对；缺字段=0 视为当前契约放行）
	Author          string            `json:"author"`
	Description     string            `json:"description,omitempty"`
	Extensions      *PluginExtensions `json:"extensions"`
	Activation      PluginActivation  `json:"activation"`
	EntryFile       string            `json:"entryFile"`
	Capabilities    []string          `json:"capabilities,omitempty"` // 声明的可选能力
	PackagePath     string            `json:"packagePath,omitempty"`
	PublicID        string            `json:"publicId,omitempty"`
}

// GetPublicID 获取插件公开ID（格式：作者/名称）
func (p *PluginManifest) GetPublicID() string {
	return p.Author + "/" + p.ID
}

// ToPluginInstallDTO 转换为安装DTO
func (p *PluginManifest) ToPluginInstallDTO(packagePath string) *PluginInstallDTO {
	return &PluginInstallDTO{
		ID:              p.ID,
		Name:            p.Name,
		Version:         p.Version,
		ContractVersion: p.ContractVersion,
		Author:          p.Author,
		Description:     p.Description,
		Extensions:      p.Extensions,
		Activation:      p.Activation,
		EntryFile:       p.EntryFile,
		Capabilities:    p.Capabilities,
		PackagePath:     packagePath,
		PublicID:        p.GetPublicID(),
	}
}

// NewPluginManifest 创建插件清单
func NewPluginManifest() *PluginManifest {
	return &PluginManifest{}
}

// PluginExtensions 插件扩展点集合（plugin.json 的 extensions 段）
type PluginExtensions struct {
	TaskHandlers       []TaskHandlerDeclaration       `json:"taskHandlers,omitempty"`
	SiteBrowsers       []SiteBrowserDeclaration       `json:"siteBrowsers,omitempty"`
	FrontendExtensions []FrontendExtensionDeclaration `json:"frontendExtensions,omitempty"`
	StaticResources    *StaticResourcesConfig         `json:"staticResources,omitempty"`
	Settings           []SettingDeclaration           `json:"settings,omitempty"`
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

// FrontendExtensionDeclaration 前端扩展声明（plugin.json 中每个 frontendExtension 条目）
type FrontendExtensionDeclaration struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Kind        string          `json:"kind"`
	Order       int             `json:"order,omitempty"`
	Content     json.RawMessage `json:"content"`
}

// EmbedContent embed 类型前端扩展配置（position 为主程序具名插槽位标识）
type EmbedContent struct {
	ContentType string          `json:"contentType"`
	Source      json.RawMessage `json:"source"`
	Position    string          `json:"position"`
	Props       json.RawMessage `json:"props,omitempty"`
}

// ReplaceViewContent replaceView 类型前端扩展配置（target 为主程序路由 name）
type ReplaceViewContent struct {
	ContentType string          `json:"contentType"`
	Source      json.RawMessage `json:"source"`
	Target      string          `json:"target"`
	Props       json.RawMessage `json:"props,omitempty"`
}

// ViewContent view 类型前端扩展配置（新增页面）
type ViewContent struct {
	ContentType string          `json:"contentType"`
	Source      json.RawMessage `json:"source"`
	Title       string          `json:"title,omitempty"`
	Props       json.RawMessage `json:"props,omitempty"`
}

// DialogContent dialog 类型前端扩展配置（弹窗层）
type DialogContent struct {
	ContentType string          `json:"contentType"`
	Source      json.RawMessage `json:"source"`
	Props       json.RawMessage `json:"props,omitempty"`
}

// MenuContent menu 类型前端扩展配置
type MenuContent struct {
	Icon     string                         `json:"icon,omitempty"`
	ViewId   string                         `json:"viewId,omitempty"`
	Children []FrontendExtensionDeclaration `json:"children,omitempty"`
}

// SiteBrowserListContent siteBrowserList 类型前端扩展配置
type SiteBrowserListContent struct {
	Icon        string `json:"icon,omitempty"`
	ExtensionId string `json:"extensionId"`
}

// ResourceViewerContent resourceViewer 类型前端扩展配置（被动响应型；resourceType 为资源类型查找键，前端按此匹配 resource.resourceType）
type ResourceViewerContent struct {
	ContentType  string          `json:"contentType"`
	Source       json.RawMessage `json:"source"`
	ResourceType string          `json:"resourceType"`
	Props        json.RawMessage `json:"props,omitempty"`
}

// StaticResourcesConfig 静态资源配置
type StaticResourcesConfig struct {
	Directories []string `json:"directories"`
}
