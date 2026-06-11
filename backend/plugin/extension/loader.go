package extension

import (
	"context"
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
	"go.uber.org/zap"

	"github.com/library-squirrel/backend/base/logger"
	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// ErrPluginLoadFailed 错误定义
var (
	ErrPluginLoadFailed = errors.New("plugin load failed")
)

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

// LoadPluginProcess 以子进程模式加载插件
// exePath: 插件可执行文件路径 (.exe)
// pluginPublicId: 插件公开ID
// deps: 加载插件所需的依赖（含 HostDeps 用于 HostService 注册）
func (l *Loader) LoadPluginProcess(exePath string, pluginPublicId string, deps PluginProcessDeps) error {
	// 构建 HostDeps，用于在 GRPCClient 中注册 HostService
	callbacks := newHostPluginCallbacks(deps.PluginInfo, l, l.taskHandlerRegistry, l.siteBrowserRegistry)
	hostDeps := &pluginsdktransport.HostDeps{
		PluginDataProvider:      &hostPluginDataProvider{ctx: deps.PluginCtx},
		SecureStorageProvider:   &hostSecureStorageProvider{ctx: deps.PluginCtx},
		WorkSetQueryProvider:    &hostWorkSetQueryProvider{ctx: deps.PluginCtx},
		SiteSaveProvider:        &hostSiteSaveProvider{ctx: deps.PluginCtx},
		TaskCreateProvider:      &hostTaskCreateProvider{ctx: deps.PluginCtx},
		UrlListenerRegistry:     &hostUrlListenerRegistry{ctx: deps.PluginCtx},
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

	// 发送 Activate 请求
	pluginData, _ := deps.PluginCtx.GetPluginData()
	_, err = services.Lifecycle.Activate(context.Background(), &gen.ActivateRequest{
		PluginPublicId:   deps.PluginInfo.PublicID,
		PluginData:       pluginData,
		RootPath:         deps.PluginInfo.RootPath,
		HostServiceId:    services.HostServiceId,
		MainWindowHandle: uint64(deps.MainHWND),
	})
	if err != nil {
		client.Kill()
		return fmt.Errorf("%w: activate plugin %s: %v", ErrPluginLoadFailed, pluginPublicId, err)
	}

	logger.Log.Infof("插件子进程已激活: %s", pluginPublicId)

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
	}

	l.taskHandlerRegistry.UnregisterAll(pluginPublicId)
	l.siteBrowserRegistry.UnregisterAll(pluginPublicId)
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
		logger.Log.Warn("插件进程崩溃，已清理", zap.String("plugin", pluginPublicId))
	}
}

// PluginInfo 插件基本信息
type PluginInfo struct {
	ID        int64
	PublicID  string
	Name      string
	Version   string
	Author    string
	EntryPath string
	RootPath  string
}

// ========== HostDeps 适配器 ==========
// 将主程序侧的 PluginContext 适配为 SDK HostDeps 接口
// 这些适配器将 PluginContext 的方法转换为 SDK HostDeps 的 context.Context 版本

type hostPluginDataProvider struct {
	ctx sdkdto.PluginContext
}

func (p *hostPluginDataProvider) GetPluginData(_ context.Context) (string, error) {
	return p.ctx.GetPluginData()
}

func (p *hostPluginDataProvider) SetPluginData(_ context.Context, data string) error {
	return p.ctx.SetPluginData(data)
}

func (p *hostPluginDataProvider) GetPluginRoot(_ context.Context, isRelative bool) string {
	return p.ctx.GetPluginRoot(isRelative)
}

type hostSecureStorageProvider struct {
	ctx sdkdto.PluginContext
}

func (p *hostSecureStorageProvider) StoreEncryptedValue(_ context.Context, plainValue, description string) (string, error) {
	return p.ctx.StoreEncryptedValue(plainValue, description)
}

func (p *hostSecureStorageProvider) GetDecryptedValue(_ context.Context, storageKey string) (string, error) {
	return p.ctx.GetDecryptedValue(storageKey)
}

func (p *hostSecureStorageProvider) RemoveEncryptedValue(_ context.Context, storageKey string) error {
	return p.ctx.RemoveEncryptedValue(storageKey)
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

func (p *hostUrlListenerRegistry) RegisterUrlListener(_ context.Context, contributionId string, patterns []string) error {
	return p.ctx.RegisterUrlListener(contributionId, patterns)
}

func (p *hostUrlListenerRegistry) UnregisterUrlListener(_ context.Context) error {
	return p.ctx.UnregisterUrlListener()
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
