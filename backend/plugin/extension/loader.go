package extension

import (
	"errors"
	"fmt"
	"plugin"

	"github.com/library-squirrel/wails/backend/base/logger"
	"github.com/library-squirrel/wails/backend/base/model/dto"
	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
)

// 错误定义
var (
	ErrPluginLoadFailed = errors.New("plugin load failed")
	ErrNoEntrySymbol    = errors.New("no Activate symbol found in plugin")
	ErrInvalidEntry     = errors.New("invalid plugin entry: Activate must be func(pluginsdk.PluginContext)")
)

// Loader 插件加载器
type Loader struct {
	taskHandlerRegistry *TaskHandlerRegistry
	siteBrowserRegistry *SiteBrowserRegistry
	slotRegistry        *SlotRegistry
}

// NewLoader 创建插件加载器
func NewLoader(
	taskHandlerRegistry *TaskHandlerRegistry,
	siteBrowserRegistry *SiteBrowserRegistry,
	slotRegistry *SlotRegistry,
) *Loader {
	return &Loader{
		taskHandlerRegistry: taskHandlerRegistry,
		siteBrowserRegistry: siteBrowserRegistry,
		slotRegistry:        slotRegistry,
	}
}

// LoadPlugin 加载插件动态库并触发其 Activate 函数完成扩展点注册
// pluginPath: 插件 DLL 路径
// pluginPublicId: 插件公开ID，用于 panic 回滚
// ctx: 插件上下文，主程序提供给插件的完整 API
func (l *Loader) LoadPlugin(pluginPath string, pluginPublicId string, ctx pluginsdk.PluginContext) (err error) {
	// 加载插件动态库
	p, err := plugin.Open(pluginPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPluginLoadFailed, err)
	}

	// 查找 Activate 入口符号
	symbol, err := p.Lookup("Activate")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNoEntrySymbol, err)
	}

	// 类型断言为 func(pluginsdk.PluginContext)
	activateFunc, ok := symbol.(func(pluginsdk.PluginContext))
	if !ok {
		return ErrInvalidEntry
	}

	// 防止插件 panic 导致主程序崩溃
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin Activate panicked: %v", r)
			logger.Log.Errorf("Plugin %s Activate panicked, rolling back registered extensions", pluginPublicId)
			l.UnloadPlugin(pluginPublicId)
		}
	}()

	// 触发插件初始化
	activateFunc(ctx)

	logger.Log.Infof("Plugin activated: %s", pluginPublicId)
	return nil
}

// UnloadPlugin 卸载插件的所有扩展点
func (l *Loader) UnloadPlugin(pluginPublicId string) error {
	l.taskHandlerRegistry.UnregisterAll(pluginPublicId)
	l.siteBrowserRegistry.UnregisterAll(pluginPublicId)
	l.slotRegistry.UnregisterAll(pluginPublicId)
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
