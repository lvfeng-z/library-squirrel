package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/go-plugin"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/gen"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
	pluginsdkliveness "github.com/lvfeng-z/library-squirrel-sdk/liveness"
	"go.uber.org/zap"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/plugin/settingresolver"
	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// ErrPluginLoadFailed 错误定义
var (
	ErrPluginLoadFailed     = errors.New("plugin load failed")
	ErrPluginContractTooNew = errors.New("插件契约版本过新，请升级主程序")
	ErrPluginContractTooOld = fmt.Errorf("插件契约版本过旧或未声明（支持的契约版本区间 %d–%d），请升级插件或在 plugin.json 声明 contractVersion",
		minSupportedContractVersion, currentContractVersion)
)

// currentContractVersion 主程序当前实现的插件契约版本（引用 SDK transport.ContractVersion，
// 与 SDK 同步发布、保持一致）。
const currentContractVersion = pluginsdktransport.ContractVersion

// minSupportedContractVersion 主程序仍兼容的最低插件契约版本；低于此版本的插件拒绝加载。
// 版本 12 分界：扩展点正名——作品拉取扩展点（workFetch / WorkFetcher / WorkFetchService）的旧名
// 退役（旧名标识符与线级破坏细节见 SDK 契约版本历史 transport/contract.go 第 12 条）：清单段名、
// SDK 接口与选项、gRPC 服务名、宿主侧类型与状态字段一并更换；
// 清单段名更换属宿主读清单的源级破坏、gRPC 服务名更换属线级破坏
const minSupportedContractVersion = 12

// ValidateContractVersion 校验插件契约版本是否与主程序兼容。
// pluginContract 为插件声明的契约版本；未声明（=0）视作低于 minSupported，拒绝加载并
// 提示需在 plugin.json 声明 contractVersion。
// 返回 ErrPluginContractTooNew（插件比主程序新，需升级主程序）、
// ErrPluginContractTooOld（插件低于最低支持版本或未声明版本，需升级/补声明）或 nil。
func ValidateContractVersion(pluginContract int) error {
	if pluginContract == 0 {
		return ErrPluginContractTooOld
	}
	if pluginContract > currentContractVersion {
		return ErrPluginContractTooNew
	}
	if pluginContract < minSupportedContractVersion {
		return ErrPluginContractTooOld
	}
	return nil
}

// 插件可选能力枚举（内置集,可随主程序版本扩展）。能力由清单声明面派生——siteAuthorFetch 段
// 与 workFetch[].options，主程序据未声明者跳过对应能力调用。
const (
	// CapabilityWorkOrderQuery 作品集原站序查询能力（插件实现 sdkdto.WorkOrderQuerier 可选接口），
	// 由 extensions.workFetch[].options 声明该值。
	CapabilityWorkOrderQuery = "workOrderQuery"
	// CapabilityWorkSetRelationQuery 作品集父集关系查询能力（插件实现 sdkdto.WorkSetRelationQuerier 可选接口），
	// 由 extensions.workFetch[].options 声明该值。
	CapabilityWorkSetRelationQuery = "workSetRelationQuery"
	// CapabilitySiteAuthorFetch 站点作者信息拉取能力（插件实现 sdkdto.SiteAuthorFetcher 可选接口），
	// 由 extensions.siteAuthorFetch 数组条目声明（任一条目在场即声明本能力）。主程序按能力声明广播
	// 路由——遍历声明本能力的已激活插件逐个调用，命中一个即止
	CapabilitySiteAuthorFetch = "siteAuthorFetch"
)

// SiteAuthorFetchEntryQuerier 按插件公开 ID 查询其站点作者拉取能力包的条目声明清单
// （Loader 实现，供 fetcher 按 (插件, 条目) 枚举候选并按条目收窄站点归属）。
type SiteAuthorFetchEntryQuerier interface {
	SiteAuthorFetchEntries(pluginPublicId string) []dto.SiteAuthorFetchDeclaration
}

// WorkFetchOptionQuerier 按（插件公开 ID, 作品拉取条目 ID）查询该条目声明的可选方法组
// （Loader 实现，供 fetcher 条目级门控：未声明某方法组的条目不经该条目被调用）。
type WorkFetchOptionQuerier interface {
	HasWorkFetchOption(pluginPublicId, extensionId, option string) bool
}

// ActivePlugin 已激活插件的公开 ID 与展示名（候选枚举面的最小载体；Name 为空串=插件未设置名）
type ActivePlugin struct {
	PublicID string
	Name     string
}

// ActivePluginLister 枚举当前已激活插件进程的公开 ID 与展示名（Loader 实现，供能力广播路由遍历与候选展示）。
type ActivePluginLister interface {
	ListActivePlugins() []ActivePlugin
}

// WorkFetchEntries 返回插件声明的作品拉取条目清单（各条目 id/name 原样；未加载/未声明
// 该段返回 nil）。参与度真相层接线后按覆盖表过滤——条目被 resolver 停用即不出现在
// 返回清单。消费方只读使用，不改动返回切片
func (l *Loader) WorkFetchEntries(pluginPublicId string) []dto.WorkFetchDeclaration {
	l.mu.RLock()
	defer l.mu.RUnlock()
	entry, ok := l.processes[pluginPublicId]
	if !ok || entry.info == nil {
		return nil
	}
	return l.filterWorkFetchEntries(pluginPublicId, entry.info.WorkFetch)
}

// SiteAuthorFetchEntries 返回插件声明的站点作者拉取能力包条目清单（各条目 id/name/sites 原样；
// 未加载/未声明该能力包返回 nil）。参与度真相层接线后按覆盖表过滤——条目被 resolver 停用
// 即不出现在返回清单。消费方只读使用，不改动返回切片
func (l *Loader) SiteAuthorFetchEntries(pluginPublicId string) []dto.SiteAuthorFetchDeclaration {
	l.mu.RLock()
	defer l.mu.RUnlock()
	entry, ok := l.processes[pluginPublicId]
	if !ok || entry.info == nil {
		return nil
	}
	return l.filterSiteAuthorFetchEntries(pluginPublicId, entry.info.SiteAuthorFetch)
}

