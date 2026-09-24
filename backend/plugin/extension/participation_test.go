package extension

import (
	"slices"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/plugin/participation"
	"github.com/library-squirrel/backend/plugin/settingresolver"
)

// fakeParticipationTruth 参与度真相层测试替身：条目覆盖表 + 订阅回调登记。
// overrides 缺键 = found=false（无会话/未声明 → 基线参与）
type fakeParticipationTruth struct {
	overrides map[string]bool // key: point "/" id → 参与方向
	handlers  []participation.ChangeHandler
}

func newFakeParticipationTruth() *fakeParticipationTruth {
	return &fakeParticipationTruth{overrides: make(map[string]bool)}
}

func participationKey(point settingresolver.Point, id string) string {
	return string(point) + "/" + id
}

func (f *fakeParticipationTruth) EntryActive(_ string, point settingresolver.Point, id string) (bool, bool) {
	active, found := f.overrides[participationKey(point, id)]
	return active, found
}

func (f *fakeParticipationTruth) SubscribeChanges(handler participation.ChangeHandler) func() {
	f.handlers = append(f.handlers, handler)
	return func() {}
}

// emit 以登记的订阅回调派发一条参与度变更
func (f *fakeParticipationTruth) emit(change participation.Change) {
	for _, handler := range f.handlers {
		handler(change)
	}
}

// newParticipationLoader 构造载有指定插件信息的 Loader（进程表填入 info——本组测试只验
// 查询过滤与参与度联动，不启子进程）
func newParticipationLoader(publicId string, info *PluginInfo) *Loader {
	loader := NewLoader(NewWorkFetchRegistry(), NewSiteBrowserRegistry())
	info.PublicID = publicId
	loader.processes[publicId] = &pluginEntry{info: info}
	return loader
}

// TestWorkFetchQueriesFilteredByParticipation 作品拉取候选查询叠加覆盖表过滤：停用条目
// 不进条目清单、条目级方法组门控对停用条目短路；参与条目与未接线语义（基线全参与）不变
func TestWorkFetchQueriesFilteredByParticipation(t *testing.T) {
	const publicId = "com.example.plugin_a"
	declarations := []dto.WorkFetchDeclaration{
		{ID: "main", Name: "主处理器", Options: []string{CapabilityWorkOrderQuery}},
		{ID: "hd", Name: "高清处理器", Options: []string{CapabilityWorkSetRelationQuery}},
	}

	// 未接线：与纯声明查询一致（原切片原样返回）
	baseline := newParticipationLoader(publicId, &PluginInfo{WorkFetch: declarations})
	if got := baseline.WorkFetchEntries(publicId); len(got) != 2 {
		t.Fatalf("未接线时条目清单应含全部声明条目, 实际 %d 条", len(got))
	}
	if !baseline.HasWorkFetchOption(publicId, "hd", CapabilityWorkSetRelationQuery) {
		t.Error("未接线时声明了方法组的停用候选条目查询应仍为 true（基线参与）")
	}

	loader := newParticipationLoader(publicId, &PluginInfo{WorkFetch: declarations})
	truth := newFakeParticipationTruth()
	truth.overrides[participationKey(settingresolver.PointWorkFetch, "hd")] = false
	loader.AttachParticipation(truth)

	got := loader.WorkFetchEntries(publicId)
	if len(got) != 1 || got[0].ID != "main" {
		t.Fatalf("停用条目 hd 应被过滤, 实际 %+v", got)
	}
	if loader.HasWorkFetchOption(publicId, "hd", CapabilityWorkSetRelationQuery) {
		t.Error("停用条目的方法组门控应返回 false（不经该条目被调用）")
	}
	if !loader.HasWorkFetchOption(publicId, "main", CapabilityWorkOrderQuery) {
		t.Error("参与条目的方法组门控应不受影响")
	}
	// 未声明条目：覆盖表无记录（found=false → 基线参与）不改变既有语义——按声明循环未命中返回 false
	if loader.HasWorkFetchOption(publicId, "no-such-entry", CapabilityWorkOrderQuery) {
		t.Error("未声明条目应保持既有语义返回 false")
	}
}

// TestSiteAuthorFetchEntriesFilteredByParticipation 站点作者拉取候选查询叠加覆盖表过滤
// （point=siteAuthorFetch）：停用条目不出现在返回清单，参与条目原样保留
func TestSiteAuthorFetchEntriesFilteredByParticipation(t *testing.T) {
	const publicId = "com.example.plugin_a"
	declarations := []dto.SiteAuthorFetchDeclaration{
		{ID: "full", Name: "完整源", Sites: []string{"pixiv"}},
		{ID: "lite", Name: "精简源", Sites: []string{"pixiv"}},
	}

	loader := newParticipationLoader(publicId, &PluginInfo{SiteAuthorFetch: declarations})
	truth := newFakeParticipationTruth()
	truth.overrides[participationKey(settingresolver.PointSiteAuthorFetch, "lite")] = false
	loader.AttachParticipation(truth)

	got := loader.SiteAuthorFetchEntries(publicId)
	if len(got) != 1 || got[0].ID != "full" {
		t.Fatalf("停用条目 lite 应被过滤, 实际 %+v", got)
	}
	if loader.SiteAuthorFetchEntries("com.example.plugin_b") != nil {
		t.Error("未加载插件应保持既有语义返回 nil")
	}
}

