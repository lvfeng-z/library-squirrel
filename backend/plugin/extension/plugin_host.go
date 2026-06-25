package extension

import (
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"go.uber.org/zap"
)

// hostPluginCallbacks 提供 HostDeps 注册回调的实现
// 在 loader.go 中通过 HostDeps.OnRegisterTaskHandler 等字段注入
type hostPluginCallbacks struct {
	pluginInfo           *PluginInfo
	serviceAccessor      ServiceAccessor
	taskHandlerRegistry  *TaskHandlerRegistry
	siteBrowserRegistry  *SiteBrowserRegistry
}

func newHostPluginCallbacks(
	pluginInfo *PluginInfo,
	serviceAccessor ServiceAccessor,
	taskHandlerRegistry *TaskHandlerRegistry,
	siteBrowserRegistry *SiteBrowserRegistry,
) *hostPluginCallbacks {
	return &hostPluginCallbacks{
		pluginInfo:           pluginInfo,
		serviceAccessor:      serviceAccessor,
		taskHandlerRegistry:  taskHandlerRegistry,
		siteBrowserRegistry:  siteBrowserRegistry,
	}
}

func (c *hostPluginCallbacks) onRegisterTaskHandler(extensionId, name, description string) error {
	proxy := &TaskHandlerProxy{
		serviceAccessor: c.serviceAccessor,
		pluginPublicId:  c.pluginInfo.PublicID,
		extensionId:  extensionId,
	}

	metadata := model.ExtensionMetadata{
		Type:           model.ExtensionTypeTaskHandler,
		ID:             extensionId,
		PluginID:       c.pluginInfo.ID,
		PluginPublicID: c.pluginInfo.PublicID,
		Name:           name,
		Description:    description,
	}
	var handler pluginsdkdto.TaskHandler = proxy
	if err := c.taskHandlerRegistry.Register(model.NewExtension(metadata, handler)); err != nil {
		return err
	}
	logger.Log.Info("TaskHandler 已注册",
		zap.String("plugin", c.pluginInfo.PublicID), zap.String("id", extensionId))
	return nil
}

func (c *hostPluginCallbacks) onRegisterSiteBrowser(extensionId, name, description string) error {
	proxy := &SiteBrowserProxy{
		serviceAccessor: c.serviceAccessor,
		pluginPublicId:  c.pluginInfo.PublicID,
		extensionId:  extensionId,
	}

	metadata := model.ExtensionMetadata{
		Type:           model.ExtensionTypeSiteBrowser,
		ID:             extensionId,
		PluginID:       c.pluginInfo.ID,
		PluginPublicID: c.pluginInfo.PublicID,
		Name:           name,
		Description:    description,
	}
	var browser pluginsdkdto.SiteBrowser = proxy
	if err := c.siteBrowserRegistry.Register(model.NewExtension(metadata, browser)); err != nil {
		return err
	}
	logger.Log.Info("SiteBrowser 已注册",
		zap.String("plugin", c.pluginInfo.PublicID), zap.String("id", extensionId))
	return nil
}

func (c *hostPluginCallbacks) onUnregisterSiteBrowser(extensionId string) error {
	return c.siteBrowserRegistry.Unregister(c.pluginInfo.PublicID, extensionId)
}
