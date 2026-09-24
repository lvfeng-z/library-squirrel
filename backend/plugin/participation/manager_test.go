package participation

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/plugin/settingresolver"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"go.uber.org/zap"
)

// TestMain 测试环境挂 nop logger（生产 logger 由应用启动时 Init，测试中为 nil；
// 求值失败路径会记日志）
func TestMain(m *testing.M) {
	if logger.Log == nil {
		logger.Log = zap.NewNop().Sugar()
	}
	os.Exit(m.Run())
}

// fakeSettingsReader 全量设置读取替身：按 pluginID 存解密后的明文值（加密解密由真实
// 读取面承担，本替身只供值）
type fakeSettingsReader struct {
	mu     sync.Mutex
	values map[int64]map[string]string
	fail   bool
}

func (f *fakeSettingsReader) GetAllValues(_ context.Context, pluginID int64) (map[string]*pluginsdkdto.StorageValue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return nil, errors.New("读取设置失败（替身注入）")
	}
	out := make(map[string]*pluginsdkdto.StorageValue, len(f.values[pluginID]))
	for k, v := range f.values[pluginID] {
		out[k] = &pluginsdkdto.StorageValue{Value: v}
	}
	return out, nil
}

func (f *fakeSettingsReader) set(pluginID int64, key, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.values == nil {
		f.values = make(map[int64]map[string]string)
	}
	if f.values[pluginID] == nil {
		f.values[pluginID] = make(map[string]string)
	}
	f.values[pluginID][key] = value
}

// changeRecorder 变更通知记录器（含互斥——通知可能在求值 goroutine 上发出）
type changeRecorder struct {
	mu      sync.Mutex
	changes []Change
}

func (r *changeRecorder) record(c Change) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.changes = append(r.changes, c)
}

func (r *changeRecorder) snapshot() []Change {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Change(nil), r.changes...)
}

func (r *changeRecorder) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.changes)
}

// 测试用 resolver 脚本：showMenu=off 停用前端扩展 menu（带理由）、enableFetch=no 停用
// 作品拉取 main；其余取值全参与
const toggleResolverScript = `function resolve(input) {
	var entries = [];
	if (input.settings.showMenu === "off") {
		entries.push({point: "frontendExtensions", id: "menu", active: false, reason: "设置关闭了菜单入口"});
	}
	if (input.settings.enableFetch === "no") {
		entries.push({point: "workFetch", id: "main", active: false});
	}
	return {version: 1, entries: entries};
}`

// 测试用 resolver 脚本：mode=boom 时抛异常（求值失败），其余取值停用前端扩展 menu
const throwingResolverScript = `function resolve(input) {
	if (input.settings.mode === "boom") {
		throw new Error("intentional failure");
	}
	return {version: 1, entries: [{point: "frontendExtensions", id: "menu", active: false}]};
}`

// 测试用慢脚本：确定性忙循环拉长求值时长（双触发乱序测试用；循环量取「评估毫秒级、
// 触发间隔微秒级」的量级差——在途位在触发点同步置位，评估时长只需盖过两次触发间的
// 间隙即行使合并路径，且竞态检测器放缓下仍远低于 500ms 求值超时），toggle=off 停用 menu
const slowResolverScript = `function resolve(input) {
	var s = 0;
	for (var i = 0; i < 50000; i++) { s += i; }
	var entries = [];
	if (input.settings.toggle === "off") {
		entries.push({point: "frontendExtensions", id: "menu", active: false});
	}
	return {version: 1, entries: entries};
}`

// testFixture 参与度真相层测试装配：临时根目录 + 假设置读取 + 清单/实体构造
type testFixture struct {
	root         string
	reader       *fakeSettingsReader
	manager      *Manager
	plugin       *entity.Plugin
	pluginRel    string
	resolverDecl *dto.SettingsResolverDeclaration
}

