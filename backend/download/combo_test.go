package download

// 查重内移与确认环测试：板块组合执行入口的查重编排（三分类分流）+ 冲突挂起等待
// （WaitReplaceConfirm 消费控制面确认通道）+ 确认决策记忆匹配消费（冲突作品集合一致才
// 复用决策，跨暂停/恢复不重复弹窗）+ 跳过收口（handle.Skip 上报）。控制面交互经
// confirmHandle（fakeHandle 的可编程扩展）模拟，查重/站点键经 stub 注入。

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/taskManager"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// ==== fakes ====

// fakeDupChecker 查重判定桩:预置判定结果并记录查询次数供"未查重"断言
type fakeDupChecker struct {
	result duplicate.DuplicateCheckResult
	calls  int
}

func (f *fakeDupChecker) Check(ctx context.Context, items []duplicate.DuplicateCheckItem) ([]duplicate.DuplicateCheckResult, error) {
	f.calls++
	out := make([]duplicate.DuplicateCheckResult, len(items))
	for i := range items {
		out[i] = f.result
	}
	return out, nil
}

// fakeSiteKeyResolver 站点 ID → 站点键桩(查重输入键形态统一:任务领域行站点 ID 反查站点键)
type fakeSiteKeyResolver struct {
	keys map[int64]string
}

func (f *fakeSiteKeyResolver) ListByIds(ctx context.Context, ids []int64) ([]*entity.Site, error) {
	out := make([]*entity.Site, 0, len(ids))
	for _, id := range ids {
		if key, ok := f.keys[id]; ok {
			s := entity.NewSite()
			s.ID = id
			s.SiteKey = key
			out = append(out, s)
		}
	}
	return out, nil
}

// fakePluginExec 插件执行器桩:Start 固定返回错误,使查重门槛通过(不弹窗/答复替换)的用例
// 在下载前失败终止,无需真实的流/存储依赖;CreateWorkInfo 正常返回空响应(仅作品信息用例);
// startSpecs 非空时 Start 返回预置流集合(derived 重产登记用例)。captureOffsets/captureStartRoles
// 供续传用例捕获下发的偏移/重产角色
type fakePluginExec struct {
	createErr   error
	createdWork bool // CreateWorkInfo 是否被调用(作品信息板块)
	startCalls  int
	startErr    error
	startSpecs  []*sdkdto.StoreSpec
	startResp   *sdkdto.WorkResponse
	resumeCalls int
	resumeSpecs []*sdkdto.StoreSpec
	resumeResp  *sdkdto.WorkResponse
	resumeErr   error

	captureOffsets    func(offsets []*sdkdto.StoreResumeOffset)
	captureStartRoles func(roles []string)
}

func (e *fakePluginExec) CreateWorkInfo(ctx context.Context, task *entity.Task, workTask *entity.WorkTask) (*sdkdto.WorkResponse, error) {
	e.createdWork = true
	return nil, e.createErr
}

func (e *fakePluginExec) Start(ctx context.Context, task *entity.Task, workTask *entity.WorkTask, storeRoles []string) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	e.startCalls++
	if e.captureStartRoles != nil {
		e.captureStartRoles(storeRoles)
	}
	if e.startSpecs != nil {
		return e.startSpecs, e.startResp, nil
	}
	if e.startErr != nil {
		return nil, nil, e.startErr
	}
	return nil, nil, fmt.Errorf("测试桩:不进入下载")
}

func (e *fakePluginExec) Pause(ctx context.Context, param *sdkdto.TaskResParam) error { return nil }

func (e *fakePluginExec) Stop(ctx context.Context, param *sdkdto.TaskResParam) error { return nil }

func (e *fakePluginExec) Resume(ctx context.Context, param *sdkdto.TaskResumeParam) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	e.resumeCalls++
	if e.captureOffsets != nil {
		e.captureOffsets(param.StreamOffsets)
	}
	return e.resumeSpecs, e.resumeResp, e.resumeErr
}

