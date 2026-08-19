package pluginTaskUrlListener

import (
	"regexp"
	"sync"

	domain "github.com/library-squirrel/backend/base/model/entity"
)

// PluginTaskUrlListener 插件任务URL监听器
type PluginTaskUrlListener struct {
	PluginID       int64  // 插件ID
	PluginPublicID string // 插件公开ID
	Pattern        string // 监听表达式（正则表达式字符串）
}

// PluginWithExtension 带贡献点的插件
type PluginWithExtension struct {
	*domain.Plugin
	ExtensionKey string // 贡献点类型
	ExtensionID  string // 贡献点ID
}

// Manager 插件任务URL监听器管理器
type Manager struct {
	mu        sync.RWMutex
	listeners map[string][]*PluginWithExtension // key: 正则表达式字符串, value: 插件列表
}

// NewManager 创建管理器
func NewManager() *Manager {
	return &Manager{
		listeners: make(map[string][]*PluginWithExtension),
	}
}

// ListListener 根据URL获取监听此链接的插件列表
func (m *Manager) ListListener(url string) []*PluginWithExtension {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*PluginWithExtension
	for pattern, plugins := range m.listeners {
		regex, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if regex.MatchString(url) {
			result = append(result, plugins...)
		}
	}
	return result
}

// Register 注册插件的URL监听器
func (m *Manager) Register(plugin *PluginWithExtension, patterns []string) {
	if plugin == nil || len(patterns) == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		plugins, exists := m.listeners[pattern]
		if !exists {
			plugins = make([]*PluginWithExtension, 0)
			m.listeners[pattern] = plugins
		}
		// 检查是否已存在
		found := false
		for _, p := range plugins {
			if p.PublicID == plugin.PublicID {
				found = true
				break
			}
		}
		if !found {
			m.listeners[pattern] = append(plugins, plugin)
		}
	}
}

// Unregister 取消注册插件的监听器
// extensionId 为空：清该插件的所有监听（卸载/崩溃场景）
// extensionId 非空：只清该插件下指定 extensionId 的监听（精细注销场景）
func (m *Manager) Unregister(pluginPublicId string, extensionId string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for pattern, plugins := range m.listeners {
		filtered := make([]*PluginWithExtension, 0)
		for _, p := range plugins {
			remove := p.PublicID.Valid && p.PublicID.String == pluginPublicId &&
				(extensionId == "" || p.ExtensionID == extensionId)
			if !remove {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) == 0 {
			delete(m.listeners, pattern)
		} else {
			m.listeners[pattern] = filtered
		}
	}
}

// Clear 清空所有监听器
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = make(map[string][]*PluginWithExtension)
}

// ListPatternsByPlugin 获取指定插件注册的所有 URL 匹配模式
func (m *Manager) ListPatternsByPlugin(pluginPublicId string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var patterns []string
	for pattern, plugins := range m.listeners {
		for _, p := range plugins {
			if p.PublicID.Valid && p.PublicID.String == pluginPublicId {
				patterns = append(patterns, pattern)
				break
			}
		}
	}
	return patterns
}
