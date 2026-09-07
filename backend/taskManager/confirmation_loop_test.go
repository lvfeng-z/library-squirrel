package taskManager

// 确认回路测试：策略任务执行内挂起等待（WaitReplaceConfirm）与终态回滚钩子
// （SetTerminalRollback）。覆盖：
// - 整体答复继续：Manager.ConfirmReplace 投递确认通道 → WaitReplaceConfirm 返回决策继续执行
// - 取消打断：RunCtx 取消（防御性，暂停/停止旁路）→ 返回 canceled、自确认表移除、槽位保持释放
// - 排队唤醒：确认挂起期间释放信号量槽位（多策略任务同时等待不挤占并发额度），答复后重新取槽
// - setFailed 单点触发登记钩子：受害者显式清单复活
// - 多次软删受害者合并登记、Finish 清空登记

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/shareLock"
	"github.com/library-squirrel/backend/task"

	"gorm.io/plugin/soft_delete"
)

// fakePusher 推送桩:捕获 PushDuplicateDetected 载荷
type fakePusher struct {
	duplicates []duplicateCall
}

type duplicateCall struct {
	taskId        int64
	existingId    int64
	conflictRoles []string
}

func (p *fakePusher) PushStateChange(int64, string, TaskState)       {}
func (p *fakePusher) PushParentStateChange(int64, string, TaskState) {}
func (p *fakePusher) PushProgress(int64, int64, int64)               {}
func (p *fakePusher) PushProgressBatch([]*taskScheduleDTO)           {}
func (p *fakePusher) PushParentProgress(int64, int64, int64)         {}
func (p *fakePusher) PushError(int64, string)                        {}
func (p *fakePusher) PushTaskRemove([]int64)                         {}
func (p *fakePusher) PushParentTaskRemove([]int64)                   {}
func (p *fakePusher) PushDuplicateDetected(taskId int64, taskName string, existingWorkId int64, existingWorkName string, conflictRoles []string) {
	p.duplicates = append(p.duplicates, duplicateCall{taskId: taskId, existingId: existingWorkId, conflictRoles: conflictRoles})
}

// stubStrategy 无操作执行面策略桩（确认回路测试直接调用 handle，不经 Execute）
type stubStrategy struct{}

func (s *stubStrategy) Execute(h StrategyHandle) {}

// confirmStrategy 执行面策略桩:Execute 内调 WaitReplaceConfirm,记录返回;非取消路径上报 Finish
type confirmStrategy struct {
	conflicts []ConflictInfo
	mu        sync.Mutex
	decision  ReplaceDecision
	canceled  bool
	execs     int
}

func newConfirmStrategy(conflicts []ConflictInfo) *confirmStrategy {
	return &confirmStrategy{conflicts: conflicts}
}

func (s *confirmStrategy) Execute(h StrategyHandle) {
	s.mu.Lock()
	s.execs++
	s.mu.Unlock()
	d, c := h.WaitReplaceConfirm(s.conflicts)
	s.mu.Lock()
	s.decision = d
	s.canceled = c
	s.mu.Unlock()
	if !c {
		h.Finish()
	}
}

func (s *confirmStrategy) outcome() (ReplaceDecision, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.decision, s.canceled
}

// ==== 终态回滚链 fakes（triggerTerminalRollback 注入真实 resource.ReplacementService，断言落桩上）====

// stubResourceReader 资源查询桩（回滚派生链取作品资源）
type stubResourceReader struct {
	resources []*entity.Resource
}

func (f *stubResourceReader) ListByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error) {
	return f.resources, nil
}

func (f *stubResourceReader) GetById(ctx context.Context, id int64) (*entity.Resource, error) {
	return nil, nil
}

// stubResourceStoreReader 关联查询桩
type stubResourceStoreReader struct {
	assocs []*entity.ResourceStore
}

func (f *stubResourceStoreReader) ListByResourceId(ctx context.Context, resourceId int64) ([]*entity.ResourceStore, error) {
	return f.assocs, nil
}

func (f *stubResourceStoreReader) ListByResourceIds(ctx context.Context, resourceIds []int64) ([]*entity.ResourceStore, error) {
	return f.assocs, nil
}

// stubStoreBackupReader store 行含删读取与复活桩
type stubStoreBackupReader struct {
	rows        []*entity.PersistentStore
	restoredIds []int64
}

