package extension

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
)

// parseManifest 按生产路径（json.Unmarshal 进 dto.PluginManifest）解析一份 plugin.json 文本
func parseManifest(t *testing.T, raw string) *dto.PluginManifest {
	t.Helper()
	var manifest dto.PluginManifest
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		t.Fatalf("解析 plugin.json 失败: %v", err)
	}
	if manifest.Extensions == nil {
		t.Fatal("解析结果 extensions 段缺失")
	}
	return &manifest
}

// manifestWithAllPackages 一份声明了三个能力包的清单：任务处理器（含两个可选方法组）、
// 站点作者拉取（两个归属站点）、自定义资源类型（一个）。
const manifestWithAllPackages = `{
  "id": "com.example.plugin_a",
  "name": "插件甲",
  "version": "1.0.0",
  "contractVersion": 9,
  "extensions": {
    "taskHandlers": [
      {"id": "main", "name": "主处理器", "options": ["workOrderQuery", "workSetRelationQuery"]}
    ],
    "siteAuthorFetch": {"sites": ["bilibili", "pixiv"]},
    "resourceTypes": [
      {"type": "com.example.panorama",
       "roles": [{"storeType": "image", "min": 1, "max": 0}],
       "primaryRoles": ["image"]}
    ]
  },
  "activation": {"type": 1},
  "entryFile": "plugin.exe"
}`

// TestApplyManifestDeclarationsCarriesPackages 三个能力包在激活期内存结构中逐字段可达：
// taskHandlers 条目的 options、siteAuthorFetch 的 sites、resourceTypes 的声明字段
func TestApplyManifestDeclarationsCarriesPackages(t *testing.T) {
	info := &PluginInfo{PublicID: "com.example.plugin_a"}
	ApplyManifestDeclarations(info, parseManifest(t, manifestWithAllPackages))

	// options：条目级可选方法组逐项可达
	if len(info.TaskHandlers) != 1 {
		t.Fatalf("taskHandlers 条目数 = %d, 期望 1", len(info.TaskHandlers))
	}
	handler := info.TaskHandlers[0]
	if handler.ID != "main" {
		t.Errorf("taskHandlers[0].ID = %q, 期望 main", handler.ID)
	}
	wantOptions := []string{CapabilityWorkOrderQuery, CapabilityWorkSetRelationQuery}
	if !slices.Equal(handler.Options, wantOptions) {
		t.Errorf("taskHandlers[0].options = %v, 期望 %v", handler.Options, wantOptions)
	}

	// sites：归属站点键逐项可达
	if info.SiteAuthorFetch == nil {
		t.Fatal("siteAuthorFetch 未落进内存结构")
	}
	wantSites := []string{"bilibili", "pixiv"}
	if !slices.Equal(info.SiteAuthorFetch.Sites, wantSites) {
		t.Errorf("siteAuthorFetch.sites = %v, 期望 %v", info.SiteAuthorFetch.Sites, wantSites)
	}

	// resourceTypes：类型值、结构角色基数、展示主体优先级逐字段可达
	if len(info.ResourceTypes) != 1 {
		t.Fatalf("resourceTypes 条目数 = %d, 期望 1", len(info.ResourceTypes))
	}
	decl := info.ResourceTypes[0]
	if decl.Type != "com.example.panorama" {
		t.Errorf("resourceTypes[0].type = %q, 期望 com.example.panorama", decl.Type)
	}
	if len(decl.Roles) != 1 || decl.Roles[0].StoreType != entity.StoreTypeImage ||
		decl.Roles[0].Min != 1 || decl.Roles[0].Max != 0 {
		t.Errorf("resourceTypes[0].roles = %+v, 期望 [{image 1 0}]", decl.Roles)
	}
	if !slices.Equal(decl.PrimaryRoles, []string{entity.StoreTypeImage}) {
		t.Errorf("resourceTypes[0].primaryRoles = %v, 期望 [image]", decl.PrimaryRoles)
	}
}

// TestDeriveCapabilitiesMatchesLegacyDeclaration 对照组：旧声明面（顶层 capabilities 段）与新声明面
// （extensions 内）描述同一插件时，激活期内存结构派生出的可选能力集合一致。
// 该断言锚定的是声明面迁移的映射忠实性——siteAuthorFetch 段在场对应旧 siteAuthorFetch 能力、
// taskHandlers[].options 各项对应旧同名能力；旧通行证 resourceTypeProvider 无派生对应物
// （自定义类型注册由 resourceTypes 段在场承担），故不在期望集合中。
func TestDeriveCapabilitiesMatchesLegacyDeclaration(t *testing.T) {
	// 对照组：旧清单声明 capabilities: ["siteAuthorFetch","workOrderQuery","workSetRelationQuery","resourceTypeProvider"]
	// + 顶层 resourceTypes 段；迁移后前三项分别落 siteAuthorFetch 段与 taskHandlers[].options
	legacyCapabilities := []string{CapabilitySiteAuthorFetch, CapabilityWorkOrderQuery, CapabilityWorkSetRelationQuery}

	info := &PluginInfo{PublicID: "com.example.plugin_a"}
	ApplyManifestDeclarations(info, parseManifest(t, manifestWithAllPackages))
	if got := deriveCapabilities(info); !slices.Equal(got, legacyCapabilities) {
		t.Errorf("派生能力集合 = %v, 期望与旧声明面一致 %v", got, legacyCapabilities)
	}

	// 未声明任何能力包的插件不派生能力（与旧清单未声明 capabilities 等价）
	if got := deriveCapabilities(&PluginInfo{}); got != nil {
		t.Errorf("无声明插件派生能力集合 = %v, 期望 nil", got)
	}
}

// TestRegisterPluginResourceTypesBySegmentPresence 自定义资源类型注册以 extensions.resourceTypes
// 段在场为启用条件（无另行声明的通行证字段）：声明即注册进 Registry
func TestRegisterPluginResourceTypesBySegmentPresence(t *testing.T) {
	const resourceType = "com.example.panorama"
	t.Cleanup(func() { entity.ResourceTypeRegistry.Unregister(resourceType) })

	info := &PluginInfo{PublicID: "com.example.plugin_a"}
	ApplyManifestDeclarations(info, parseManifest(t, manifestWithAllPackages))
	registerPluginResourceTypes(info)

	spec, ok := entity.ResourceTypeRegistry.Lookup(resourceType)
	if !ok {
		t.Fatalf("资源类型 %s 未注册进 Registry", resourceType)
	}
	if len(spec.Roles) != 1 || spec.Roles[0].StoreType != entity.StoreTypeImage || spec.Roles[0].Min != 1 {
		t.Errorf("Registry 中 roles = %+v, 期望 [{image 1 0}]", spec.Roles)
	}
	if !slices.Equal(spec.PrimaryRoles, []string{entity.StoreTypeImage}) {
		t.Errorf("Registry 中 primaryRoles = %v, 期望 [image]", spec.PrimaryRoles)
	}
}
