package fsmonitor

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// recordingStoreRepairer 记录型 store 域修复替身（记录同步/失效调用，可模拟失败）
type recordingStoreRepairer struct {
	mu              sync.Mutex
	updated         []string    // UpdateFilePath 调用的新路径（按序）
	invalid         []int64     // MarkInvalid 调用的记录 ID（按序）
	renamed         [][2]string // RenameDirectoryPrefix 调用的 old/new 前缀
	failMarkInvalid bool        // MarkInvalid 失败开关（测降级入队）
}

func (r *recordingStoreRepairer) UpdateFilePath(ctx context.Context, id int64, newFilePath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updated = append(r.updated, newFilePath)
	return nil
}

func (r *recordingStoreRepairer) MarkInvalid(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failMarkInvalid {
		return fmt.Errorf("模拟 MarkInvalid 失败")
	}
	r.invalid = append(r.invalid, id)
	return nil
}

func (r *recordingStoreRepairer) RenameDirectoryPrefix(ctx context.Context, oldPrefix, newPrefix string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.renamed = append(r.renamed, [2]string{oldPrefix, newPrefix})
	return 0, nil
}

func (r *recordingStoreRepairer) updatedPaths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.updated))
	copy(out, r.updated)
	return out
}

func (r *recordingStoreRepairer) invalidIDs() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]int64, len(r.invalid))
	copy(out, r.invalid)
	return out
}

// payloadEmitter 记录派发事件与 payload 的发射器替身（自动修复测试断言 autoHandled 标志）
type payloadEmitter struct {
	mu     sync.Mutex
	events []string
	datas  []any
}

func (r *payloadEmitter) Emit(eventName string, data ...any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, eventName)
	if len(data) > 0 {
		r.datas = append(r.datas, data[0])
	}
	return true
}

func (r *payloadEmitter) lastPayload() (map[string]any, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.datas) == 0 {
		return nil, false
	}
	p, ok := r.datas[len(r.datas)-1].(map[string]any)
	return p, ok
}

// newAutoRepairService 构造带自动修复读取器的 Service 替身（emitter 可为 nil）
func newAutoRepairService(repairer StoreRepairer, backupRepairer BackupRepairer, cleaner BackupRefCleaner, emitter EventEmitter, cfg AutoRepairConfig) *Service {
	deps := &Deps{StoreRepairer: repairer, BackupRepairer: backupRepairer, BackupRefCleaner: cleaner}
	svc := NewService(deps, func() string { return "X:/wd" }, func() EventEmitter { return emitter })
	if cfg.Enabled || cfg.Policies != nil {
		svc.SetAutoRepairReader(func() AutoRepairConfig { return cfg })
	}
	return svc
}

// TestAutoRepairPolicyTable 策略常量表锚定（决策1）：每组合可选项与内置默认、键生成、Untracked 不可配置
func TestAutoRepairPolicyTable(t *testing.T) {
	byKey := make(map[string]AutoRepairPolicy, len(autoRepairPolicies))
	for _, p := range autoRepairPolicies {
		byKey[p.Key] = p
	}
	assertPolicy := func(key string, wantOpts []RepairAction, wantDefault RepairAction) {
		t.Helper()
		p, ok := byKey[key]
		if !ok {
			t.Fatalf("策略表缺 %s", key)
		}
		if len(p.Options) != len(wantOpts) {
			t.Fatalf("%s 可选项=%v 应为 %v", key, p.Options, wantOpts)
		}
		for i, o := range wantOpts {
			if p.Options[i] != o {
				t.Fatalf("%s 可选项[%d]=%s 应为 %s", key, i, p.Options[i], o)
			}
		}
		if p.Default != wantDefault {
			t.Fatalf("%s 默认=%s 应为 %s", key, p.Default, wantDefault)
		}
	}
	assertPolicy("store:Move", []RepairAction{ActionSync, ActionRestore}, ActionSync)
	assertPolicy("store:DirMove", []RepairAction{ActionSync, ActionRestore}, ActionSync)
	assertPolicy("backup:Move", []RepairAction{ActionSync, ActionRestore}, ActionSync)
	assertPolicy("store:Delete", []RepairAction{ActionAck}, ActionAck)
	assertPolicy("backup:Delete", []RepairAction{ActionAck}, ActionAck)

	// 键生成与表内键一致
	cases := []struct {
		domain ChangeDomain
		kind   SemanticKind
		want   string
	}{
		{DomainStore, SemanticMove, "store:Move"},
		{DomainStore, SemanticDirMove, "store:DirMove"},
		{DomainBackup, SemanticMove, "backup:Move"},
		{DomainStore, SemanticDelete, "store:Delete"},
		{DomainBackup, SemanticDelete, "backup:Delete"},
	}
	for _, c := range cases {
		if got := autoRepairPolicyKey(c.domain, c.kind); got != c.want {
			t.Fatalf("autoRepairPolicyKey(%v,%v)=%s 应为 %s", c.domain, c.kind, got, c.want)
		}
	}

	// Untracked 不入队无动作，不可配置
	for _, p := range autoRepairPolicies {
		if p.Key == "store:Untracked" || p.Key == "backup:Untracked" {
			t.Fatalf("Untracked 不应可配置: %s", p.Key)
		}
	}
}

