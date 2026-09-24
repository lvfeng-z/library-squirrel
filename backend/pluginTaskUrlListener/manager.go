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

// declaredEntry 已登记条目的记忆：条目级摘除后恢复参与时重挂索引的数据源
// （候选载体含插件实体、条目 id 与条目声明的站点域）
type declaredEntry struct {
	candidate *PluginWithExtension
	patterns  []string
}

// Manager URL 监听派生索引：激活期按清单作品拉取条目的 urlPatterns 登记，
// 停用/崩溃随插件清理回调整插件注销。纯内存态，无持久化。
type Manager struct {
	mu        sync.RWMutex
	listeners map[string][]*PluginWithExtension // key: 正则表达式字符串, value: 监听条目列表
	// declared 登记记忆：插件公开 ID → 条目 id → 登记时候选载体与模式清单。
	// 整插件注销（卸载/崩溃）时随索引一并清除；条目级摘除保留记忆供恢复参与重挂
	declared map[string]map[string]declaredEntry
}

// NewManager 创建管理器
func NewManager() *Manager {
	return &Manager{
		listeners: make(map[string][]*PluginWithExtension),
		declared:  make(map[string]map[string]declaredEntry),
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
// 登记同时写入登记记忆，供参与度条目级摘除后恢复参与时重挂。
func (m *Manager) RegisterDeclared(plugin *domain.Plugin, handlers []dto.WorkFetchDeclaration) {
	if plugin == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, handler := range handlers {
		patterns := make([]string, 0, len(handler.UrlPatterns))
		candidate := &PluginWithExtension{
			Plugin:       plugin,
			ExtensionKey: string(model.ExtensionTypeWorkFetch),
			ExtensionID:  handler.ID,
			SiteKey:      strings.TrimSpace(handler.SiteKey),
		}
		for _, pattern := range handler.UrlPatterns {
			if pattern == "" {
				continue
			}
			patterns = append(patterns, pattern)
			m.insertListenerEntry(pattern, candidate)
		}
		if len(patterns) == 0 {
			continue
		}
		entries := m.declared[plugin.PublicID.String]
		if entries == nil {
			entries = make(map[string]declaredEntry)
			m.declared[plugin.PublicID.String] = entries
		}
		entries[handler.ID] = declaredEntry{candidate: candidate, patterns: patterns}
	}
}

// insertListenerEntry 将候选挂入模式索引；同模式下按（插件公开 ID, 条目 id）复合键查重，
// 同一（插件, 条目）重复登记只保留一份。调用方持 m.mu
func (m *Manager) insertListenerEntry(pattern string, candidate *PluginWithExtension) {
	plugins, exists := m.listeners[pattern]
	if !exists {
		plugins = make([]*PluginWithExtension, 0)
		m.listeners[pattern] = plugins
	}
	for _, p := range plugins {
		if p.PublicID.String == candidate.PublicID.String && p.ExtensionID == candidate.ExtensionID {
			return
		}
	}
	m.listeners[pattern] = append(plugins, candidate)
}

// ApplyWorkFetchEntryParticipation 作品拉取条目参与度变化的派生索引联动。URL 监听条目在
// 参与度词汇（workFetch | siteAuthorFetch | siteBrowsers | resourceTypes | frontendExtensions
// 五 point）中无独立条目——URL 监听的声明来源 = 清单 workFetch 条目的 urlPatterns，故按
// point=workFetch 的条目级变化做最小映射联动：条目停用 → 条目级摘除其全部模式；
// 恢复参与 → 按登记记忆重挂该条目。未登记过的条目（未声明 urlPatterns 或已整插件注销）
// 无操作
func (m *Manager) ApplyWorkFetchEntryParticipation(pluginPublicId, entryId string, active bool) {
	if !active {
		m.Unregister(pluginPublicId, entryId)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.declared[pluginPublicId][entryId]
	if !ok {
		return
	}
	for _, pattern := range entry.patterns {
		m.insertListenerEntry(pattern, entry.candidate)
	}
}

// Unregister 取消注册插件的监听条目
// extensionId 为空：清该插件的所有监听（卸载/崩溃场景），登记记忆一并清除
// extensionId 非空：只清该插件下指定 extensionId 的监听（精细注销场景），登记记忆保留
// 供恢复参与重挂
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
	if extensionId == "" {
		delete(m.declared, pluginPublicId)
	}
}
