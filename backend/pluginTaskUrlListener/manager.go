package pluginTaskUrlListener

import (
	"regexp"
	"strings"
	"sync"

	"github.com/library-squirrel/backend/base/model"
	dto "github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
)

// PluginWithExtension 带贡献点的插件（任务创建候选枚举的单个条目）
type PluginWithExtension struct {
	*domain.Plugin
	ExtensionKey string // 贡献点类型
	ExtensionID  string // 贡献点ID
	SiteKey      string // 条目声明的站点域（清单 workFetch[].siteKey，登记时去首尾空白；缺省空 = 消费方按任务 URL host 兜底判定站点域）
}

// Manager URL 监听派生索引：激活期按清单作品拉取条目的 urlPatterns 登记，
// 停用/崩溃随插件清理回调整插件注销。纯内存态，无持久化。
type Manager struct {
	mu        sync.RWMutex
	listeners map[string][]*PluginWithExtension // key: 正则表达式字符串, value: 监听条目列表
}

// NewManager 创建管理器
func NewManager() *Manager {
	return &Manager{
		listeners: make(map[string][]*PluginWithExtension),
	}
}

// ListListener 根据URL获取监听此链接的插件条目列表。同一清单条目（插件公开 ID + 条目 id
// 复合键）经多个模式命中时只产出一次。
func (m *Manager) ListListener(url string) []*PluginWithExtension {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*PluginWithExtension
	seen := make(map[string]bool)
	for pattern, plugins := range m.listeners {
		regex, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if !regex.MatchString(url) {
			continue
		}
		for _, p := range plugins {
			key := p.PublicID.String + "\x00" + p.ExtensionID
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, p)
		}
	}
	return result
}

// RegisterDeclared 按清单声明的作品拉取条目登记派生索引：条目键 = 插件公开 ID + 条目 id
// 复合键，逐条目的 urlPatterns 各模式入索引；未声明 urlPatterns 的条目不监听、跳过。
// 条目声明的站点域（siteKey）随候选存入，任务面在候选上直接可读、无需回查清单。
func (m *Manager) RegisterDeclared(plugin *domain.Plugin, handlers []dto.WorkFetchDeclaration) {
	if plugin == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, handler := range handlers {
		for _, pattern := range handler.UrlPatterns {
			if pattern == "" {
				continue
			}
			plugins, exists := m.listeners[pattern]
			if !exists {
				plugins = make([]*PluginWithExtension, 0)
				m.listeners[pattern] = plugins
			}
			// 同模式下按复合键查重：同一（插件, 条目）重复登记只保留一份
			found := false
			for _, p := range plugins {
				if p.PublicID.String == plugin.PublicID.String && p.ExtensionID == handler.ID {
					found = true
					break
				}
			}
			if !found {
				m.listeners[pattern] = append(plugins, &PluginWithExtension{
					Plugin:       plugin,
					ExtensionKey: string(model.ExtensionTypeWorkFetch),
					ExtensionID:  handler.ID,
					SiteKey:      strings.TrimSpace(handler.SiteKey),
				})
			}
		}
	}
}

// Unregister 取消注册插件的监听条目
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
