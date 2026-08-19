package dto

import (
	"encoding/json"
	"fmt"
	"regexp"
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
	ID                  string                    `json:"id"`
	Name                string                    `json:"name"`
	Version             string                    `json:"version"`
	BuildID             string                    `json:"buildId,omitempty"`   // 构建身份标识（构建管线注入 git describe 输出；同源码状态永远同值，主程序以此判同构建）
	ContractVersion     int                       `json:"contractVersion"`     // 插件编译时锁定的契约版本（主程序加载时与 currentContractVersion/minSupportedContractVersion 比对；缺字段=0 视为当前契约放行）
	ConfigSchemaVersion int                       `json:"configSchemaVersion"` // 插件配置 schema 版本（plugin.json 声明；0=legacy/未管理，host 写入时盖戳到 plugin_storage.schema_version）
	Author              string                    `json:"author"`
	Description         string                    `json:"description,omitempty"`
	Extensions          *PluginExtensions         `json:"extensions"`
	Activation          PluginActivation          `json:"activation"`
	EntryFile           string                    `json:"entryFile"`
	Capabilities        []string                  `json:"capabilities,omitempty"`  // 声明的可选能力（内置枚举,可随主程序版本扩展;主程序加载时读取,未声明者跳过对应能力调用）
	ResourceTypes       []ResourceTypeDeclaration `json:"resourceTypes,omitempty"` // 插件自定义资源类型声明(须配合 capabilities 含 resourceTypeProvider 通行证)
}

// PluginInstallDTO 插件安装数据传输对象
type PluginInstallDTO struct {
	ID                  string                    `json:"id"`
	Name                string                    `json:"name"`
	Version             string                    `json:"version"`
	BuildID             string                    `json:"buildId,omitempty"`   // 构建身份标识（构建管线注入 git describe 输出；同源码状态永远同值，主程序以此判同构建）
	ContractVersion     int                       `json:"contractVersion"`     // 插件编译时锁定的契约版本（主程序加载时与 currentContractVersion/minSupportedContractVersion 比对；缺字段=0 视为当前契约放行）
	ConfigSchemaVersion int                       `json:"configSchemaVersion"` // 插件配置 schema 版本（plugin.json 声明；0=legacy/未管理，host 写入时盖戳到 plugin_storage.schema_version）
	Author              string                    `json:"author"`
	Description         string                    `json:"description,omitempty"`
	Extensions          *PluginExtensions         `json:"extensions"`
	Activation          PluginActivation          `json:"activation"`
	EntryFile           string                    `json:"entryFile"`
	Capabilities        []string                  `json:"capabilities,omitempty"`  // 声明的可选能力
	ResourceTypes       []ResourceTypeDeclaration `json:"resourceTypes,omitempty"` // 插件自定义资源类型声明(透传 manifest)
	PackagePath         string                    `json:"packagePath,omitempty"`
	PublicID            string                    `json:"publicId,omitempty"`
}

// ToPluginInstallDTO 转换为安装DTO。publicId 即插件 id（纯反向域名，全局唯一身份键），
// author 是纯展示属性、不参与身份
func (p *PluginManifest) ToPluginInstallDTO(packagePath string) *PluginInstallDTO {
	return &PluginInstallDTO{
		ID:                  p.ID,
		Name:                p.Name,
		Version:             p.Version,
		BuildID:             p.BuildID,
		ContractVersion:     p.ContractVersion,
		ConfigSchemaVersion: p.ConfigSchemaVersion,
		Author:              p.Author,
		Description:         p.Description,
		Extensions:          p.Extensions,
		Activation:          p.Activation,
		EntryFile:           p.EntryFile,
		Capabilities:        p.Capabilities,
		ResourceTypes:       p.ResourceTypes,
		PackagePath:         packagePath,
		PublicID:            p.ID,
	}
}

// pluginIDFormatRe 插件 id 格式：反向域名，至少两段 label，字符集限字母/数字/连字符/点
var pluginIDFormatRe = regexp.MustCompile(`^[a-zA-Z0-9-]+(\.[a-zA-Z0-9-]+)+$`)

// ValidatePluginID 校验插件 id（= publicId 身份键）：须为反向域名格式
func ValidatePluginID(id string) error {
	if !pluginIDFormatRe.MatchString(id) {
		return fmt.Errorf("invalid plugin id %q: expect reverse-domain format like com.example.plugin", id)
	}
	return nil
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

// ResourceTypeDeclaration 插件自定义资源类型声明（plugin.json 顶层 resourceTypes 段每项）。
// 主程序加载时见 CapabilityResourceTypeProvider 通行证后解析此段,转为 entity.ResourceTypeSpec 注册进 Registry。
// 注册时强校验(决策7同名拒绝+决策8反向域名前缀+Roles合法性),坏 spec 拒绝并记日志跳过、不株连插件其他能力。
type ResourceTypeDeclaration struct {
	Type         string                 `json:"type"`         // 类型值(强制反向域名前缀如 com.example.xxx;决策8)
	Roles        []StoreRoleDeclaration `json:"roles"`        // 结构角色 + 基数(完整性校验)
	PrimaryRoles []string               `json:"primaryRoles"` // 展示主体优先级链(每项须在 Roles.storeType 集合内)
}

// StoreRoleDeclaration ResourceTypeDeclaration 的结构角色声明(plugin.json 解析用,独立于 entity.StoreRoleSpec)。
type StoreRoleDeclaration struct {
	StoreType string `json:"storeType"` // store_type(内置 7 角色之一;插件自定义角色延后 G')
	Min       int    `json:"min"`       // 最少数量(0=可选,1=必含)
	Max       int    `json:"max"`       // 最多数量(0=不限,1=单例)
}
