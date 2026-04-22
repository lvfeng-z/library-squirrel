package extension

import (
	"errors"
	"fmt"
	"plugin"

	"github.com/library-squirrel/wails/pkg"
	"github.com/library-squirrel/wails/pkg/model/dto"
	"go.uber.org/zap"

	"github.com/library-squirrel/wails/pkg/logger"
	"github.com/library-squirrel/wails/pkg/model"
)

// 错误定义
var (
	ErrPluginLoadFailed = errors.New("plugin load failed")
	ErrNoEntrySymbol    = errors.New("no entry symbol found in plugin")
	ErrInvalidEntry     = errors.New("invalid plugin entry")
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

// LoadPlugin 加载插件
// pluginPath: 插件 DLL 路径
// pluginInfo: 插件基本信息
func (l *Loader) LoadPlugin(pluginPath string, pluginInfo *PluginInfo) error {
	// 加载插件动态库
	p, err := plugin.Open(pluginPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPluginLoadFailed, err)
	}

	// 查找入口符号
	symbol, err := p.Lookup("PluginEntry")
	if err != nil {
		return fmt.Errorf("%w: no PluginEntry symbol", ErrNoEntrySymbol)
	}

	// 类型断言为函数
	entryFunc, ok := symbol.(func() ExtensionPoint)
	if !ok {
		return fmt.Errorf("%w: PluginEntry is not a valid function", ErrInvalidEntry)
	}

	// 调用入口函数获取扩展点
	entry := entryFunc()
	if entry == nil {
		return fmt.Errorf("%w: entry returned nil", ErrInvalidEntry)
	}

	// 分发扩展点到注册中心
	return l.registerExtensions(entry, pluginInfo)
}

// ExtensionPoint 扩展点接口
// 由插件实现，主程序调用获取其提供的扩展点
type ExtensionPoint interface {
	// GetMetadata 返回扩展点元数据
	GetMetadata() model.ExtensionMetadata
}

// registerExtensions 将扩展点注册到对应的注册中心
func (l *Loader) registerExtensions(entry ExtensionPoint, pluginInfo *PluginInfo) error {
	metadata := entry.GetMetadata()
	metadata.PluginID = pluginInfo.ID
	metadata.PluginPublicID = pluginInfo.PublicID

	switch ext := entry.(type) {
	case TaskHandlerExtension:
		taskHandler := ext.GetTaskHandler()
		metadata.Type = model.ExtensionTypeTaskHandler
		extWrapper := model.NewExtension(metadata, taskHandler)
		if err := l.taskHandlerRegistry.Register(extWrapper); err != nil {
			return err
		}
		logger.Log.Info("TaskHandler registered", zap.String("plugin", pluginInfo.PublicID), zap.String("id", metadata.ID))

	case SiteBrowserExtension:
		siteBrowser := ext.GetSiteBrowser()
		metadata.Type = model.ExtensionTypeSiteBrowser
		extWrapper := model.NewExtension(metadata, siteBrowser)
		if err := l.siteBrowserRegistry.Register(extWrapper); err != nil {
			return err
		}
		logger.Log.Info("SiteBrowser registered", zap.String("plugin", pluginInfo.PublicID), zap.String("id", metadata.ID))

	case SlotExtension:
		slotConfig := ext.GetSlotConfig()
		metadata.Type = model.ExtensionTypeSlot
		// 构建 domain.SlotConfig
		domainSlot := pkg.NewSlotConfig()
		domainSlot.ExtensionMetadata = &model.ExtensionMetadata{
			Type:           metadata.Type,
			ID:             metadata.ID,
			PluginID:       pluginInfo.ID,
			PluginPublicID: pluginInfo.PublicID,
			Name:           metadata.Name,
			Description:    metadata.Description,
		}
		domainSlot.SlotType = pkg.SlotType(slotConfig.SlotType)
		domainSlot.Content = slotConfig.Content
		domainSlot.ContentType = pkg.ContentType(slotConfig.ContentType)
		domainSlot.Title = slotConfig.Title
		domainSlot.Icon = slotConfig.Icon
		domainSlot.Order = slotConfig.Order

		extWrapper := model.NewExtension(*domainSlot.ExtensionMetadata, domainSlot)
		if err := l.slotRegistry.Register(extWrapper); err != nil {
			return err
		}
		logger.Log.Info("Slot registered", zap.String("plugin", pluginInfo.PublicID), zap.String("id", metadata.ID))

	default:
		return fmt.Errorf("unknown extension type: %T", ext)
	}

	return nil
}

// UnloadPlugin 卸载插件的所有扩展点
func (l *Loader) UnloadPlugin(pluginPublicId string) error {
	// 从所有注册中心移除插件的扩展点
	l.taskHandlerRegistry.UnregisterAll(pluginPublicId)
	l.siteBrowserRegistry.UnregisterAll(pluginPublicId)
	l.slotRegistry.UnregisterAll(pluginPublicId)
	logger.Log.Info("Plugin unloaded", zap.String("plugin", pluginPublicId))
	return nil
}

// GetTaskHandler 获取任务处理器
func (l *Loader) GetTaskHandler(pluginPublicId, contributionId string) (dto.TaskHandler, error) {
	ext, err := l.taskHandlerRegistry.Get(pluginPublicId, contributionId)
	if err != nil {
		return nil, err
	}
	return ext.Instance, nil
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

// TaskHandlerExtension 任务处理器扩展点接口
type TaskHandlerExtension interface {
	ExtensionPoint
	GetTaskHandler() dto.TaskHandler
}

// SiteBrowserExtension 站点浏览器扩展点接口
type SiteBrowserExtension interface {
	ExtensionPoint
	GetSiteBrowser() SiteBrowser
}

// SlotExtension 插槽扩展点接口
type SlotExtension interface {
	ExtensionPoint
	GetSlotConfig() *pkg.SlotConfig
}
