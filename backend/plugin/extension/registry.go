package extension

import (
	"strings"
	"sync"

	domain "github.com/library-squirrel/backend/base"
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"go.uber.org/zap"
)

// FrontendExtensionRegistry 前端扩展注册中心（管全部 7 种 kind，单一统一管道）
type FrontendExtensionRegistry struct {
	mu         sync.RWMutex
	extensions map[string]*model.Extension[*domain.FrontendExtensionConfig] // key: pluginPublicId/extensionId
	pusher     FrontendExtensionPusher
}

// NewFrontendExtensionRegistry 创建前端扩展注册中心
func NewFrontendExtensionRegistry() *FrontendExtensionRegistry {
	return &FrontendExtensionRegistry{
		extensions: make(map[string]*model.Extension[*domain.FrontendExtensionConfig]),
	}
}

// SetPusher 设置事件推送器
func (r *FrontendExtensionRegistry) SetPusher(pusher FrontendExtensionPusher) {
	r.pusher = pusher
}

// GetPusher 获取事件推送器
func (r *FrontendExtensionRegistry) GetPusher() FrontendExtensionPusher {
	return r.pusher
}

// Register 注册前端扩展
func (r *FrontendExtensionRegistry) Register(extension *model.Extension[*domain.FrontendExtensionConfig]) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeKey(extension.Metadata.PluginPublicID, extension.Metadata.ID)
	if _, exists := r.extensions[key]; exists {
		return ErrExtensionAlreadyExists
	}
	r.extensions[key] = extension
	logger.Log.Info("前端扩展已注册",
		zap.String("key", key),
		zap.String("kind", string(extension.Instance.Kind)))

	// 推送注册事件（payload 与注销事件同源，frontendExtensionId 为裸 extensionId）
	if r.pusher != nil {
		resp := FrontendExtensionConfigToResponse(extension.Instance)
		r.pusher.PushRegister(*resp)
	}

	return nil
}

// Unregister 取消注册
func (r *FrontendExtensionRegistry) Unregister(pluginPublicId string, extensionId string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeKey(pluginPublicId, extensionId)
	ext, exists := r.extensions[key]
	if !exists {
		return ErrExtensionNotFound
	}
	delete(r.extensions, key)

	// 推送注销事件（item 构造与注册事件同源）
	if r.pusher != nil && ext != nil {
		r.pusher.PushUnregister(newUnregisterItem(ext))
	}

	return nil
}

// UnregisterAll 取消插件的所有前端扩展
func (r *FrontendExtensionRegistry) UnregisterAll(pluginPublicId string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := pluginPublicId + "/"
	var items []FrontendExtensionUnregisterItem
	for key, ext := range r.extensions {
		if strings.HasPrefix(key, prefix) {
			items = append(items, newUnregisterItem(ext))
			delete(r.extensions, key)
		}
	}

	// 推送批量注销事件
	if r.pusher != nil && len(items) > 0 {
		r.pusher.PushBatchUnregister(items)
		logger.Log.Info("前端扩展已批量注销", zap.String("plugin", pluginPublicId), zap.Int("count", len(items)))
	}

	return nil
}

// Get 获取前端扩展
func (r *FrontendExtensionRegistry) Get(pluginPublicId string, extensionId string) (*model.Extension[*domain.FrontendExtensionConfig], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := makeKey(pluginPublicId, extensionId)
	ext, exists := r.extensions[key]
	if !exists {
		return nil, ErrExtensionNotFound
	}
	return ext, nil
}

// GetByPlugin 获取插件的所有前端扩展
func (r *FrontendExtensionRegistry) GetByPlugin(pluginPublicId string) ([]*model.Extension[*domain.FrontendExtensionConfig], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Extension[*domain.FrontendExtensionConfig]
	for _, ext := range r.extensions {
		if ext.Metadata.PluginPublicID == pluginPublicId {
			result = append(result, ext)
		}
	}
	return result, nil
}

// List 列出所有前端扩展
func (r *FrontendExtensionRegistry) List() []*model.Extension[*domain.FrontendExtensionConfig] {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*model.Extension[*domain.FrontendExtensionConfig], 0, len(r.extensions))
	for _, ext := range r.extensions {
		result = append(result, ext)
	}
	return result
}

// GetFrontendExtensionConfigs 获取所有前端扩展配置（扁平列表）
func (r *FrontendExtensionRegistry) GetFrontendExtensionConfigs() []*domain.FrontendExtensionConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*domain.FrontendExtensionConfig, 0, len(r.extensions))
	for _, ext := range r.extensions {
		result = append(result, ext.Instance)
	}
	return result
}