func (f *stubStoreBackupReader) ListByIdsIncludeDeleted(ctx context.Context, ids []int64) []*entity.PersistentStore {
	want := make(map[int64]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	result := make([]*entity.PersistentStore, 0, len(f.rows))
	for _, row := range f.rows {
		if want[row.GetID()] {
			result = append(result, row)
		}
	}
	return result
}

func (f *stubStoreBackupReader) RestoreByIds(ctx context.Context, ids []int64) error {
	f.restoredIds = append(f.restoredIds, ids...)
	return nil
}

// stubStoreReplacer 替换软删原语桩
type stubStoreReplacer struct {
	backupIds    []int64
	discardedIds []int64
}

func (f *stubStoreReplacer) DeleteWithBackup(ctx context.Context, id int64) (int64, error) {
	f.backupIds = append(f.backupIds, id)
	return 0, nil
}

func (f *stubStoreReplacer) SoftDeleteAndDiscardFile(ctx context.Context, id int64) error {
	f.discardedIds = append(f.discardedIds, id)
	return nil
}

// stubWorkLivenessReader 作品活性桩：nil work = 已软删
type stubWorkLivenessReader struct {
	work *entity.Work
}

func (f *stubWorkLivenessReader) GetById(ctx context.Context, id int64) (*entity.Work, error) {
	return f.work, nil
}

// stubBackupFileRestorer 备份文件还原桩：记录还原与删除的清单行 ID
type stubBackupFileRestorer struct {
	restoredBackupIds []int64
	deletedBackupIds  []int64
}

func (f *stubBackupFileRestorer) GetById(ctx context.Context, id int64) (*entity.Backup, error) {
	b := entity.NewBackup()
	b.SetID(id)
	return b, nil
}

func (f *stubBackupFileRestorer) GetBackupPath(backup *entity.Backup) string { return "" }

func (f *stubBackupFileRestorer) RestoreFile(ctx context.Context, backupPath string, targetPath string) error {
	return nil
}

func (f *stubBackupFileRestorer) DeleteBackup(ctx context.Context, id int64) error {
	f.deletedBackupIds = append(f.deletedBackupIds, id)
	return nil
}

// stubResourceRecomputer 完整度重算记账桩
type stubResourceRecomputer struct {
	calledResourceIds []int64
}

func (f *stubResourceRecomputer) RecomputeResourceComplete(ctx context.Context, resourceId int64) {
	f.calledResourceIds = append(f.calledResourceIds, resourceId)
}

// stubAssocRemover 回滚摘除新建行关联记账桩
type stubAssocRemover struct {
	removedByStoreIds []int64
}

func (f *stubAssocRemover) DeleteByStoreIds(ctx context.Context, storeIds []int64) error {
	f.removedByStoreIds = append(f.removedByStoreIds, storeIds...)
	return nil
}

// stubRowHardDeleter 回滚物理删新建行记账桩
type stubRowHardDeleter struct {
	hardDeleted []int64
}

func (f *stubRowHardDeleter) HardDelete(ctx context.Context, id int64, backup bool) (int64, error) {
	f.hardDeleted = append(f.hardDeleted, id)
	return 0, nil
}

// stubWorkDirProvider 工作目录桩
type stubWorkDirProvider struct {
	dir string
}

func (p stubWorkDirProvider) GetWorkDir() string { return p.dir }

// rollbackStubs 终态回滚测试的桩集合
type rollbackStubs struct {
	res       *stubResourceReader
	rs        *stubResourceStoreReader
	rows      *stubStoreBackupReader
	replacer  *stubStoreReplacer
	liveness  *stubWorkLivenessReader
	restorer  *stubBackupFileRestorer
	recompute *stubResourceRecomputer
	assocRm   *stubAssocRemover
	rowDel    *stubRowHardDeleter
}

// newRollbackTestTask 构造终态回滚测试任务（不启动 actor）：替换链复活能力注入真实
// resource.ReplacementService（本文件 stub 作其依赖接口）——复活逻辑在 resource 域执行，
// 断言仍落到同一批桩上
func newRollbackTestTask() (*ManagedTask, *rollbackStubs) {
	m := newTestManagedTask()
	stubs := &rollbackStubs{
		res:       &stubResourceReader{},
		rs:        &stubResourceStoreReader{},
		rows:      &stubStoreBackupReader{},
		replacer:  &stubStoreReplacer{},
		liveness:  &stubWorkLivenessReader{work: entity.NewWork()},
		restorer:  &stubBackupFileRestorer{},
		recompute: &stubResourceRecomputer{},
		assocRm:   &stubAssocRemover{},
		rowDel:    &stubRowHardDeleter{},
	}
	m.deps = &TaskDeps{
		ReplaceStoreOps: resource.NewReplacementService(
			stubs.res, stubs.rs, stubs.rows, stubs.replacer,
			stubs.restorer, stubs.liveness, stubs.recompute,
			stubWorkDirProvider{dir: "E:/lib"},
			shareLock.NewShareLockRegistry(),
			stubs.assocRm,
			stubs.rowDel,
		),
	}
	return m, stubs
}

// makeRollbackStoreRow 造一行 persistent_store（供 ListByIdsIncludeDeleted 桩返回）
func makeRollbackStoreRow(id int64, completed int64, deletedAt int64, backupId int64, path string) *entity.PersistentStore {
	row := entity.NewPersistentStore()
	row.SetID(id)
	row.CompletedAt = completed
	if deletedAt > 0 {
		row.DeletedAt = soft_delete.DeletedAt(deletedAt)
	}
	row.BackupID = sql.NullInt64{Int64: backupId, Valid: true}
	row.FilePath = sql.NullString{String: path, Valid: true}
	return row
}

// newConfirmTestTask 构建确认回路测试任务（不启动 actor）：直接调 strategyHandle.WaitReplaceConfirm，
// 管理器仅提供确认表与信号量。strategy 恒非 nil，使 Manager.ConfirmReplace 投递确认通道
func newConfirmTestTask(maxParallel int) (*ManagedTask, *Manager, *fakePusher) {
	mgr := NewManager(maxParallel, nil, nil, nil, nil, nil, nil)
	pusher := &fakePusher{}
	// Manager 与任务共享同一 TaskDeps（对齐生产装配形态；确认面锁预检读 Manager 侧依赖）
	mgr.deps = &TaskDeps{Pusher: pusher, WorkLockChecker: shareLock.NewShareLockRegistry()}
	m := newTestManagedTask()
	m.task = newBuiltinTask(1, "demo")
	m.manager = mgr
	m.semaphore = mgr.semaphore
	m.confirmCh = make(chan replaceConfirmResult, 1)
	m.strategy = &stubStrategy{}
	m.deps = mgr.deps
	return m, mgr, pusher
}

// waitRegistered 轮询等待任务注册进等待确认表
func waitRegistered(t *testing.T, mgr *Manager, taskId int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mgr.waitingForInputMu.Lock()
		_, ok := mgr.waitingForInputMap[taskId]
		mgr.waitingForInputMu.Unlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("任务未注册进等待确认表")
}

// waitSlotReleased 轮询等待信号量槽位释放（确认挂起期间不占并发额度）
func waitSlotReleased(t *testing.T, m *ManagedTask) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(m.semaphore) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("确认挂起期间应释放信号量槽位")
}

