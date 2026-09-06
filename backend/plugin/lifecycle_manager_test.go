package plugin

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	dto "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
)

// activatedPluginEntity 构造激活路径测试用的插件实体（publicId 即状态机键）
func activatedPluginEntity(publicId string) *entity2.Plugin {
	plugin := entity2.NewPlugin()
	plugin.PublicID = sql.NullString{String: publicId, Valid: true}
	plugin.Trusted = sql.NullBool{Bool: true, Valid: true}
	plugin.RootPath = sql.NullString{String: "plugin/fake", Valid: true}
	return plugin
}

// newManagerWithStubManifest 构造状态机并注入 manifest 读替身（返回带空扩展点集合的 manifest），
// 记录参与者按 names 顺序注册（注册顺序即相位顺序）
func newManagerWithStubManifest(events *[]string, names ...string) *lifecycleManager {
	m := newLifecycleManager()
	m.readManifest = func(plugin *entity2.Plugin) (*dto.PluginManifest, error) {
		return &dto.PluginManifest{Extensions: &dto.PluginExtensions{}}, nil
	}
	for _, name := range names {
		m.registerParticipant(&recordingParticipant{name: name, events: events})
	}
	return m
}

// TestActivateForwardPhaseOrder 验证激活相位按参与者注册顺序正向执行，成功后状态置运行中
func TestActivateForwardPhaseOrder(t *testing.T) {
	var events []string
	m := newManagerWithStubManifest(&events, "static", "frontend", "proc")

	if err := m.activate(context.Background(), activatedPluginEntity("com.example.plugin")); err != nil {
		t.Fatalf("activate 失败: %v", err)
	}

	want := []string{"static.Activate", "frontend.Activate", "proc.Activate"}
	if len(events) != len(want) {
		t.Fatalf("相位事件数不符: %v", events)
	}
	for i, e := range want {
		if events[i] != e {
			t.Fatalf("激活相位顺序不符: 期望 %v, 实际 %v", want, events)
		}
	}
	if m.states["com.example.plugin"] != lifecycleActive {
		t.Fatal("激活成功后状态应为运行中")
	}
}

// TestActivateFailureRollsBackAllParticipants 验证激活失败统一回滚：任一参与者 Activate
// 失败即中止后续相位，全体参与者按注册逆序执行 OnStopped（含未执行过 Activate 的参与者），
// 状态回未激活并记录失败原因
func TestActivateFailureRollsBackAllParticipants(t *testing.T) {
	var events []string
	m := newLifecycleManager()
	m.readManifest = func(plugin *entity2.Plugin) (*dto.PluginManifest, error) {
		return &dto.PluginManifest{Extensions: &dto.PluginExtensions{}}, nil
	}
	m.registerParticipant(&recordingParticipant{name: "static", events: &events})
	m.registerParticipant(&recordingParticipant{name: "frontend", activateErr: errors.New("注册失败"), events: &events})
	m.registerParticipant(&recordingParticipant{name: "proc", events: &events})

	err := m.activate(context.Background(), activatedPluginEntity("com.example.plugin"))
	if err == nil {
		t.Fatal("相位失败应返回错误")
	}

	want := []string{
		"static.Activate", "frontend.Activate",
		"proc.OnStopped", "frontend.OnStopped", "static.OnStopped",
	}
	if len(events) != len(want) {
		t.Fatalf("相位事件数不符: %v", events)
	}
	for i, e := range want {
		if events[i] != e {
			t.Fatalf("回滚相位顺序不符: 期望 %v, 实际 %v", want, events)
		}
	}
	if _, still := m.states["com.example.plugin"]; still {
		t.Fatal("激活失败后状态应回未激活（表项移除）")
	}
	if m.lastActivateErr["com.example.plugin"] == nil {
		t.Fatal("激活失败原因应被记录")
	}
}

