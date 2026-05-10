package extension

import (
	"strings"
	"sync"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
	"go.uber.org/zap"
)

// SiteBrowserRegistry 站点浏览器注册中心
type SiteBrowserRegistry struct {
	mu         sync.RWMutex
	extensions map[string]*model.Extension[pluginsdk.SiteBrowser] // key: pluginPublicId/extensionId
}

// NewSiteBrowserRegistry 创建站点浏览器注册中心
func NewSiteBrowserRegistry() *SiteBrowserRegistry {
	return &SiteBrowserRegistry{
		extensions: make(map[string]*model.Extension[pluginsdk.SiteBrowser]),
	}
}

// Register 注册扩展点
func (r *SiteBrowserRegistry) Register(extension *model.Extension[pluginsdk.SiteBrowser]) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeKey(extension.Metadata.PluginPublicID, extension.Metadata.ID)
	if _, exists := r.extensions[key]; exists {
		return ErrExtensionAlreadyExists
	}
	r.extensions[key] = extension
	logger.Log.Info("SiteBrowser registered",
		zap.String("key", key),
		zap.String("name", extension.Metadata.Name))
	return nil
}

// Unregister 取消注册
func (r *SiteBrowserRegistry) Unregister(pluginPublicId string, extensionId string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeKey(pluginPublicId, extensionId)
	if _, exists := r.extensions[key]; !exists {
		return ErrExtensionNotFound
	}
	delete(r.extensions, key)
	return nil
}

// UnregisterAll 取消插件的所有扩展点
func (r *SiteBrowserRegistry) UnregisterAll(pluginPublicId string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := pluginPublicId + "/"
	count := 0
	for key := range r.extensions {
		if strings.HasPrefix(key, prefix) {
			delete(r.extensions, key)
			count++
		}
	}
	if count > 0 {
		logger.Log.Info("SiteBrowser unregistered", zap.String("plugin", pluginPublicId), zap.Int("count", count))
	}
	return nil
}

// Get 获取扩展点
func (r *SiteBrowserRegistry) Get(pluginPublicId string, extensionId string) (*model.Extension[pluginsdk.SiteBrowser], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := makeKey(pluginPublicId, extensionId)
	ext, exists := r.extensions[key]
	if !exists {
		return nil, ErrExtensionNotFound
	}
	return ext, nil
}

// GetByPlugin 获取插件的所有扩展点
func (r *SiteBrowserRegistry) GetByPlugin(pluginPublicId string) ([]*model.Extension[pluginsdk.SiteBrowser], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Extension[pluginsdk.SiteBrowser]
	prefix := pluginPublicId + "/"
	for _, ext := range r.extensions {
		if strings.HasPrefix(ext.Metadata.PluginPublicID, prefix) {
			result = append(result, ext)
		}
	}
	return result, nil
}

// List 列出所有扩展点
func (r *SiteBrowserRegistry) List() []*model.Extension[pluginsdk.SiteBrowser] {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*model.Extension[pluginsdk.SiteBrowser], 0, len(r.extensions))
	for _, ext := range r.extensions {
		result = append(result, ext)
	}
	return result
}