// stubWorkInfoSaver 作品信息保存桩
type stubWorkInfoSaver struct {
	savedWorkId int64
	err         error
}

func (s *stubWorkInfoSaver) SaveWorkInfo(ctx context.Context, task *entity.Task, workTask *entity.WorkTask, workResp *sdkdto.WorkResponse) (int64, error) {
	return s.savedWorkId, s.err
}

// confirmHandle fakeHandle 的可编程扩展：覆盖确认答复/取消、决策记忆注入、跳过收口与
// 终态回滚登记记录（指针嵌入复用基础终态/进度/排空阶段记录，避免拷贝内嵌锁）
type confirmHandle struct {
	*fakeHandle
	mu           sync.Mutex
	confirmCalls int
	conflicts    [][]taskManager.ConflictInfo
	decision     taskManager.ReplaceDecision // 预设整体答复
	canceled     bool                        // 预设取消返回
	memo         *taskManager.ReplaceConfirmMemo
	skipMsgs     []string
	rollbacks    []taskManager.TerminalRollback
	resumeFlag   bool // 预设恢复信号（控制面在本次执行进入策略前置位）
}

func newConfirmHandle() (*confirmHandle, context.CancelFunc) {
	base, cancel := newFakeHandle()
	h := &confirmHandle{fakeHandle: base, decision: taskManager.ReplaceDecisionReplace}
	return h, cancel
}

var _ taskManager.StrategyHandle = (*confirmHandle)(nil)

func (h *confirmHandle) ConfirmMemo() *taskManager.ReplaceConfirmMemo { return h.memo }

func (h *confirmHandle) ResumeRequested() bool { return h.resumeFlag }

func (h *confirmHandle) SetTerminalRollback(rollback taskManager.TerminalRollback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rollbacks = append(h.rollbacks, rollback)
}

func (h *confirmHandle) WaitReplaceConfirm(conflicts []taskManager.ConflictInfo) (taskManager.ReplaceDecision, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.confirmCalls++
	h.conflicts = append(h.conflicts, conflicts)
	return h.decision, h.canceled
}

func (h *confirmHandle) Skip(errMsg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.skipMsgs = append(h.skipMsgs, errMsg)
}

func (h *confirmHandle) skipped() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.skipMsgs) > 0
}

func (h *confirmHandle) lastSkipMsg() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.skipMsgs) == 0 {
		return ""
	}
	return h.skipMsgs[len(h.skipMsgs)-1]
}

// newComboSession 构造进入板块组合查重编排的执行会话。门槛后各段依赖注入桩:
// Start 桩固定报错,不弹窗/答复替换的用例在下载前终止
func newComboSession(taskId int64, mode runMode, checker *fakeDupChecker, resolver *fakeSiteKeyResolver, exec *fakePluginExec, saver WorkInfoSaver, replaceOps resource.ReplaceStoreOps) (*execSession, *confirmHandle, context.CancelFunc) {
	h, cancel := newConfirmHandle()
	wt := entity.NewWorkTask(taskId)
	wt.SiteID = sql.NullInt64{Int64: 1, Valid: true}
	wt.SiteWorkID = sql.NullString{String: "sw1", Valid: true}
	wt.ResourceType = sql.NullString{String: entity.ResourceTypeImage, Valid: true}
	deps := &Deps{
		WorkDirProvider:     stubWorkDirProvider{dir: "E:/lib"},
		DuplicateChecker:    checker,
		SiteKeyResolver:     resolver,
		WorkInfoSaver:       saver,
		PluginExecFactory:   nil,
		ReplaceStoreOps:     replaceOps,
		ResourceStoreWriter: &stubResourceStoreWriter{},
	}
	sess := newExecSession(deps, h, wt)
	sess.mode = mode
	sess.pluginExec = exec
	return sess, h, cancel
}