// HasWorkFetchOption 查询插件指定作品拉取条目是否声明了该方法组（未加载/无该条目/该条目未声明
// 均返回 false，调用方据此不经该条目调用）。条目被参与度覆盖表停用时同样返回 false——
// 停用条目不经此门控被调用。
func (l *Loader) HasWorkFetchOption(pluginPublicId, extensionId, option string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	entry, ok := l.processes[pluginPublicId]
	if !ok || entry.info == nil {
		return false
	}
	if !l.entryParticipates(pluginPublicId, settingresolver.PointWorkFetch, extensionId) {
		return false
	}
	for _, handler := range entry.info.WorkFetch {
		if handler.ID != extensionId {
			continue
		}
		for _, declared := range handler.Options {
			if declared == option {
				return true
			}
		}
		return false
	}
	return false
}

// deriveCapabilities 由插件声明面派生可选能力集合：siteAuthorFetch 数组任一条目在场 →
// CapabilitySiteAuthorFetch；workFetch 各条目的 options 逐项取用。resourceTypes 段不派生
// 能力——该段在场即注册自定义资源类型。去重保序（声明序）。
func deriveCapabilities(info *PluginInfo) []string {
	if info == nil {
		return nil
	}
	var caps []string
	seen := make(map[string]struct{})
	appendCap := func(capability string) {
		if _, ok := seen[capability]; ok {
			return
		}
		seen[capability] = struct{}{}
		caps = append(caps, capability)
	}
	if len(info.SiteAuthorFetch) > 0 {
		appendCap(CapabilitySiteAuthorFetch)
	}
	for _, handler := range info.WorkFetch {
		for _, option := range handler.Options {
			appendCap(option)
		}
	}
	return caps
}

// ApplyManifestDeclarations 把已解析清单的扩展点声明填入插件信息（作品拉取条目及其可选方法组
// 与 URL 监听模式、站点浏览器条目、站点作者拉取能力包条目、自定义资源类型）。清单或 extensions 段缺失时声明字段
// 保持零值（等同未声明）。
func ApplyManifestDeclarations(info *PluginInfo, manifest *dto.PluginManifest) {
	if info == nil || manifest == nil || manifest.Extensions == nil {
		return
	}
	ext := manifest.Extensions
	info.WorkFetch = ext.WorkFetch
	info.SiteBrowsers = ext.SiteBrowsers
	info.SiteAuthorFetch = ext.SiteAuthorFetch
	info.ResourceTypes = ext.ResourceTypes
}

// ErrManifestDeclarationInvalid 清单声明面校验不合格（不合格项与期望形态见错误文本）。
var ErrManifestDeclarationInvalid = errors.New("插件清单声明面校验失败")

// validWorkFetchOptions 作品拉取条目可选方法组的内置封闭枚举（键为枚举值）；
// 骑在作品拉取服务上的可选能力都住在这里，见 Capability* 常量。
var validWorkFetchOptions = map[string]struct{}{
	CapabilityWorkOrderQuery:       {},
	CapabilityWorkSetRelationQuery: {},
}

// isValidWorkFetchOption 判断可选方法组取值是否在枚举内。
func isValidWorkFetchOption(option string) bool {
	_, ok := validWorkFetchOptions[option]
	return ok
}

// validWorkFetchOptionList 可选方法组枚举文本（字母序，供校验错误文本列出合法值）。
func validWorkFetchOptionList() string {
	values := make([]string, 0, len(validWorkFetchOptions))
	for value := range validWorkFetchOptions {
		values = append(values, value)
	}
	sort.Strings(values)
	return strings.Join(values, "、")
}

// registeredSiteKeyList 站点注册表全量键文本（注册序，供校验错误文本列出合法值）。
func registeredSiteKeyList() string {
	entries := identity.All()
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.Key)
	}
	return strings.Join(keys, "、")
}

