package extension

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/go-plugin"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/gen"
	pluginsdkliveness "github.com/lvfeng-z/library-squirrel-sdk/liveness"
	"go.uber.org/zap"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// ErrPluginLoadFailed 错误定义
var (
	ErrPluginLoadFailed     = errors.New("plugin load failed")
	ErrPluginContractTooNew = errors.New("插件契约版本过新，请升级主程序")
	ErrPluginContractTooOld = errors.New("插件契约版本过旧，请升级插件")
)

// currentContractVersion 主程序当前实现的插件契约版本（引用 SDK transport.ContractVersion，
// 与 SDK 同步发布、保持一致）。
const currentContractVersion = pluginsdktransport.ContractVersion

// minSupportedContractVersion 主程序仍兼容的最低插件契约版本；低于此版本的插件拒绝加载。
const minSupportedContractVersion = 1

// ValidateContractVersion 校验插件契约版本是否与主程序兼容。
// pluginContract 为插件声明的契约版本；0 表示未声明/缺字段，视为当前契约放行（决策9：
// 首发 minSupported=1，旧/手工插件缺字段视为最旧兼容版本，不拒绝）。
// 返回 ErrPluginContractTooNew（插件比主程序新，需升级主程序）、
// ErrPluginContractTooOld（插件低于最低支持版本，需升级插件）或 nil。
func ValidateContractVersion(pluginContract int) error {
	if pluginContract == 0 {
		pluginContract = currentContractVersion
	}
	if pluginContract > currentContractVersion {
		return ErrPluginContractTooNew
	}
	if pluginContract < minSupportedContractVersion {
		return ErrPluginContractTooOld
	}
	return nil
}

// UnmarshalCapabilities 解析 entity 持久化的 capabilities JSON 字符串为切片。
// s 无效/空/"null" 返回 nil（插件未声明能力）。
func UnmarshalCapabilities(s sql.NullString) []string {
	if !s.Valid || s.String == "" || s.String == "null" {
		return nil
	}
	var caps []string
	if err := json.Unmarshal([]byte(s.String), &caps); err != nil {
		return nil
	}
	return caps
}

// UnmarshalResourceTypes 解析 entity 持久化的 resourceTypes JSON 字符串为声明切片。
// s 无效/空/"null" 返回 nil(插件未声明自定义资源类型)。
func UnmarshalResourceTypes(s sql.NullString) []dto.ResourceTypeDeclaration {
	if !s.Valid || s.String == "" || s.String == "null" {
		return nil
	}
	var decls []dto.ResourceTypeDeclaration
	if err := json.Unmarshal([]byte(s.String), &decls); err != nil {
		return nil
	}
	return decls
}

// 插件能力枚举（内置集,可随主程序版本扩展;插件 manifest capabilities 引用这些值,主程序据未声明者跳过对应能力调用）。
const (
	// CapabilityWorkOrderQuery 作品集原站序查询能力（插件实现 sdkdto.WorkOrderQuerier 可选接口）。
	CapabilityWorkOrderQuery = "workOrderQuery"
	// CapabilityWorkSetRelationQuery 作品集父集关系查询能力（插件实现 sdkdto.WorkSetRelationQuerier 可选接口）。
	CapabilityWorkSetRelationQuery = "workSetRelationQuery"
	// CapabilityResourceTypeProvider 自定义资源类型提供能力(插件 manifest 声明 resourceTypes 段;
	// 主程序加载时解析并注册进 ResourceTypeRegistry,使插件 Create 可声明该类型资源)。
	CapabilityResourceTypeProvider = "resourceTypeProvider"
)

// CapabilityQuerier 按插件公开 ID 查询其声明的能力集合（Loader 实现，供 fetcher 声明驱动调用）。
type CapabilityQuerier interface {
	GetCapabilities(pluginPublicId string) []string
}

// GetCapabilities 返回插件声明的可选能力集合（供主程序决定是否调用对应能力；未加载/未声明返回 nil）。
func (l *Loader) GetCapabilities(pluginPublicId string) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if entry, ok := l.processes[pluginPublicId]; ok && entry.info != nil {
		return entry.info.Capabilities
	}
	return nil
}

// hasCapability 判断插件是否声明了指定能力。
func hasCapability(caps []string, cap string) bool {
	for _, c := range caps {
		if c == cap {
			return true
		}
	}
	return false
}

