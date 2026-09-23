package stickymemory

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/plugin/extension"
)

// CandidateMetaQuerier 冲突候选的条目声明与插件名只读查询（插件加载器实现）：管理列表把
// 记忆里的候选全键解析为显示名时使用。条目显示名唯一来源 = 清单条目声明的 name 字段；
// 插件未加载（停用/卸载）时查不到声明，展示回落原始 id——记忆行不因插件离场而隐藏或报错
type CandidateMetaQuerier interface {
	// ListActivePlugins 当前已加载插件的公开 ID 与显示名
	ListActivePlugins() []extension.ActivePlugin
	// WorkFetchEntries 插件的作品拉取条目声明（各条目 id/name）
	WorkFetchEntries(pluginPublicId string) []dto.WorkFetchDeclaration
	// SiteAuthorFetchEntries 插件的站点作者拉取条目声明（各条目 id/name）
	SiteAuthorFetchEntries(pluginPublicId string) []dto.SiteAuthorFetchDeclaration
}

// Handler 粘性记忆 Handler：设置页管理列表 + 删除（只删不编辑）
type Handler struct {
	svc  *Service
	meta CandidateMetaQuerier
}

// NewHandler 创建粘性记忆 Handler
func NewHandler(svc *Service, meta CandidateMetaQuerier) *Handler {
	return &Handler{svc: svc, meta: meta}
}

// domainLabels 记忆域 → 中文标签（管理列表展示用；新冲突面接入粘性记忆时在此登记）
var domainLabels = map[string]string{
	entity.DomainTaskURLDisambiguation:         "链接建任务",
	entity.DomainSiteAuthorFetchDisambiguation: "站点作者拉取",
}

// ListMemories 全量粘性记忆的管理展示列表：记忆行的上下文键与显选值拆解为站点域、候选
// 名单与当前选中者，候选显示名按域从对应清单条目声明解析（任务面取 workFetch 声明、
// 作者面取 siteAuthorFetch 声明），取不到回落原始 id
func (h *Handler) ListMemories(ctx context.Context) *model.ApiResponse[[]*dto.StickyMemoryEntryDTO] {
	rows, err := h.svc.List(ctx)
	if err != nil {
		return model.HandleError[[]*dto.StickyMemoryEntryDTO](err)
	}
	resolver := newCandidateNameResolver(h.meta)
	entries := make([]*dto.StickyMemoryEntryDTO, 0, len(rows))
	for _, row := range rows {
		siteDomain, fullKeys := ParseContextKey(row.ContextKey)
		candidates := make([]*dto.StickyMemoryCandidateDTO, 0, len(fullKeys))
		for _, fullKey := range fullKeys {
			candidates = append(candidates, resolver.resolve(row.Domain, fullKey))
		}
		// 未知域（无登记标签）回落域原值展示，不阻塞列表
		domainLabel, hasLabel := domainLabels[row.Domain]
		if !hasLabel {
			domainLabel = row.Domain
		}
		entries = append(entries, &dto.StickyMemoryEntryDTO{
			ID:          row.GetID(),
			Domain:      row.Domain,
			DomainLabel: domainLabel,
			SiteDomain:  siteDomain,
			Candidates:  candidates,
			Selected:    resolver.resolve(row.Domain, row.Value),
			CreateTime:  row.GetCreateTime(),
			UpdateTime:  row.GetUpdateTime(),
		})
	}
	return model.Success(entries)
}

// DeleteMemory 按 id 删除一条记忆（管理面唯一写操作：只删除不编辑；删除后同冲突自然
// 重新询问）
func (h *Handler) DeleteMemory(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Forget(ctx, id))
}

// candidateNameResolver 候选显示名解析器：按域把候选全键（插件公开 ID + 条目 id）解析为
// 插件/条目显示名。插件显示名一次性预取；条目声明按插件公开 ID 各查一次后缓存，同插件的
// 多条候选不重复查询
type candidateNameResolver struct {
	meta         CandidateMetaQuerier
	pluginNames  map[string]string            // 插件公开 ID → 显示名（未加载/未设名不在表内）
	workNames    map[string]map[string]string // 插件公开 ID → (条目 id → 条目显示名)
	authorNames  map[string]map[string]string // 插件公开 ID → (条目 id → 条目显示名)
}

// newCandidateNameResolver 创建解析器并预取当前已加载插件的显示名
func newCandidateNameResolver(meta CandidateMetaQuerier) *candidateNameResolver {
	pluginNames := make(map[string]string)
	for _, p := range meta.ListActivePlugins() {
		pluginNames[p.PublicID] = p.Name
	}
	return &candidateNameResolver{
		meta:        meta,
		pluginNames: pluginNames,
		workNames:   make(map[string]map[string]string),
		authorNames: make(map[string]map[string]string),
	}
}

// resolve 候选全键 → 管理列表展示项。条目显示名按域从对应清单条目声明解析，插件未加载
// 或未声明该条目时回落条目 id；插件显示名取不到回落插件公开 ID
func (r *candidateNameResolver) resolve(domain, fullKey string) *dto.StickyMemoryCandidateDTO {
	pluginPublicId, extensionId := SplitCandidateFullKey(fullKey)
	var entryName string
	switch domain {
	case entity.DomainTaskURLDisambiguation:
		entryName = r.workNamesOf(pluginPublicId)[extensionId]
	case entity.DomainSiteAuthorFetchDisambiguation:
		entryName = r.authorNamesOf(pluginPublicId)[extensionId]
	}
	displayName := entryName
	if displayName == "" {
		displayName = extensionId
	}
	return &dto.StickyMemoryCandidateDTO{
		PluginPublicId: pluginPublicId,
		PluginName:     r.pluginNames[pluginPublicId],
		ExtensionId:    extensionId,
		ExtensionName:  entryName,
		DisplayName:    displayName,
	}
}

// workNamesOf 插件的作品拉取条目显示名表（条目 id → name）；未加载/未声明返回空表，
// 查询结果按插件缓存
func (r *candidateNameResolver) workNamesOf(pluginPublicId string) map[string]string {
	if names, ok := r.workNames[pluginPublicId]; ok {
		return names
	}
	names := make(map[string]string)
	for _, entry := range r.meta.WorkFetchEntries(pluginPublicId) {
		names[entry.ID] = entry.Name
	}
	r.workNames[pluginPublicId] = names
	return names
}

// authorNamesOf 插件的站点作者拉取条目显示名表（条目 id → name）；未加载/未声明返回空表，
// 查询结果按插件缓存
func (r *candidateNameResolver) authorNamesOf(pluginPublicId string) map[string]string {
	if names, ok := r.authorNames[pluginPublicId]; ok {
		return names
	}
	names := make(map[string]string)
	for _, entry := range r.meta.SiteAuthorFetchEntries(pluginPublicId) {
		names[entry.ID] = entry.Name
	}
	r.authorNames[pluginPublicId] = names
	return names
}