// newFixture 构造装配并落盘 resolver 脚本（script 为空串时不落盘）
func newFixture(t *testing.T, publicId, script string, contractVersion int) *testFixture {
	t.Helper()
	root := t.TempDir()
	pluginRel := filepath.Join("plugin", "package", publicId, "1.0.0")
	if script != "" {
		dir := filepath.Join(root, pluginRel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("建插件目录失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "resolver.js"), []byte(script), 0o644); err != nil {
			t.Fatalf("写 resolver 脚本失败: %v", err)
		}
	}
	reader := &fakeSettingsReader{}
	fx := &testFixture{
		root:      root,
		reader:    reader,
		manager:   NewManager(reader, root),
		pluginRel: pluginRel,
	}
	fx.plugin = newPluginEntity(publicId, pluginRel)
	fx.setResolverDecl(script, contractVersion)
	return fx
}

// setResolverDecl 设定清单 settingsResolver 声明（script 非空时在场）
func (fx *testFixture) setResolverDecl(script string, contractVersion int) {
	if script == "" {
		fx.resolverDecl = nil
		return
	}
	fx.resolverDecl = &dto.SettingsResolverDeclaration{Script: "resolver.js", ContractVersion: contractVersion}
}

// manifestWith 构造测试清单：前端扩展 menu + 作品拉取 main + 设置声明（各键默认值由用例
// 定制），settingsResolver 声明随形参
func manifestWith(decl *dto.SettingsResolverDeclaration, settings ...dto.SettingDeclaration) *dto.PluginManifest {
	return &dto.PluginManifest{
		Settings:         settings,
		SettingsResolver: decl,
		Extensions: &dto.PluginExtensions{
			FrontendExtensions: []dto.FrontendExtensionDeclaration{{ID: "menu", Name: "菜单入口", Kind: "menu"}},
			WorkFetch:          []dto.WorkFetchDeclaration{{ID: "main", Name: "主处理器"}},
		},
	}
}

// newPluginEntity 构造激活路径测试用插件实体（RootPath 相对根目录）
func newPluginEntity(publicId, rootPath string) *entity.Plugin {
	plugin := entity.NewPlugin()
	plugin.PublicID = sql.NullString{String: publicId, Valid: true}
	plugin.RootPath = sql.NullString{String: rootPath, Valid: true}
	return plugin
}

// waitForIdle 等待该插件会话求值静默（无在途、无待跑）；会话已消解（从管理器映射移除）
// 时按指定旧会话对象等待其退出
func (m *Manager) waitIdle(t *testing.T, publicId string, stale *session) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		s := m.sessions[publicId]
		if stale != nil && s != stale {
			s = stale // 会话已换代：盯旧对象等其收尾
		}
		idle := s == nil || (!s.evalRunning && !s.evalPending)
		m.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("等待插件 %s 求值静默超时", publicId)
}

// currentSession 取当前会话（白箱——乱序测试观察旧会话在途用）
func (m *Manager) currentSession(publicId string) *session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[publicId]
}