// ValidateManifestDeclarations 校验 plugin.json 原文的声明面，安装期预检与加载期终检共用同一判据：
//   - 顶层 capabilities 键在场（值恰为 null 亦然——键在场即该段在场）判不合格：该段不构成声明面，
//     能力声明住在 extensions 各条目内
//   - extensions 子对象内 settings 键在场（值恰为 null 亦然——键在场即该段在场）判不合格：
//     settings（用户设置项声明）住清单根级，不属 extensions 能力包
//   - extensions.siteAuthorFetch：数组条目化——非空数组；条目 id 非空且插件内唯一、name 非空、
//     sites 须非空且每项为 SDK 站点注册表内的已注册键
//   - extensions.workFetch[].name 与 extensions.siteBrowsers[].name：须非空（清单为条目显示名
//     的唯一来源）
//   - extensions.workFetch[].options：每项须为内置可选方法组枚举值
//   - extensions.workFetch[].urlPatterns：可选字段，在场时须为非空数组且逐项为非空、可编译的
//     正则模式串
//
// manifestRaw 为 plugin.json 原文：顶层 capabilities 与 extensions 下 settings 在已解析结构里
// 没有承载字段，只能就原文探测键是否在场。
// 返回的错误逐项点名不合格项与期望形态；清单无 extensions 段（等同无声明）通过。
func ValidateManifestDeclarations(manifestRaw []byte) error {
	// 顶层键探针：只问键在场与否，不看取值
	topLevelKeys := map[string]json.RawMessage{}
	if err := json.Unmarshal(manifestRaw, &topLevelKeys); err != nil {
		return fmt.Errorf("%w: 清单非合法 JSON: %v", ErrManifestDeclarationInvalid, err)
	}
	if _, present := topLevelKeys["capabilities"]; present {
		return fmt.Errorf("%w: 顶层 capabilities 段不在声明面内，能力声明须住 extensions 各条目（siteAuthorFetch 段 / workFetch[].options）",
			ErrManifestDeclarationInvalid)
	}
	// extensions 子对象探针：只问 settings 键在场与否，不看取值；extensions 非法形态
	// （非对象/非法 JSON）交由下方整体解析判不合格
	if extRaw, present := topLevelKeys["extensions"]; present {
		extKeys := map[string]json.RawMessage{}
		if err := json.Unmarshal(extRaw, &extKeys); err == nil {
			if _, hasSettings := extKeys["settings"]; hasSettings {
				return fmt.Errorf("%w: extensions 内 settings 段不在声明面内，settings 须住清单根级（用户设置项声明）",
					ErrManifestDeclarationInvalid)
			}
		}
	}
	var manifest dto.PluginManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return fmt.Errorf("%w: 清单非合法 JSON: %v", ErrManifestDeclarationInvalid, err)
	}
	if manifest.Extensions == nil {
		return nil
	}
	ext := manifest.Extensions
	// siteAuthorFetch：数组在场但为空（键在场、条目数为零）判不合格；未声明（键不在场）通过
	if ext.SiteAuthorFetch != nil && len(ext.SiteAuthorFetch) == 0 {
		return fmt.Errorf("%w: extensions.siteAuthorFetch 为空数组，须至少声明一个条目（条目含 id/name/sites）",
			ErrManifestDeclarationInvalid)
	}
	seenFetchIDs := make(map[string]struct{}, len(ext.SiteAuthorFetch))
	for i, fetch := range ext.SiteAuthorFetch {
		if fetch.ID == "" {
			return fmt.Errorf("%w: extensions.siteAuthorFetch[%d] 条目缺 id，条目须携带 id（插件内唯一）",
				ErrManifestDeclarationInvalid, i)
		}
		if _, dup := seenFetchIDs[fetch.ID]; dup {
			return fmt.Errorf("%w: extensions.siteAuthorFetch 含重复条目 id %q，条目 id 须插件内唯一",
				ErrManifestDeclarationInvalid, fetch.ID)
		}
		seenFetchIDs[fetch.ID] = struct{}{}
		if fetch.Name == "" {
			return fmt.Errorf("%w: extensions.siteAuthorFetch[%s].name 为空，条目须携带显示名",
				ErrManifestDeclarationInvalid, fetch.ID)
		}
		if len(fetch.Sites) == 0 {
			return fmt.Errorf("%w: extensions.siteAuthorFetch[%s].sites 为空，须列出该条目服务的站点键（已注册键：%s）",
				ErrManifestDeclarationInvalid, fetch.ID, registeredSiteKeyList())
		}
		for _, siteKey := range fetch.Sites {
			if _, ok := identity.Lookup(siteKey); !ok {
				return fmt.Errorf("%w: extensions.siteAuthorFetch[%s].sites 含未注册站点键 %q，须为 SDK 站点注册表内的键（已注册键：%s）",
					ErrManifestDeclarationInvalid, fetch.ID, siteKey, registeredSiteKeyList())
			}
		}
	}
	for _, handler := range ext.WorkFetch {
		if handler.Name == "" {
			return fmt.Errorf("%w: extensions.workFetch[%s].name 为空，条目须携带显示名",
				ErrManifestDeclarationInvalid, handler.ID)
		}
		// urlPatterns 可选：字段不在场（缺省/null）通过；在场即须为非空数组且逐项可编译
		if handler.UrlPatterns != nil && len(handler.UrlPatterns) == 0 {
			return fmt.Errorf("%w: extensions.workFetch[%s].urlPatterns 为空数组，声明该字段须列出至少一个 URL 模式",
				ErrManifestDeclarationInvalid, handler.ID)
		}
		for j, pattern := range handler.UrlPatterns {
			if pattern == "" {
				return fmt.Errorf("%w: extensions.workFetch[%s].urlPatterns[%d] 为空串，模式项须为非空正则",
					ErrManifestDeclarationInvalid, handler.ID, j)
			}
			if _, err := regexp.Compile(pattern); err != nil {
				return fmt.Errorf("%w: extensions.workFetch[%s].urlPatterns[%d] 含无法编译的正则 %q: %v",
					ErrManifestDeclarationInvalid, handler.ID, j, pattern, err)
			}
		}
		for _, option := range handler.Options {
			if !isValidWorkFetchOption(option) {
				return fmt.Errorf("%w: extensions.workFetch[%s].options 含未识别的可选方法组 %q，合法取值：%s",
					ErrManifestDeclarationInvalid, handler.ID, option, validWorkFetchOptionList())
			}
		}
	}
	for _, browser := range ext.SiteBrowsers {
		if browser.Name == "" {
			return fmt.Errorf("%w: extensions.siteBrowsers[%s].name 为空，条目须携带显示名",
				ErrManifestDeclarationInvalid, browser.ID)
		}
	}
	return nil
}

// ListActivePlugins 返回当前已激活插件进程的公开 ID 与展示名清单（字典序——广播路由按序遍历，
// 「命中一个即止」的候选序须可复现；未激活返回空清单）
func (l *Loader) ListActivePlugins() []ActivePlugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	plugins := make([]ActivePlugin, 0, len(l.processes))
	for id, entry := range l.processes {
		plugin := ActivePlugin{PublicID: id}
		if entry.info != nil {
			plugin.Name = entry.info.Name
		}
		plugins = append(plugins, plugin)
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].PublicID < plugins[j].PublicID })
	return plugins
}

// registerPluginResourceTypes 注册插件声明的自定义资源类型到 ResourceTypeRegistry。
// 每个 declaration 转 Spec 注册,强校验失败(同名/前缀/Roles合法性)记日志跳过、不株连其他类型与插件能力。
func registerPluginResourceTypes(info *PluginInfo) {
	if info == nil {
		return
	}
	for _, decl := range info.ResourceTypes {
		registerPluginResourceType(info, decl)
	}
}

