package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/library-squirrel/backend/plugin/participation"
	"github.com/library-squirrel/backend/plugin/settingresolver"
	"github.com/library-squirrel/backend/util"
)

// 真相层 Manager 结构性实现参与度概要提供者（装配处直接注入，无需适配器）
var _ ParticipationStatusProvider = (*participation.Manager)(nil)

// stubParticipationProvider 参与度概要提供者替身：按 publicId 返回预置条目态与求值状态
type stubParticipationProvider struct {
	entries  map[string][]participation.EntryState
	statuses map[string]*participation.Status
}

func (p *stubParticipationProvider) Entries(pluginPublicId string) []participation.EntryState {
	return p.entries[pluginPublicId]
}

func (p *stubParticipationProvider) StatusOf(pluginPublicId string) *participation.Status {
	return p.statuses[pluginPublicId]
}

// TestGetPluginStatusParticipationOverview 有会话插件：概要字段逐项映射——条目态
// （point/id/参与/停用理由）与求值状态（降级分类、人读信息、拒收数、毫秒时间戳）
func TestGetPluginStatusParticipationOverview(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	const publicId = "com.status.overview"
	plantPluginRow(t, svc, publicId, 0)

	evalAt := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	svc.SetParticipationStatusProvider(&stubParticipationProvider{
		entries: map[string][]participation.EntryState{
			publicId: {
				{Point: settingresolver.PointWorkFetch, ID: "main", Active: false, Reason: "用户关闭了高质量模式"},
				{Point: settingresolver.PointFrontendExtensions, ID: "menu", Active: true},
			},
		},
		statuses: map[string]*participation.Status{
			publicId: {
				HasResolver:    true,
				LastEvalAt:     evalAt,
				LastFailure:    "timeout",
				LastFailureMsg: "脚本执行超时",
				LastRejected:   1,
			},
		},
	})

	status, err := svc.GetPluginStatus(context.Background(), publicId)
	if err != nil {
		t.Fatalf("获取插件状态失败: %v", err)
	}
	if status.Participation == nil {
		t.Fatal("有会话插件的状态应携带参与度概要")
	}
	entries := status.Participation.Entries
	if len(entries) != 2 {
		t.Fatalf("概要条目数 = %d, 期望 2", len(entries))
	}
	if entries[0].Point != "workFetch" || entries[0].ID != "main" || entries[0].Active {
		t.Errorf("停用条目映射错误: %+v", entries[0])
	}
	if entries[0].Reason != "用户关闭了高质量模式" {
		t.Errorf("停用理由应透传 resolver 给出的理由, 实际: %q", entries[0].Reason)
	}
	if entries[1].Point != "frontendExtensions" || entries[1].ID != "menu" || !entries[1].Active {
		t.Errorf("参与条目映射错误: %+v", entries[1])
	}
	if entries[1].Reason != "" {
		t.Errorf("参与条目不应携带理由, 实际: %q", entries[1].Reason)
	}
	st := status.Participation.Status
	if !st.HasResolver || st.LastFailure != "timeout" || st.LastFailureMsg != "脚本执行超时" || st.LastRejected != 1 {
		t.Errorf("降级态字段映射错误: %+v", st)
	}
	if want := evalAt.UnixMilli(); st.LastEvalAt != want {
		t.Errorf("最近求值时间 = %d, 期望 %d（Unix 毫秒）", st.LastEvalAt, want)
	}
}