// TestActivateIdempotentWhenActive 验证对已运行插件重复激活直接成功、不重入相位（B-2 上层守卫）
func TestActivateIdempotentWhenActive(t *testing.T) {
	var events []string
	m := newManagerWithStubManifest(&events, "static")
	m.states["com.example.plugin"] = lifecycleActive

	if err := m.activate(context.Background(), activatedPluginEntity("com.example.plugin")); err != nil {
		t.Fatalf("重复激活应幂等成功: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("已运行插件的激活不应触发相位: %v", events)
	}
}

// TestLifecycleBusyRejectedInTransient 验证瞬态拒绝：激活中/停用中收到新的生命周期操作
// 返回 ErrPluginLifecycleBusy
func TestLifecycleBusyRejectedInTransient(t *testing.T) {
	var events []string
	m := newManagerWithStubManifest(&events, "static")
	plugin := activatedPluginEntity("com.example.plugin")

	m.states["com.example.plugin"] = lifecycleActivating
	if err := m.activate(context.Background(), plugin); !errors.Is(err, ErrPluginLifecycleBusy) {
		t.Fatalf("激活中重复激活应被瞬态拒绝, got %v", err)
	}
	if err := m.deactivate(context.Background(), "com.example.plugin", PluginStopOpUninstall, false); !errors.Is(err, ErrPluginLifecycleBusy) {
		t.Fatalf("激活中停用应被瞬态拒绝, got %v", err)
	}

	m.states["com.example.plugin"] = lifecycleStopping
	if err := m.deactivate(context.Background(), "com.example.plugin", PluginStopOpUninstall, false); !errors.Is(err, ErrPluginLifecycleBusy) {
		t.Fatalf("停用中重复停用应被瞬态拒绝, got %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("瞬态拒绝不应触发相位: %v", events)
	}
}

// TestActivateManifestFailureRecordsError 验证 manifest 读取失败不进相位，状态回未激活并记录原因
func TestActivateManifestFailureRecordsError(t *testing.T) {
	var events []string
	m := newManagerWithStubManifest(&events, "static")
	readErr := errors.New("plugin.json 不存在")
	m.readManifest = func(plugin *entity2.Plugin) (*dto.PluginManifest, error) {
		return nil, readErr
	}

	err := m.activate(context.Background(), activatedPluginEntity("com.example.plugin"))
	if !errors.Is(err, readErr) {
		t.Fatalf("应透传 manifest 读取失败, got %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("manifest 失败不应触发相位: %v", events)
	}
	if _, still := m.states["com.example.plugin"]; still {
		t.Fatal("失败后状态应回未激活")
	}
	if m.lastActivateErr["com.example.plugin"] == nil {
		t.Fatal("失败原因应被记录")
	}
}

// TestActivateSuccessClearsLastError 验证下次激活成功时清除上次激活失败记录
func TestActivateSuccessClearsLastError(t *testing.T) {
	var events []string
	m := newManagerWithStubManifest(&events, "static")
	m.lastActivateErr["com.example.plugin"] = errors.New("上次失败")

	if err := m.activate(context.Background(), activatedPluginEntity("com.example.plugin")); err != nil {
		t.Fatalf("activate 失败: %v", err)
	}
	if m.lastActivateErr["com.example.plugin"] != nil {
		t.Fatal("激活成功应清除失败记录")
	}
}

// TestActivateManifestWithoutExtensionsSkipsPhases 验证 manifest 无扩展点时不进相位、视为激活成功
func TestActivateManifestWithoutExtensionsSkipsPhases(t *testing.T) {
	var events []string
	m := newManagerWithStubManifest(&events, "static")
	m.readManifest = func(plugin *entity2.Plugin) (*dto.PluginManifest, error) {
		return &dto.PluginManifest{}, nil // Extensions 为 nil
	}

	if err := m.activate(context.Background(), activatedPluginEntity("com.example.plugin")); err != nil {
		t.Fatalf("无扩展点激活应成功: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("无扩展点不应触发相位: %v", events)
	}
	if m.states["com.example.plugin"] != lifecycleActive {
		t.Fatal("无扩展点激活成功后状态应为运行中")
	}
}

// TestDeactivateNoOpWhenInactive 验证未激活插件停用幂等返回、不触发相位（卸载从未激活的插件路径）
func TestDeactivateNoOpWhenInactive(t *testing.T) {
	var events []string
	m := newManagerWithStubManifest(&events, "static", "proc")

	if err := m.deactivate(context.Background(), "com.example.plugin", PluginStopOpUninstall, false); err != nil {
		t.Fatalf("未激活停用应幂等成功: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("未激活停用不应触发相位: %v", events)
	}
}

// TestNotifyCrashedReverseCleanupOnlyWhenActive 验证崩溃清理：仅运行中状态执行（幂等），
// 清理按注册逆序、状态回未激活；非运行中状态的重复崩溃通知不重复清理
func TestNotifyCrashedReverseCleanupOnlyWhenActive(t *testing.T) {
	var events []string
	m := newLifecycleManager()
	m.registerParticipant(&recordingParticipant{name: "static", events: &events})
	m.registerParticipant(&recordingParticipant{name: "frontend", events: &events})
	m.registerParticipant(&recordingParticipant{name: "proc", events: &events})
	m.states["com.example.plugin"] = lifecycleActive

	m.notifyCrashed(context.Background(), "com.example.plugin")

	want := []string{"proc.OnStopped", "frontend.OnStopped", "static.OnStopped"}
	if len(events) != len(want) {
		t.Fatalf("清理事件数不符: %v", events)
	}
	for i, e := range want {
		if events[i] != e {
			t.Fatalf("崩溃清理顺序不符: 期望 %v, 实际 %v", want, events)
		}
	}
	if _, still := m.states["com.example.plugin"]; still {
		t.Fatal("崩溃清理后状态应回未激活")
	}

	// 非运行中状态的重复通知：不重复清理
	events = nil
	m.notifyCrashed(context.Background(), "com.example.plugin")
	if len(events) != 0 {
		t.Fatalf("非运行中崩溃通知不应触发清理: %v", events)
	}
}