// newGateStubs 组装真实替换链能力的 stub 集（软删/替换定位用例复用 replace_softdelete_test 的桩体系）
func newGateStubs() *replaceStubs {
	stubs := newReplaceStubs()
	return stubs
}

// fakeCheckResult 构造查重判定结果
func fakeCheckResult(class duplicate.DuplicateClass, conflictRoles []string) duplicate.DuplicateCheckResult {
	return duplicate.DuplicateCheckResult{Class: class, WorkID: 500, WorkName: "已存在作品", ConflictRoles: conflictRoles}
}

// ==== 表驱动：三分类分流 ====

// TestCheckDuplicate_RowLevel 验证查重编排对三分类判定的分流
func TestCheckDuplicate_RowLevel(t *testing.T) {
	cases := []struct {
		name     string
		result   duplicate.DuplicateCheckResult
		wantPush bool
		// wantMiss 未命中:不定位已有作品(资源重执行无法挂载转 Failed);false 表示命中无冲突(定位为替换目标)
		wantMiss bool
		// wantRoles 期望载荷板块明细;wantPush 且 wantNil 时期望载荷为 nil
		wantRoles []string
		wantNil   bool
	}{
		{
			name:     "命中无冲突_交集空_不弹窗",
			result:   fakeCheckResult(duplicate.DuplicateHitNoConflict, nil),
			wantPush: false,
		},
		{
			name:      "命中冲突_载荷为交集",
			result:    fakeCheckResult(duplicate.DuplicateHitConflict, []string{entity.StoreTypeThumbnail}),
			wantPush:  true,
			wantRoles: []string{entity.StoreTypeThumbnail},
		},
		{
			name:      "命中冲突_载荷为已有行全集",
			result:    fakeCheckResult(duplicate.DuplicateHitConflict, []string{entity.StoreTypeImage, entity.StoreTypeThumbnail}),
			wantPush:  true,
			wantRoles: []string{entity.StoreTypeImage, entity.StoreTypeThumbnail},
		},
		{
			name:     "命中冲突_行级信息不可得_保守弹窗_载荷nil", // 宁多弹不漏弹(行级查询失败)
			result:   fakeCheckResult(duplicate.DuplicateHitConflict, nil),
			wantPush: true,
			wantNil:  true,
		},
		{
			name:     "命中无冲突_零行_不弹窗",
			result:   fakeCheckResult(duplicate.DuplicateHitNoConflict, nil),
			wantPush: false,
		},
		{
			name:     "未命中_不弹窗",
			result:   duplicate.DuplicateCheckResult{Class: duplicate.DuplicateMiss},
			wantPush: false,
			wantMiss: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checker := &fakeDupChecker{result: tc.result}
			stubs := newGateStubs()
			// 命中冲突且答复替换（预设 decision=Replace）后继续替换定位；Start 桩报错转失败收口
			sess, h, cancel := newComboSession(300, runMode{storeScope: storeScope{kind: scopeSelected, roles: []string{entity.StoreTypeThumbnail}}},
				checker, &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}}, &fakePluginExec{},
				nil, newRealReplacementService(stubs, nil))
			defer cancel()
			// 预置资源图：作品 500 → 资源 700（无冲突命中定位替换目标后的软删对象）
			res := entity.NewResource()
			res.ID = 700
			res.WorkID = 500
			stubs.res.resources = []*entity.Resource{res}

			sess.runSectionCombo()

			pushed := h.confirmCalls > 0
			if pushed != tc.wantPush {
				t.Fatalf("弹窗期望 %v, 实际 %v (confirmCalls=%d)", tc.wantPush, pushed, h.confirmCalls)
			}
			if !tc.wantPush {
				if !h.failed {
					t.Fatal("未弹窗场景门槛通过后应经 Start 桩报错转失败收口")
				}
				if tc.wantMiss {
					// 未命中:不定位已有作品,资源重执行无法挂载
					if sess.workId == 300 || sess.isReplace {
						t.Fatalf("未命中不应定位已有作品(workId=%d isReplace=%v)", sess.workId, sess.isReplace)
					}
				} else if sess.workId != 500 || !sess.isReplace {
					// 命中无冲突:已有作品定位为替换目标
					t.Fatalf("未弹窗时应定位已有作品为替换目标(workId=%d isReplace=%v)", sess.workId, sess.isReplace)
				}
				return
			}
			call := h.conflicts[0]
			if len(call) != 1 || call[0].WorkID != 500 {
				t.Fatalf("冲突载荷应为已有作品 ID 500, 实际 %+v", call)
			}
			if tc.wantNil {
				if call[0].ConflictRoles != nil {
					t.Fatalf("载荷 ConflictRoles 期望 nil, 实际 %v", call[0].ConflictRoles)
				}
				return
			}
			if len(call[0].ConflictRoles) != len(tc.wantRoles) {
				t.Fatalf("载荷 ConflictRoles 期望 %v, 实际 %v", tc.wantRoles, call[0].ConflictRoles)
			}
			for i, r := range tc.wantRoles {
				if call[0].ConflictRoles[i] != r {
					t.Fatalf("载荷 ConflictRoles 期望 %v, 实际 %v", tc.wantRoles, call[0].ConflictRoles)
				}
			}
		})
	}
}

