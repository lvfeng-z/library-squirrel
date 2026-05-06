package extension

import (
	"fmt"

	"github.com/library-squirrel/wails/backend/base"
	"github.com/library-squirrel/wails/backend/base/logger"
	"github.com/library-squirrel/wails/backend/base/model"
	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
	"go.uber.org/zap"
)

// Registrar 插件扩展点注册器，由主程序提供给插件的注册接口
type Registrar interface {
	// RegisterTaskHandler 注册任务处理器扩展点
	RegisterTaskHandler(id string, name string, description string, handler pluginsdk.TaskHandler) error
	// RegisterSiteBrowser 注册站点浏览器扩展点
	RegisterSiteBrowser(id string, name string, description string, browser pluginsdk.SiteBrowser) error
	// RegisterSlot 注册插槽扩展点
	RegisterSlot(id string, name string, description string, slotType pluginsdk.SlotType, content string, contentType pluginsdk.ContentType, title string, icon string, order int) error
}

// registrar 注册器实现
type registrar struct {
	pluginInfo          *PluginInfo
	taskHandlerRegistry *TaskHandlerRegistry
	siteBrowserRegistry *SiteBrowserRegistry
	slotRegistry        *SlotRegistry
}

func newRegistrar(pluginInfo *PluginInfo, thReg *TaskHandlerRegistry, sbReg *SiteBrowserRegistry, slotReg *SlotRegistry) *registrar {
	return &registrar{
		pluginInfo:          pluginInfo,
		taskHandlerRegistry: thReg,
		siteBrowserRegistry: sbReg,
		slotRegistry:        slotReg,
	}
}

// buildMetadata 构建扩展点元数据，自动填充插件信息
func (r *registrar) buildMetadata(extType model.ExtensionType, id string, name string, description string) model.ExtensionMetadata {
	return model.ExtensionMetadata{
		Type:           extType,
		ID:             id,
		PluginID:       r.pluginInfo.ID,
		PluginPublicID: r.pluginInfo.PublicID,
		Name:           name,
		Description:    description,
	}
}

// RegisterTaskHandler 注册任务处理器扩展点
func (r *registrar) RegisterTaskHandler(id string, name string, description string, handler pluginsdk.TaskHandler) error {
	metadata := r.buildMetadata(model.ExtensionTypeTaskHandler, id, name, description)
	if err := r.taskHandlerRegistry.Register(model.NewExtension(metadata, handler)); err != nil {
		return err
	}
	logger.Log.Info("TaskHandler registered", zap.String("plugin", r.pluginInfo.PublicID), zap.String("id", id))
	return nil
}

// RegisterSiteBrowser 注册站点浏览器扩展点
func (r *registrar) RegisterSiteBrowser(id string, name string, description string, browser pluginsdk.SiteBrowser) error {
	metadata := r.buildMetadata(model.ExtensionTypeSiteBrowser, id, name, description)
	if err := r.siteBrowserRegistry.Register(model.NewExtension(metadata, browser)); err != nil {
		return err
	}
	logger.Log.Info("SiteBrowser registered", zap.String("plugin", r.pluginInfo.PublicID), zap.String("id", id))
	return nil
}

// RegisterSlot 注册插槽扩展点
func (r *registrar) RegisterSlot(id string, name string, description string, slotType pluginsdk.SlotType, content string, contentType pluginsdk.ContentType, title string, icon string, order int) error {
	metadata := r.buildMetadata(model.ExtensionTypeSlot, id, name, description)

	domainSlot := base.NewSlotConfig()
	domainSlot.ExtensionMetadata = &model.ExtensionMetadata{
		Type:           metadata.Type,
		ID:             metadata.ID,
		PluginID:       metadata.PluginID,
		PluginPublicID: metadata.PluginPublicID,
		Name:           metadata.Name,
		Description:    metadata.Description,
	}
	domainSlot.SlotType = base.SlotType(slotType)
	domainSlot.Content = content
	domainSlot.ContentType = base.ContentType(contentType)
	domainSlot.Title = title
	domainSlot.Icon = icon
	domainSlot.Order = order

	if err := r.slotRegistry.Register(model.NewExtension(metadata, domainSlot)); err != nil {
		return err
	}
	logger.Log.Info("Slot registered", zap.String("plugin", r.pluginInfo.PublicID), zap.String("id", id), zap.String("slotType", string(slotType)))
	return nil
}

// registeredExtensionIDs 返回已注册的扩展点ID列表，用于错误报告
func (r *registrar) registeredExtensionIDs() []string {
	var ids []string
	for _, ext := range r.taskHandlerRegistry.List() {
		ids = append(ids, fmt.Sprintf("taskHandler/%s", ext.Metadata.ID))
	}
	for _, ext := range r.siteBrowserRegistry.List() {
		ids = append(ids, fmt.Sprintf("siteBrowser/%s", ext.Metadata.ID))
	}
	for _, ext := range r.slotRegistry.List() {
		ids = append(ids, fmt.Sprintf("slot/%s", ext.Metadata.ID))
	}
	return ids
}
