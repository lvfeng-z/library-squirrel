package pluginTaskUrlListener

import (
	"regexp"
	"sync"

	domain "github.com/library-squirrel/wails/pkg/model/entity"
)

// PluginTaskUrlListener 插件任务URL监听器
type PluginTaskUrlListener struct {
	PluginID       int64  // 插件ID
	PluginPublicID string // 插件公开ID
	Pattern        string // 监听表达式（正则表达式字符串）
}

// PluginWithContribution 带贡献点的插件
type PluginWithContribution struct {
	*domain.Plugin
	ContributeKey  string // 贡献点类型
	ContributionID string // 贡献点ID
}

// Manager 插件任务URL监听器管理器
type Manager struct {
	mu        sync.RWMutex
	listeners map[string][]*PluginWithContribution // key: 正则表达式字符串, value: 插件列表
}

// NewManager 创建管理器
func NewManager() *Manager {
	return &Manager{
		listeners: make(map[string][]*PluginWithContribution),
	}
}

// ListListener 根据URL获取监听此链接的插件列表
func (m *Manager) ListListener(url string) []*PluginWithContribution {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*PluginWithContribution
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
func (m *Manager) Register(plugin *PluginWithContribution, patterns []string) {
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
			plugins = make([]*PluginWithContribution, 0)
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

// Unregister 取消注册插件的所有监听器
func (m *Manager) Unregister(pluginPublicId string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for pattern, plugins := range m.listeners {
		// 过滤掉指定插件
		filtered := make([]*PluginWithContribution, 0)
		for _, p := range plugins {
			if !p.PublicID.Valid || p.PublicID.String != pluginPublicId {
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
	m.listeners = make(map[string][]*PluginWithContribution)
}