// TestCheckDuplicate_EmptyIntersectionKeepsExistingWorkId 命中无冲突不弹窗但视为替换:
// 定位到已有作品(workId 置位、isReplace=true)；下载窗口零副作用——软删在提交点，查重后
// 直接失败也不触碰旧 store
func TestCheckDuplicate_EmptyIntersectionKeepsExistingWorkId(t *testing.T) {
	checker := &fakeDupChecker{result: fakeCheckResult(duplicate.DuplicateHitNoConflict, nil)}
	stubs := newGateStubs()
	sess, h, cancel := newComboSession(300, runMode{storeScope: storeScope{kind: scopeSelected, roles: []string{entity.StoreTypeThumbnail}}},
		checker, &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}}, &fakePluginExec{},
		nil, newRealReplacementService(stubs, nil))
	defer cancel()
	// 预置资源图：作品 500 → 资源 700 → thumbnail 关联(store 800，已完成活行)
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	stubs.res.resources = []*entity.Resource{res}
	stubs.rs.byResourceIds = []*entity.ResourceStore{makeReplaceAssoc(700, entity.StoreTypeThumbnail, 0, 800)}
	stubs.rows.rows = []*entity.PersistentStore{makeReplaceStoreRow(800, 1, 0, 0, "store/work/a/t.jpg")}

	sess.runSectionCombo()

	if h.confirmCalls > 0 {
		t.Fatal("空交集不应弹窗")
	}
	if sess.existingWorkId != 0 || sess.workId != 500 || !sess.isReplace {
		t.Fatalf("空交集应定位到已有作品并视为替换(existingWorkId=%d workId=%d isReplace=%v)",
			sess.existingWorkId, sess.workId, sess.isReplace)
	}
	// 替换定位后、下载前的失败（Start 桩报错）零软删零登记（坍缩后长下载窗口无 DB 副作用）
	if len(stubs.replacer.backupIds) != 0 || len(stubs.replacer.discardedIds) != 0 {
		t.Fatalf("下载窗口不应触碰旧 store, 实际 备份软删 %v 废弃 %v", stubs.replacer.backupIds, stubs.replacer.discardedIds)
	}
	if len(h.rollbacks) != 0 {
		t.Fatalf("下载窗口零回滚登记, 实际 %v", h.rollbacks)
	}
}

// ==== 确认环：答复分流 ====

