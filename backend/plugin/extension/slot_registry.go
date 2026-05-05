package extension

import (
	"strings"
	"sync"

	domain "github.com/library-squirrel/wails/backend/base"
	"github.com/library-squirrel/wails/backend/base/model"
)

// SlotRegistry 插槽注册中心
type SlotRegistry struct {
	mu         sync.RWMutex
	extensions map[string]*model.Extension[*domain.SlotConfig] // key: pluginPublicId/extensionId
	pusher     SlotPusher
}

// NewSlotRegistry 创建插槽注册中心
func NewSlotRegistry() *SlotRegistry {
	return &SlotRegistry{
		extensions: make(map[string]*model.Extension[*domain.SlotConfig]),
	}
}

// SetPusher 设置事件推送器
func (r *SlotRegistry) SetPusher(pusher SlotPusher) {
	r.pusher = pusher
}

// GetPusher 获取事件推送器
func (r *SlotRegistry) GetPusher() SlotPusher {
	return r.pusher
}

// Register 注册扩展点
func (r *SlotRegistry) Register(extension *model.Extension[*domain.SlotConfig]) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeKey(extension.Metadata.PluginPublicID, extension.Metadata.ID)
	if _, exists := r.extensions[key]; exists {
		return ErrExtensionAlreadyExists
	}
	r.extensions[key] = extension

	// 推送注册事件
	if r.pusher != nil {
		r.pusher.PushRegister(key, extension.Instance)
	}

	return nil
}

// Unregister 取消注册
func (r *SlotRegistry) Unregister(pluginPublicId string, extensionId string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeKey(pluginPublicId, extensionId)
	ext, exists := r.extensions[key]
	if !exists {
		return ErrExtensionNotFound
	}
	delete(r.extensions, key)

	// 推送注销事件
	if r.pusher != nil && ext != nil {
		r.pusher.PushUnregister(key, ext.Metadata.PluginID)
	}

	return nil
}

// UnregisterAll 取消插件的所有扩展点
func (r *SlotRegistry) UnregisterAll(pluginPublicId string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := pluginPublicId + "/"
	var slots []interface{}
	for key := range r.extensions {
		if strings.HasPrefix(key, prefix) {
			slots = append(slots, key)
			delete(r.extensions, key)
		}
	}

	// 推送批量注销事件
	if r.pusher != nil && len(slots) > 0 {
		r.pusher.PushBatchRegister(slots)
	}

	return nil
}

// Get 获取扩展点
func (r *SlotRegistry) Get(pluginPublicId string, extensionId string) (*model.Extension[*domain.SlotConfig], error) {
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
func (r *SlotRegistry) GetByPlugin(pluginPublicId string) ([]*model.Extension[*domain.SlotConfig], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Extension[*domain.SlotConfig]
	prefix := pluginPublicId + "/"
	for _, ext := range r.extensions {
		if strings.HasPrefix(ext.Metadata.PluginPublicID, prefix) {
			result = append(result, ext)
		}
	}
	return result, nil
}

// List 列出所有扩展点
func (r *SlotRegistry) List() []*model.Extension[*domain.SlotConfig] {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*model.Extension[*domain.SlotConfig], 0, len(r.extensions))
	for _, ext := range r.extensions {
		result = append(result, ext)
	}
	return result
}

// GetSlotConfigs 获取所有插槽配置（扁平列表）
func (r *SlotRegistry) GetSlotConfigs() []*domain.SlotConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*domain.SlotConfig, 0, len(r.extensions))
	for _, ext := range r.extensions {
		result = append(result, ext.Instance)
	}
	return result
}