// TestStrategyWaitConfirmOverallAnswer 整体答复继续：逐条推送冲突事件（同 taskId、不同
// existingWorkId）、置 WaitingForInput、释放槽位；Manager.ConfirmReplace 分流投递确认通道 →
// 返回整体决策 replace、回到 Processing、重新取槽
func TestStrategyWaitConfirmOverallAnswer(t *testing.T) {
	m, mgr, pusher := newConfirmTestTask(2)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	// 模拟 handleRunCmd 已取槽后进入 Execute
	m.slotHeld = true
	m.semaphore <- struct{}{}
	h := newStrategyHandle(m)
	conflicts := []ConflictInfo{
		{WorkID: 100, WorkName: "作品A", ConflictRoles: []string{"image", "video"}},
		{WorkID: 200, WorkName: "作品B", ConflictRoles: []string{"thumbnail"}},
	}

	var decision ReplaceDecision
	var canceled bool
	done := make(chan struct{})
	go func() {
		decision, canceled = h.WaitReplaceConfirm(conflicts)
		close(done)
	}()

	waitRegistered(t, mgr, 1)
	waitSlotReleased(t, m)

	if err := mgr.ConfirmReplace(1, "replace"); err != nil {
		t.Fatalf("确认失败: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("WaitReplaceConfirm 未返回")
	}
	if canceled {
		t.Fatal("整体答复不应视为取消")
	}
	if decision != ReplaceDecisionReplace {
		t.Fatalf("决策应为替换,实际 %v", decision)
	}
	// 答复后回到 Processing 并重新取槽
	if m.GetState() != TaskStateProcessing {
		t.Fatalf("答复后应回到 Processing,实际 %s", taskStateName(m.GetState()))
	}
	if !m.slotHeld {
		t.Fatal("答复后应重新持有信号量槽位")
	}
	// 逐条推送冲突事件（同 taskId、不同 existingWorkId）
	if len(pusher.duplicates) != len(conflicts) {
		t.Fatalf("应逐条推送 %d 条冲突事件,实际 %d", len(conflicts), len(pusher.duplicates))
	}
	if pusher.duplicates[0].existingId != 100 || pusher.duplicates[1].existingId != 200 {
		t.Fatalf("冲突事件载荷应为各自 existingWorkId,实际 %+v", pusher.duplicates)
	}
	// 答复后已自确认表移除（ConfirmReplace 提取时删除）
	if _, ok := mgr.waitingForInputMap[1]; ok {
		t.Fatal("答复后应从等待确认表移除")
	}
}

// TestStrategyWaitConfirmCanceled 取消打断：RunCtx 取消（防御性分支）→ 返回 canceled、
// 自确认表移除、槽位保持释放（交回控制面，由 handleRunCmd 释放路径按 slotHeld 守卫防重复释放）
func TestStrategyWaitConfirmCanceled(t *testing.T) {
	m, mgr, _ := newConfirmTestTask(2)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	m.slotHeld = true
	m.semaphore <- struct{}{}
	h := newStrategyHandle(m)

	var canceled bool
	done := make(chan struct{})
	go func() {
		_, canceled = h.WaitReplaceConfirm([]ConflictInfo{{WorkID: 100, WorkName: "作品A"}})
		close(done)
	}()
	waitRegistered(t, mgr, 1)

	// 模拟 watcher 对暂停/停止的 runCancel
	m.runCancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("WaitReplaceConfirm 未返回")
	}
	if !canceled {
		t.Fatal("RunCtx 取消应返回取消信号")
	}
	if _, ok := mgr.waitingForInputMap[1]; ok {
		t.Fatal("取消后应从等待确认表移除(防后续确认命令误投)")
	}
	if m.slotHeld {
		t.Fatal("取消路径不应重新取槽")
	}
	if len(m.semaphore) != 0 {
		t.Fatal("取消路径槽位应保持释放")
	}
	if h.ConfirmMemo() != nil {
		t.Fatal("取消时无答复不应记录确认记忆")
	}
}