// TestResolveAutoRepairAction 动作解析：未配置回落内置默认；非法动作回落默认；Untracked 无默认动作
func TestResolveAutoRepairAction(t *testing.T) {
	sc := &SemanticChange{Kind: SemanticMove, StoreID: 1} // Domain 零值 = store
	if action, ok := resolveAutoRepairAction(sc, nil); !ok || action != ActionSync {
		t.Fatalf("store:Move 未配置应回落默认 sync，实际 %v ok=%v", action, ok)
	}
	if action, ok := resolveAutoRepairAction(sc, map[string]string{"store:Move": "bogus"}); !ok || action != ActionSync {
		t.Fatalf("非法动作应回落默认 sync，实际 %v ok=%v", action, ok)
	}
	if action, ok := resolveAutoRepairAction(sc, map[string]string{"store:Move": "restore"}); !ok || action != ActionRestore {
		t.Fatalf("用户覆盖 restore 应生效，实际 %v ok=%v", action, ok)
	}
	if _, ok := resolveAutoRepairAction(&SemanticChange{Kind: SemanticUntracked}, nil); ok {
		t.Fatal("Untracked 应无默认动作（不可配置）")
	}
}

// TestAutoRepairLiveAutoApplied live 自动：开启 + 策略命中 → 复用既有 apply 执行、不入队、事件带 autoHandled=true
func TestAutoRepairLiveAutoApplied(t *testing.T) {
	repairer := &recordingStoreRepairer{}
	emitter := &payloadEmitter{}
	svc := newAutoRepairService(repairer, nil, nil, emitter, AutoRepairConfig{
		Enabled:  true,
		Policies: map[string]string{"store:Move": "sync"},
	})
	svc.dispatchSemanticChange(context.Background(),
		&SemanticChange{Kind: SemanticMove, FromPath: "store/resource/A/old.jpg", ToPath: "store/resource/B/new.jpg", StoreID: 42},
		changeSourceLive)

	if got := repairer.updatedPaths(); len(got) != 1 || got[0] != "store/resource/B/new.jpg" {
		t.Fatalf("live Move 应自动 sync 更新路径，实际 %v", got)
	}
	if len(svc.ListPendingChanges()) != 0 {
		t.Fatalf("自动处理后不应入队，实际 %d 条待修复", len(svc.ListPendingChanges()))
	}
	payload, ok := emitter.lastPayload()
	if !ok {
		t.Fatal("应派发 fsmonitor:change 事件")
	}
	if autoHandled, _ := payload["autoHandled"].(bool); !autoHandled {
		t.Fatalf("自动处理后 autoHandled 应为 true，实际 %v", payload["autoHandled"])
	}
	if id, _ := payload["id"].(int64); id != 0 {
		t.Fatalf("自动处理后 id 应为 0（未入队），实际 %v", payload["id"])
	}
}

// TestAutoRepairOfflineNotAuto offline（启动对账）不自动（决策2）：即便开启也入队人工确认、autoHandled=false
func TestAutoRepairOfflineNotAuto(t *testing.T) {
	repairer := &recordingStoreRepairer{}
	emitter := &payloadEmitter{}
	svc := newAutoRepairService(repairer, nil, nil, emitter, AutoRepairConfig{
		Enabled:  true,
		Policies: map[string]string{"store:Move": "sync"},
	})
	svc.dispatchSemanticChange(context.Background(),
		&SemanticChange{Kind: SemanticMove, FromPath: "store/resource/A/old.jpg", ToPath: "store/resource/B/new.jpg", StoreID: 42},
		changeSourceOffline)

	if got := repairer.updatedPaths(); len(got) != 0 {
		t.Fatalf("offline 变更不应自动处理，实际更新 %v", got)
	}
	if len(svc.ListPendingChanges()) != 1 {
		t.Fatalf("offline 变更应入队人工确认，实际 %d", len(svc.ListPendingChanges()))
	}
	payload, _ := emitter.lastPayload()
	if autoHandled, _ := payload["autoHandled"].(bool); autoHandled {
		t.Fatal("offline 变更 autoHandled 应为 false")
	}
}

// TestAutoRepairDisabled 开关关闭不自动（决策1 默认关）：live 变更入队人工确认
func TestAutoRepairDisabled(t *testing.T) {
	repairer := &recordingStoreRepairer{}
	svc := newAutoRepairService(repairer, nil, nil, nil, AutoRepairConfig{Enabled: false, Policies: nil})
	svc.dispatchSemanticChange(context.Background(),
		&SemanticChange{Kind: SemanticMove, FromPath: "store/resource/a.jpg", ToPath: "store/resource/b.jpg", StoreID: 1},
		changeSourceLive)
	if got := repairer.updatedPaths(); len(got) != 0 {
		t.Fatalf("开关关闭不应自动处理，实际 %v", got)
	}
	if len(svc.ListPendingChanges()) != 1 {
		t.Fatal("开关关闭应入队人工确认")
	}
}

