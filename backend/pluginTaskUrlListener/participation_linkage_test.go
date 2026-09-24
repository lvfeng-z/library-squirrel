package pluginTaskUrlListener

import (
	"testing"

	"github.com/library-squirrel/backend/plugin/participation"
	"github.com/library-squirrel/backend/plugin/settingresolver"

	dto "github.com/library-squirrel/backend/base/model/dto"
)

// fakeParticipationSource 参与度订阅面测试替身：登记回调供测试直接派发
type fakeParticipationSource struct {
	handler participation.ChangeHandler
}

func (f *fakeParticipationSource) SubscribeChanges(handler participation.ChangeHandler) func() {
	f.handler = handler
	return func() {}
}

// TestApplyWorkFetchEntryParticipationTogglesIndex 条目级参与度联动：停用摘除该条目全部
// 模式（同插件其他条目不株连），恢复参与按登记记忆重挂且候选载体字段（站点域等）保留
func TestApplyWorkFetchEntryParticipationTogglesIndex(t *testing.T) {
	m := NewManager()
	plugin := declaredPlugin("com.example.a", "插件A")
	m.RegisterDeclared(plugin, []dto.WorkFetchDeclaration{
		{ID: "main", Name: "主处理器", SiteKey: " example.com ", UrlPatterns: []string{
			`^https://example\.com/art/`, `^https://example\.net/`}},
		{ID: "aux", Name: "辅助处理器", UrlPatterns: []string{`^https://example\.com/dyn/`}},
	})

	m.ApplyWorkFetchEntryParticipation("com.example.a", "main", false)

	if got := m.ListListener("https://example.com/art/123"); got != nil {
		t.Fatalf("停用条目 main 的模式不应再命中, 实际 %+v", got)
	}
	if got := m.ListListener("https://example.net/x"); got != nil {
		t.Fatalf("停用条目 main 的第二条模式不应再命中, 实际 %+v", got)
	}
	if got := m.ListListener("https://example.com/dyn/1"); len(got) != 1 || got[0].ExtensionID != "aux" {
		t.Fatalf("同插件参与条目 aux 不应被株连, 实际 %+v", got)
	}

	m.ApplyWorkFetchEntryParticipation("com.example.a", "main", true)

	for _, url := range []string{"https://example.com/art/123", "https://example.net/x"} {
		got := m.ListListener(url)
		if len(got) != 1 || got[0].ExtensionID != "main" {
			t.Fatalf("恢复参与后 main 应重新命中 %s, 实际 %+v", url, got)
		}
		if got[0].SiteKey != "example.com" {
			t.Errorf("恢复重挂的候选应保留站点域, SiteKey = %q", got[0].SiteKey)
		}
	}
}

// TestApplyWorkFetchEntryParticipationUnknownEntryNoop 未登记记忆的条目（未声明 urlPatterns
// 或条目 id 不存在）两方向联动均为无操作，不 panic
func TestApplyWorkFetchEntryParticipationUnknownEntryNoop(t *testing.T) {
	m := NewManager()
	m.RegisterDeclared(declaredPlugin("com.example.a", "插件A"), []dto.WorkFetchDeclaration{
		{ID: "main", Name: "主处理器", UrlPatterns: []string{`^https://example\.com/art/`}},
		{ID: "aux", Name: "辅助处理器"}, // 未声明 urlPatterns：无登记记忆
	})

	m.ApplyWorkFetchEntryParticipation("com.example.a", "aux", false)
	m.ApplyWorkFetchEntryParticipation("com.example.a", "aux", true)
	m.ApplyWorkFetchEntryParticipation("com.example.a", "no-such-entry", true)
	m.ApplyWorkFetchEntryParticipation("com.example.b", "main", true)

	if got := m.ListListener("https://example.com/art/123"); len(got) != 1 || got[0].ExtensionID != "main" {
		t.Fatalf("无记忆条目的联动不应影响既有索引, 实际 %+v", got)
	}
}

// TestWholesaleUnregisterPurgesDeclarationMemory 整插件注销（卸载/崩溃）清除登记记忆：
// 之后的恢复参与联动不再复活已注销条目
func TestWholesaleUnregisterPurgesDeclarationMemory(t *testing.T) {
	m := NewManager()
	m.RegisterDeclared(declaredPlugin("com.example.a", "插件A"), []dto.WorkFetchDeclaration{
		{ID: "main", Name: "主处理器", UrlPatterns: []string{`^https://example\.com/art/`}},
	})

	m.Unregister("com.example.a", "")
	m.ApplyWorkFetchEntryParticipation("com.example.a", "main", true)

	if got := m.ListListener("https://example.com/art/123"); got != nil {
		t.Fatalf("整插件注销后恢复参与联动不应复活条目, 实际 %+v", got)
	}
}

// TestServiceAttachParticipationRoutesWorkFetchOnly 服务层订阅接线：point=workFetch 的变更
// 联动派生索引条目级摘除/重挂；其他 point 的变更不触动索引
func TestServiceAttachParticipationRoutesWorkFetchOnly(t *testing.T) {
	svc := NewService(NewManager())
	svc.RegisterDeclared(declaredPlugin("com.example.a", "插件A"), []dto.WorkFetchDeclaration{
		{ID: "main", Name: "主处理器", UrlPatterns: []string{`^https://example\.com/art/`}},
		{ID: "hd", Name: "高清处理器", UrlPatterns: []string{`^https://example\.com/hd/`}},
	})

	source := &fakeParticipationSource{}
	svc.AttachParticipation(source)

	source.handler(participation.Change{PluginPublicId: "com.example.a",
		Point: settingresolver.PointWorkFetch, ID: "hd", Active: false})
	if got := svc.ListListener("https://example.com/hd/1"); got != nil {
		t.Fatalf("停用条目 hd 不应再命中, 实际 %+v", got)
	}

	// 其他 point（如资源类型）的变更不触动 URL 监听索引
	source.handler(participation.Change{PluginPublicId: "com.example.a",
		Point: settingresolver.PointResourceTypes, ID: "com.example.ptype-a", Active: false})
	if got := svc.ListListener("https://example.com/art/123"); len(got) != 1 || got[0].ExtensionID != "main" {
		t.Fatalf("非 workFetch 变更不应触动 URL 监听索引, 实际 %+v", got)
	}

	source.handler(participation.Change{PluginPublicId: "com.example.a",
		Point: settingresolver.PointWorkFetch, ID: "hd", Active: true})
	if got := svc.ListListener("https://example.com/hd/1"); len(got) != 1 || got[0].ExtensionID != "hd" {
		t.Fatalf("恢复参与后 hd 应重新命中, 实际 %+v", got)
	}
}