// TestCheckDuplicate_ReplaceAnswerContinues 冲突弹窗后答复替换：继续板块组合（替换定位），
// 不跳过、不走失败
func TestCheckDuplicate_ReplaceAnswerContinues(t *testing.T) {
	checker := &fakeDupChecker{result: fakeCheckResult(duplicate.DuplicateHitConflict, []string{entity.StoreTypeImage})}
	stubs := newGateStubs()
	sess, h, cancel := newComboSession(300, runMode{storeScope: storeScope{kind: scopeSelected, roles: []string{entity.StoreTypeImage}}},
		checker, &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}}, &fakePluginExec{},
		nil, newRealReplacementService(stubs, nil))
	defer cancel()
	h.decision = taskManager.ReplaceDecisionReplace
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	stubs.res.resources = []*entity.Resource{res}
	stubs.rs.byResourceIds = []*entity.ResourceStore{makeReplaceAssoc(700, entity.StoreTypeImage, 0, 800)}
	stubs.rows.rows = []*entity.PersistentStore{makeReplaceStoreRow(800, 1, 0, 0, "store/work/a/x.png")}

	sess.runSectionCombo()

	if h.confirmCalls != 1 {
		t.Fatalf("冲突命中应弹窗一次, 实际 %d", h.confirmCalls)
	}
	if h.skipped() {
		t.Fatal("答复替换不应跳过收口")
	}
	if sess.workId != 500 || !sess.isReplace {
		t.Fatalf("答复替换应定位已有作品为替换目标(workId=%d isReplace=%v)", sess.workId, sess.isReplace)
	}
	// 答复替换后 Start 桩报错 → 失败收口；下载窗口零软删零登记（软删在提交点）
	if !h.failed {
		t.Fatal("答复替换后门槛续行应由 Start 桩报错转失败收口")
	}
	if len(stubs.replacer.backupIds) != 0 || len(h.rollbacks) != 0 {
		t.Fatalf("下载窗口应零软删零登记, 实际 备份软删 %v 登记 %v", stubs.replacer.backupIds, h.rollbacks)
	}
}

// TestCheckDuplicate_SkipAnswerCloses 冲突弹窗后答复跳过：经 handle.Skip 上报收口
// （空错误说明），不产生终态（不 Finish 不 Fail）
func TestCheckDuplicate_SkipAnswerCloses(t *testing.T) {
	checker := &fakeDupChecker{result: fakeCheckResult(duplicate.DuplicateHitConflict, []string{entity.StoreTypeImage})}
	stubs := newGateStubs()
	sess, h, cancel := newComboSession(300, runMode{storeScope: storeScope{kind: scopeSelected, roles: []string{entity.StoreTypeImage}}},
		checker, &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}}, &fakePluginExec{},
		nil, newRealReplacementService(stubs, nil))
	defer cancel()
	h.decision = taskManager.ReplaceDecisionSkip
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	stubs.res.resources = []*entity.Resource{res}

	sess.runSectionCombo()

	if h.confirmCalls != 1 {
		t.Fatalf("冲突命中应弹窗一次, 实际 %d", h.confirmCalls)
	}
	if !h.skipped() {
		t.Fatal("答复跳过应经 Skip 上报收口")
	}
	if h.lastSkipMsg() != "" {
		t.Fatalf("用户裁决跳过的错误说明应为空, 实际 %q", h.lastSkipMsg())
	}
	if h.finished || h.failed {
		t.Fatal("跳过收口不应产生终态")
	}
	if sess.isReplace || len(stubs.replacer.backupIds) != 0 {
		t.Fatalf("答复跳过不应进入替换(软删)，实际 isReplace=%v 软删=%v", sess.isReplace, stubs.replacer.backupIds)
	}
}