// TestStrategyWaitConfirmSlotReleased 排队唤醒：maxParallel=1 下策略任务持有唯一槽位,
// 挂起等待时释放(其他任务可取槽),整体答复后重新排队取槽
func TestStrategyWaitConfirmSlotReleased(t *testing.T) {
	m, mgr, _ := newConfirmTestTask(1)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	m.slotHeld = true
	m.semaphore <- struct{}{}
	h := newStrategyHandle(m)

	var decision ReplaceDecision
	done := make(chan struct{})
	go func() {
		decision, _ = h.WaitReplaceConfirm([]ConflictInfo{{WorkID: 100}})
		close(done)
	}()
	waitRegistered(t, mgr, 1)
	waitSlotReleased(t, m)

	// 挂起等待期间唯一槽位被释放：其他任务可取槽（多等待任务不挤占并发额度）
	select {
	case mgr.semaphore <- struct{}{}:
		<-mgr.semaphore // 取到即证明已释放,归还供等待任务重新取用
	case <-time.After(3 * time.Second):
		t.Fatal("等待期间槽位应被释放(其他任务可取)")
	}

	if err := mgr.ConfirmReplace(1, "replace"); err != nil {
		t.Fatalf("确认失败: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("WaitReplaceConfirm 未返回")
	}
	if decision != ReplaceDecisionReplace {
		t.Fatalf("决策应为替换,实际 %v", decision)
	}
	if !m.slotHeld {
		t.Fatal("答复后应重新持有信号量槽位")
	}
	if len(m.semaphore) != 1 {
		t.Fatalf("答复后应重新取回 1 个槽位,实际 %d", len(m.semaphore))
	}
}

// TestConfirmReplaceBatchRoutesStrategy 批量确认对策略任务同样分流确认通道（skip → 跳过决策）
func TestConfirmReplaceBatchRoutesStrategy(t *testing.T) {
	m, mgr, _ := newConfirmTestTask(2)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	m.slotHeld = true
	m.semaphore <- struct{}{}
	h := newStrategyHandle(m)

	var decision ReplaceDecision
	done := make(chan struct{})
	go func() {
		decision, _ = h.WaitReplaceConfirm([]ConflictInfo{{WorkID: 100}})
		close(done)
	}()
	waitRegistered(t, mgr, 1)

	mgr.ConfirmReplaceBatch([]int64{1}, "skip")
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("WaitReplaceConfirm 未返回")
	}
	if decision != ReplaceDecisionSkip {
		t.Fatalf("批量确认 skip 应返回跳过决策,实际 %v", decision)
	}
}

// TestSetFailedTriggersRegisteredStrategyRollback setFailed 单点触发登记钩子：
// 软删成功后 SetTerminalRollback 登记的受害者显式清单经单点复活（备份还原+清备份+复活+完整度重算）；
// 一次性：触发后清空登记
func TestSetFailedTriggersRegisteredStrategyRollback(t *testing.T) {
	m, stubs := newRollbackTestTask()
	m.strategy = &stubStrategy{}
	h := newStrategyHandle(m)

	h.SetTerminalRollback(TerminalRollback{Victims: []resource.StoreRef{
		{StoreID: 811, ResourceID: 700, BackupID: 92, FilePath: "store/resource/a/被替换.png"},
	}})
	stubs.rows.rows = []*entity.PersistentStore{
		makeRollbackStoreRow(811, 1, 2000, 92, "store/resource/a/被替换.png"),
	}

	m.setFailed("任务被用户停止")

	if len(stubs.rows.restoredIds) != 1 || stubs.rows.restoredIds[0] != 811 {
		t.Fatalf("setFailed 应触发登记钩子复活 victim(811),实际 %v", stubs.rows.restoredIds)
	}
	if len(stubs.restorer.deletedBackupIds) != 1 || stubs.restorer.deletedBackupIds[0] != 92 {
		t.Fatalf("victim 备份(92)应被清理,实际 %v", stubs.restorer.deletedBackupIds)
	}
	if len(stubs.recompute.calledResourceIds) != 1 || stubs.recompute.calledResourceIds[0] != 700 {
		t.Fatalf("回滚后应重算 victim 所属资源(700)完整度,实际 %v", stubs.recompute.calledResourceIds)
	}
	if m.terminalRollback != nil {
		t.Fatal("终态回滚触发后应清空登记")
	}
	if m.GetState() != TaskStateFailed {
		t.Fatalf("setFailed 后任务应为 Failed,实际 %d", m.GetState())
	}
}