// TestCapabilitiesDerivedFromEffectiveEntries 能力集合按覆盖后条目派生：停用的 siteAuthorFetch
// 条目不贡献 siteAuthorFetch 能力（全部停用时能力消失）、停用的 workFetch 条目不贡献其
// options；未接线时与纯声明派生一致
func TestCapabilitiesDerivedFromEffectiveEntries(t *testing.T) {
	const publicId = "com.example.plugin_a"
	info := &PluginInfo{
		WorkFetch: []dto.WorkFetchDeclaration{
			{ID: "main", Options: []string{CapabilityWorkOrderQuery}},
			{ID: "hd", Options: []string{CapabilityWorkSetRelationQuery}},
		},
		SiteAuthorFetch: []dto.SiteAuthorFetchDeclaration{
			{ID: "full", Name: "完整源", Sites: []string{"pixiv"}},
			{ID: "lite", Name: "精简源", Sites: []string{"pixiv"}},
		},
	}
	wantAll := []string{CapabilitySiteAuthorFetch, CapabilityWorkOrderQuery, CapabilityWorkSetRelationQuery}

	baseline := newParticipationLoader(publicId, info)
	if got := baseline.Capabilities(publicId); !slices.Equal(got, wantAll) {
		t.Fatalf("未接线时能力集合 = %v, 期望 %v", got, wantAll)
	}

	loader := newParticipationLoader(publicId, info)
	truth := newFakeParticipationTruth()
	truth.overrides[participationKey(settingresolver.PointWorkFetch, "hd")] = false
	truth.overrides[participationKey(settingresolver.PointSiteAuthorFetch, "lite")] = false
	loader.AttachParticipation(truth)

	wantEffective := []string{CapabilitySiteAuthorFetch, CapabilityWorkOrderQuery}
	if got := loader.Capabilities(publicId); !slices.Equal(got, wantEffective) {
		t.Fatalf("覆盖后能力集合 = %v, 期望 %v", got, wantEffective)
	}

	// siteAuthorFetch 条目全部停用 → 能力消失
	truth.overrides[participationKey(settingresolver.PointSiteAuthorFetch, "full")] = false
	if got := loader.Capabilities(publicId); !slices.Equal(got, []string{CapabilityWorkOrderQuery}) {
		t.Fatalf("siteAuthorFetch 全停用后能力集合 = %v, 期望 [%s]", got, CapabilityWorkOrderQuery)
	}
	if loader.Capabilities("com.example.plugin_b") != nil {
		t.Error("未加载插件能力集合应为 nil")
	}
}

// TestResourceTypeParticipationLinkage resourceTypes 条目参与度联动：停用 → 该自定义类型
// 反注册（Registry 未命中，资源行渲染走 LookupResourceTypeSpec=nil 的降级链——与内置
// unknown 同一现状语义）；恢复参与 → 按清单声明重注册；其他 point 的变更不触动资源类型
func TestResourceTypeParticipationLinkage(t *testing.T) {
	const (
		publicId = "com.example.plugin_a"
		typeA    = "com.example.ptype-a"
		typeB    = "com.example.ptype-b"
	)
	t.Cleanup(func() {
		entity.ResourceTypeRegistry.Unregister(typeA)
		entity.ResourceTypeRegistry.Unregister(typeB)
	})

	info := &PluginInfo{}
	ApplyManifestDeclarations(info, parseManifest(t, declarationManifest(`{"resourceTypes":[
		{"type":"`+typeA+`","roles":[{"storeType":"image","min":1,"max":0}],"primaryRoles":["image"]},
		{"type":"`+typeB+`","roles":[{"storeType":"document","min":1,"max":1}],"primaryRoles":["document"]}]}`)))
	registerPluginResourceTypes(info) // 激活期基线注册（现状路径）
	loader := newParticipationLoader(publicId, info)
	truth := newFakeParticipationTruth()
	loader.AttachParticipation(truth)

	truth.emit(participation.Change{PluginPublicId: publicId, Point: settingresolver.PointResourceTypes,
		ID: typeA, Active: false, Reason: "用户关闭了全景模式"})

	if _, ok := entity.ResourceTypeRegistry.Lookup(typeA); ok {
		t.Fatal("停用条目的自定义类型应已反注册")
	}
	if entity.LookupResourceTypeSpec(typeA) != nil {
		t.Error("停用后消费面查询应得 nil 规约（渲染回落降级链）")
	}
	if _, ok := entity.ResourceTypeRegistry.Lookup(typeB); !ok {
		t.Error("同插件另一类型不应被株连")
	}

	// 其他 point 的变更不触动资源类型注册态
	truth.emit(participation.Change{PluginPublicId: publicId, Point: settingresolver.PointWorkFetch,
		ID: "main", Active: false})
	if _, ok := entity.ResourceTypeRegistry.Lookup(typeB); !ok {
		t.Error("workFetch 条目变更不应联动资源类型")
	}

	truth.emit(participation.Change{PluginPublicId: publicId, Point: settingresolver.PointResourceTypes,
		ID: typeA, Active: true})
	spec, ok := entity.ResourceTypeRegistry.Lookup(typeA)
	if !ok {
		t.Fatal("恢复参与后自定义类型应重注册")
	}
	if !slices.Equal(spec.PrimaryRoles, []string{entity.StoreTypeImage}) {
		t.Errorf("重注册规格 PrimaryRoles = %v, 期望 [image]", spec.PrimaryRoles)
	}
}