// TestCheckDuplicate_CanceledReturns 确认等待被取消（暂停/停止打断）：不收口不跳过，
// 板块组合返回中断交控制面
func TestCheckDuplicate_CanceledReturns(t *testing.T) {
	checker := &fakeDupChecker{result: fakeCheckResult(duplicate.DuplicateHitConflict, []string{entity.StoreTypeImage})}
	stubs := newGateStubs()
	sess, h, cancel := newComboSession(300, runMode{storeScope: storeScope{kind: scopeSelected, roles: []string{entity.StoreTypeImage}}},
		checker, &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}}, &fakePluginExec{},
		nil, newRealReplacementService(stubs, nil))
	defer cancel()
	h.canceled = true

	res := sess.runSectionCombo()

	if res != comboInterrupted {
		t.Fatalf("确认等待取消应返回中断交控制面, 实际 %d", res)
	}
	if h.skipped() || h.finished || h.failed {
		t.Fatal("取消返回不应有任何收口上报")
	}
}

// ==== 决策记忆匹配消费 ====

// TestCheckDuplicate_MemoHitReusesDecision 决策记忆命中（冲突作品 ID 集一致）复用决策不弹窗：
// 记忆为替换 → 直接定位替换目标继续软删；记忆为跳过 → 直接 Skip 收口
func TestCheckDuplicate_MemoHitReusesDecision(t *testing.T) {
	t.Run("记忆替换", func(t *testing.T) {
		checker := &fakeDupChecker{result: fakeCheckResult(duplicate.DuplicateHitConflict, []string{entity.StoreTypeImage})}
		stubs := newGateStubs()
		sess, h, cancel := newComboSession(300, runMode{storeScope: storeScope{kind: scopeSelected, roles: []string{entity.StoreTypeImage}}},
			checker, &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}}, &fakePluginExec{},
			nil, newRealReplacementService(stubs, nil))
		defer cancel()
		h.memo = &taskManager.ReplaceConfirmMemo{ConflictWorkIds: []int64{500}, Decision: taskManager.ReplaceDecisionReplace}
		res := entity.NewResource()
		res.ID = 700
		res.WorkID = 500
		stubs.res.resources = []*entity.Resource{res}

		sess.runSectionCombo()

		if h.confirmCalls != 0 {
			t.Fatalf("记忆命中不应重复弹窗, 实际 %d 次", h.confirmCalls)
		}
		if sess.workId != 500 || !sess.isReplace {
			t.Fatalf("记忆替换应直接定位替换目标(workId=%d isReplace=%v)", sess.workId, sess.isReplace)
		}
	})
	t.Run("记忆跳过", func(t *testing.T) {
		checker := &fakeDupChecker{result: fakeCheckResult(duplicate.DuplicateHitConflict, []string{entity.StoreTypeImage})}
		stubs := newGateStubs()
		sess, h, cancel := newComboSession(300, runMode{storeScope: storeScope{kind: scopeSelected, roles: []string{entity.StoreTypeImage}}},
			checker, &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}}, &fakePluginExec{},
			nil, newRealReplacementService(stubs, nil))
		defer cancel()
		h.memo = &taskManager.ReplaceConfirmMemo{ConflictWorkIds: []int64{500}, Decision: taskManager.ReplaceDecisionSkip}

		sess.runSectionCombo()

		if h.confirmCalls != 0 {
			t.Fatalf("记忆命中不应重复弹窗, 实际 %d 次", h.confirmCalls)
		}
		if !h.skipped() {
			t.Fatal("记忆跳过应直接 Skip 收口")
		}
		if h.finished || h.failed {
			t.Fatal("记忆跳过不应产生终态")
		}
	})
}