// TestAutoRepairFailureFallback 自动执行失败降级入队（决策1）：apply 报错 → 不入队结论取消、留人工、autoHandled=false
func TestAutoRepairFailureFallback(t *testing.T) {
	repairer := &recordingStoreRepairer{failMarkInvalid: true}
	emitter := &payloadEmitter{}
	svc := newAutoRepairService(repairer, nil, nil, emitter, AutoRepairConfig{
		Enabled:  true,
		Policies: map[string]string{"store:Delete": "ack"},
	})
	svc.dispatchSemanticChange(context.Background(),
		&SemanticChange{Kind: SemanticDelete, FromPath: "store/resource/gone.jpg", StoreID: 9},
		changeSourceLive)
	if len(svc.ListPendingChanges()) != 1 {
		t.Fatalf("自动执行失败应降级入队，实际 %d", len(svc.ListPendingChanges()))
	}
	payload, _ := emitter.lastPayload()
	if autoHandled, _ := payload["autoHandled"].(bool); autoHandled {
		t.Fatal("自动执行失败 autoHandled 应为 false")
	}
}

// TestAutoRepairStoreDeleteAck 风险4：store Delete 自动 ack 走既有 MarkInvalid（记录级软删，资源从库中消失）
func TestAutoRepairStoreDeleteAck(t *testing.T) {
	repairer := &recordingStoreRepairer{}
	svc := newAutoRepairService(repairer, nil, nil, nil, AutoRepairConfig{
		Enabled:  true,
		Policies: map[string]string{"store:Delete": "ack"},
	})
	svc.dispatchSemanticChange(context.Background(),
		&SemanticChange{Kind: SemanticDelete, FromPath: "store/resource/gone.jpg", StoreID: 9},
		changeSourceLive)
	if got := repairer.invalidIDs(); len(got) != 1 || got[0] != 9 {
		t.Fatalf("store Delete 自动 ack 应 MarkInvalid(9)，实际 %v", got)
	}
}

// TestAutoRepairBackupDeleteAck 风险5：backup Delete 自动 ack 走删清单行 + 清引用方列（不可逆性高于 store 域）
func TestAutoRepairBackupDeleteAck(t *testing.T) {
	repairer := &fakeBackupRepairer{}
	cleaner := &fakeRefCleaner{}
	svc := newAutoRepairService(noopStoreRepairer{}, repairer, cleaner, nil, AutoRepairConfig{
		Enabled:  true,
		Policies: map[string]string{"backup:Delete": "ack"},
	})
	svc.dispatchSemanticChange(context.Background(),
		&SemanticChange{Domain: DomainBackup, Kind: SemanticDelete, FromPath: "backup/2026/a.mp4", BackupID: 7},
		changeSourceLive)
	if len(repairer.deletedIds) != 1 || repairer.deletedIds[0] != 7 {
		t.Fatalf("backup Delete 自动 ack 应删清单行 7，实际 %v", repairer.deletedIds)
	}
	if len(cleaner.ids) != 1 || cleaner.ids[0] != 7 {
		t.Fatalf("backup Delete 自动 ack 应清引用 7，实际 %v", cleaner.ids)
	}
}

// TestAutoRepairUntrackedNotAuto Untracked 不入队不自动（声明2）：开启自动也不动作、不入队
func TestAutoRepairUntrackedNotAuto(t *testing.T) {
	repairer := &recordingStoreRepairer{}
	emitter := &payloadEmitter{}
	svc := newAutoRepairService(repairer, nil, nil, emitter, AutoRepairConfig{Enabled: true, Policies: nil})
	svc.dispatchSemanticChange(context.Background(),
		&SemanticChange{Kind: SemanticUntracked, ToPath: "store/resource/new.jpg"},
		changeSourceLive)
	if got := repairer.updatedPaths(); len(got) != 0 {
		t.Fatalf("Untracked 不应自动处理，实际 %v", got)
	}
	if len(svc.ListPendingChanges()) != 0 {
		t.Fatal("Untracked 不入队（既有行为）")
	}
	payload, _ := emitter.lastPayload()
	if autoHandled, _ := payload["autoHandled"].(bool); autoHandled {
		t.Fatal("Untracked 无动作 autoHandled 应为 false")
	}
}

// TestAutoRepairReaderUnset 未注入读取器（未装配）不自动：live 变更走既有入队路径
func TestAutoRepairReaderUnset(t *testing.T) {
	repairer := &recordingStoreRepairer{}
	svc := NewService(&Deps{StoreRepairer: repairer}, func() string { return "X:/wd" }, func() EventEmitter { return nil })
	svc.dispatchSemanticChange(context.Background(),
		&SemanticChange{Kind: SemanticMove, FromPath: "store/resource/a.jpg", ToPath: "store/resource/b.jpg", StoreID: 1},
		changeSourceLive)
	if got := repairer.updatedPaths(); len(got) != 0 {
		t.Fatalf("未装配自动修复不应自动处理，实际 %v", got)
	}
	if len(svc.ListPendingChanges()) != 1 {
		t.Fatal("未装配自动修复应走既有入队路径")
	}
}