// TestTerminalRollbackMergeAndFinishClear 多次软删受害者合并去重登记；新建行清单跨执行轮次
// （两段会话）并集去重登记；Finish 清空登记（替换完成软删行进入终态,重试从空态重新登记）
func TestTerminalRollbackMergeAndFinishClear(t *testing.T) {
	m, stubs := newRollbackTestTask()
	m.strategy = &stubStrategy{}
	h := newStrategyHandle(m)

	h.SetTerminalRollback(TerminalRollback{Victims: []resource.StoreRef{{StoreID: 1, ResourceID: 10}}})
	h.SetTerminalRollback(TerminalRollback{Victims: []resource.StoreRef{
		{StoreID: 1, ResourceID: 10},
		{StoreID: 2, ResourceID: 20},
	}})
	if m.terminalRollback == nil || len(m.terminalRollback.Victims) != 2 {
		t.Fatalf("多次软删受害者应合并去重,实际 %+v", m.terminalRollback)
	}
	if m.terminalRollback.Victims[0].StoreID != 1 || m.terminalRollback.Victims[1].StoreID != 2 {
		t.Fatalf("合并清单应为去重后条目,实际 %+v", m.terminalRollback.Victims)
	}

	// 两段执行会话的新建行登记并集：首段登记 [5]，恢复段对续接行 5 重登记并新增重建行 6
	h.SetTerminalRollback(TerminalRollback{CreatedStoreIDs: []int64{5}})
	h.SetTerminalRollback(TerminalRollback{CreatedStoreIDs: []int64{5, 6}})
	if len(m.terminalRollback.CreatedStoreIDs) != 2 || m.terminalRollback.CreatedStoreIDs[0] != 5 || m.terminalRollback.CreatedStoreIDs[1] != 6 {
		t.Fatalf("跨会话新建行应并集去重登记,实际 %v", m.terminalRollback.CreatedStoreIDs)
	}

	h.Finish()
	if m.terminalRollback != nil {
		t.Fatal("Finish 后应清空终态回滚登记")
	}
	// 成功终态不触发回滚：受害者不复活、新建行不丢弃（重试从空态重新登记）
	if len(stubs.rows.restoredIds) != 0 || len(stubs.rowDel.hardDeleted) != 0 {
		t.Fatalf("Finish 不应触发回滚（受害者不复活、新建行保留），实际 复活 %v 物理删 %v",
			stubs.rows.restoredIds, stubs.rowDel.hardDeleted)
	}
}

// TestSetFailedDiscardsCreatedStoresBeforeRevive 停止/失败经 setFailed 单点回滚：先按登记清单
// 丢弃替换执行期新建行（摘关联+物理删），再复活受害者——新建行清理由控制面统一执行，
// 覆盖停止于替换执行中（不经会话收口）的路径
func TestSetFailedDiscardsCreatedStoresBeforeRevive(t *testing.T) {
	m, stubs := newRollbackTestTask()
	m.strategy = &stubStrategy{}
	h := newStrategyHandle(m)

	h.SetTerminalRollback(TerminalRollback{CreatedStoreIDs: []int64{901, 902}})
	h.SetTerminalRollback(TerminalRollback{Victims: []resource.StoreRef{
		{StoreID: 811, ResourceID: 700, BackupID: 92, FilePath: "store/resource/a/被替换.png"},
	}})
	stubs.rows.rows = []*entity.PersistentStore{
		makeRollbackStoreRow(811, 1, 2000, 92, "store/resource/a/被替换.png"),
	}

	m.setFailed("任务被用户停止")

	if len(stubs.assocRm.removedByStoreIds) != 2 ||
		stubs.assocRm.removedByStoreIds[0] != 901 || stubs.assocRm.removedByStoreIds[1] != 902 {
		t.Fatalf("新建行(901,902)关联应被摘除,实际 %v", stubs.assocRm.removedByStoreIds)
	}
	if len(stubs.rowDel.hardDeleted) != 2 || stubs.rowDel.hardDeleted[0] != 901 || stubs.rowDel.hardDeleted[1] != 902 {
		t.Fatalf("新建行(901,902)应被物理删,实际 %v", stubs.rowDel.hardDeleted)
	}
	if len(stubs.rows.restoredIds) != 1 || stubs.rows.restoredIds[0] != 811 {
		t.Fatalf("受害者(811)应复活,实际 %v", stubs.rows.restoredIds)
	}
	if m.terminalRollback != nil {
		t.Fatal("终态回滚触发后应清空登记")
	}
}