// registerPluginResourceTypes 注册插件声明的自定义资源类型到 ResourceTypeRegistry。
// 仅当声明 CapabilityResourceTypeProvider 通行证时解析 resourceTypes 段;每个 declaration 转 Spec 注册,
// 强校验失败(决策7同名/决策8前缀/Roles合法性)记日志跳过、不株连其他类型与插件能力。
func registerPluginResourceTypes(info *PluginInfo) {
	if info == nil || !hasCapability(info.Capabilities, CapabilityResourceTypeProvider) {
		return
	}
	for _, decl := range info.ResourceTypes {
		spec := entity.ResourceTypeSpec{
			ResourceType: decl.Type,
			Roles:        toStoreRoleSpecs(decl.Roles),
			PrimaryRoles: decl.PrimaryRoles,
		}
		if err := entity.ResourceTypeRegistry.Register(spec); err != nil {
			logger.Log.Warnf("插件 %s 自定义资源类型 %s 注册失败,跳过(不株连其他能力): %v", info.PublicID, decl.Type, err)
			continue
		}
		logger.Log.Infof("插件 %s 自定义资源类型已注册: %s", info.PublicID, decl.Type)
	}
}

// unregisterPluginResourceTypes 反注册插件声明的自定义资源类型(卸载时清理);内置类型受白名单保护不会被删。
func unregisterPluginResourceTypes(info *PluginInfo) {
	if info == nil || !hasCapability(info.Capabilities, CapabilityResourceTypeProvider) {
		return
	}
	for _, decl := range info.ResourceTypes {
		entity.ResourceTypeRegistry.Unregister(decl.Type)
	}
}

// toStoreRoleSpecs 将 manifest 声明角色转 Registry StoreRoleSpec。
func toStoreRoleSpecs(roles []dto.StoreRoleDeclaration) []entity.StoreRoleSpec {
	out := make([]entity.StoreRoleSpec, 0, len(roles))
	for _, r := range roles {
		out = append(out, entity.StoreRoleSpec{StoreType: r.StoreType, Min: r.Min, Max: r.Max})
	}
	return out
}

// CreateNoWindow Windows 子进程创建标志：不创建控制台窗口
const CreateNoWindow = 0x08000000

// PluginProcessDeps 加载插件进程所需的依赖
type PluginProcessDeps struct {
	PluginInfo          *PluginInfo
	PluginCtx           sdkdto.PluginContext
	TaskHandlerRegistry *TaskHandlerRegistry
	SiteBrowserRegistry *SiteBrowserRegistry
	MainHWND            uintptr
}

// pluginEntry 存储单个插件的 hashicorp/go-plugin 客户端和 gRPC 服务客户端
type pluginEntry struct {
	client      *plugin.Client                       // hashicorp/go-plugin 进程管理客户端
	services    *pluginsdktransport.GRPCPluginClient // gRPC 服务客户端（Task、Browser、Lifecycle）
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
	taskHandlerRegistry *TaskHandlerRegistry
	siteBrowserRegistry *SiteBrowserRegistry

	// URL 监听清理回调（插件卸载/崩溃时调用，清理该插件的 URL 监听）
	urlListenerCleaner func(pluginPublicId string)

	// 子进程模式：跟踪活跃的插件进程
	processes map[string]*pluginEntry // publicId -> entry
	mu        sync.RWMutex

	// 进程分组：Job Object 管理子进程（主进程退出时自动终止）
	processGroup *ProcessGroup
}

// NewLoader 创建插件加载器
func NewLoader(
	taskHandlerRegistry *TaskHandlerRegistry,
	siteBrowserRegistry *SiteBrowserRegistry,
) *Loader {
	// 初始化 Job Object（Windows 下主进程退出时自动终止子进程，其他平台为空操作）
	pg, err := NewProcessGroup()
	if err != nil {
		logger.Log.Warnf("创建进程分组失败（不影响功能）: %v", err)
	}

	return &Loader{
		taskHandlerRegistry: taskHandlerRegistry,
		siteBrowserRegistry: siteBrowserRegistry,
		processes:           make(map[string]*pluginEntry),
		processGroup:        pg,
	}
}

// SetUrlListenerCleaner 设置 URL 监听清理回调（插件卸载/崩溃时由 Loader 调用，清理该插件的 URL 监听）
func (l *Loader) SetUrlListenerCleaner(fn func(pluginPublicId string)) {
	l.urlListenerCleaner = fn
}

// unregisterUrlListener 清理插件 URL 监听（回调未设置时跳过）
func (l *Loader) unregisterUrlListener(pluginPublicId string) {
	if l.urlListenerCleaner != nil {
		l.urlListenerCleaner(pluginPublicId)
	}
}

