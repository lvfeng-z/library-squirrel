package extension

import (
	"strings"
	"sync"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"go.uber.org/zap"
)

const (
	KeySeparator = "/"
)

// WorkFetchRegistry 作品拉取注册中心
type WorkFetchRegistry struct {
	mu         sync.RWMutex
	extensions map[string]*model.Extension[pluginsdkdto.WorkFetcher] // key: pluginPublicId/extensionId
}

// NewWorkFetchRegistry 创建作品拉取注册中心
func NewWorkFetchRegistry() *WorkFetchRegistry {
	return &WorkFetchRegistry{
		extensions: make(map[string]*model.Extension[pluginsdkdto.WorkFetcher]),
	}
}

// makeKey 生成存储键
func makeKey(pluginPublicId, extensionId string) string {
	return pluginPublicId + KeySeparator + extensionId
}

// Register 注册扩展点
func (r *WorkFetchRegistry) Register(extension *model.Extension[pluginsdkdto.WorkFetcher]) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeKey(extension.Metadata.PluginPublicID, extension.Metadata.ID)
	if _, exists := r.extensions[key]; exists {
		return ErrExtensionAlreadyExists
	}
	r.extensions[key] = extension
	logger.Log.Info("WorkFetch 已注册",
		zap.String("key", key),
		zap.String("name", extension.Metadata.Name))
	return nil
}

// Unregister 取消注册
func (r *WorkFetchRegistry) Unregister(pluginPublicId string, extensionId string) error {
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
func (r *WorkFetchRegistry) UnregisterAll(pluginPublicId string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := pluginPublicId + KeySeparator
	count := 0
	for key := range r.extensions {
		if strings.HasPrefix(key, prefix) {
			delete(r.extensions, key)
			count++
		}
	}
	if count > 0 {
		logger.Log.Info("WorkFetch 已注销", zap.String("plugin", pluginPublicId), zap.Int("count", count))
	}
	return nil
}

// Get 获取扩展点
func (r *WorkFetchRegistry) Get(pluginPublicId string, extensionId string) (*model.Extension[pluginsdkdto.WorkFetcher], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := makeKey(pluginPublicId, extensionId)
	ext, exists := r.extensions[key]
	if !exists {
		return nil, ErrExtensionNotFound
	}
	return ext, nil
}

// GetWorkFetcher 获取作品拉取实例（便捷方法）
// 返回注册的 WorkFetcher 实例，满足 task.WorkFetchProvider 接口
func (r *WorkFetchRegistry) GetWorkFetcher(pluginPublicId, extensionId string) (pluginsdkdto.WorkFetcher, error) {
	ext, err := r.Get(pluginPublicId, extensionId)
	if err != nil {
		return nil, err
	}
	return ext.Instance, nil
}

// GetByPlugin 获取插件的所有扩展点
func (r *WorkFetchRegistry) GetByPlugin(pluginPublicId string) ([]*model.Extension[pluginsdkdto.WorkFetcher], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Extension[pluginsdkdto.WorkFetcher]
	for _, ext := range r.extensions {
		if ext.Metadata.PluginPublicID == pluginPublicId {
			result = append(result, ext)
		}
	}
	return result, nil
}

// List 列出所有扩展点
func (r *WorkFetchRegistry) List() []*model.Extension[pluginsdkdto.WorkFetcher] {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*model.Extension[pluginsdkdto.WorkFetcher], 0, len(r.extensions))
	for _, ext := range r.extensions {
		result = append(result, ext)
	}
	return result
}