// TestGetPluginStatusParticipationNil 无会话（插件未激活/已停用）或未装配提供者时概要为
// nil，前端不渲染该节；从未求值的会话 LastEvalAt 映射为 0
func TestGetPluginStatusParticipationNil(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	const publicId = "com.status.nosession"
	plantPluginRow(t, svc, publicId, 0)

	// 未装配提供者
	status, err := svc.GetPluginStatus(context.Background(), publicId)
	if err != nil {
		t.Fatalf("获取插件状态失败: %v", err)
	}
	if status.Participation != nil {
		t.Errorf("未装配提供者时概要应为 nil, 实际: %+v", status.Participation)
	}

	// 装配提供者但插件无会话（StatusOf 返回 nil）
	svc.SetParticipationStatusProvider(&stubParticipationProvider{
		statuses: map[string]*participation.Status{publicId: nil},
	})
	status, err = svc.GetPluginStatus(context.Background(), publicId)
	if err != nil {
		t.Fatalf("获取插件状态失败: %v", err)
	}
	if status.Participation != nil {
		t.Errorf("无会话插件概要应为 nil, 实际: %+v", status.Participation)
	}

	// 有会话但从未求值：零值时间映射为 0（区别于 Unix 纪元前的负毫秒）
	const withSession = "com.status.nevereval"
	plantPluginRow(t, svc, withSession, 0)
	svc.SetParticipationStatusProvider(&stubParticipationProvider{
		entries:  map[string][]participation.EntryState{withSession: {}},
		statuses: map[string]*participation.Status{withSession: {HasResolver: false}},
	})
	status, err = svc.GetPluginStatus(context.Background(), withSession)
	if err != nil {
		t.Fatalf("获取插件状态失败: %v", err)
	}
	if status.Participation == nil {
		t.Fatal("有会话插件的状态应携带参与度概要")
	}
	if status.Participation.Status.LastEvalAt != 0 {
		t.Errorf("从未求值时 LastEvalAt 应为 0, 实际: %d", status.Participation.Status.LastEvalAt)
	}
	if status.Participation.Status.HasResolver {
		t.Error("清单未声明 resolver 时 HasResolver 应为 false")
	}
}

// TestGetPluginStatusParticipationFromTruthLayer 真相层端到端：真 Manager 会话
// （StartSession 同步首评）→ GetPluginStatus 概要即内存表现势——停用条目带 resolver 理由，
// 求值状态为最近一次成功
func TestGetPluginStatusParticipationFromTruthLayer(t *testing.T) {
	svc, _ := newCleanupTestService(t)
	const publicId = "com.status.truthlayer"
	manifest := resolverManifestWith(publicId,
		`"settingsResolver":{"script":"resolver.js","contractVersion":1},`)
	row := plantPluginWithManifestOnDisk(t, svc, publicId, manifest)
	script := `function resolve(input) {
		return {version: 1, entries: [
			{point: "frontendExtensions", id: "v1", active: false, reason: "设置开关已关闭"}
		]};
	}`
	absRoot := filepath.Join(util.RootPath(), row.RootPath.String)
	if err := os.WriteFile(filepath.Join(absRoot, "resolver.js"), []byte(script), 0o644); err != nil {
		t.Fatalf("写 resolver 脚本失败: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(util.RootPath(), PluginPackageRoot, publicId))
	})

	mgr := participation.NewManager(NewPluginStorageService(newMockStorageRepo()), util.RootPath())
	svc.SetParticipationStatusProvider(mgr)

	parsed, err := readPluginManifest(row)
	if err != nil {
		t.Fatalf("读清单失败: %v", err)
	}
	mgr.StartSession(context.Background(), row, parsed)

	status, err := svc.GetPluginStatus(context.Background(), publicId)
	if err != nil {
		t.Fatalf("获取插件状态失败: %v", err)
	}
	if status.Participation == nil {
		t.Fatal("激活会话插件的状态应携带参与度概要")
	}
	var v1 *ParticipationEntryState
	for i := range status.Participation.Entries {
		if status.Participation.Entries[i].ID == "v1" {
			v1 = &status.Participation.Entries[i]
		}
	}
	if v1 == nil {
		t.Fatalf("概要应含清单声明条目 v1, 实际: %+v", status.Participation.Entries)
	}
	if v1.Active || v1.Reason != "设置开关已关闭" {
		t.Errorf("resolver 停用条目应为 active=false 且带理由, 实际: %+v", v1)
	}
	st := status.Participation.Status
	if !st.HasResolver || st.LastFailure != "" || st.LastEvalAt == 0 {
		t.Errorf("首评成功后求值状态应为无失败且有完成时间, 实际: %+v", st)
	}

	// 停用清会话后概要回落 nil
	mgr.StopSession(publicId)
	status, err = svc.GetPluginStatus(context.Background(), publicId)
	if err != nil {
		t.Fatalf("获取插件状态失败: %v", err)
	}
	if status.Participation != nil {
		t.Errorf("停用清会话后概要应为 nil, 实际: %+v", status.Participation)
	}
}