// TestHandleStopCmdTriggersRegisteredRollback 停止命令路由：替换执行被 runCancel 中断（执行面
// 不上报终态返回）后，watcher 暂存的 stop 命令经 handleStopCmd → setFailed 单点触发回滚——
// 新建行丢弃与受害者复活都在此路径完成
func TestHandleStopCmdTriggersRegisteredRollback(t *testing.T) {
	m, stubs := newRollbackTestTask()
	m.strategy = &stubStrategy{}
	h := newStrategyHandle(m)
	h.SetTerminalRollback(TerminalRollback{
		Victims:         []resource.StoreRef{{StoreID: 811, ResourceID: 700}},
		CreatedStoreIDs: []int64{901},
	})
	stubs.rows.rows = []*entity.PersistentStore{
		makeRollbackStoreRow(811, 1, 2000, 0, "store/resource/a/被替换.png"),
	}

	m.handleStopCmd(taskCmd{kind: cmdStop})

	if m.GetState() != TaskStateFailed {
		t.Fatalf("停止后任务应为 Failed,实际 %s", taskStateName(m.GetState()))
	}
	if len(stubs.rowDel.hardDeleted) != 1 || stubs.rowDel.hardDeleted[0] != 901 {
		t.Fatalf("停止路径应丢弃登记的新建行(901),实际 %v", stubs.rowDel.hardDeleted)
	}
	if len(stubs.rows.restoredIds) != 1 || stubs.rows.restoredIds[0] != 811 {
		t.Fatalf("停止路径应复活受害者(811),实际 %v", stubs.rows.restoredIds)
	}
}

// TestSetFailedNoVictimsKeepsCreatedStores 非替换失败（无软删受害者）保留已下载成果：
// 登记的新建行清单随回滚登记作废，不触发丢弃与复活
func TestSetFailedNoVictimsKeepsCreatedStores(t *testing.T) {
	m, stubs := newRollbackTestTask()
	m.strategy = &stubStrategy{}
	h := newStrategyHandle(m)
	h.SetTerminalRollback(TerminalRollback{CreatedStoreIDs: []int64{901, 902}})

	m.setFailed("下载失败")

	if len(stubs.rowDel.hardDeleted) != 0 || len(stubs.assocRm.removedByStoreIds) != 0 {
		t.Fatalf("非替换失败不应丢弃新建行,实际 物理删 %v 摘关联 %v", stubs.rowDel.hardDeleted, stubs.assocRm.removedByStoreIds)
	}
	if len(stubs.rows.restoredIds) != 0 {
		t.Fatalf("无受害者即无复活对象,实际 %v", stubs.rows.restoredIds)
	}
	if m.terminalRollback != nil {
		t.Fatal("终态回滚触发后应清空登记")
	}
}

// TestStrategyConfirmThroughManager 控制面集成：策略经 Manager 全链——启动取槽 → Execute 内
// WaitReplaceConfirm（释放槽位）→ ConfirmReplace 投递 → 整体答复后重新取槽继续 → Finish 终态落盘
func TestStrategyConfirmThroughManager(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strategy := newConfirmStrategy([]ConflictInfo{{WorkID: 100, WorkName: "作品A", ConflictRoles: []string{"image"}}})
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher(), WorkLockChecker: shareLock.NewShareLockRegistry()},
		map[string]ExecutionStrategy{"demo": strategy}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	waitState(t, mgr, 1, TaskStateWaitingForInput, 3*time.Second)

	if err := mgr.ConfirmReplace(1, "replace"); err != nil {
		t.Fatalf("确认失败: %v", err)
	}
	// 任务 Finish → 终态即时落盘 → 清理移除 taskMap（waitState(Finished) 与清理竞态,
	// 用 waitIdle + 落盘断言替代,对齐 TestBuiltinTaskFinishLifecycle 模式）
	waitIdle(t, mgr, 3*time.Second)

	if d, c := strategy.outcome(); d != ReplaceDecisionReplace || c {
		t.Fatalf("策略应收到替换决策且未取消,实际 decision=%v canceled=%v", d, c)
	}
	if u, ok := repo.statusOf(1); !ok || u.Status != task.TaskStatusFinished {
		t.Fatalf("终态应即时落盘: %+v ok=%v", u, ok)
	}
}

// TestStrategyPauseDuringConfirmWait 确认挂起中暂停（防御性取消路径）：
// Pause 经 watcher runCancel → WaitReplaceConfirm 返回取消 → 不上报终态 → Paused，
// 槽位保持释放（releaseSlot 守卫防重复释放,不死锁）
func TestStrategyPauseDuringConfirmWait(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strategy := newConfirmStrategy([]ConflictInfo{{WorkID: 100}})
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher(), WorkLockChecker: shareLock.NewShareLockRegistry()},
		map[string]ExecutionStrategy{"demo": strategy}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	waitState(t, mgr, 1, TaskStateWaitingForInput, 3*time.Second)

	if err := mgr.PauseTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("暂停失败: %v", err)
	}
	waitState(t, mgr, 1, TaskStatePaused, 3*time.Second)

	if _, c := strategy.outcome(); !c {
		t.Fatal("暂停应令确认等待返回取消")
	}
	// 槽位释放后未被重复释放（若重复释放会阻塞在 <-m.semaphore 上死锁）
	if len(mgr.semaphore) != 0 {
		t.Fatalf("取消路径槽位应全部释放,实际 len=%d", len(mgr.semaphore))
	}
	// 暂停后自确认表移除
	if _, ok := mgr.waitingForInputMap[1]; ok {
		t.Fatal("暂停后应从等待确认表移除")
	}
}

