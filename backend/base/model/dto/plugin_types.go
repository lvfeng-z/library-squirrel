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

// PluginManifest 插件清单（从 plugin.json 解析）；能力声明住 extensions 段，
// settings（用户设置项声明）住根级
type PluginManifest struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Version             string               `json:"version"`
	BuildID             string               `json:"buildId,omitempty"`   // 构建身份标识（构建管线注入 git describe 输出；同源码状态永远同值，主程序以此判同构建）
	ContractVersion     int                  `json:"contractVersion"`     // 插件编译时锁定的契约版本（主程序加载时与 currentContractVersion/minSupportedContractVersion 比对；未声明=0 拒载，须声明）
	ConfigSchemaVersion int                  `json:"configSchemaVersion"` // 插件配置 schema 版本（plugin.json 声明；0=legacy/未管理，host 写入时盖戳到 plugin_storage.schema_version）
	Author              string               `json:"author"`
	Description         string               `json:"description,omitempty"`
	Settings            []SettingDeclaration `json:"settings,omitempty"` // 用户设置项声明（住清单根级，不属 extensions 能力包）
	Extensions          *PluginExtensions    `json:"extensions"`
	Activation          PluginActivation     `json:"activation"`
	EntryFile           string               `json:"entryFile"`
}

// PluginInstallDTO 插件安装数据传输对象
type PluginInstallDTO struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Version             string               `json:"version"`
	BuildID             string               `json:"buildId,omitempty"`   // 构建身份标识（构建管线注入 git describe 输出；同源码状态永远同值，主程序以此判同构建）
	ContractVersion     int                  `json:"contractVersion"`     // 插件编译时锁定的契约版本（主程序加载时与 currentContractVersion/minSupportedContractVersion 比对；未声明=0 拒载，须声明）
	ConfigSchemaVersion int                  `json:"configSchemaVersion"` // 插件配置 schema 版本（plugin.json 声明；0=legacy/未管理，host 写入时盖戳到 plugin_storage.schema_version）
	Author              string               `json:"author"`
	Description         string               `json:"description,omitempty"`
	Settings            []SettingDeclaration `json:"settings,omitempty"` // 用户设置项声明（住清单根级，不属 extensions 能力包）
	Extensions          *PluginExtensions    `json:"extensions"`
	Activation          PluginActivation     `json:"activation"`
	EntryFile           string               `json:"entryFile"`
	PackagePath         string               `json:"packagePath,omitempty"`
	PublicID            string               `json:"publicId,omitempty"`
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
		Settings:            p.Settings,
		Extensions:          p.Extensions,
		Activation:          p.Activation,
		EntryFile:           p.EntryFile,
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

// PluginExtensions 插件扩展点集合（plugin.json 的 extensions 段）；
// 每段 = 插件对外提供的一个能力包，包内的可选项与作用域声明在包内条目上
type PluginExtensions struct {
	WorkFetch          []WorkFetchDeclaration         `json:"workFetch,omitempty"`
	SiteBrowsers       []SiteBrowserDeclaration       `json:"siteBrowsers,omitempty"`
	SiteAuthorFetch    []SiteAuthorFetchDeclaration   `json:"siteAuthorFetch,omitempty"`
	ResourceTypes      []ResourceTypeDeclaration      `json:"resourceTypes,omitempty"`
	FrontendExtensions []FrontendExtensionDeclaration `json:"frontendExtensions,omitempty"`
}

// SiteAuthorFetchDeclaration 站点作者拉取能力包的单个实例声明（plugin.json extensions.siteAuthorFetch
// 数组项）；一插件可声明多个拉取实例，各按 id 独立寻址（拉取请求携带条目 id，插件侧分派到对应实现）。
// sites 为该实例服务的站点键清单（作用域 = 归属），取值须为 SDK 站点注册表内的注册键
type SiteAuthorFetchDeclaration struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"` // 实例显示名（候选冲突选择器展示「插件名 · 条目名」）
	Sites []string `json:"sites"`
}

// SettingDeclaration 用户设置项声明（plugin.json 根级 settings 段每项）
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

// WorkFetchDeclaration 作品拉取声明
type WorkFetchDeclaration struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"` // 显示名（派生注册表元数据的唯一源）
	Description string   `json:"description,omitempty"`
	Options     []string `json:"options,omitempty"`     // 本条目启用的可选方法组（内置枚举见 extension 包 Capability* 常量）
	UrlPatterns []string `json:"urlPatterns,omitempty"` // URL 监听模式（正则模式串数组；匹配的 URL 创建任务时路由到本条目，缺省=不监听）
}

// SiteBrowserDeclaration 站点浏览器声明
type SiteBrowserDeclaration struct {
	ID          string `json:"id"`
	Name        string `json:"name"` // 显示名（派生注册表元数据的唯一源）
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

// ResourceTypeDeclaration 插件自定义资源类型声明（plugin.json extensions.resourceTypes 段每项）。
// 该段存在即注册进 ResourceTypeRegistry,使插件 Create 可声明该类型资源。
// 注册时强校验(同名拒绝+反向域名前缀+Roles合法性),坏 spec 拒绝并记日志跳过、不株连插件其他能力。
type ResourceTypeDeclaration struct {
	Type           string                              `json:"type"`                     // 类型值(强制反向域名前缀如 com.example.xxx;决策8)
	Roles          []StoreRoleDeclaration              `json:"roles"`                    // 结构角色 + 基数(完整性校验)
	PrimaryRoles   []string                            `json:"primaryRoles"`             // 展示主体优先级链(每项须在 Roles.storeType 集合内)
	StoreStandards map[string]StoreStandardDeclaration `json:"storeStandards,omitempty"` // 各 store 角色的文件标准(key=storeType);可选,描述性,不强制声明
}

// StoreStandardDeclaration 单个 store 角色的文件标准声明(plugin.json 解析用,与宿主侧规约
// entity.StoreStandard 对齐):描述、期望扩展名、典型产出方式,均为描述性信息(不做内容校验)。
type StoreStandardDeclaration struct {
	Description string   `json:"description"`          // 角色用途说明
	Formats     []string `json:"formats,omitempty"`    // 期望文件扩展名(描述性,非强制)
	Generation  string   `json:"generation,omitempty"` // 该角色的典型 generation(downloaded/derived);可跨多种 generation 的角色留空
}

// StoreRoleDeclaration ResourceTypeDeclaration 的结构角色声明(plugin.json 解析用,独立于 entity.StoreRoleSpec)。
type StoreRoleDeclaration struct {
	StoreType string `json:"storeType"` // store_type(内置 7 角色之一;插件自定义角色延后 G')
	Min       int    `json:"min"`       // 最少数量(0=可选,1=必含)
	Max       int    `json:"max"`       // 最多数量(0=不限,1=单例)
}