// TestStartSessionEvaluatesAndNotifies 触发点①（激活相位末尾）：StartSession 同步完成首次
// 求值，激活返回时覆盖表已就位；停用条目带理由，有效参与态翻转各发一条变更通知
func TestStartSessionEvaluatesAndNotifies(t *testing.T) {
	fx := newFixture(t, "com.test.activate", toggleResolverScript, 1)
	fx.reader.set(fx.plugin.GetID(), "showMenu", "off") // enableFetch 缺键回落默认值 yes
	rec := &changeRecorder{}
	fx.manager.SubscribeChanges(rec.record)

	fx.manager.StartSession(context.Background(), fx.plugin,
		manifestWith(fx.resolverDecl,
			dto.SettingDeclaration{Key: "showMenu", Default: "on"},
			dto.SettingDeclaration{Key: "enableFetch", Default: "yes"}))

	// 同步首评：StartSession 返回即就位（无需等待）
	menuActive, found := fx.manager.EntryActive("com.test.activate", settingresolver.PointFrontendExtensions, "menu")
	if !found || menuActive {
		t.Errorf("menu 有效参与态 = (%v, %v), 期望 (false, true)——KV 值 off 覆盖基线", menuActive, found)
	}
	fetchActive, found := fx.manager.EntryActive("com.test.activate", settingresolver.PointWorkFetch, "main")
	if !found || !fetchActive {
		t.Errorf("main 有效参与态 = (%v, %v), 期望 (true, true)——未覆盖 = 基线参与", fetchActive, found)
	}
	if _, found := fx.manager.EntryActive("com.test.activate", settingresolver.PointWorkFetch, "undeclared"); found {
		t.Error("声明集外条目应不可查（found=false）")
	}

	entries := fx.manager.Entries("com.test.activate")
	if len(entries) != 2 {
		t.Fatalf("声明条目数 = %d, 期望 2", len(entries))
	}
	if entries[0].Point != settingresolver.PointFrontendExtensions || entries[0].ID != "menu" ||
		entries[0].Active || entries[0].Reason != "设置关闭了菜单入口" {
		t.Errorf("menu 条目态 = %+v, 期望停用且带理由", entries[0])
	}
	if !entries[1].Active || entries[1].Reason != "" {
		t.Errorf("main 条目态 = %+v, 期望基线参与无理由", entries[1])
	}

	status := fx.manager.StatusOf("com.test.activate")
	if status == nil || !status.HasResolver || status.LastEvalAt.IsZero() || status.LastFailure != "" {
		t.Errorf("状态面 = %+v, 期望 HasResolver 且最近一次成功", status)
	}

	changes := rec.snapshot()
	if len(changes) != 1 {
		t.Fatalf("首评变更通知数 = %d, 期望 1（仅 menu 停用翻转）: %+v", len(changes), changes)
	}
	if changes[0].PluginPublicId != "com.test.activate" || changes[0].Point != settingresolver.PointFrontendExtensions ||
		changes[0].ID != "menu" || changes[0].Active || changes[0].Reason != "设置关闭了菜单入口" {
		t.Errorf("变更通知 = %+v, 期望 menu 停用方向带理由", changes[0])
	}
}

// TestStartSessionWithoutResolverZeroBehavior 清单未声明 settingsResolver：不求值、无覆盖、
// 无通知，查询面退化为纯声明集；落库触发为空操作
func TestStartSessionWithoutResolverZeroBehavior(t *testing.T) {
	fx := newFixture(t, "com.test.noresolver", "", 1) // 不落盘脚本、不设声明
	rec := &changeRecorder{}
	fx.manager.SubscribeChanges(rec.record)

	fx.manager.StartSession(context.Background(), fx.plugin,
		manifestWith(nil, dto.SettingDeclaration{Key: "showMenu", Default: "on"}))

	if entries := fx.manager.Entries("com.test.noresolver"); len(entries) != 2 {
		t.Fatalf("声明条目数 = %d, 期望 2（声明集照常登记）", len(entries))
	} else if !entries[0].Active || !entries[1].Active {
		t.Errorf("无 resolver 时条目态 = %+v, 期望全基线参与", entries)
	}
	if status := fx.manager.StatusOf("com.test.noresolver"); status == nil || status.HasResolver {
		t.Errorf("状态面 = %+v, 期望 HasResolver=false", status)
	}

	fx.manager.TriggerEvaluation("com.test.noresolver") // 空操作：不得 panic、不得起求值
	fx.manager.waitIdle(t, "com.test.noresolver", nil)
	if rec.len() != 0 {
		t.Errorf("无 resolver 会话变更通知数 = %d, 期望 0", rec.len())
	}
	if status := fx.manager.StatusOf("com.test.noresolver"); !status.LastEvalAt.IsZero() {
		t.Errorf("无 resolver 会话不应有求值记录: %+v", status)
	}
}

// TestTriggerEvaluationAfterSettingsChange 触发点②③共用路径（落库后异步触发）：
// KV 值变化后触发求值，覆盖表按最新输入更新并通知
func TestTriggerEvaluationAfterSettingsChange(t *testing.T) {
	fx := newFixture(t, "com.test.save", toggleResolverScript, 1)
	manifest := manifestWith(fx.resolverDecl,
		dto.SettingDeclaration{Key: "showMenu", Default: "on"},
		dto.SettingDeclaration{Key: "enableFetch", Default: "yes"})
	fx.manager.StartSession(context.Background(), fx.plugin, manifest)
	if active, _ := fx.manager.EntryActive("com.test.save", settingresolver.PointFrontendExtensions, "menu"); !active {
		t.Fatal("前置：默认取值下 menu 应为基线参与")
	}
	rec := &changeRecorder{}
	fx.manager.SubscribeChanges(rec.record)

	fx.reader.set(fx.plugin.GetID(), "showMenu", "off")
	fx.manager.TriggerEvaluation("com.test.save")
	fx.manager.waitIdle(t, "com.test.save", nil)

	if active, _ := fx.manager.EntryActive("com.test.save", settingresolver.PointFrontendExtensions, "menu"); active {
		t.Error("触发求值后 menu 应停用（最新输入 off）")
	}
	changes := rec.snapshot()
	if len(changes) != 1 || changes[0].ID != "menu" || changes[0].Active {
		t.Errorf("变更通知 = %+v, 期望单条 menu 停用", changes)
	}
}