// registerPluginResourceType 注册单个自定义资源类型声明（激活期整批注册与参与度恢复的
// 单条重注册共用）；注册失败记日志跳过，不株连其他类型。
func registerPluginResourceType(info *PluginInfo, decl dto.ResourceTypeDeclaration) {
	spec := entity.ResourceTypeSpec{
		ResourceType:   decl.Type,
		Roles:          toStoreRoleSpecs(decl.Roles),
		PrimaryRoles:   decl.PrimaryRoles,
		StoreStandards: toStoreStandards(decl.StoreStandards),
	}
	if err := entity.ResourceTypeRegistry.Register(spec); err != nil {
		logger.Log.Warnf("插件 %s 自定义资源类型 %s 注册失败,跳过(不株连其他能力): %v", info.PublicID, decl.Type, err)
		return
	}
	logger.Log.Infof("插件 %s 自定义资源类型已注册: %s", info.PublicID, decl.Type)
}

// unregisterPluginResourceTypes 反注册插件声明的自定义资源类型(卸载时清理);内置类型受白名单保护不会被删。
func unregisterPluginResourceTypes(info *PluginInfo) {
	if info == nil {
		return
	}
	for _, decl := range info.ResourceTypes {
		entity.ResourceTypeRegistry.Unregister(decl.Type)
	}
}

// registerDeclaredExtensions 激活期按清单声明逐条建代理注册进作品拉取/站点浏览器两注册表。
// 注册键 = 插件公开 ID + 条目 id；元数据 name/description 取清单条目（清单为条目显示名的
// 唯一来源）。代理无状态，调用期经进程表解析 gRPC 客户端——清单已声明而插件未实现的条目
// 在注册表有条目、调用期得 gRPC 错误。任一条目注册失败（如同键条目已存在）即整体失败，
// 由激活相位的统一回滚清两注册表；停用/崩溃清理走 UnloadPlugin 的 UnregisterAll。
func (l *Loader) registerDeclaredExtensions(info *PluginInfo) error {
	for _, entry := range info.WorkFetch {
		var handler sdkdto.WorkFetcher = newWorkFetchProxy(l, info.PublicID, entry.ID)
		metadata := model.ExtensionMetadata{
			Type:           model.ExtensionTypeWorkFetch,
			ID:             entry.ID,
			PluginID:       info.ID,
			PluginPublicID: info.PublicID,
			Name:           entry.Name,
			Description:    entry.Description,
		}
		if err := l.workFetchRegistry.Register(model.NewExtension(metadata, handler)); err != nil {
			return fmt.Errorf("注册作品拉取条目 %s: %w", entry.ID, err)
		}
	}
	for _, entry := range info.SiteBrowsers {
		if err := l.registerSiteBrowserEntry(info, entry); err != nil {
			return err
		}
	}
	return nil
}

// registerSiteBrowserEntry 注册单个站点浏览器条目代理（激活期整批注册与参与度恢复的
// 单条重注册共用）：代理无状态，调用期经进程表解析 gRPC 客户端。注册失败（如同键条目
// 已存在）返回错误——激活路径据此整体回滚
func (l *Loader) registerSiteBrowserEntry(info *PluginInfo, entry dto.SiteBrowserDeclaration) error {
	var browser sdkdto.SiteBrowser = &SiteBrowserProxy{
		serviceAccessor: l,
		pluginPublicId:  info.PublicID,
		extensionId:     entry.ID,
	}
	metadata := model.ExtensionMetadata{
		Type:           model.ExtensionTypeSiteBrowser,
		ID:             entry.ID,
		PluginID:       info.ID,
		PluginPublicID: info.PublicID,
		Name:           entry.Name,
		Description:    entry.Description,
	}
	if err := l.siteBrowserRegistry.Register(model.NewExtension(metadata, browser)); err != nil {
		return fmt.Errorf("注册站点浏览器条目 %s: %w", entry.ID, err)
	}
	return nil
}

// toStoreRoleSpecs 将 manifest 声明角色转 Registry StoreRoleSpec。
func toStoreRoleSpecs(roles []dto.StoreRoleDeclaration) []entity.StoreRoleSpec {
	out := make([]entity.StoreRoleSpec, 0, len(roles))
	for _, r := range roles {
		out = append(out, entity.StoreRoleSpec{StoreType: r.StoreType, Min: r.Min, Max: r.Max})
	}
	return out
}

// toStoreStandards 将 manifest 声明的各角色文件标准转 Registry StoreStandard（key=storeType；
// 未声明该字段时返回 nil，规格里的文件标准整体缺失）。
func toStoreStandards(standards map[string]dto.StoreStandardDeclaration) map[string]entity.StoreStandard {
	if len(standards) == 0 {
		return nil
	}
	out := make(map[string]entity.StoreStandard, len(standards))
	for storeType, s := range standards {
		out[storeType] = entity.StoreStandard{Description: s.Description, Formats: s.Formats, Generation: s.Generation}
	}
	return out
}

// CreateNoWindow Windows 子进程创建标志：不创建控制台窗口
const CreateNoWindow = 0x08000000

// PluginProcessDeps 加载插件进程所需的依赖
type PluginProcessDeps struct {
	PluginInfo *PluginInfo
	PluginCtx  sdkdto.PluginContext
	MainHWND   uintptr
}

// pluginEntry 存储单个插件的 hashicorp/go-plugin 客户端和 gRPC 服务客户端
type pluginEntry struct {
	client      *plugin.Client                       // hashicorp/go-plugin 进程管理客户端
	services    *pluginsdktransport.GRPCPluginClient // gRPC 服务客户端（WorkFetch、Browser、Lifecycle）
	info        *PluginInfo                          // 插件基本信息
	activatedAt time.Time                            // 进程激活时间
}

