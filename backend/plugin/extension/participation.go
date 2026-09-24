package extension

import (
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/plugin/participation"
	"github.com/library-squirrel/backend/plugin/settingresolver"
)

// ParticipationTruth 参与度真相层消费面（真相层 participation.Manager 结构性实现）：
// 条目有效参与查询 + 条目级变更订阅。Loader 各声明查询面经查询叠加覆盖表过滤；
// resourceTypes / siteBrowsers 条目的参与度变化经订阅联动注册面（自定义资源类型与
// 站点浏览器代理的条目级注册/反注册）。查询在 Loader.mu 读锁内调用，订阅回调在
// 真相层求值 goroutine 上串行调用
type ParticipationTruth interface {
	// EntryActive 查询插件声明条目的当前参与态（active=false 且 found=true = 条目被
	// resolver 停用；found=false = 无会话或未声明，按基线参与处理）
	EntryActive(pluginPublicId string, point settingresolver.Point, id string) (active, found bool)
	// SubscribeChanges 订阅条目级参与度变更，返回退订函数
	SubscribeChanges(handler participation.ChangeHandler) (unsubscribe func())
}

// AttachParticipation 接线参与度真相层：声明查询面（拉取条目清单、条目级方法组门控、
// 能力集合派生）叠加覆盖表过滤，并订阅 resourceTypes / siteBrowsers 条目变更联动
// 自定义资源类型与站点浏览器代理的条目级注册/反注册。须在插件激活开始前接线——激活
// 末位的首次求值即可能发出停用变更，订阅在场才能即时联动。重复接线时先退订旧订阅；
// truth 为 nil 等效退订并清空查询面
func (l *Loader) AttachParticipation(truth ParticipationTruth) {
	l.mu.Lock()
	if l.detachParticipation != nil {
		l.detachParticipation()
		l.detachParticipation = nil
	}
	l.participation = truth
	l.mu.Unlock()
	if truth == nil {
		return
	}
	detach := truth.SubscribeChanges(l.handleParticipationChange)
	l.mu.Lock()
	l.detachParticipation = detach
	l.mu.Unlock()
}

// entryParticipates 条目有效参与态：真相层未接线、条目无会话或未在清单声明 = 基线参与。
// 调用方持 l.mu
func (l *Loader) entryParticipates(pluginPublicId string, point settingresolver.Point, id string) bool {
	if l.participation == nil {
		return true
	}
	active, found := l.participation.EntryActive(pluginPublicId, point, id)
	if !found {
		return true
	}
	return active
}

// filterWorkFetchEntries 按参与度覆盖表过滤作品拉取条目（point=workFetch）。真相层未接线时
// 原样返回原切片（零拷贝）。调用方持 l.mu
func (l *Loader) filterWorkFetchEntries(pluginPublicId string, entries []dto.WorkFetchDeclaration) []dto.WorkFetchDeclaration {
	if l.participation == nil {
		return entries
	}
	filtered := make([]dto.WorkFetchDeclaration, 0, len(entries))
	for _, decl := range entries {
		if !l.entryParticipates(pluginPublicId, settingresolver.PointWorkFetch, decl.ID) {
			continue
		}
		filtered = append(filtered, decl)
	}
	return filtered
}

// filterSiteAuthorFetchEntries 按参与度覆盖表过滤站点作者拉取条目（point=siteAuthorFetch）。
// 真相层未接线时原样返回原切片（零拷贝）。调用方持 l.mu
func (l *Loader) filterSiteAuthorFetchEntries(pluginPublicId string, entries []dto.SiteAuthorFetchDeclaration) []dto.SiteAuthorFetchDeclaration {
	if l.participation == nil {
		return entries
	}
	filtered := make([]dto.SiteAuthorFetchDeclaration, 0, len(entries))
	for _, decl := range entries {
		if !l.entryParticipates(pluginPublicId, settingresolver.PointSiteAuthorFetch, decl.ID) {
			continue
		}
		filtered = append(filtered, decl)
	}
	return filtered
}