// TestEvaluationFailureKeepsPreviousTable 失败语义：求值失败保留上一次覆盖表，失败分类与
// 时间入状态面，不发变更通知
func TestEvaluationFailureKeepsPreviousTable(t *testing.T) {
	fx := newFixture(t, "com.test.fail", throwingResolverScript, 1)
	manifest := manifestWith(fx.resolverDecl, dto.SettingDeclaration{Key: "mode", Default: "ok"})
	fx.manager.StartSession(context.Background(), fx.plugin, manifest)
	if active, _ := fx.manager.EntryActive("com.test.fail", settingresolver.PointFrontendExtensions, "menu"); active {
		t.Fatal("前置：正常取值下 resolver 停用 menu，覆盖表在位")
	}
	before := fx.manager.StatusOf("com.test.fail")
	rec := &changeRecorder{}
	fx.manager.SubscribeChanges(rec.record)

	fx.reader.set(fx.plugin.GetID(), "mode", "boom")
	fx.manager.TriggerEvaluation("com.test.fail")
	fx.manager.waitIdle(t, "com.test.fail", nil)

	if active, _ := fx.manager.EntryActive("com.test.fail", settingresolver.PointFrontendExtensions, "menu"); active {
		t.Error("求值失败后有效参与态应保持旧表（menu 停用）")
	}
	status := fx.manager.StatusOf("com.test.fail")
	if status.LastFailure != string(settingresolver.FailureRuntime) {
		t.Errorf("失败分类 = %q, 期望 %q", status.LastFailure, settingresolver.FailureRuntime)
	}
	if status.LastFailureMsg == "" {
		t.Error("失败信息应入状态面")
	}
	if status.LastEvalAt.Before(before.LastEvalAt) {
		t.Error("失败求值完成时间不应早于上次成功求值（Windows 时钟粒度下同刻时间戳合法）")
	}
	if rec.len() != 0 {
		t.Errorf("失败求值不应发变更通知, 实际 %d 条", rec.len())
	}
}

// TestFirstEvaluationFailureAllBaseline 首次求值失败 = 无覆盖 = 全基线：所有声明条目照常参与，
// 降级态可查
func TestFirstEvaluationFailureAllBaseline(t *testing.T) {
	fx := newFixture(t, "com.test.firstfail", throwingResolverScript, 1)
	fx.reader.set(fx.plugin.GetID(), "mode", "boom") // 首评即抛
	rec := &changeRecorder{}
	fx.manager.SubscribeChanges(rec.record)

	fx.manager.StartSession(context.Background(), fx.plugin,
		manifestWith(fx.resolverDecl, dto.SettingDeclaration{Key: "mode", Default: "ok"}))

	for _, e := range fx.manager.Entries("com.test.firstfail") {
		if !e.Active {
			t.Errorf("首次失败后条目 %+v 应为基线参与（无覆盖）", e)
		}
	}
	status := fx.manager.StatusOf("com.test.firstfail")
	if status.LastFailure != string(settingresolver.FailureRuntime) {
		t.Errorf("失败分类 = %q, 期望 runtime", status.LastFailure)
	}
	if rec.len() != 0 {
		t.Errorf("首次失败不应发变更通知, 实际 %d 条", rec.len())
	}
}