// ServiceAccessor 插件 gRPC 服务访问器
// 由 Loader 实现，供 proxy 类型获取 gRPC 客户端
type ServiceAccessor interface {
	// GetServices 获取插件的 gRPC 服务客户端（含崩溃检测）
	GetServices(pluginPublicId string) (*pluginsdktransport.GRPCPluginClient, bool)
}

// Loader 插件加载器，使用 hashicorp/go-plugin 管理插件子进程
type Loader struct {
	workFetchRegistry   *WorkFetchRegistry
	siteBrowserRegistry *SiteBrowserRegistry

	// URL 监听清理回调（插件卸载/崩溃时调用，清理该插件的 URL 监听）
	urlListenerCleaner func(pluginPublicId string)

	// 崩溃清理通知回调：Loader 完成自身清理（进程表+所属注册表+URL 监听）后调用，
	// 供宿主触发生命周期参与者的 OnStopped，使崩溃路径与显式停用的清理集合对称
	crashNotifier func(pluginPublicId string)

	// 参与度真相层消费面（可选接线，装配期注入）：声明查询面（拉取条目清单、条目级
	// 方法组门控、能力集合派生）叠加覆盖表过滤；resourceTypes / siteBrowsers 条目参与度
	// 变化联动自定义资源类型与站点浏览器代理的条目级注册/反注册。未接线时各查询面退化
	// 为纯声明（全基线参与）
	participation       ParticipationTruth
	detachParticipation func()

	// 子进程模式：跟踪活跃的插件进程
	processes map[string]*pluginEntry // publicId -> entry
	mu        sync.RWMutex

	// 进程分组：Job Object 管理子进程（主进程退出时自动终止）
	processGroup *ProcessGroup
}

// NewLoader 创建插件加载器
func NewLoader(
	workFetchRegistry *WorkFetchRegistry,
	siteBrowserRegistry *SiteBrowserRegistry,
) *Loader {
	// 初始化 Job Object（Windows 下主进程退出时自动终止子进程，其他平台为空操作）
	pg, err := NewProcessGroup()
	if err != nil {
		logger.Log.Warnf("创建进程分组失败（不影响功能）: %v", err)
	}

	return &Loader{
		workFetchRegistry:   workFetchRegistry,
		siteBrowserRegistry: siteBrowserRegistry,
		processes:           make(map[string]*pluginEntry),
		processGroup:        pg,
	}
}

// SetUrlListenerCleaner 设置 URL 监听清理回调（插件卸载/崩溃时由 Loader 调用，清理该插件的 URL 监听）
func (l *Loader) SetUrlListenerCleaner(fn func(pluginPublicId string)) {
	l.urlListenerCleaner = fn
}

// SetCrashNotifier 设置崩溃清理通知回调：崩溃时 Loader 完成自身清理后调用，
// 供宿主触发生命周期参与者的 OnStopped（与显式停用的清理集合对称）
func (l *Loader) SetCrashNotifier(fn func(pluginPublicId string)) {
	l.crashNotifier = fn
}

// unregisterUrlListener 清理插件 URL 监听（回调未设置时跳过）
func (l *Loader) unregisterUrlListener(pluginPublicId string) {
	if l.urlListenerCleaner != nil {
		l.urlListenerCleaner(pluginPublicId)
	}
}

