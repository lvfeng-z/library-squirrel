package extension

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
)

// declarationManifest 以给定 extensions 段内容拼一份清单原文（其余字段取合法值），
// 供校验矩阵逐形态构造不合格声明
func declarationManifest(extensions string) string {
	return `{"id":"com.example.plugin_a","name":"插件甲","version":"1.0.0","contractVersion":10,"extensions":` + extensions + `}`
}

// TestValidateManifestDeclarationsRejectsMatrix 拒收矩阵：不合格形态逐种被点名拒收
// （缺 sites / 空 sites / 未注册站点键 / 未识别 options / 残留顶层 capabilities 键 /
// extensions 内 settings 键），合格声明与无 extensions 段的清单放行。输入一律为清单原文
// （顶层残留学段无承载字段，只能就原文探测）
func TestValidateManifestDeclarationsRejectsMatrix(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		wantErr  bool
		wantText string
	}{
		{
			name:     "缺 sites",
			manifest: declarationManifest(`{"siteAuthorFetch":{}}`),
			wantErr:  true,
			wantText: "siteAuthorFetch.sites 为空",
		},
		{
			name:     "空 sites",
			manifest: declarationManifest(`{"siteAuthorFetch":{"sites":[]}}`),
			wantErr:  true,
			wantText: "siteAuthorFetch.sites 为空",
		},
		{
			name:     "未注册站点键",
			manifest: declarationManifest(`{"siteAuthorFetch":{"sites":["bilibili","nosite"]}}`),
			wantErr:  true,
			wantText: `未注册站点键 "nosite"`,
		},
		{
			name:     "未识别 options",
			manifest: declarationManifest(`{"taskHandlers":[{"id":"main","options":["workSetRelationQuer"]}]}`),
			wantErr:  true,
			wantText: `taskHandlers[main].options 含未识别的可选方法组 "workSetRelationQuer"`,
		},
		{
			name: "残留顶层 capabilities",
			manifest: `{"id":"com.example.plugin_a","name":"插件甲","version":"1.0.0","contractVersion":10,` +
				`"capabilities":["siteAuthorFetch"],` +
				`"extensions":{"siteAuthorFetch":{"sites":["bilibili"]}}}`,
			wantErr:  true,
			wantText: "顶层 capabilities 段不在声明面内",
		},
		{
			name: "残留顶层 capabilities(null 值)",
			manifest: `{"id":"com.example.plugin_a","name":"插件甲","version":"1.0.0","contractVersion":10,` +
				`"capabilities":null,` +
				`"extensions":{"siteAuthorFetch":{"sites":["bilibili"]}}}`,
			wantErr:  true,
			wantText: "顶层 capabilities 段不在声明面内",
		},
		{
			name:     "extensions 内 settings 键",
			manifest: declarationManifest(`{"settings":[{"key":"a","type":"string","title":"甲"}]}`),
			wantErr:  true,
			wantText: "settings 须住清单根级",
		},
		{
			name:     "extensions 内 settings 键(null 值)",
			manifest: declarationManifest(`{"settings":null}`),
			wantErr:  true,
			wantText: "settings 须住清单根级",
		},
		{
			name:     "合格声明",
			manifest: declarationManifest(`{"taskHandlers":[{"id":"main","options":["workOrderQuery","workSetRelationQuery"]}],"siteAuthorFetch":{"sites":["bilibili","pixiv","local"]}}`),
		},
		{
			name: "根级 settings 放行",
			manifest: `{"id":"com.example.plugin_a","name":"插件甲","version":"1.0.0","contractVersion":10,` +
				`"settings":[{"key":"a","type":"string","title":"甲"}],` +
				`"extensions":{"siteAuthorFetch":{"sites":["bilibili"]}}}`,
		},
		{
			name:     "无 extensions 段",
			manifest: `{"id":"com.example.plugin_a","name":"插件甲","version":"1.0.0","contractVersion":10}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateManifestDeclarations([]byte(tc.manifest))
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("合格声明不应拒收, 实际: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("不合格声明应拒收")
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("拒收原因应点名不合格项, 实际: %v（期望含 %q）", err, tc.wantText)
			}
			if !strings.Contains(err.Error(), ErrManifestDeclarationInvalid.Error()) {
				t.Errorf("拒收原因应携带哨兵错误 %v, 实际: %v", ErrManifestDeclarationInvalid, err)
			}
		})
	}
}

// TestValidateManifestDeclarationsEmptyObject 无任何声明字段的清单对象放行（校验只约束已声明的内容）；
// 非 JSON 原文一律判不合格
func TestValidateManifestDeclarationsEmptyObject(t *testing.T) {
	if err := ValidateManifestDeclarations([]byte(`{}`)); err != nil {
		t.Errorf("无声明字段的清单应放行, 实际: %v", err)
	}
	if err := ValidateManifestDeclarations(nil); err == nil ||
		!strings.Contains(err.Error(), "清单非合法 JSON") {
		t.Errorf("非 JSON 原文应判不合格并点名原因, 实际: %v", err)
	}
}

// TestRegisterPluginResourceTypesCarriesStoreStandards 自定义资源类型的 storeStandards 声明逐字段
// 落进 Registry（描述、期望扩展名、典型产出方式），供展示面读取；未声明该字段时通路过而不报错
func TestRegisterPluginResourceTypesCarriesStoreStandards(t *testing.T) {
	const resourceType = "com.example.panorama"
	t.Cleanup(func() { entity.ResourceTypeRegistry.Unregister(resourceType) })

	info := &PluginInfo{PublicID: "com.example.plugin_a"}
	ApplyManifestDeclarations(info, parseManifest(t, declarationManifest(`{"resourceTypes":[
		{"type":"`+resourceType+`",
		 "roles":[{"storeType":"image","min":1,"max":0},{"storeType":"thumbnail","min":0,"max":1}],
		 "primaryRoles":["image"],
		 "storeStandards":{
		   "image":{"description":"全景图主体","formats":[".jpg",".png"],"generation":"downloaded"},
		   "thumbnail":{"description":"封面"}}}]}`)))
	registerPluginResourceTypes(info)

	spec, ok := entity.ResourceTypeRegistry.Lookup(resourceType)
	if !ok {
		t.Fatalf("资源类型 %s 未注册进 Registry", resourceType)
	}
	if len(spec.StoreStandards) != 2 {
		t.Fatalf("Registry 中 storeStandards 条目数 = %d, 期望 2", len(spec.StoreStandards))
	}
	main := spec.StoreStandards[entity.StoreTypeImage]
	if main.Description != "全景图主体" || !slices.Equal(main.Formats, []string{".jpg", ".png"}) ||
		main.Generation != entity.GenerationDownloaded {
		t.Errorf("Registry 中 image 文件标准 = %+v, 期望 {全景图主体 [.jpg .png] downloaded}", main)
	}
	// 未声明的子字段保持零值：description 之外一律空
	if thumb := spec.StoreStandards[entity.StoreTypeThumbnail]; thumb.Description != "封面" ||
		thumb.Formats != nil || thumb.Generation != "" {
		t.Errorf("Registry 中 thumbnail 文件标准 = %+v, 期望 {封面 [] }", thumb)
	}
}

// TestRegisterPluginResourceTypesStoreStandardsOptional 未声明 storeStandards 的类型照常注册，
// 规格内该字段为 nil（不因缺失报错、不阻塞注册）
func TestRegisterPluginResourceTypesStoreStandardsOptional(t *testing.T) {
	const resourceType = "com.example.plain"
	t.Cleanup(func() { entity.ResourceTypeRegistry.Unregister(resourceType) })

	info := &PluginInfo{PublicID: "com.example.plugin_a"}
	ApplyManifestDeclarations(info, parseManifest(t, declarationManifest(
		`{"resourceTypes":[{"type":"`+resourceType+`","roles":[{"storeType":"image","min":1,"max":1}],"primaryRoles":["image"]}]}`)))
	registerPluginResourceTypes(info)

	spec, ok := entity.ResourceTypeRegistry.Lookup(resourceType)
	if !ok {
		t.Fatalf("资源类型 %s 未注册进 Registry", resourceType)
	}
	if spec.StoreStandards != nil {
		t.Errorf("未声明 storeStandards 时规格内该字段应为 nil, 实际: %+v", spec.StoreStandards)
	}
}

// newTestLoaderWithDeclarations 构造载有指定声明的 Loader（进程表填入声明条目——本组测试只验门控与
// 声明查询，不启子进程）
func newTestLoaderWithDeclarations(publicId string, handlers []dto.TaskHandlerDeclaration) *Loader {
	loader := NewLoader(NewTaskHandlerRegistry(), NewSiteBrowserRegistry())
	loader.processes[publicId] = &pluginEntry{info: &PluginInfo{PublicID: publicId, TaskHandlers: handlers}}
	return loader
}

// TestHasTaskHandlerOptionIsPerEntry 条目级声明查询：同一插件两条处理器条目，各自的可选方法组
// 互不串味（未声明者、未知条目、未加载插件一律 false）
func TestHasTaskHandlerOptionIsPerEntry(t *testing.T) {
	const publicId = "com.example.plugin_a"
	loader := newTestLoaderWithDeclarations(publicId, []dto.TaskHandlerDeclaration{
		{ID: "with-order", Options: []string{CapabilityWorkOrderQuery}},
		{ID: "without-option"},
	})

	if !loader.HasTaskHandlerOption(publicId, "with-order", CapabilityWorkOrderQuery) {
		t.Error("声明了 workOrderQuery 的条目查询应为 true")
	}
	if loader.HasTaskHandlerOption(publicId, "with-order", CapabilityWorkSetRelationQuery) {
		t.Error("同插件另条目声明的 workSetRelationQuery 不应记在 with-order 条目上")
	}
	if loader.HasTaskHandlerOption(publicId, "without-option", CapabilityWorkOrderQuery) {
		t.Error("未声明任何方法组的条目查询应为 false")
	}
	if loader.HasTaskHandlerOption(publicId, "no-such-entry", CapabilityWorkOrderQuery) {
		t.Error("未知条目查询应为 false")
	}
	if loader.HasTaskHandlerOption("com.example.plugin_b", "with-order", CapabilityWorkOrderQuery) {
		t.Error("未加载插件查询应为 false")
	}
}

// TestWorkSetOrderFetcherGatesPerEntry 门控为（插件, 扩展点）条目级：同插件未声明 workOrderQuery 的
// 条目在门控处短路（即便 registry 中无该条目也不报查找失败），声明了的条目过门控后进 registry 查找
func TestWorkSetOrderFetcherGatesPerEntry(t *testing.T) {
	const publicId = "com.example.plugin_a"
	registry := NewTaskHandlerRegistry()
	fetcher := NewWorkSetOrderFetcher(registry, newTestLoaderWithDeclarations(publicId, []dto.TaskHandlerDeclaration{
		{ID: "with-order", Options: []string{CapabilityWorkOrderQuery}},
		{ID: "without-option"},
	}))

	// 未声明条目：短路返回，不进 registry（registry 中无任何条目，进了必报查找失败）
	entries, err := fetcher.QueryWorkSetOrder(context.Background(), publicId, "without-option", 1, "ws")
	if err != nil || entries != nil {
		t.Errorf("未声明条目应短路返回 (nil, nil), 实际 entries=%v err=%v", entries, err)
	}

	// 声明条目：过门控后进 registry 查找，条目缺失故报查找失败——反证门控按条目放行
	if _, err := fetcher.QueryWorkSetOrder(context.Background(), publicId, "with-order", 1, "ws"); err == nil ||
		!strings.Contains(err.Error(), "查找插件 TaskHandler 失败") {
		t.Errorf("声明条目应过门控并进 registry 查找, 实际 err=%v", err)
	}
}

// TestWorkSetRelationFetcherGatesPerEntry 同型断言：父集关系获取器亦按（插件, 扩展点）条目级门控
func TestWorkSetRelationFetcherGatesPerEntry(t *testing.T) {
	const publicId = "com.example.plugin_a"
	fetcher := NewWorkSetRelationFetcher(NewTaskHandlerRegistry(), newTestLoaderWithDeclarations(publicId, []dto.TaskHandlerDeclaration{
		{ID: "with-relation", Options: []string{CapabilityWorkSetRelationQuery}},
		{ID: "without-option"},
	}))

	relations, err := fetcher.QueryWorkSetRelations(context.Background(), publicId, "without-option", 1, "ws")
	if err != nil || relations != nil {
		t.Errorf("未声明条目应短路返回 (nil, nil), 实际 relations=%v err=%v", relations, err)
	}

	if _, err := fetcher.QueryWorkSetRelations(context.Background(), publicId, "with-relation", 1, "ws"); err == nil ||
		!strings.Contains(err.Error(), "查找插件 TaskHandler 失败") {
		t.Errorf("声明条目应过门控并进 registry 查找, 实际 err=%v", err)
	}
}