// TestCheckDuplicate_MemoMissFallsBackToConfirm 决策记忆不匹配（冲突作品 ID 集不一致）：
// 重新弹窗等待答复
func TestCheckDuplicate_MemoMissFallsBackToConfirm(t *testing.T) {
	checker := &fakeDupChecker{result: fakeCheckResult(duplicate.DuplicateHitConflict, []string{entity.StoreTypeImage})}
	stubs := newGateStubs()
	sess, h, cancel := newComboSession(300, runMode{storeScope: storeScope{kind: scopeSelected, roles: []string{entity.StoreTypeImage}}},
		checker, &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}}, &fakePluginExec{},
		nil, newRealReplacementService(stubs, nil))
	defer cancel()
	// 记忆冲突集为另一作品（集不一致）
	h.memo = &taskManager.ReplaceConfirmMemo{ConflictWorkIds: []int64{999}, Decision: taskManager.ReplaceDecisionSkip}
	res := entity.NewResource()
	res.ID = 700
	res.WorkID = 500
	stubs.res.resources = []*entity.Resource{res}

	sess.runSectionCombo()

	if h.confirmCalls != 1 {
		t.Fatalf("记忆不匹配应重新弹窗, 实际 %d 次", h.confirmCalls)
	}
	if h.skipped() {
		t.Fatal("记忆不匹配时预设整体答复为替换，不应跳过")
	}
}

// ==== 仅作品信息板块 ====

// TestCheckDuplicate_InfoOnlySkipsCheck 仅作品信息任务(无资源板块)不查重:
// 查重零调用、无弹窗，作品信息板块正常执行后经 Skip 空说明收口（非终态完成）
func TestCheckDuplicate_InfoOnlySkipsCheck(t *testing.T) {
	checker := &fakeDupChecker{result: fakeCheckResult(duplicate.DuplicateHitConflict, nil)}
	sess, h, cancel := newComboSession(300, runMode{workInfo: true, storeScope: storeScope{kind: scopeNone}},
		checker, &fakeSiteKeyResolver{keys: map[int64]string{1: "test-site"}}, &fakePluginExec{},
		&stubWorkInfoSaver{savedWorkId: 100}, nil)
	defer cancel()

	res := sess.runSectionCombo()

	if checker.calls != 0 {
		t.Fatalf("仅作品信息任务不应触发查重, 查重被调用 %d 次", checker.calls)
	}
	if h.confirmCalls != 0 {
		t.Fatalf("仅作品信息任务不应弹覆盖确认, 实际 %d 次", h.confirmCalls)
	}
	if !sess.pluginExec.(*fakePluginExec).createdWork {
		t.Fatal("作品信息板块应正常执行 CreateWorkInfo")
	}
	if sess.workId != 100 {
		t.Fatalf("作品信息保存后应定位新作品 workId=100, 实际 %d", sess.workId)
	}
	if res != comboFinished {
		t.Fatalf("非终态板块完成应收口完成, 实际 %d", res)
	}
	if !h.skipped() || h.lastSkipMsg() != "" {
		t.Fatalf("非终态板块完成应经 Skip 空说明收口, 实际 skipped=%v msg=%q", h.skipped(), h.lastSkipMsg())
	}
	if h.finished || h.failed {
		t.Fatal("非终态板块完成不应产生终态")
	}
}

// TestComboFail_InfoOnlyCarriesErrMsg 仅作品信息板块失败：Skip 收口携带错误说明
// （非终态失败对用户可见的唯一通道——回退态不落库）
func TestComboFail_InfoOnlyCarriesErrMsg(t *testing.T) {
	sess, h, cancel := newComboSession(300, runMode{workInfo: true, storeScope: storeScope{kind: scopeNone}},
		&fakeDupChecker{}, &fakeSiteKeyResolver{}, &fakePluginExec{createErr: fmt.Errorf("插件不可用")},
		nil, nil)
	defer cancel()

	res := sess.runSectionCombo()

	if res != comboFinished {
		t.Fatalf("非终态板块失败应收口完成, 实际 %d", res)
	}
	if !h.skipped() || h.lastSkipMsg() == "" {
		t.Fatalf("非终态板块失败应经 Skip 携带错误说明收口, 实际 skipped=%v msg=%q", h.skipped(), h.lastSkipMsg())
	}
	if h.finished || h.failed {
		t.Fatal("非终态板块失败不应产生终态")
	}
}