// LoadPluginProcess 以子进程模式加载插件：起子进程、握手并 Activate，随后按清单声明派生注册
// 作品拉取/站点浏览器代理进两注册表
// exePath: 插件可执行文件路径 (.exe)
// pluginPublicId: 插件公开ID
// deps: 加载插件所需的依赖（插件信息、宿主上下文、主窗口句柄）
func (l *Loader) LoadPluginProcess(exePath string, pluginPublicId string, deps PluginProcessDeps) error {
	// 活条目拒绝守卫：同名插件已有活跃进程条目时拒绝二次加载（与三注册表 already-exists
	// 守卫对齐——裸覆盖旧条目会令旧进程失去管理句柄成为孤儿）
	l.mu.Lock()
	_, loaded := l.processes[pluginPublicId]
	l.mu.Unlock()
	if loaded {
		return fmt.Errorf("%w: %s", ErrPluginAlreadyLoaded, pluginPublicId)
	}

	// 契约版本兼容校验（加载期终检，与安装期 loadPluginPackage 预检互为兜底）
	if err := ValidateContractVersion(deps.PluginInfo.ContractVersion); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrPluginLoadFailed, pluginPublicId, err)
	}
	// 构建 HostDeps，用于在 GRPCClient 中注册 HostService
	hostDeps := &pluginsdktransport.HostDeps{
		StorageProvider:       &hostStorageProvider{ctx: deps.PluginCtx},
		PluginRootProvider:    &hostPluginRootProvider{ctx: deps.PluginCtx},
		TaskCreateProvider:    &hostTaskCreateProvider{ctx: deps.PluginCtx},
		FrontendEventProvider: &hostFrontendEventProvider{ctx: deps.PluginCtx},
		LibraryQueryProvider:  &hostLibraryQueryProvider{ctx: deps.PluginCtx},
		LogFunc: func(level int32, template string, args []string, loggerName string) {
			sugar := deps.PluginCtx.(*pluginContext).ResolveLogger(loggerName)
			anyArgs := make([]any, len(args))
			for i, a := range args {
				anyArgs[i] = a
			}
			switch level {
			case 0:
				sugar.Debugf(template, anyArgs...)
			case 1:
				sugar.Infof(template, anyArgs...)
			case 2:
				sugar.Warnf(template, anyArgs...)
			case 3:
				sugar.Errorf(template, anyArgs...)
			}
		},
	}

	// 创建 LSPlugin 实例（主程序侧，注入 HostDeps）
	lsPlugin := &pluginsdktransport.LSPlugin{
		HostDeps: hostDeps,
	}

	// 创建 hashicorp/go-plugin 客户端配置
	cmd := exec.Command(exePath)
	cmd.Env = os.Environ()
	// Windows 下隐藏插件子进程的控制台窗口
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: CreateNoWindow,
	}
	config := &plugin.ClientConfig{
		HandshakeConfig: pluginsdktransport.Handshake,
		Plugins: map[string]plugin.Plugin{
			"library_squirrel": lsPlugin,
		},
		Cmd:              cmd,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		// keepalive dial options:client 主动探测插件进程存活,进程崩溃或网络中断时及时判定连接死
		GRPCDialOptions: pluginsdkliveness.ClientDialOptions(),
	}

	// 启动插件子进程
	client := plugin.NewClient(config)

	// 获取 gRPC 客户端
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("%w: connect plugin %s: %v", ErrPluginLoadFailed, pluginPublicId, err)
	}

	// 将插件子进程加入 Job Object（主进程退出时自动终止）
	if l.processGroup != nil && cmd.Process != nil {
		if err := l.processGroup.Assign(cmd.Process.Pid); err != nil {
			logger.Log.Warnf("将插件进程 %s (pid=%d) 加入 Job Object 失败: %v", pluginPublicId, cmd.Process.Pid, err)
		}
	}

	// 通过 Dispense 获取插件服务接口
	raw, err := rpcClient.Dispense("library_squirrel")
	if err != nil {
		client.Kill()
		return fmt.Errorf("%w: dispense plugin %s: %v", ErrPluginLoadFailed, pluginPublicId, err)
	}

	services, ok := raw.(*pluginsdktransport.GRPCPluginClient)
	if !ok {
		client.Kill()
		return fmt.Errorf("%w: unexpected plugin type for %s", ErrPluginLoadFailed, pluginPublicId)
	}

	// 发送 Activate 请求（插件自存信息已由统一 KV 取代，不再传递插件级 plugin_data）
	_, err = services.Lifecycle.Activate(context.Background(), &gen.ActivateRequest{
		PluginPublicId:   deps.PluginInfo.PublicID,
		RootPath:         deps.PluginInfo.RootPath,
		HostServiceId:    services.HostServiceId,
		MainWindowHandle: uint64(deps.MainHWND),
	})
	if err != nil {
		client.Kill()
		return fmt.Errorf("%w: activate plugin %s: %v", ErrPluginLoadFailed, pluginPublicId, err)
	}

	logger.Log.Infof("插件子进程已激活: %s", pluginPublicId)

	// 配置 schema 版本漂移告警（第⑤节安全网）：激活（含插件自迁移）后扫 plugin_storage 行，
	// 存在 schema_version < 声明 configSchemaVersion 的行则告警（迁移未完成或插件未声明 MigrateConfig）。
	// legacy 插件（configSchemaVersion=0）整体跳过。best-effort：读失败静默，不阻断加载。
	if deps.PluginInfo.ConfigSchemaVersion > 0 {
		if entries, err := deps.PluginCtx.GetAllValues(); err == nil {
			for k, v := range entries {
				if v.SchemaVersion < int32(deps.PluginInfo.ConfigSchemaVersion) {
					logger.Log.Warnf("插件 %s 配置 schema 漂移: key=%s schemaVersion=%d < target=%d（迁移未完成或插件未声明 MigrateConfig）",
						pluginPublicId, k, v.SchemaVersion, deps.PluginInfo.ConfigSchemaVersion)
				}
			}
		}
	}

	// 按清单声明派生注册作品拉取/站点浏览器代理；注册失败（如同键条目残留）即本次
	// 加载失败，由激活相位统一回滚清两注册表
	if err := l.registerDeclaredExtensions(deps.PluginInfo); err != nil {
		client.Kill()
		return fmt.Errorf("%w: register declared extensions %s: %v", ErrPluginLoadFailed, pluginPublicId, err)
	}

	// 注册插件自定义资源类型(声明 extensions.resourceTypes 段时);
	// best-effort:坏 spec/同名记日志跳过,不阻断加载、不株连插件其他能力。
	registerPluginResourceTypes(deps.PluginInfo)

	entry := &pluginEntry{
		client:      client,
		services:    services,
		info:        deps.PluginInfo,
		activatedAt: time.Now(),
	}

	l.mu.Lock()
	l.processes[pluginPublicId] = entry
	l.mu.Unlock()

	return nil
}

// UnloadPlugin 卸载插件的所有扩展点并停止子进程
func (l *Loader) UnloadPlugin(pluginPublicId string) error {
	l.mu.Lock()
	entry, ok := l.processes[pluginPublicId]
	if ok {
		delete(l.processes, pluginPublicId)
	}
	l.mu.Unlock()

	if ok {
		l.gracefulShutdown(entry)
		entry.client.Kill()
		unregisterPluginResourceTypes(entry.info)
	}

	l.workFetchRegistry.UnregisterAll(pluginPublicId)
	l.siteBrowserRegistry.UnregisterAll(pluginPublicId)
	l.unregisterUrlListener(pluginPublicId)
	logger.Log.Info("插件已卸载", "plugin", pluginPublicId)
	return nil
}

// UnloadAll 卸载所有已加载的插件，返回卸载的插件 ID 列表
func (l *Loader) UnloadAll() []string {
	l.mu.Lock()
	ids := make([]string, 0, len(l.processes))
	for id := range l.processes {
		ids = append(ids, id)
	}
	l.mu.Unlock()

	for _, id := range ids {
		l.UnloadPlugin(id)
	}

	// 关闭 Job Object（设置了 KILL_ON_JOB_CLOSE，残留子进程会被自动终止）
	if l.processGroup != nil {
		l.processGroup.Close()
	}

	return ids
}

const shutdownTimeout = 5 * time.Second

// gracefulShutdown 通知插件进程执行清理逻辑，超时后放弃等待（由后续 client.Kill() 强制终止）
func (l *Loader) gracefulShutdown(entry *pluginEntry) {
	if entry.client.Exited() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	_, err := entry.services.Lifecycle.Shutdown(ctx, &gen.Empty{})
	if err != nil {
		logger.Log.Warnf("插件优雅关闭失败（将强制终止）: %v", err)
	}
}