// LoadPluginProcess 以子进程模式加载插件
// exePath: 插件可执行文件路径 (.exe)
// pluginPublicId: 插件公开ID
// deps: 加载插件所需的依赖（含 HostDeps 用于 HostService 注册）
func (l *Loader) LoadPluginProcess(exePath string, pluginPublicId string, deps PluginProcessDeps) error {
	// 契约版本兼容校验（加载期终检，与安装期 loadPluginPackage 预检互为兜底）
	if err := ValidateContractVersion(deps.PluginInfo.ContractVersion); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrPluginLoadFailed, pluginPublicId, err)
	}
	// 构建 HostDeps，用于在 GRPCClient 中注册 HostService
	callbacks := newHostPluginCallbacks(deps.PluginInfo, l, l.taskHandlerRegistry, l.siteBrowserRegistry)
	hostDeps := &pluginsdktransport.HostDeps{
		StorageProvider:         &hostStorageProvider{ctx: deps.PluginCtx},
		PluginRootProvider:      &hostPluginRootProvider{ctx: deps.PluginCtx},
		WorkSetQueryProvider:    &hostWorkSetQueryProvider{ctx: deps.PluginCtx},
		SiteSaveProvider:        &hostSiteSaveProvider{ctx: deps.PluginCtx},
		TaskCreateProvider:      &hostTaskCreateProvider{ctx: deps.PluginCtx},
		UrlListenerRegistry:     &hostUrlListenerRegistry{ctx: deps.PluginCtx},
		StorePathQueryProvider:  &hostStorePathProvider{ctx: deps.PluginCtx},
		FrontendEventProvider:   &hostFrontendEventProvider{ctx: deps.PluginCtx},
		OnRegisterTaskHandler:   callbacks.onRegisterTaskHandler,
		OnRegisterSiteBrowser:   callbacks.onRegisterSiteBrowser,
		OnUnregisterSiteBrowser: callbacks.onUnregisterSiteBrowser,
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

	// 注册插件自定义资源类型(声明 CapabilityResourceTypeProvider 通行证时);
	// best-effort:坏 spec/同名(决策7)记日志跳过,不阻断加载、不株连插件其他能力。
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

	l.taskHandlerRegistry.UnregisterAll(pluginPublicId)
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

// handlePluginCrash 处理插件进程崩溃
func (l *Loader) handlePluginCrash(pluginPublicId string) {
	l.mu.Lock()
	_, stillInMap := l.processes[pluginPublicId]
	if stillInMap {
		delete(l.processes, pluginPublicId)
	}
	l.mu.Unlock()

	if stillInMap {
		l.taskHandlerRegistry.UnregisterAll(pluginPublicId)
		l.siteBrowserRegistry.UnregisterAll(pluginPublicId)
		l.unregisterUrlListener(pluginPublicId)
		logger.Log.Warn("插件进程崩溃，已清理", zap.String("plugin", pluginPublicId))
	}
}

// PluginInfo 插件基本信息
type PluginInfo struct {
	ID              int64
	PublicID        string
	Name            string
	Version         string
	ContractVersion     int      // 插件编译时锁定的契约版本（0=未声明/缺字段，校验时视为当前契约放行）
	ConfigSchemaVersion int64    // 插件配置 schema 版本（来自 plugin 记录；0=legacy/未管理，pluginContext.SetValue 据此盖戳到 plugin_storage.schema_version）
	Capabilities        []string // 声明的可选能力（来自 manifest，主程序据此决定是否调用对应能力）
	ResourceTypes       []dto.ResourceTypeDeclaration // 插件自定义资源类型声明(来自 manifest;声明 resourceTypeProvider 通行证时注册进 Registry)
	Author          string
	EntryPath       string
	RootPath        string
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

type hostStorePathProvider struct {
	ctx sdkdto.PluginContext
}

func (p *hostStorePathProvider) GetStoreRelPath(ctx context.Context, taskId int64, role string, storeSeq int) (string, error) {
	return p.ctx.GetStoreRelPath(taskId, role, storeSeq)
}

type hostWorkSetQueryProvider struct {
	ctx sdkdto.PluginContext
}

func (p *hostWorkSetQueryProvider) GetWorkSetBySiteWorkSetId(_ context.Context, siteWorkSetId, siteName string) (*sdkdto.WorkSetDTO, error) {
	return p.ctx.GetWorkSetBySiteWorkSetId(siteWorkSetId, siteName)
}

type hostSiteSaveProvider struct {
	ctx sdkdto.PluginContext
}

func (p *hostSiteSaveProvider) AddSite(_ context.Context, sites []*sdkdto.SiteDTO) error {
	return p.ctx.AddSite(sites)
}

type hostTaskCreateProvider struct {
	ctx sdkdto.PluginContext
}

func (p *hostTaskCreateProvider) CreateTask(_ context.Context, url string) (*sdkdto.CreateTaskResult, error) {
	return p.ctx.CreateTask(url)
}

type hostUrlListenerRegistry struct {
	ctx sdkdto.PluginContext
}

func (p *hostUrlListenerRegistry) RegisterUrlListener(_ context.Context, extensionId string, patterns []string) error {
	return p.ctx.RegisterUrlListener(extensionId, patterns)
}

func (p *hostUrlListenerRegistry) UnregisterUrlListener(_ context.Context, extensionId string) error {
	return p.ctx.UnregisterUrlListener(extensionId)
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