// TestScriptLoadFailureRecordedInStatus 装载失败（脚本缺失/契约版本不受支持）同样入降级态：
// 每轮触发直接落败，无覆盖
func TestScriptLoadFailureRecordedInStatus(t *testing.T) {
	// 声明在场但脚本未落盘
	fx := newFixture(t, "com.test.noscript", "", 1)
	fx.resolverDecl = &dto.SettingsResolverDeclaration{Script: "resolver.js", ContractVersion: 1}
	fx.manager.StartSession(context.Background(), fx.plugin,
		manifestWith(fx.resolverDecl, dto.SettingDeclaration{Key: "mode", Default: "ok"}))
	status := fx.manager.StatusOf("com.test.noscript")
	if status == nil || status.LastFailure != FailureKindScriptLoad {
		t.Fatalf("脚本缺失的失败分类 = %+v, 期望 %s", status, FailureKindScriptLoad)
	}

	// 契约版本不受支持（清单漂移）
	fx2 := newFixture(t, "com.test.badcontract", toggleResolverScript, 2)
	fx2.manager.StartSession(context.Background(), fx2.plugin,
		manifestWith(fx2.resolverDecl, dto.SettingDeclaration{Key: "showMenu", Default: "on"}))
	status2 := fx2.manager.StatusOf("com.test.badcontract")
	if status2 == nil || status2.LastFailure != FailureKindScriptLoad {
		t.Fatalf("契约版本不受支持的失败分类 = %+v, 期望 %s", status2, FailureKindScriptLoad)
	}
}

// TestSettingsReadFailureRecorded 全量设置读取失败：保留旧表/全基线 + 降级态分类 settings_read
func TestSettingsReadFailureRecorded(t *testing.T) {
	fx := newFixture(t, "com.test.readfail", toggleResolverScript, 1)
	fx.manager.StartSession(context.Background(), fx.plugin,
		manifestWith(fx.resolverDecl, dto.SettingDeclaration{Key: "showMenu", Default: "on"}))

	fx.reader.fail = true
	fx.manager.TriggerEvaluation("com.test.readfail")
	fx.manager.waitIdle(t, "com.test.readfail", nil)

	status := fx.manager.StatusOf("com.test.readfail")
	if status == nil || status.LastFailure != FailureKindSettingsRead {
		t.Errorf("设置读取失败分类 = %+v, 期望 %s", status, FailureKindSettingsRead)
	}
	if active, _ := fx.manager.EntryActive("com.test.readfail", settingresolver.PointFrontendExtensions, "menu"); !active {
		t.Error("读取失败应保留旧表（menu 基线参与）")
	}
}

// TestSnapshotReplaceIdempotent 快照替换幂等：输入不变的重复触发不产生新覆盖与新通知
func TestSnapshotReplaceIdempotent(t *testing.T) {
	fx := newFixture(t, "com.test.idempotent", toggleResolverScript, 1)
	fx.reader.set(fx.plugin.GetID(), "showMenu", "off")
	fx.manager.StartSession(context.Background(), fx.plugin,
		manifestWith(fx.resolverDecl, dto.SettingDeclaration{Key: "showMenu", Default: "on"}))
	rec := &changeRecorder{}
	fx.manager.SubscribeChanges(rec.record)
	if rec.len() != 0 {
		t.Fatalf("订阅晚于首评，前置通知数应为 0")
	}

	for i := 0; i < 3; i++ {
		fx.manager.TriggerEvaluation("com.test.idempotent")
		fx.manager.waitIdle(t, "com.test.idempotent", nil)
	}

	if rec.len() != 0 {
		t.Errorf("同输入重复触发的变更通知数 = %d, 期望 0（幂等）", rec.len())
	}
	if entries := fx.manager.Entries("com.test.idempotent"); len(entries) != 2 || entries[0].Active {
		t.Errorf("重复触发后条目态 = %+v, 期望稳定（menu 停用）", entries)
	}
}