// GetServices 获取插件的 gRPC 服务客户端（供 proxy 使用）
func (l *Loader) GetServices(pluginPublicId string) (*pluginsdktransport.GRPCPluginClient, bool) {
	l.mu.RLock()
	entry, ok := l.processes[pluginPublicId]
	l.mu.RUnlock()
	if !ok {
		return nil, false
	}
	// 检测插件进程是否已崩溃
	if entry.client.Exited() {
		l.handlePluginCrash(pluginPublicId)
		return nil, false
	}
	return entry.services, true
}

// RuntimeStatus 插件运行时状态
type RuntimeStatus struct {
	IsRunning   bool
	PID         int
	ActivatedAt time.Time
}

// GetPluginRuntimeStatus 获取插件运行时状态
func (l *Loader) GetPluginRuntimeStatus(pluginPublicId string) *RuntimeStatus {
	l.mu.RLock()
	entry, ok := l.processes[pluginPublicId]
	l.mu.RUnlock()

	if !ok {
		return &RuntimeStatus{IsRunning: false}
	}

	pid := 0
	if reattach := entry.client.ReattachConfig(); reattach != nil {
		pid = reattach.Pid
	}

	return &RuntimeStatus{
		IsRunning:   !entry.client.Exited(),
		PID:         pid,
		ActivatedAt: entry.activatedAt,
	}
}

// handlePluginCrash 处理插件进程崩溃：摘除进程表条目并触发崩溃通知。
// 作品拉取/站点浏览器/URL 监听等运行痕迹的清理统一经生命周期参与者的 OnStopped 执行
func (l *Loader) handlePluginCrash(pluginPublicId string) {
	l.mu.Lock()
	_, stillInMap := l.processes[pluginPublicId]
	if stillInMap {
		delete(l.processes, pluginPublicId)
	}
	l.mu.Unlock()

	if stillInMap {
		if l.crashNotifier != nil {
			l.crashNotifier(pluginPublicId)
		}
		logger.Log.Warn("插件进程崩溃，已清理", zap.String("plugin", pluginPublicId))
	}
}

// PluginInfo 插件基本信息
type PluginInfo struct {
	ID                  int64
	PublicID            string
	Name                string
	Version             string
	ContractVersion     int                              // 插件编译时锁定的契约版本（0=未声明/缺字段，校验时拒载，须声明）
	ConfigSchemaVersion int64                            // 插件配置 schema 版本（来自 plugin 记录；0=legacy/未管理，pluginContext.SetValue 据此盖戳到 plugin_storage.schema_version）
	WorkFetch           []dto.WorkFetchDeclaration       // 作品拉取条目声明（含各自的 options 可选方法组与 urlPatterns 监听模式；来自 manifest）
	SiteBrowsers        []dto.SiteBrowserDeclaration     // 站点浏览器条目声明（各条目 id/name/description；激活期据此派生注册代理；来自 manifest）
	SiteAuthorFetch     []dto.SiteAuthorFetchDeclaration // 站点作者拉取能力包条目声明（各条目 id/name/sites；空=未声明该能力包）
	ResourceTypes       []dto.ResourceTypeDeclaration    // 插件自定义资源类型声明（来自 manifest；该段在场即注册进 Registry）
	Author              string
	EntryPath           string
	RootPath            string
}

// ========== HostDeps 适配器 ==========
// 将主程序侧的 PluginContext 适配为 SDK HostDeps 接口
// 这些适配器将 PluginContext 的方法转换为 SDK HostDeps 的 context.Context 版本

type hostStorageProvider struct {
	ctx sdkdto.PluginContext
}

func (p *hostStorageProvider) GetValue(_ context.Context, key string) (*sdkdto.StorageValue, error) {
	return p.ctx.GetValue(key)
}

func (p *hostStorageProvider) SetValue(_ context.Context, key, value string) error {
	return p.ctx.SetValue(key, value)
}

func (p *hostStorageProvider) SetValueEncrypted(_ context.Context, key, value string) error {
	return p.ctx.SetValueEncrypted(key, value)
}

func (p *hostStorageProvider) DeleteValue(_ context.Context, key string) error {
	return p.ctx.DeleteValue(key)
}

func (p *hostStorageProvider) GetAllValues(_ context.Context) (map[string]*sdkdto.StorageValue, error) {
	return p.ctx.GetAllValues()
}

type hostPluginRootProvider struct {
	ctx sdkdto.PluginContext
}

func (p *hostPluginRootProvider) GetPluginRoot(_ context.Context, isRelative bool) string {
	return p.ctx.GetPluginRoot(isRelative)
}

type hostTaskCreateProvider struct {
	ctx sdkdto.PluginContext
}

func (p *hostTaskCreateProvider) CreateTask(_ context.Context, url string) (*sdkdto.CreateTaskResult, error) {
	return p.ctx.CreateTask(url)
}

type hostFrontendEventProvider struct {
	ctx sdkdto.PluginContext
}

func (p *hostFrontendEventProvider) PublishToFrontend(topic string, data []byte) error {
	return p.ctx.PublishToFrontend(topic, data)
}

func (p *hostFrontendEventProvider) SubscribeFrontend(topic string, pushCh func([]byte)) (func(), error) {
	ch, err := p.ctx.SubscribeFrontend(topic)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for data := range ch {
			pushCh(data)
		}
	}()
	return func() { <-done }, nil
}

func (p *hostFrontendEventProvider) UnsubscribeFrontend(topic string) error {
	return p.ctx.UnsubscribeFrontend(topic)
}

// hostLibraryQueryProvider 将 PluginContext 的库查询方法组适配为 HostDeps 的
// dto.LibraryQueryProvider（context.Context + 契约请求消息形态），供 LibraryQuery
// gRPC 服务端委托；未命中/未配置等语义码错误由 pluginContext 内部产生并原样透传
type hostLibraryQueryProvider struct {
	ctx sdkdto.PluginContext
}

