package pluginTaskUrlListener

import (
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model"
	dto "github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
)

// declaredPlugin 构造带公开 ID 与名称的插件实体（派生索引登记的归属主体）
func declaredPlugin(publicId, name string) *domain.Plugin {
	plugin := domain.NewPlugin()
	plugin.PublicID = sql.NullString{String: publicId, Valid: true}
	plugin.Name = sql.NullString{String: name, Valid: true}
	return plugin
}

// listenerOf 取 ListListener 产出中指定（插件公开 ID, 条目 id）的条目，不存在返回 nil
func listenerOf(listeners []*PluginWithExtension, publicId, extId string) *PluginWithExtension {
	for _, l := range listeners {
		if l.PublicID.String == publicId && l.ExtensionID == extId {
			return l
		}
	}
	return nil
}

// TestRegisterDeclaredBuildsIndex 派生登记：声明 urlPatterns 的条目逐模式入索引，未声明的
// 条目跳过；命中产出带插件实体与条目 id，贡献点类型为 workFetch
func TestRegisterDeclaredBuildsIndex(t *testing.T) {
	m := NewManager()
	plugin := declaredPlugin("com.example.a", "插件A")

	m.RegisterDeclared(plugin, []dto.WorkFetchDeclaration{
		{ID: "main", Name: "主处理器", UrlPatterns: []string{`^https://example\.com/art/`, `^https://example\.net/`}},
		{ID: "aux", Name: "辅助处理器"}, // 未声明 urlPatterns：不监听
	})

	art := m.ListListener("https://example.com/art/123")
	if len(art) != 1 || art[0].ExtensionID != "main" || art[0].PublicID.String != "com.example.a" {
		t.Fatalf("example.com 命中应只有 main 条目, 实际 %+v", art)
	}
	if art[0].ExtensionKey != string(model.ExtensionTypeWorkFetch) {
		t.Errorf("ExtensionKey = %q, 期望 workFetch", art[0].ExtensionKey)
	}
	if art[0].Name.String != "插件A" {
		t.Errorf("命中条目应携带插件实体, Name = %q", art[0].Name.String)
	}

	if got := m.ListListener("https://example.net/x"); len(got) != 1 || got[0].ExtensionID != "main" {
		t.Fatalf("第二条模式应独立命中, 实际 %+v", got)
	}
	if got := m.ListListener("https://other.org/x"); got != nil {
		t.Fatalf("未声明模式不应命中, 实际 %+v", got)
	}
	if got := m.ListListener("https://example.com/other"); got != nil {
		t.Fatalf("模式为前缀正则, 非命中路径不应产出, 实际 %+v", got)
	}
}

// TestRegisterDeclaredCompositeKeySamePattern 同一模式由同插件两个条目声明：两候选并存
// （复合键 = 公开 ID + 条目 id，条目粒度登记）
func TestRegisterDeclaredCompositeKeySamePattern(t *testing.T) {
	m := NewManager()
	m.RegisterDeclared(declaredPlugin("com.example.a", "插件A"), []dto.WorkFetchDeclaration{
		{ID: "main", Name: "主处理器", UrlPatterns: []string{"^https://x\\.com/"}},
		{ID: "legacy", Name: "旧处理器", UrlPatterns: []string{"^https://x\\.com/"}},
	})

	got := m.ListListener("https://x.com/1")
	if len(got) != 2 || listenerOf(got, "com.example.a", "main") == nil || listenerOf(got, "com.example.a", "legacy") == nil {
		t.Fatalf("同模式两条目应各成一候选, 实际 %+v", got)
	}
}

// TestRegisterDeclaredMultiPatternSingleEntry 同一条目的多个模式同时命中同一 URL：
// 该条目只产出一次（候选 = 清单条目，不是模式）
func TestRegisterDeclaredMultiPatternSingleEntry(t *testing.T) {
	m := NewManager()
	m.RegisterDeclared(declaredPlugin("com.example.a", "插件A"), []dto.WorkFetchDeclaration{
		{ID: "main", Name: "主处理器", UrlPatterns: []string{"^https://x\\.com/", "^https://x\\.com/special/"}},
	})

	got := m.ListListener("https://x.com/special/1")
	if len(got) != 1 || got[0].ExtensionID != "main" {
		t.Fatalf("同条目多模式命中应去重为一个候选, 实际 %+v", got)
	}
}

// TestRegisterDeclaredCarriesSiteKey 条目声明的站点域（siteKey）随候选入派生索引：
// ListListener 产出上直接可读，登记时去首尾空白；未声明的条目缺省空串
func TestRegisterDeclaredCarriesSiteKey(t *testing.T) {
	m := NewManager()
	m.RegisterDeclared(declaredPlugin("com.example.a", "插件A"), []dto.WorkFetchDeclaration{
		{ID: "main", Name: "主处理器", SiteKey: " pixiv ", UrlPatterns: []string{"^https://x\\.com/"}},
	})

	got := m.ListListener("https://x.com/1")
	if len(got) != 1 {
		t.Fatalf("应命中一个候选, 实际 %+v", got)
	}
	if got[0].SiteKey != "pixiv" {
		t.Errorf("候选 siteKey = %q, 期望 pixiv（登记时去首尾空白）", got[0].SiteKey)
	}

	m.RegisterDeclared(declaredPlugin("com.example.b", "插件B"), []dto.WorkFetchDeclaration{
		{ID: "main", Name: "主处理器", UrlPatterns: []string{"^https://y\\.com/"}}, // 未声明 siteKey
	})
	gotB := m.ListListener("https://y.com/1")
	if len(gotB) != 1 || gotB[0].SiteKey != "" {
		t.Fatalf("未声明 siteKey 的条目应缺省空串, 实际 %+v", gotB)
	}
}

// TestUnregisterClearsPluginEntries 整插件注销清空其全部条目（卸载/崩溃清理路径），
// 其他插件的条目不受影响；条目级精细注销只摘对应条目
func TestUnregisterClearsPluginEntries(t *testing.T) {
	m := NewManager()
	m.RegisterDeclared(declaredPlugin("com.example.a", "插件A"), []dto.WorkFetchDeclaration{
		{ID: "main", UrlPatterns: []string{"^https://x\\.com/"}},
	})
	m.RegisterDeclared(declaredPlugin("com.example.b", "插件B"), []dto.WorkFetchDeclaration{
		{ID: "main", UrlPatterns: []string{"^https://x\\.com/"}},
	})

	m.Unregister("com.example.a", "")
	got := m.ListListener("https://x.com/1")
	if len(got) != 1 || got[0].PublicID.String != "com.example.b" {
		t.Fatalf("整插件注销后应只剩插件B条目, 实际 %+v", got)
	}

	m.RegisterDeclared(declaredPlugin("com.example.b", "插件B"), []dto.WorkFetchDeclaration{
		{ID: "aux", UrlPatterns: []string{"^https://y\\.com/"}},
	})
	m.Unregister("com.example.b", "aux")
	if got := m.ListListener("https://y.com/1"); got != nil {
		t.Fatalf("条目级注销应摘除 aux 条目, 实际 %+v", got)
	}
	if got := m.ListListener("https://x.com/1"); len(got) != 1 || got[0].ExtensionID != "main" {
		t.Fatalf("条目级注销不应株连同插件其他条目, 实际 %+v", got)
	}
}