// TestDoubleTriggerCoalescesToLatestInput 双触发乱序（风险9 锚定）：快速两次保存（第二次
// 输入在第一次求值在途期间落库）后，终态 = 第二次输入——在途完成后仅跑最新输入，
// 旧结果后到整体覆盖新结果的乱序窗口不存在
func TestDoubleTriggerCoalescesToLatestInput(t *testing.T) {
	cases := []struct {
		name     string
		first    string
		second   string
		wantMenu bool
	}{
		{"终态取第二次输入（停用）", "on", "off", false},
		{"终态取第二次输入（恢复参与）", "off", "on", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t, "com.test.race", slowResolverScript, 1)
			fx.reader.set(fx.plugin.GetID(), "toggle", "on")
			fx.manager.StartSession(context.Background(), fx.plugin,
				manifestWith(fx.resolverDecl, dto.SettingDeclaration{Key: "toggle", Default: "on"}))

			// 两次快速保存（落库即触发，慢求值在途）：第一次写 first，第二次写 second
			fx.reader.set(fx.plugin.GetID(), "toggle", tc.first)
			fx.manager.TriggerEvaluation("com.test.race")
			fx.reader.set(fx.plugin.GetID(), "toggle", tc.second)
			fx.manager.TriggerEvaluation("com.test.race")

			fx.manager.waitIdle(t, "com.test.race", nil)

			active, found := fx.manager.EntryActive("com.test.race", settingresolver.PointFrontendExtensions, "menu")
			if !found || active != tc.wantMenu {
				t.Errorf("双触发后 menu 有效参与态 = (%v, found=%v), 期望 %v（第二次输入 %q）",
					active, found, tc.wantMenu, tc.second)
			}
			if status := fx.manager.StatusOf("com.test.race"); status.LastFailure != "" {
				t.Errorf("双触发求值不应失败: %+v", status)
			}
		})
	}
}

// TestStopSessionClearsTable OnStopped 清表：会话消解后查询面全部不可见；在途求值的结果
// 经现势检查丢弃、不复活已清空的覆盖表、不发通知
func TestStopSessionClearsTable(t *testing.T) {
	fx := newFixture(t, "com.test.stop", slowResolverScript, 1)
	fx.reader.set(fx.plugin.GetID(), "toggle", "on")
	fx.manager.StartSession(context.Background(), fx.plugin,
		manifestWith(fx.resolverDecl, dto.SettingDeclaration{Key: "toggle", Default: "on"}))
	rec := &changeRecorder{}
	fx.manager.SubscribeChanges(rec.record)

	// 在途慢求值 + 立即停用：旧会话引用留作等待其收尾
	fx.reader.set(fx.plugin.GetID(), "toggle", "off")
	fx.manager.TriggerEvaluation("com.test.stop")
	stale := fx.manager.currentSession("com.test.stop")
	fx.manager.StopSession("com.test.stop")

	if entries := fx.manager.Entries("com.test.stop"); entries != nil {
		t.Errorf("清表后条目清单 = %+v, 期望 nil", entries)
	}
	if _, found := fx.manager.EntryActive("com.test.stop", settingresolver.PointFrontendExtensions, "menu"); found {
		t.Error("清表后条目查询应 found=false")
	}
	if status := fx.manager.StatusOf("com.test.stop"); status != nil {
		t.Errorf("清表后状态面 = %+v, 期望 nil", status)
	}

	// 在途求值收尾后不复活（等旧会话对象退出求值位）
	fx.manager.waitIdle(t, "com.test.stop", stale)
	if entries := fx.manager.Entries("com.test.stop"); entries != nil {
		t.Errorf("在途求值收尾后条目清单 = %+v, 期望仍为 nil（现势检查丢弃）", entries)
	}
	if rec.len() != 0 {
		t.Errorf("清表后在途求值收尾不应发通知, 实际 %d 条", rec.len())
	}

	// 幂等：重复清表不 panic
	fx.manager.StopSession("com.test.stop")
}

// TestStopThenRestartRebuildsFromKV 停用后重新激活：新会话由持久 KV 重算重建（跨会话生效
// 语义），旧会话状态不残留
func TestStopThenRestartRebuildsFromKV(t *testing.T) {
	fx := newFixture(t, "com.test.rebuild", toggleResolverScript, 1)
	manifest := manifestWith(fx.resolverDecl, dto.SettingDeclaration{Key: "showMenu", Default: "on"})

	fx.reader.set(fx.plugin.GetID(), "showMenu", "off")
	fx.manager.StartSession(context.Background(), fx.plugin, manifest)
	fx.manager.StopSession("com.test.rebuild")

	fx.manager.StartSession(context.Background(), fx.plugin, manifest) // 重激活：KV 仍为 off
	if active, _ := fx.manager.EntryActive("com.test.rebuild", settingresolver.PointFrontendExtensions, "menu"); active {
		t.Error("重激活后覆盖表应由 KV 重算（menu 停用）")
	}
}