// ===== 作品 =====

func (p *hostLibraryQueryProvider) GetWorkById(ctx context.Context, req *gen.GetWorkByIdRequest) (*gen.WorkWithSite, error) {
	return p.ctx.GetWorkById(req.WorkId)
}

func (p *hostLibraryQueryProvider) GetWorkBySiteKey(ctx context.Context, req *gen.GetWorkBySiteKeyRequest) (*gen.WorkWithSite, error) {
	return p.ctx.GetWorkBySiteKey(req.SiteKey, req.SiteWorkId)
}

func (p *hostLibraryQueryProvider) QueryWorks(ctx context.Context, req *gen.QueryWorksRequest) (*gen.QueryWorksResponse, error) {
	return p.ctx.QueryWorks(req)
}

// ===== 资源与 store =====

func (p *hostLibraryQueryProvider) ListResourcesByWorkId(ctx context.Context, req *gen.ListResourcesByWorkIdRequest) (*gen.ListResourcesByWorkIdResponse, error) {
	return p.ctx.ListResourcesByWorkId(req.WorkId)
}

// ===== 作者 =====

func (p *hostLibraryQueryProvider) GetLocalAuthorById(ctx context.Context, req *gen.GetLocalAuthorByIdRequest) (*gen.LocalAuthorDTO, error) {
	return p.ctx.GetLocalAuthorById(req.LocalAuthorId)
}

func (p *hostLibraryQueryProvider) QueryLocalAuthors(ctx context.Context, req *gen.QueryLocalAuthorsRequest) (*gen.QueryLocalAuthorsResponse, error) {
	return p.ctx.QueryLocalAuthors(req)
}

func (p *hostLibraryQueryProvider) GetSiteAuthorBySiteKey(ctx context.Context, req *gen.GetSiteAuthorBySiteKeyRequest) (*gen.SiteAuthorInfo, error) {
	return p.ctx.GetSiteAuthorBySiteKey(req.SiteKey, req.SiteAuthorId)
}

func (p *hostLibraryQueryProvider) QuerySiteAuthors(ctx context.Context, req *gen.QuerySiteAuthorsRequest) (*gen.QuerySiteAuthorsResponse, error) {
	return p.ctx.QuerySiteAuthors(req)
}

func (p *hostLibraryQueryProvider) ListAuthorsByWorkId(ctx context.Context, req *gen.ListAuthorsByWorkIdRequest) (*gen.ListAuthorsByWorkIdResponse, error) {
	return p.ctx.ListAuthorsByWorkId(req.WorkId)
}

// ===== 标签 =====

func (p *hostLibraryQueryProvider) GetLocalTagById(ctx context.Context, req *gen.GetLocalTagByIdRequest) (*gen.LocalTagDTO, error) {
	return p.ctx.GetLocalTagById(req.LocalTagId)
}

func (p *hostLibraryQueryProvider) QueryLocalTags(ctx context.Context, req *gen.QueryLocalTagsRequest) (*gen.QueryLocalTagsResponse, error) {
	return p.ctx.QueryLocalTags(req)
}

func (p *hostLibraryQueryProvider) GetSiteTagBySiteKey(ctx context.Context, req *gen.GetSiteTagBySiteKeyRequest) (*gen.SiteTagInfo, error) {
	return p.ctx.GetSiteTagBySiteKey(req.SiteKey, req.SiteTagId)
}

func (p *hostLibraryQueryProvider) QuerySiteTags(ctx context.Context, req *gen.QuerySiteTagsRequest) (*gen.QuerySiteTagsResponse, error) {
	return p.ctx.QuerySiteTags(req)
}

func (p *hostLibraryQueryProvider) ListTagsByWorkId(ctx context.Context, req *gen.ListTagsByWorkIdRequest) (*gen.ListTagsByWorkIdResponse, error) {
	return p.ctx.ListTagsByWorkId(req.WorkId)
}

// ===== 作品集 =====

func (p *hostLibraryQueryProvider) GetWorkSetById(ctx context.Context, req *gen.GetWorkSetByIdRequest) (*gen.WorkSet, error) {
	return p.ctx.GetWorkSetById(req.WorkSetId)
}

func (p *hostLibraryQueryProvider) GetWorkSetBySiteKey(ctx context.Context, req *gen.GetWorkSetBySiteKeyRequest) (*gen.WorkSet, error) {
	return p.ctx.GetWorkSetBySiteKey(req.SiteKey, req.SiteWorkSetId)
}

func (p *hostLibraryQueryProvider) ListWorkSetsByWorkId(ctx context.Context, req *gen.ListWorkSetsByWorkIdRequest) (*gen.ListWorkSetsByWorkIdResponse, error) {
	return p.ctx.ListWorkSetsByWorkId(req.WorkId)
}

func (p *hostLibraryQueryProvider) ListParentWorkSets(ctx context.Context, req *gen.ListParentWorkSetsRequest) (*gen.ListParentWorkSetsResponse, error) {
	return p.ctx.ListParentWorkSets(req.WorkSetId)
}

func (p *hostLibraryQueryProvider) ListChildWorkSets(ctx context.Context, req *gen.ListChildWorkSetsRequest) (*gen.ListChildWorkSetsResponse, error) {
	return p.ctx.ListChildWorkSets(req.WorkSetId)
}

// ===== 站点 / 工作目录 =====

func (p *hostLibraryQueryProvider) ListSites(ctx context.Context, req *gen.Empty) (*gen.ListSitesResponse, error) {
	return p.ctx.ListSites()
}

func (p *hostLibraryQueryProvider) GetWorkDir(ctx context.Context, req *gen.Empty) (*gen.GetWorkDirResponse, error) {
	return p.ctx.GetWorkDir()
}