// TestStrategyMultipleWaitingNotHogConcurrency 多策略任务同时等待确认不挤占并发额度：
// maxParallel=2 下两个任务均进入确认挂起 → 两个槽位全空;逐条答复后各自完成
func TestStrategyMultipleWaitingNotHogConcurrency(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"), newBuiltinTask(2, "demo"))
	s1 := newConfirmStrategy([]ConflictInfo{{WorkID: 100}})
	s2 := newConfirmStrategy([]ConflictInfo{{WorkID: 200}})
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher(), WorkLockChecker: shareLock.NewShareLockRegistry()},
		map[string]ExecutionStrategy{"demo": &multiStrategy{s1: s1, s2: s2}}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1, 2}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	waitState(t, mgr, 1, TaskStateWaitingForInput, 3*time.Second)
	waitState(t, mgr, 2, TaskStateWaitingForInput, 3*time.Second)

	// 两个等待任务均释放槽位:并发额度不被等待任务挤占
	if len(mgr.semaphore) != 0 {
		t.Fatalf("两个等待任务应释放全部槽位,实际 len=%d", len(mgr.semaphore))
	}

	_ = mgr.ConfirmReplace(1, "replace")
	_ = mgr.ConfirmReplace(2, "replace")
	waitIdle(t, mgr, 3*time.Second)
	for _, id := range []int64{1, 2} {
		if u, ok := repo.statusOf(id); !ok || u.Status != task.TaskStatusFinished {
			t.Fatalf("任务 %d 终态应落盘: %+v ok=%v", id, u, ok)
		}
	}
}

// multiStrategy 按 taskId 分派不同 confirmStrategy（fakeBuiltinRepo 对任意查询返回全部任务,
// 单一策略实例不适用多任务集成场景）
type multiStrategy struct {
	s1 *confirmStrategy
	s2 *confirmStrategy
}

func (s *multiStrategy) Execute(h StrategyHandle) {
	if h.Task().GetID() == 1 {
		s.s1.Execute(h)
		return
	}
	s.s2.Execute(h)
}

// sameWorkIDSet 判定两个作品 ID 集合相等（顺序无关）
func sameWorkIDSet(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[int64]struct{}, len(a))
	for _, id := range a {
		seen[id] = struct{}{}
	}
	for _, id := range b {
		if _, ok := seen[id]; !ok {
			return false
		}
	}
	return true
}