// TestStartSessionReplacesStaleSession 重复登记同 publicId：旧会话被替换，旧会话在途求值
// 结果丢弃
func TestStartSessionReplacesStaleSession(t *testing.T) {
	fx := newFixture(t, "com.test.replace", slowResolverScript, 1)
	manifest := manifestWith(fx.resolverDecl, dto.SettingDeclaration{Key: "toggle", Default: "on"})

	fx.reader.set(fx.plugin.GetID(), "toggle", "off")
	fx.manager.TriggerEvaluation("com.test.replace") // 无会话：空操作
	fx.manager.StartSession(context.Background(), fx.plugin, manifest)

	fx.reader.set(fx.plugin.GetID(), "toggle", "off")
	fx.manager.TriggerEvaluation("com.test.replace")
	stale := fx.manager.currentSession("com.test.replace")
	fx.manager.StartSession(context.Background(), fx.plugin, manifest) // 换代（新会话以 off 求值）
	fx.manager.waitIdle(t, "com.test.replace", stale)

	// 终态 = 新会话的同步首评结果（off → menu 停用），旧会话在途结果不得覆盖
	if active, _ := fx.manager.EntryActive("com.test.replace", settingresolver.PointFrontendExtensions, "menu"); active {
		t.Error("换代后终态应为新会话求值结果（menu 停用），旧会话在途结果被丢弃")
	}
}

// TestUndeclaredOverrideEntryRejectedPerEntry 声明集外覆盖条目单条拒收：不株连其余条目，
// 计入状态面拒收数
func TestUndeclaredOverrideEntryRejectedPerEntry(t *testing.T) {
	script := `function resolve(input) {
	return {version: 1, entries: [
		{point: "frontendExtensions", id: "ghost", active: false},
		{point: "workFetch", id: "bogus", active: false},
		{point: "frontendExtensions", id: "menu", active: false, reason: "合法停用"}
	]};
}`
	fx := newFixture(t, "com.test.undeclared", script, 1)
	fx.manager.StartSession(context.Background(), fx.plugin,
		manifestWith(fx.resolverDecl, dto.SettingDeclaration{Key: "showMenu", Default: "on"}))

	if active, found := fx.manager.EntryActive("com.test.undeclared", settingresolver.PointFrontendExtensions, "menu"); !found || active {
		t.Errorf("menu 有效参与态 = (%v, %v), 期望合法覆盖生效（停用）", active, found)
	}
	if _, found := fx.manager.EntryActive("com.test.undeclared", settingresolver.PointWorkFetch, "bogus"); found {
		t.Error("声明集外条目被拒收后不应可查")
	}
	status := fx.manager.StatusOf("com.test.undeclared")
	if status == nil || status.LastRejected != 2 {
		t.Errorf("拒收条目数 = %+v, 期望 LastRejected=2", status)
	}
}

// TestSubscribeChangesUnsubscribe 退订后不再收通知
func TestSubscribeChangesUnsubscribe(t *testing.T) {
	fx := newFixture(t, "com.test.unsub", toggleResolverScript, 1)
	manifest := manifestWith(fx.resolverDecl, dto.SettingDeclaration{Key: "showMenu", Default: "on"})
	fx.manager.StartSession(context.Background(), fx.plugin, manifest)

	rec := &changeRecorder{}
	unsubscribe := fx.manager.SubscribeChanges(rec.record)
	unsubscribe()

	fx.reader.set(fx.plugin.GetID(), "showMenu", "off")
	fx.manager.TriggerEvaluation("com.test.unsub")
	fx.manager.waitIdle(t, "com.test.unsub", nil)

	if rec.len() != 0 {
		t.Errorf("退订后通知数 = %d, 期望 0", rec.len())
	}
	if active, _ := fx.manager.EntryActive("com.test.unsub", settingresolver.PointFrontendExtensions, "menu"); active {
		t.Error("退订不影响求值本身（menu 应停用）")
	}
}
