package extension

import (
	"strings"
	"sync"

	"github.com/library-squirrel/wails/backend/base/model"
	domain "github.com/library-squirrel/wails/backend/base/model/dto"
)

// TaskHandlerRegistry 任务处理器注册中心
type TaskHandlerRegistry struct {
	mu         sync.RWMutex
	extensions map[string]*model.Extension[domain.TaskHandler] // key: pluginPublicId/extensionId
}

// NewTaskHandlerRegistry 创建任务处理器注册中心
func NewTaskHandlerRegistry() *TaskHandlerRegistry {
	return &TaskHandlerRegistry{
		extensions: make(map[string]*model.Extension[domain.TaskHandler]),
	}
}

// makeKey 生成存储键
func makeKey(pluginPublicId, extensionId string) string {
	return pluginPublicId + "/" + extensionId
}

// Register 注册扩展点
func (r *TaskHandlerRegistry) Register(extension *model.Extension[domain.TaskHandler]) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeKey(extension.Metadata.PluginPublicID, extension.Metadata.ID)
	if _, exists := r.extensions[key]; exists {
		return ErrExtensionAlreadyExists
	}
	r.extensions[key] = extension
	return nil
}

// Unregister 取消注册
func (r *TaskHandlerRegistry) Unregister(pluginPublicId string, extensionId string) error {
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
func (r *TaskHandlerRegistry) UnregisterAll(pluginPublicId string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := pluginPublicId + "/"
	for key := range r.extensions {
		if strings.HasPrefix(key, prefix) {
			delete(r.extensions, key)
		}
	}
	return nil
}

// Get 获取扩展点
func (r *TaskHandlerRegistry) Get(pluginPublicId string, extensionId string) (*model.Extension[domain.TaskHandler], error) {
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
func (r *TaskHandlerRegistry) GetByPlugin(pluginPublicId string) ([]*model.Extension[domain.TaskHandler], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Extension[domain.TaskHandler]
	prefix := pluginPublicId + "/"
	for _, ext := range r.extensions {
		if strings.HasPrefix(ext.Metadata.PluginPublicID, prefix) {
			result = append(result, ext)
		}
	}
	return result, nil
}

// List 列出所有扩展点
func (r *TaskHandlerRegistry) List() []*model.Extension[domain.TaskHandler] {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*model.Extension[domain.TaskHandler], 0, len(r.extensions))
	for _, ext := range r.extensions {
		result = append(result, ext)
	}
	return result
}