// TestStrategyWaitConfirmRecordsMemoOnReply 正常答复路径记录记忆：返回后 ConfirmMemo 非 nil，
// ConflictWorkIds 为 conflictWorkIds 保序去重输出（同作品多冲突行去重）、Decision 与输入答复一致
func TestStrategyWaitConfirmRecordsMemoOnReply(t *testing.T) {
	m, mgr, _ := newConfirmTestTask(2)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	m.slotHeld = true
	m.semaphore <- struct{}{}
	h := newStrategyHandle(m)
	conflicts := []ConflictInfo{
		{WorkID: 100, WorkName: "作品A", ConflictRoles: []string{"image"}},
		{WorkID: 100, WorkName: "作品A", ConflictRoles: []string{"video"}}, // 同作品多冲突行:记忆键去重
		{WorkID: 200, WorkName: "作品B", ConflictRoles: []string{"thumbnail"}},
	}

	var decision ReplaceDecision
	var canceled bool
	done := make(chan struct{})
	go func() {
		decision, canceled = h.WaitReplaceConfirm(conflicts)
		close(done)
	}()
	waitRegistered(t, mgr, 1)
	waitSlotReleased(t, m)

	if err := mgr.ConfirmReplace(1, "replace"); err != nil {
		t.Fatalf("确认失败: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("WaitReplaceConfirm 未返回")
	}
	if canceled {
		t.Fatal("正常答复不应视为取消")
	}
	if decision != ReplaceDecisionReplace {
		t.Fatalf("决策应为替换,实际 %v", decision)
	}
	memo := h.ConfirmMemo()
	if memo == nil {
		t.Fatal("正常答复后应记录确认记忆")
	}
	if memo.Decision != ReplaceDecisionReplace {
		t.Fatalf("记忆决策应为替换,实际 %v", memo.Decision)
	}
	if len(memo.ConflictWorkIds) != 2 || !sameWorkIDSet(memo.ConflictWorkIds, []int64{100, 200}) {
		t.Fatalf("记忆冲突集应为保序去重 [100 200],实际 %v", memo.ConflictWorkIds)
	}
}

// TestStrategyWaitConfirmRecordsMemoOnInnerCancel 内层取消记录记忆：答复投递后重新取槽被
// runCtx 取消打断 → 返回 (决策, true) 且记忆已记录（答复已被 confirmCh 消费,仅取槽被打断）
func TestStrategyWaitConfirmRecordsMemoOnInnerCancel(t *testing.T) {
	m, mgr, _ := newConfirmTestTask(1)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	m.slotHeld = true
	m.semaphore <- struct{}{}
	h := newStrategyHandle(m)
	conflicts := []ConflictInfo{{WorkID: 100, WorkName: "作品A"}}

	var decision ReplaceDecision
	var canceled bool
	done := make(chan struct{})
	go func() {
		decision, canceled = h.WaitReplaceConfirm(conflicts)
		close(done)
	}()
	waitRegistered(t, mgr, 1)
	waitSlotReleased(t, m)

	// 外部任务取走释放的槽位:答复后重新取槽将阻塞(内层取消的前提)
	mgr.semaphore <- struct{}{}
	if err := mgr.ConfirmReplace(1, "replace"); err != nil {
		t.Fatalf("确认失败: %v", err)
	}
	// 轮询到记忆已记录:答复已被 confirmCh 消费、goroutine 阻塞在重新取槽,随后 runCancel 打断取槽
	deadline := time.Now().Add(3 * time.Second)
	for h.ConfirmMemo() == nil {
		if time.Now().After(deadline) {
			t.Fatal("答复后应记录确认记忆")
		}
		time.Sleep(5 * time.Millisecond)
	}
	m.runCancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("WaitReplaceConfirm 未返回")
	}
	if !canceled {
		t.Fatal("内层取消应返回取消信号")
	}
	if decision != ReplaceDecisionReplace {
		t.Fatalf("内层取消应返回已消费答复的决策,实际 %v", decision)
	}
	memo := h.ConfirmMemo()
	if memo == nil || memo.Decision != ReplaceDecisionReplace || !sameWorkIDSet(memo.ConflictWorkIds, []int64{100}) {
		t.Fatalf("内层取消后记忆应记录答复决策,实际 %+v", memo)
	}
	if _, ok := mgr.waitingForInputMap[1]; ok {
		t.Fatal("取消后应从等待确认表移除")
	}
}

// TestStrategyWaitConfirmConsumesRacedReply 外层取消竞态消费答复：取消时 confirmCh 已有答复
// （用户答复与暂停竞态）→ 答复被消费记入记忆。答复先于取消入通道（缓冲投递同步完成），
// 无论 select 选中答复路径还是取消路径,竞态答复都不白点
func TestStrategyWaitConfirmConsumesRacedReply(t *testing.T) {
	m, mgr, _ := newConfirmTestTask(2)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	m.slotHeld = true
	m.semaphore <- struct{}{}
	h := newStrategyHandle(m)

	done := make(chan struct{})
	go func() {
		h.WaitReplaceConfirm([]ConflictInfo{{WorkID: 100, WorkName: "作品A"}})
		close(done)
	}()
	waitRegistered(t, mgr, 1)
	waitSlotReleased(t, m)

	// 答复投递（同步入缓冲通道）后立即取消:竞态窗口——答复在通道中、runCtx 同时取消
	if err := mgr.ConfirmReplace(1, "replace"); err != nil {
		t.Fatalf("确认失败: %v", err)
	}
	m.runCancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("WaitReplaceConfirm 未返回")
	}
	memo := h.ConfirmMemo()
	if memo == nil {
		t.Fatal("竞态答复应被消费记入确认记忆")
	}
	if memo.Decision != ReplaceDecisionReplace {
		t.Fatalf("记忆决策应来自竞态答复(replace),实际 %v", memo.Decision)
	}
	if !sameWorkIDSet(memo.ConflictWorkIds, []int64{100}) {
		t.Fatalf("记忆冲突集应为 [100],实际 %v", memo.ConflictWorkIds)
	}
}

// TestStrategyWaitConfirmCancelNoMemo 取消时无答复 → 记忆保持 nil（用户确实未答,恢复重新弹窗）
func TestStrategyWaitConfirmCancelNoMemo(t *testing.T) {
	m, mgr, _ := newConfirmTestTask(2)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	m.slotHeld = true
	m.semaphore <- struct{}{}
	h := newStrategyHandle(m)

	done := make(chan struct{})
	go func() {
		h.WaitReplaceConfirm([]ConflictInfo{{WorkID: 100, WorkName: "作品A"}})
		close(done)
	}()
	waitRegistered(t, mgr, 1)
	waitSlotReleased(t, m)

	m.runCancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("WaitReplaceConfirm 未返回")
	}
	if h.ConfirmMemo() != nil {
		t.Fatal("取消时无答复不应记录确认记忆")
	}
}