// TestResourceTypeChangeWithoutProcessEntryNoop 无进程条目插件（纯 UI 插件不经进程加载，
// 从未注册过自定义资源类型）的 resourceTypes 变更联动为无操作，不 panic
func TestResourceTypeChangeWithoutProcessEntryNoop(t *testing.T) {
	loader := NewLoader(NewWorkFetchRegistry(), NewSiteBrowserRegistry())
	truth := newFakeParticipationTruth()
	loader.AttachParticipation(truth)

	truth.emit(participation.Change{PluginPublicId: "com.example.ui_only",
		Point: settingresolver.PointResourceTypes, ID: "com.example.ptype-x", Active: false})
	truth.emit(participation.Change{PluginPublicId: "com.example.ui_only",
		Point: settingresolver.PointResourceTypes, ID: "com.example.ptype-x", Active: true})
}

// TestSiteBrowserParticipationLinkage siteBrowsers 条目参与度联动：停用 → 站点浏览器代理
// 条目级注销（同插件其他条目不株连），恢复参与 → 按激活期清单缓存重建代理重注册
// （元数据 name 取清单条目）；其他 point 的变更不触动站点浏览器注册表
func TestSiteBrowserParticipationLinkage(t *testing.T) {
	const publicId = "com.example.plugin_a"
	siteBrowserRegistry := NewSiteBrowserRegistry()
	loader := NewLoader(NewWorkFetchRegistry(), siteBrowserRegistry)
	info := &PluginInfo{
		SiteBrowsers: []dto.SiteBrowserDeclaration{
			{ID: "main", Name: "主浏览器"},
			{ID: "aux", Name: "辅助浏览器"},
		},
	}
	info.PublicID = publicId
	loader.processes[publicId] = &pluginEntry{info: info}
	for _, entry := range info.SiteBrowsers { // 激活期基线注册（现状路径）
		if err := loader.registerSiteBrowserEntry(info, entry); err != nil {
			t.Fatalf("基线注册站点浏览器条目 %s 失败: %v", entry.ID, err)
		}
	}
	truth := newFakeParticipationTruth()
	loader.AttachParticipation(truth)

	truth.emit(participation.Change{PluginPublicId: publicId, Point: settingresolver.PointSiteBrowsers,
		ID: "main", Active: false, Reason: "用户关闭了浏览器入口"})

	if _, err := siteBrowserRegistry.Get(publicId, "main"); err == nil {
		t.Fatal("停用条目 main 的站点浏览器代理应已条目级注销")
	}
	if _, err := siteBrowserRegistry.Get(publicId, "aux"); err != nil {
		t.Fatalf("同插件参与条目 aux 不应被株连: %v", err)
	}

	// 其他 point 的变更不触动站点浏览器注册表
	truth.emit(participation.Change{PluginPublicId: publicId, Point: settingresolver.PointWorkFetch,
		ID: "main", Active: false})
	if _, err := siteBrowserRegistry.Get(publicId, "aux"); err != nil {
		t.Error("workFetch 条目变更不应联动站点浏览器注册表")
	}

	truth.emit(participation.Change{PluginPublicId: publicId, Point: settingresolver.PointSiteBrowsers,
		ID: "main", Active: true})
	ext, err := siteBrowserRegistry.Get(publicId, "main")
	if err != nil {
		t.Fatal("恢复参与后站点浏览器代理应重注册")
	}
	if ext.Metadata.Name != "主浏览器" {
		t.Errorf("重注册代理元数据 Name = %q, 期望取清单条目名 主浏览器", ext.Metadata.Name)
	}
}
