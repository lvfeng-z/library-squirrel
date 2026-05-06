package extension

import (
	"fmt"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
	"go.uber.org/zap"
)

// Registrar 插件扩展点注册器，由主程序提供给插件的注册接口
type Registrar interface {
	// RegisterTaskHandler 注册任务处理器扩展点
	RegisterTaskHandler(id string, name string, description string, handler pluginsdk.TaskHandler) error
	// RegisterSiteBrowser 注册站点浏览器扩展点
	RegisterSiteBrowser(id string, name string, description string, browser pluginsdk.SiteBrowser) error
}

// registrar 注册器实现
type registrar struct {
	pluginInfo          *PluginInfo
	taskHandlerRegistry *TaskHandlerRegistry
	siteBrowserRegistry *SiteBrowserRegistry
}

func newRegistrar(pluginInfo *PluginInfo, thReg *TaskHandlerRegistry, sbReg *SiteBrowserRegistry) *registrar {
	return &registrar{
		pluginInfo:          pluginInfo,
		taskHandlerRegistry: thReg,
		siteBrowserRegistry: sbReg,
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

// registeredExtensionIDs 返回已注册的扩展点ID列表，用于错误报告
func (r *registrar) registeredExtensionIDs() []string {
	var ids []string
	for _, ext := range r.taskHandlerRegistry.List() {
		ids = append(ids, fmt.Sprintf("taskHandler/%s", ext.Metadata.ID))
	}
	for _, ext := range r.siteBrowserRegistry.List() {
		ids = append(ids, fmt.Sprintf("siteBrowser/%s", ext.Metadata.ID))
	}
	return ids
}
