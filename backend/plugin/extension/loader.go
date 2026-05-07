package extension

import (
	"errors"
	"fmt"
	"sync"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
)

// 错误定义
var (
	ErrPluginLoadFailed = errors.New("plugin load failed")
)

// Loader 插件加载器
type Loader struct {
	taskHandlerRegistry *TaskHandlerRegistry
	siteBrowserRegistry *SiteBrowserRegistry

	// 子进程模式：跟踪活跃的插件进程
	processes map[string]*PluginProcess // publicId -> process
	mu        sync.RWMutex
}

// NewLoader 创建插件加载器
func NewLoader(
	taskHandlerRegistry *TaskHandlerRegistry,
	siteBrowserRegistry *SiteBrowserRegistry,
) *Loader {
	return &Loader{
		taskHandlerRegistry: taskHandlerRegistry,
		siteBrowserRegistry: siteBrowserRegistry,
		processes:           make(map[string]*PluginProcess),
	}
}

// LoadPluginProcess 以子进程模式加载插件
// exePath: 插件可执行文件路径 (.exe)
// pluginPublicId: 插件公开ID
// deps: 创建 PluginProcess 所需的依赖（含 PluginContext 用于处理 ctx/* 回调）
func (l *Loader) LoadPluginProcess(exePath string, pluginPublicId string, deps PluginProcessDeps) error {
	process := NewPluginProcess(deps)
	if err := process.Start(exePath); err != nil {
		return fmt.Errorf("%w: start plugin process %s: %v", ErrPluginLoadFailed, pluginPublicId, err)
	}

	// 发送 activate 通知
	init := &pluginsdk.PluginContextInit{
		PluginPublicID: deps.PluginInfo.PublicID,
		RootPath:       deps.PluginInfo.RootPath,
	}
	// 获取 PluginData 用于初始化
	data, err := deps.PluginCtx.GetPluginData()
	if err == nil {
		init.PluginData = data
	}

	if err := process.SendActivate(init); err != nil {
		process.Stop()
		return fmt.Errorf("%w: activate plugin %s: %v", ErrPluginLoadFailed, pluginPublicId, err)
	}

	l.mu.Lock()
	l.processes[pluginPublicId] = process
	l.mu.Unlock()

	return nil
}

// UnloadPlugin 卸载插件的所有扩展点并停止子进程
func (l *Loader) UnloadPlugin(pluginPublicId string) error {
	l.mu.Lock()
	if proc, ok := l.processes[pluginPublicId]; ok {
		proc.Stop()
		delete(l.processes, pluginPublicId)
	}
	l.mu.Unlock()

	l.taskHandlerRegistry.UnregisterAll(pluginPublicId)
	l.siteBrowserRegistry.UnregisterAll(pluginPublicId)
	// Slot 注销由 StaticResourceService + SlotRegistry 在外层处理
	logger.Log.Info("Plugin unloaded", "plugin", pluginPublicId)
	return nil
}

// GetTaskHandler 获取任务处理器
func (l *Loader) GetTaskHandler(pluginPublicId, contributionId string) (dto.TaskHandler, error) {
	ext, err := l.taskHandlerRegistry.Get(pluginPublicId, contributionId)
	if err != nil {
		return nil, err
	}
	return &taskHandlerAdapter{handler: ext.Instance}, nil
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