// Capabilities 返回插件当前有效能力集合：声明面按参与度覆盖表过滤后派生——siteAuthorFetch
// 条目与 workFetch 条目（含各自 options）均按条目参与态取用，停用条目不贡献能力。
// 真相层未接线时与纯声明派生一致。未加载返回 nil
func (l *Loader) Capabilities(pluginPublicId string) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	entry, ok := l.processes[pluginPublicId]
	if !ok || entry.info == nil {
		return nil
	}
	if l.participation == nil {
		return deriveCapabilities(entry.info)
	}
	effective := *entry.info
	effective.WorkFetch = l.filterWorkFetchEntries(pluginPublicId, entry.info.WorkFetch)
	effective.SiteAuthorFetch = l.filterSiteAuthorFetchEntries(pluginPublicId, entry.info.SiteAuthorFetch)
	return deriveCapabilities(&effective)
}

// handleParticipationChange 参与度变更联动（真相层订阅回调）：resourceTypes 条目停用 →
// 该自定义类型反注册（既有资源行的渲染回落 Registry 未命中分支——LookupResourceTypeSpec
// 返回 nil 后展示主体走降级链，与内置 unknown 同一现状语义）、恢复参与 → 按清单声明重注册
// 该类型；siteBrowsers 条目停用 → 站点浏览器代理条目级注销（站点浏览器列表 API 与打开调用
// 不再产出该条目）、恢复参与 → 按激活期清单缓存重建代理重注册。其余 point 不归本层联动
func (l *Loader) handleParticipationChange(change participation.Change) {
	if change.Point != settingresolver.PointResourceTypes && change.Point != settingresolver.PointSiteBrowsers {
		return
	}
	l.mu.RLock()
	entry, ok := l.processes[change.PluginPublicId]
	var info *PluginInfo
	if ok {
		info = entry.info
	}
	l.mu.RUnlock()
	if info == nil {
		// 无进程条目 = 该插件未注册过自定义资源类型/站点浏览器代理（纯 UI 插件不经进程
		// 加载），无联动对象
		return
	}
	switch change.Point {
	case settingresolver.PointResourceTypes:
		l.applyResourceTypeParticipation(info, change.ID, change.Active, change.Reason)
	case settingresolver.PointSiteBrowsers:
		l.applySiteBrowserParticipation(info, change.ID, change.Active, change.Reason)
	}
}

// applyResourceTypeParticipation 单个自定义资源类型的参与度联动：恢复参与按清单声明重注册
// （与激活期整批注册同一注册原语与失败语义）；停用即反注册（内置类型受 Registry 白名单
// 保护不会被删）
func (l *Loader) applyResourceTypeParticipation(info *PluginInfo, resourceType string, active bool, reason string) {
	if active {
		for _, decl := range info.ResourceTypes {
			if decl.Type != resourceType {
				continue
			}
			registerPluginResourceType(info, decl)
			return
		}
		return
	}
	logger.Log.Infof("插件 %s 自定义资源类型 %s 因参与度停用反注册（理由: %s）", info.PublicID, resourceType, reason)
	entity.ResourceTypeRegistry.Unregister(resourceType)
}

// applySiteBrowserParticipation 单个站点浏览器条目代理的参与度联动：停用 → 注册表条目级
// 注销（best-effort，目标不在注册表仅记日志）；恢复参与 → 按激活期清单缓存重建代理重注册
// （与激活期整批注册同一注册原语，重注册失败记日志保持降级、不中断其余联动）
func (l *Loader) applySiteBrowserParticipation(info *PluginInfo, entryId string, active bool, reason string) {
	if !active {
		logger.Log.Infof("插件 %s 站点浏览器条目 %s 因参与度停用注销（理由: %s）", info.PublicID, entryId, reason)
		if err := l.siteBrowserRegistry.Unregister(info.PublicID, entryId); err != nil {
			logger.Log.Warnf("插件 %s 站点浏览器条目 %s 停用注销未命中注册表: %v", info.PublicID, entryId, err)
		}
		return
	}
	for _, entry := range info.SiteBrowsers {
		if entry.ID != entryId {
			continue
		}
		if err := l.registerSiteBrowserEntry(info, entry); err != nil {
			logger.Log.Warnf("插件 %s 站点浏览器条目 %s 恢复参与重注册失败: %v", info.PublicID, entryId, err)
		}
		return
	}
}
