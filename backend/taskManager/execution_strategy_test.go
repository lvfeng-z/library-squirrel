package taskManager

// 执行面策略路径测试：策略解析、控制面集成（启动/暂停/停止/终态落盘）、
// 执行器违约防御、恢复信号置位与重下载两步编排。

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/task"
)

// fakeBuiltinRepo 策略路径测试仓储桩：仅 ListTaskTreeCore 有值，其余记录调用
type fakeBuiltinRepo struct {
	mu       sync.Mutex
	tasks    []*domain.Task
	statuses map[int64]task.StatusUpdate
}

func newFakeBuiltinRepo(tasks ...*domain.Task) *fakeBuiltinRepo {
	return &fakeBuiltinRepo{tasks: tasks, statuses: make(map[int64]task.StatusUpdate)}
}

func (r *fakeBuiltinRepo) ListTaskTreeCore(ctx context.Context, taskIds []int64, includeStatus ...task.TaskStatusEnum) ([]*domain.Task, error) {
	return r.tasks, nil
}

func (r *fakeBuiltinRepo) SetTaskTreeStatus(ctx context.Context, taskIds []int64, status task.TaskStatusEnum, includeStatus ...task.TaskStatusEnum) (int64, error) {
	return 0, nil
}

func (r *fakeBuiltinRepo) BatchSetStatus(ctx context.Context, statuses map[int64]task.StatusUpdate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, u := range statuses {
		r.statuses[id] = u
	}
	return nil
}

func (r *fakeBuiltinRepo) statusOf(id int64) (task.StatusUpdate, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.statuses[id]
	return u, ok
}

func (r *fakeBuiltinRepo) ListBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) ([]*domain.Task, error) {
	return nil, nil
}

// recordSectionRecorder 板块选择写行桩（重下载两步编排测试用）
type recordSectionRecorder struct {
	mu              sync.Mutex
	taskIds         []int64
	storeRoles      sql.NullString
	includeWorkInfo bool
}

func (r *recordSectionRecorder) RecordSections(ctx context.Context, taskIds []int64, storeRoles sql.NullString, includeWorkInfo bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.taskIds = append(r.taskIds, taskIds...)
	r.storeRoles = storeRoles
	r.includeWorkInfo = includeWorkInfo
	return nil
}

// scriptedStrategy 脚本化执行面策略：每次 Execute 通知 entered 并阻塞至 release；
// release 后按 mode 决定上报（finish → Finish / fail → Fail / interrupt → 不上报）
type scriptedStrategy struct {
	mu      sync.Mutex
	execs   int
	handles []StrategyHandle
	entered chan struct{} // 每次 Execute 进入（缓冲，多次执行累计可收）
	release chan struct{} // 释放信号（缓冲 1，测试逐次投递）
	mode    string        // finish | fail | interrupt
}

func newScriptedStrategy(mode string) *scriptedStrategy {
	return &scriptedStrategy{entered: make(chan struct{}, 4), release: make(chan struct{}, 4), mode: mode}
}

func (s *scriptedStrategy) Execute(h StrategyHandle) {
	s.mu.Lock()
	s.execs++
	s.handles = append(s.handles, h)
	s.mu.Unlock()
	s.entered <- struct{}{}
	<-s.release
	switch s.mode {
	case "finish":
		h.Finish()
	case "fail":
		h.Fail("执行失败样例")
	default:
		// interrupt：不上报终态（runCtx 取消场景）
	}
}

func (s *scriptedStrategy) execCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.execs
}

func (s *scriptedStrategy) lastHandle() StrategyHandle {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.handles) == 0 {
		return nil
	}
	return s.handles[len(s.handles)-1]
}

// allHandles 返回全部执行轮次句柄的快照
func (s *scriptedStrategy) allHandles() []StrategyHandle {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]StrategyHandle(nil), s.handles...)
}

// newBuiltinTask 构造策略类型任务实体（Created、根级）
func newBuiltinTask(id int64, taskType string) *domain.Task {
	t := domain.NewTask()
	t.SetID(id)
	t.TaskType = sql.NullString{String: taskType, Valid: true}
	t.TaskName = sql.NullString{String: "策略任务样例", Valid: true}
	t.Status = int(TaskStateCreated)
	return t
}

// waitState 轮询等待任务到达指定状态（内存命中路径）
func waitState(t *testing.T, mgr *Manager, taskId int64, want TaskState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if st, err := mgr.GetTaskState(taskId); err == nil && st == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等待任务 %d 进入 %s 超时", taskId, taskStateName(want))
}

// waitIdle 轮询等待管理器空闲（全部任务终态清理完毕；终态即时落盘先于清理，落盘断言随后稳定）
func waitIdle(t *testing.T, mgr *Manager, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if mgr.IsIdle() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("等待任务终态清理超时")
}

// TestBuiltinTaskFinishLifecycle 策略任务经 Manager 全链执行：启动 → 策略 Execute →
// Finish 终态即时落盘 → taskMap 清理
func TestBuiltinTaskFinishLifecycle(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := newScriptedStrategy("finish")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	// 策略被调度，handle 携带任务实体
	select {
	case <-strat.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("策略未被调度")
	}
	h := strat.lastHandle()
	if h == nil || h.Task() == nil || h.Task().GetID() != 1 {
		t.Fatal("handle 应携带任务实体")
	}
	// 进度上报经控制面链路（不 panic 即通过，无状态断言）
	h.ReportProgress(3, 1)

	strat.release <- struct{}{}
	waitIdle(t, mgr, 3*time.Second)
	if u, ok := repo.statusOf(1); !ok || u.Status != task.TaskStatusFinished {
		t.Fatalf("终态应即时落盘: %+v ok=%v", u, ok)
	}
}

// TestRedownloadTwoStepOrchestration 重下载两步编排（handler 层）：板块选择先写行（三态落库：
// 子集=所选、空集=仅作品信息）→ 整树启动执行；失败终态不回写板块选择（板块模式唯一源=领域行，
// 控制面无清空通道，重试按原板块再来一次）
func TestRedownloadTwoStepOrchestration(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := newScriptedStrategy("fail")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()
	recorder := &recordSectionRecorder{}
	h := NewHandler(mgr, recorder)

	if resp := h.Redownload(context.Background(), []int64{1}, []string{domain.StoreTypeThumbnail}, false); !resp.Success {
		t.Fatalf("重下载两步编排应成功, 实际 %+v", resp)
	}
	<-strat.entered
	strat.release <- struct{}{}
	waitIdle(t, mgr, 3*time.Second)

	// 第一步：板块选择已按三态写行（子集=所选、includeWorkInfo 透传）
	if len(recorder.taskIds) != 1 || recorder.taskIds[0] != 1 {
		t.Fatalf("板块选择应对请求任务写行, 实际 %v", recorder.taskIds)
	}
	if !recorder.storeRoles.Valid || recorder.storeRoles.String != domain.StoreTypeThumbnail || recorder.includeWorkInfo {
		t.Fatalf("板块选择应记录 thumbnail 子集(不含作品信息), 实际 %+v", recorder.storeRoles)
	}
	// 第二步：整树已启动并失败落盘（板块选择不受影响——控制面无清空通道）
	if u, ok := repo.statusOf(1); !ok || u.Status != task.TaskStatusFailed {
		t.Fatalf("任务应已失败落盘: %+v ok=%v", u, ok)
	}
}

// TestRedownloadInfoOnlyRecordsEmptyScope 仅作品信息重下载：空资源集写行为空串（三态 None）
func TestRedownloadInfoOnlyRecordsEmptyScope(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := newScriptedStrategy("finish")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()
	recorder := &recordSectionRecorder{}
	h := NewHandler(mgr, recorder)

	if resp := h.Redownload(context.Background(), []int64{1}, nil, true); !resp.Success {
		t.Fatalf("仅作品信息重下载应成功, 实际 %+v", resp)
	}
	<-strat.entered
	strat.release <- struct{}{}
	waitIdle(t, mgr, 3*time.Second)

	if !recorder.storeRoles.Valid || recorder.storeRoles.String != "" || !recorder.includeWorkInfo {
		t.Fatalf("空资源集应写行为仅作品信息(None+workInfo), 实际 %+v includeWorkInfo=%v", recorder.storeRoles, recorder.includeWorkInfo)
	}
	if u, ok := repo.statusOf(1); !ok || u.Status != task.TaskStatusFinished {
		t.Fatalf("任务应已完成落盘: %+v ok=%v", u, ok)
	}
}

// TestStartTwoStepOrchestration 开始入口两步编排（handler 层）：首跑板块全量写行
// （store_roles 置 NULL=全量、include_work_info=true——创建默认不含作品信息，不写行则执行面
// 派生出不含作品信息的板块组合）→ 整树启动执行
func TestStartTwoStepOrchestration(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := newScriptedStrategy("finish")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()
	recorder := &recordSectionRecorder{}
	h := NewHandler(mgr, recorder)

	if resp := h.StartTaskTrees(context.Background(), []int64{1}); !resp.Success {
		t.Fatalf("开始两步编排应成功, 实际 %+v", resp)
	}
	<-strat.entered
	strat.release <- struct{}{}
	waitIdle(t, mgr, 3*time.Second)

	// 第一步：首跑板块按全量写行（NULL=全量、含作品信息）
	if len(recorder.taskIds) != 1 || recorder.taskIds[0] != 1 {
		t.Fatalf("首跑板块选择应对请求任务写行, 实际 %v", recorder.taskIds)
	}
	if recorder.storeRoles.Valid || !recorder.includeWorkInfo {
		t.Fatalf("首跑应写行全量板块+含作品信息(NULL roles), 实际 roles=%+v workInfo=%v",
			recorder.storeRoles, recorder.includeWorkInfo)
	}
	// 第二步：任务已启动并完成落盘
	if u, ok := repo.statusOf(1); !ok || u.Status != task.TaskStatusFinished {
		t.Fatalf("任务应已完成落盘: %+v ok=%v", u, ok)
	}
}

// softPauseStrategy 下载阶段策略桩：进入执行即上报可排空阶段并阻塞，收到软暂停广播后返回
// （无终态上报、不取消 RunCtx——模拟执行面排空在途数据块落盘后的正常收尾）
type softPauseStrategy struct {
	mu      sync.Mutex
	execs   int
	entered chan struct{}
}

func (s *softPauseStrategy) Execute(h StrategyHandle) {
	h.MarkDrainPhase(true)
	s.mu.Lock()
	s.execs++
	s.mu.Unlock()
	s.entered <- struct{}{}
	select {
	case <-h.SoftPauseSignal():
	case <-h.RunCtx().Done():
		// 广播窗口外的取消（暂停落在可排空阶段上报之前）同样不上报终态，交控制面接管
	}
}

func (s *softPauseStrategy) execCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.execs
}

// TestSoftPauseDrainCompletionLandsPaused 软暂停排空完成的收口：下载阶段暂停走软暂停广播，
// 执行面排空在途后返回（无终态、RunCtx 未取消、排空早于超时兜底）→ 按中断交还控制面，待处理
// 暂停命令随后置 Paused（不落违约防御的 Failed）。恢复后二次暂停的排空同样生效——软暂停广播
// 通道随每条运行命令重建，第二轮广播可再次发出
func TestSoftPauseDrainCompletionLandsPaused(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := &softPauseStrategy{entered: make(chan struct{}, 4)}
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-strat.entered
	waitState(t, mgr, 1, TaskStateProcessing, 3*time.Second)

	// 第一次暂停：排空完成后落 Paused
	if err := mgr.PauseTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("暂停失败: %v", err)
	}
	waitState(t, mgr, 1, TaskStatePaused, 3*time.Second)

	// 恢复后二次暂停：第二轮软暂停广播照常发出、排空照常完成
	if err := mgr.ResumeTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	<-strat.entered
	waitState(t, mgr, 1, TaskStateProcessing, 3*time.Second)
	if err := mgr.PauseTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("二次暂停失败: %v", err)
	}
	waitState(t, mgr, 1, TaskStatePaused, 3*time.Second)

	if strat.execCount() != 2 {
		t.Fatalf("恢复应重新执行策略: %d", strat.execCount())
	}
	// 暂停为稳定状态经批量通道落库（200ms 合并窗口），轮询等待落盘完成
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if u, ok := repo.statusOf(1); ok && u.Status == task.TaskStatusPaused {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	u, ok := repo.statusOf(1)
	t.Fatalf("暂停终态应落盘 Paused: %+v ok=%v", u, ok)
}

// TestBuiltinTaskPauseStopResume 暂停/停止走标准控制语义：策略阻塞 → Pause 置 Paused、
// 恢复重新进入 Execute；Stop 置 Failed
func TestBuiltinTaskPauseStopResume(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := newScriptedStrategy("interrupt")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-strat.entered
	h := strat.lastHandle()
	waitState(t, mgr, 1, TaskStateProcessing, 3*time.Second)

	// 暂停：watcher runCancel → 等策略 runCtx 取消后释放（interrupt 模式不上报）→ Paused。
	// 暂停按应答收口（返回即已置 Paused），与策略释放交错——异步发起，取消确认后释放
	pauseDone := make(chan error, 1)
	go func() { pauseDone <- mgr.PauseTaskTrees(context.Background(), []int64{1}) }()
	waitRunCtxCanceled(t, h)
	strat.release <- struct{}{}
	select {
	case err := <-pauseDone:
		if err != nil {
			t.Fatalf("暂停失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("PauseTaskTrees 未返回")
	}
	waitState(t, mgr, 1, TaskStatePaused, 3*time.Second)

	// 恢复：重新进入 Execute
	if err := mgr.ResumeTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	<-strat.entered
	if strat.execCount() != 2 {
		t.Fatalf("恢复应重新执行策略: %d", strat.execCount())
	}

	// 停止：置 Failed（"任务被用户停止"）并即时清理内存；StopTaskTrees 带 ack 等待收口完成，须先取消后释放
	stopDone := make(chan error, 1)
	go func() { stopDone <- mgr.StopTaskTrees(context.Background(), []int64{1}) }()
	waitRunCtxCanceled(t, strat.lastHandle())
	strat.release <- struct{}{}
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("停止失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StopTaskTrees 未返回")
	}
	// 停止收口：Failed 即时落盘 + taskMap 摘除（应答后于清理，StopTaskTrees 返回即已收口）
	if u, ok := repo.statusOf(1); !ok || u.Status != task.TaskStatusFailed {
		t.Fatalf("停止终态应落盘: %+v ok=%v", u, ok)
	}
	if !mgr.IsIdle() {
		t.Fatal("停止后任务应已从内存清理（IsIdle 应为真）")
	}
}

// waitRunCtxCanceled 等待执行面 runCtx 取消（暂停/停止命令已被 watcher 处理）
func waitRunCtxCanceled(t *testing.T, h StrategyHandle) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if h.RunCtx().Err() != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待 runCtx 取消超时")
}

// newBuiltinParent 构造父任务实体（HasChild）
func newBuiltinParent(id int64) *domain.Task {
	t := newBuiltinTask(id, "")
	t.TaskName = sql.NullString{String: "父任务样例", Valid: true}
	t.HasChild = sql.NullBool{Bool: true, Valid: true}
	return t
}

// newBuiltinLeaf 构造叶子任务实体（pid>0）
func newBuiltinLeaf(id, parentId int64, taskType string) *domain.Task {
	t := newBuiltinTask(id, taskType)
	t.Pid = sql.NullInt64{Int64: parentId, Valid: true}
	return t
}

// stopInterruptedTask 停止处于执行中（脚本化 interrupt 策略）的单任务并等待 StopTaskTrees 收口
// 返回：中断路径策略不上报终态，收口（Failed 落盘+内存清理）由停止命令处理完成，应答后于清理。
// 只取消并释放最新一轮执行句柄（单任务的在途执行）
func stopInterruptedTask(t *testing.T, mgr *Manager, strat *scriptedStrategy, taskId int64) {
	t.Helper()
	stopDone := make(chan error, 1)
	go func() { stopDone <- mgr.StopTaskTrees(context.Background(), []int64{taskId}) }()
	waitRunCtxCanceled(t, strat.lastHandle())
	strat.release <- struct{}{}
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("停止失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StopTaskTrees 未返回")
	}
}

// TestStopLeafCleansAndRetryWorks 停止叶子任务的内存收口与重试可用性（停止轨收口锚定）：
// 停止叶子 → Failed 落盘 + taskMap/parentMap 清理（叶子停止无停止编排侧的整树清理兜底，收口全在
// 停止命令处理内）；随后重试同叶子应重建对象重新执行（残留旧对象会被 dispatch 幂等丢弃）
func TestStopLeafCleansAndRetryWorks(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinParent(10), newBuiltinLeaf(11, 10, "demo"))
	strat := newScriptedStrategy("interrupt")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{11}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-strat.entered
	waitState(t, mgr, 11, TaskStateProcessing, 3*time.Second)

	stopInterruptedTask(t, mgr, strat, 11)
	if u, ok := repo.statusOf(11); !ok || u.Status != task.TaskStatusFailed {
		t.Fatalf("停止终态应落盘: %+v ok=%v", u, ok)
	}
	if !mgr.IsIdle() {
		t.Fatal("停止叶子后 taskMap/parentMap 应全清理（IsIdle 应为真）")
	}

	// 重试同叶子：应重建对象重新执行（第二次进入策略）
	if err := mgr.StartTaskTrees(context.Background(), []int64{11}); err != nil {
		t.Fatalf("重试失败: %v", err)
	}
	select {
	case <-strat.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("重试应重新执行策略（残留旧对象会导致请求被静默丢弃）")
	}
	if strat.execCount() != 2 {
		t.Fatalf("重试应重新执行策略, 实际执行 %d 次", strat.execCount())
	}
	// 二次停止同样可收口
	stopInterruptedTask(t, mgr, strat, 11)
	if !mgr.IsIdle() {
		t.Fatal("二次停止后应全清理")
	}
}

// TestPauseTerminalTargetReturnsNotProcessing 暂停语义收紧：对已终态任务暂停返回
// ErrTaskNotProcessing（不假成功），状态不被改写
func TestPauseTerminalTargetReturnsNotProcessing(t *testing.T) {
	repo := newFakeBuiltinRepo()
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": newScriptedStrategy("interrupt")}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	mt := mgr.newManagedTask(newBuiltinTask(1, "demo"))
	mt.setState(TaskStateFinished)
	mgr.mu.Lock()
	mgr.taskMap[1] = mt
	mgr.mu.Unlock()

	if err := mgr.PauseTaskTrees(context.Background(), []int64{1}); !errors.Is(err, ErrTaskNotProcessing) {
		t.Fatalf("暂停已终态任务应返回 ErrTaskNotProcessing, 实际 %v", err)
	}
	if mt.GetState() != TaskStateFinished {
		t.Fatalf("终态不应被改写, 实际 %s", taskStateName(mt.GetState()))
	}
	mt.cancel()
}

// TestPauseTreeSkipsFinishedSibling 混合树暂停：终态兄弟静默跳过不阻塞其余目标，
// 调用整体成功、运行中子任务正常落 Paused
func TestPauseTreeSkipsFinishedSibling(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinParent(30), newBuiltinLeaf(31, 30, "finisher"), newBuiltinLeaf(32, 30, "demo"))
	finisher := newScriptedStrategy("finish")
	runner := newScriptedStrategy("interrupt")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"finisher": finisher, "demo": runner}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{30}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-finisher.entered
	<-runner.entered
	// 先完成叶子 31（终态兄弟）
	finisher.release <- struct{}{}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if u, ok := repo.statusOf(31); ok && u.Status == task.TaskStatusFinished {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 暂停整树：31 终态跳过、32 投递暂停，整体成功
	pauseDone := make(chan error, 1)
	go func() { pauseDone <- mgr.PauseTaskTrees(context.Background(), []int64{30}) }()
	waitRunCtxCanceled(t, runner.lastHandle())
	runner.release <- struct{}{}
	select {
	case err := <-pauseDone:
		if err != nil {
			t.Fatalf("混合树暂停应整体成功（终态兄弟跳过）: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("PauseTaskTrees 未返回")
	}
	waitState(t, mgr, 32, TaskStatePaused, 3*time.Second)
	if u, ok := repo.statusOf(31); !ok || u.Status != task.TaskStatusFinished {
		t.Fatalf("终态兄弟应保持 Finished: %+v ok=%v", u, ok)
	}
}

// TestPauseRacingFinishSurfacesError 暂停竞态完成的如实上抛：暂停投递后任务在排空窗口内
// 转入终态（策略上报 Finish），handlePauseCmd 终态分支回 ErrTaskNotProcessing——已投目标
// 全部应答失败时 PauseTaskTrees 聚合上抛，不假成功
func TestPauseRacingFinishSurfacesError(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "finisher"))
	strat := newScriptedStrategy("finish")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"finisher": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-strat.entered
	waitState(t, mgr, 1, TaskStateProcessing, 3*time.Second)

	pauseDone := make(chan error, 1)
	go func() { pauseDone <- mgr.PauseTaskTrees(context.Background(), []int64{1}) }()
	// 暂停已被 watcher 处理（runCancel 到达执行面）后释放，策略上报 Finish 转入终态
	waitRunCtxCanceled(t, strat.lastHandle())
	strat.release <- struct{}{}
	select {
	case err := <-pauseDone:
		if !errors.Is(err, ErrTaskNotProcessing) {
			t.Fatalf("暂停竞态完成应上抛 ErrTaskNotProcessing, 实际 %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("PauseTaskTrees 未返回")
	}
	waitIdle(t, mgr, 3*time.Second)
}

// TestStopTreeCleansAllAndSingleParentRemove 整树停止的收口一致性：各子任务经停止命令自清理，
// 最后终态子任务连带清 parentMap；停止编排的整树清理对已清树幂等跳过（父移除恰推送一次）
func TestStopTreeCleansAllAndSingleParentRemove(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinParent(20), newBuiltinLeaf(21, 20, "demo"), newBuiltinLeaf(22, 20, "demo"))
	strat := newScriptedStrategy("interrupt")
	pusher := &skipPusher{}
	mgr := NewManager(2, repo, pusher, &TaskDeps{Pusher: pusher},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{20}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	// 两个叶子均进入执行（并发上限 2）
	<-strat.entered
	<-strat.entered

	// 经父任务 id 整树停止：两个叶子的执行同时被取消，须全部取消后再逐个释放
	stopDone := make(chan error, 1)
	go func() { stopDone <- mgr.StopTaskTrees(context.Background(), []int64{20}) }()
	for _, h := range strat.allHandles() {
		waitRunCtxCanceled(t, h)
	}
	strat.release <- struct{}{}
	strat.release <- struct{}{}
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("停止失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StopTaskTrees 未返回")
	}
	for _, id := range []int64{21, 22} {
		if u, ok := repo.statusOf(id); !ok || u.Status != task.TaskStatusFailed {
			t.Fatalf("子任务 %d 停止终态应落盘: %+v ok=%v", id, u, ok)
		}
	}
	if !mgr.IsIdle() {
		t.Fatal("整树停止后 taskMap/parentMap 应全清理")
	}
	pusher.mu.Lock()
	parentRemoved := len(pusher.parentRemoved)
	pusher.mu.Unlock()
	if parentRemoved != 1 {
		t.Fatalf("父移除应恰推送一次（整树清理幂等跳过已清树）, 实际 %d 次", parentRemoved)
	}
}

// TestBuiltinTaskFailTerminal 策略上报失败终态：Failed 即时落盘并携带错误信息
func TestBuiltinTaskFailTerminal(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(7, "demo"))
	strat := newScriptedStrategy("fail")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	_ = mgr.StartTaskTrees(context.Background(), []int64{7})
	<-strat.entered
	strat.release <- struct{}{}
	waitIdle(t, mgr, 3*time.Second)
	u, ok := repo.statusOf(7)
	if !ok || u.Status != task.TaskStatusFailed {
		t.Fatalf("失败终态应落盘: %+v ok=%v", u, ok)
	}
	if !u.ErrorMessage.Valid || u.ErrorMessage.String != "执行失败样例" {
		t.Fatalf("失败原因应落盘: %+v", u.ErrorMessage)
	}
}

// TestBuiltinTaskUnknownTypeStrategy 未注册策略的任务类型不可构建（无执行面）
func TestBuiltinTaskUnknownTypeStrategy(t *testing.T) {
	mgr := NewManager(2, nil, nil, nil, nil, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()
	if mt := mgr.newManagedTask(newBuiltinTask(1, "ghost-type")); mt != nil {
		t.Fatal("未注册策略的任务类型不应构建 ManagedTask")
	}
	// 已注册类型正常构建
	mgr2 := NewManager(2, nil, nil, nil, map[string]ExecutionStrategy{"demo": newScriptedStrategy("finish")}, nil, nil)
	defer func() { close(mgr2.closeCh); <-mgr2.flushDone }()
	if mt := mgr2.newManagedTask(newBuiltinTask(1, "demo")); mt == nil || mt.strategy == nil {
		t.Fatal("已注册类型应构建 ManagedTask 并绑定策略")
	}
}

// TestResumeRequestedSignal 恢复信号置位：执行进入策略前的实时内存状态==Paused 时句柄恢复
// 信号为真（恢复语义），其余状态（首启 Processing/Created、终态重试）为假（全新执行）——
// 执行面据此分叉跨重启续传与全新重走
func TestResumeRequestedSignal(t *testing.T) {
	paused := newTestManagedTask()
	paused.setState(TaskStatePaused)
	if h := newStrategyHandle(paused); !h.ResumeRequested() {
		t.Fatal("进入执行前实时状态为 Paused 应置恢复信号")
	}

	fresh := newTestManagedTask() // Processing（执行中构造句柄的常态）
	if h := newStrategyHandle(fresh); h.ResumeRequested() {
		t.Fatal("非 Paused 进入执行应为全新执行(恢复信号假)")
	}

	finished := newTestManagedTask()
	finished.setState(TaskStateFinished)
	if h := newStrategyHandle(finished); h.ResumeRequested() {
		t.Fatal("终态重试应为全新执行(恢复信号假)")
	}
}

// TestResumeSignalColdLoad 冷加载路径的恢复信号：任务不在内存（进程重启后），恢复/开始/重试
// 入口均从 DB 加载执行——DB 行 status 即执行前稳态（仅稳定态落库），Paused 行带着该状态进入
// 执行命中恢复信号（跨重启续传分叉可达），Created 行首启、终态行重试全新执行
func TestResumeSignalColdLoad(t *testing.T) {
	cases := []struct {
		name       string
		rowStatus  TaskState
		start      func(mgr *Manager) error
		wantResume bool
	}{
		{
			name:       "Paused 行经恢复入口冷加载",
			rowStatus:  TaskStatePaused,
			start:      func(mgr *Manager) error { return mgr.ResumeTaskTrees(context.Background(), []int64{1}) },
			wantResume: true,
		},
		{
			name:       "Created 行经开始入口冷加载",
			rowStatus:  TaskStateCreated,
			start:      func(mgr *Manager) error { return mgr.StartTaskTrees(context.Background(), []int64{1}) },
			wantResume: false,
		},
		{
			name:       "Failed 行经重试入口冷加载",
			rowStatus:  TaskStateFailed,
			start:      func(mgr *Manager) error { return mgr.RetryTaskTrees(context.Background(), []int64{1}) },
			wantResume: false,
		},
		{
			name:       "Finished 行经重试入口冷加载",
			rowStatus:  TaskStateFinished,
			start:      func(mgr *Manager) error { return mgr.RetryTaskTrees(context.Background(), []int64{1}) },
			wantResume: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := newBuiltinTask(1, "demo")
			row.Status = int(tc.rowStatus)
			repo := newFakeBuiltinRepo(row)
			strat := newScriptedStrategy("fail")
			mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
				map[string]ExecutionStrategy{"demo": strat}, nil, nil)
			defer func() { close(mgr.closeCh); <-mgr.flushDone }()

			if err := tc.start(mgr); err != nil {
				t.Fatalf("冷加载执行入口失败: %v", err)
			}
			select {
			case <-strat.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("冷加载后策略未被调度")
			}
			if got := strat.lastHandle().ResumeRequested(); got != tc.wantResume {
				t.Fatalf("恢复信号应=%v, 实际 %v", tc.wantResume, got)
			}
			strat.release <- struct{}{}
			waitIdle(t, mgr, 3*time.Second)
		})
	}
}

// TestResumeSignalHotPath 同会话热路径的恢复信号：执行→暂停→恢复链全程内存态演化（不经 DB
// 重加载），首启执行信号假、暂停后恢复的执行信号真——判据恒为执行前实时内存态（行快照/DB 行
// 在暂停落库批量窗口内滞后，不得作判据）
func TestResumeSignalHotPath(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := newScriptedStrategy("interrupt")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-strat.entered
	first := strat.lastHandle()
	if first.ResumeRequested() {
		t.Fatal("首启执行应为全新执行(恢复信号假)")
	}

	// 暂停：非可排空阶段立即取消 → interrupt 模式不上报终态 → Paused。
	// 暂停按应答收口，与策略释放交错——异步发起，取消确认后释放
	pauseDone := make(chan error, 1)
	go func() { pauseDone <- mgr.PauseTaskTrees(context.Background(), []int64{1}) }()
	waitRunCtxCanceled(t, first)
	strat.release <- struct{}{}
	select {
	case err := <-pauseDone:
		if err != nil {
			t.Fatalf("暂停失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("PauseTaskTrees 未返回")
	}
	waitState(t, mgr, 1, TaskStatePaused, 3*time.Second)

	// 恢复：内存命中投 cmdResume，二次执行信号真（热路径续传分叉）
	if err := mgr.ResumeTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	<-strat.entered
	if second := strat.lastHandle(); !second.ResumeRequested() {
		t.Fatal("热路径恢复执行应置恢复信号(续传分叉)")
	}

	// 收场：停止置 Failed 并清理内存（interrupt 模式先取消再释放）
	stopDone := make(chan error, 1)
	go func() { stopDone <- mgr.StopTaskTrees(context.Background(), []int64{1}) }()
	waitRunCtxCanceled(t, strat.lastHandle())
	strat.release <- struct{}{}
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("停止失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("停止超时")
	}
	if u, ok := repo.statusOf(1); !ok || u.Status != task.TaskStatusFailed {
		t.Fatalf("停止终态应落盘: %+v ok=%v", u, ok)
	}
	if !mgr.IsIdle() {
		t.Fatal("停止后任务应已从内存清理")
	}
}

// TestRunStrategyTerminalGuard 执行器违约防御：既未上报终态也未被取消 → 防御性 Failed
func TestRunStrategyTerminalGuard(t *testing.T) {
	strat := newScriptedStrategy("interrupt")
	mt := newTestManagedTask()
	mt.strategy = strat
	strat.release <- struct{}{} // 预先释放：Execute 直接返回（未上报终态、runCtx 未取消）
	if got := mt.runStrategy(); got != runResultDone {
		t.Fatalf("违约防御应按终态完成处理: %v", got)
	}
	if mt.GetState() != TaskStateFailed {
		t.Fatalf("违约应置防御性 Failed: %s", taskStateName(mt.GetState()))
	}
}

// TestRunStrategyInterrupted 中断映射：Execute 执行中被取消 → runResultPaused（终态由控制面接管）
func TestRunStrategyInterrupted(t *testing.T) {
	strat := newScriptedStrategy("interrupt")
	mt := newTestManagedTask()
	mt.strategy = strat
	done := make(chan runResult, 1)
	go func() { done <- mt.runStrategy() }()
	<-strat.entered
	mt.runCancel() // 模拟 watcher 对暂停/停止的 runCancel
	strat.release <- struct{}{}
	select {
	case got := <-done:
		if got != runResultPaused {
			t.Fatalf("runCtx 取消应映射中断: %v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runStrategy 未返回")
	}
	if mt.GetState() == TaskStateFinished || mt.GetState() == TaskStateFailed {
		t.Fatal("中断路径不应产生终态")
	}
}

// TestStopAndWaitTerminalStopsRunningTask 停止等待终态：执行中任务经 StopAndWaitTerminal →
// runCtx 立即取消、策略检查点退出、停止收口置 Failed 终态即时落盘，等待按内存运行态轮询，
// 任务真达终态后返回 nil（终态收口在策略退出后由 pendingCmds 路径完成，与 StopTaskTrees 先例同序）
func TestStopAndWaitTerminalStopsRunningTask(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := newScriptedStrategy("interrupt") // 阻塞执行，停止靠控制面取消后释放
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-strat.entered
	waitState(t, mgr, 1, TaskStateProcessing, 3*time.Second)

	stopDone := make(chan error, 1)
	go func() { stopDone <- mgr.StopAndWaitTerminal(context.Background(), []int64{1}, 35*time.Second) }()
	waitRunCtxCanceled(t, strat.lastHandle())
	strat.release <- struct{}{} // 策略从检查点退出（interrupt 模式不上报终态，交停止收口）
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("停止等待终态应成功: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StopAndWaitTerminal 未返回")
	}
	if u, ok := repo.statusOf(1); !ok || u.Status != task.TaskStatusFailed {
		t.Fatalf("停止后任务应即时落盘 Failed 终态: %+v ok=%v", u, ok)
	}
	// 停止路径内存对象保留至进程内后续清理（与 StopTaskTrees 先例同路径），不等待 IsIdle
}

// TestStopAndWaitTerminalNonRunningFastReturn 非运行 ids 快速直通：行未启动（不在内存，同建树
// 回滚删 Created 态树形态）无 actor 可停可等，不经轮询挂等即返回 nil
func TestStopAndWaitTerminalNonRunningFastReturn(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"), newBuiltinTask(2, "demo"))
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": newScriptedStrategy("finish")}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	start := time.Now()
	if err := mgr.StopAndWaitTerminal(context.Background(), []int64{1, 2}, 3*time.Second); err != nil {
		t.Fatalf("非运行任务应直通返回: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("非运行任务应立即返回（不得挂到超时量级），实际耗时 %v", elapsed)
	}
}

// TestStopAndWaitTerminalUndispatchedSiblingFastReturn 叶子目标解析：单独启动父树下某叶子后，
// 未派发的 Created 兄弟（整树加载驻留内存但从未执行、无 actor 活动）经 StopAndWaitTerminal
// 直通——运行判定经 resolveTargets 按目标集自查，叶子只作用自身、不等待运行中的兄弟
func TestStopAndWaitTerminalUndispatchedSiblingFastReturn(t *testing.T) {
	parent := newBuiltinTask(10, "demo")
	parent.HasChild = sql.NullBool{Bool: true, Valid: true}
	leaf1 := newBuiltinTask(1, "demo")
	leaf1.Pid = sql.NullInt64{Int64: 10, Valid: true}
	leaf2 := newBuiltinTask(2, "demo")
	leaf2.Pid = sql.NullInt64{Int64: 10, Valid: true}
	repo := newFakeBuiltinRepo(parent, leaf1, leaf2)
	strat := newScriptedStrategy("interrupt")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	// 单独启动叶子 1：整树加载进内存（兄弟 2 驻留 children/taskMap）但仅派发叶子 1
	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-strat.entered
	waitState(t, mgr, 1, TaskStateProcessing, 3*time.Second)

	start := time.Now()
	if err := mgr.StopAndWaitTerminal(context.Background(), []int64{2}, 3*time.Second); err != nil {
		t.Fatalf("未派发兄弟应直通返回: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("未派发 Created 兄弟应立即返回（运行中兄弟不阻塞叶子判定），实际耗时 %v", elapsed)
	}

	// 收场：停整树（叶子 1 释放策略后收口 Failed），防策略 goroutine 泄漏。停止前捕获叶子
	// 对象——父树停止会经 cleanupStoppedTree 把子任务移出 taskMap，事后只能按对象断言终态
	mgr.mu.RLock()
	leaf1MT := mgr.taskMap[1]
	mgr.mu.RUnlock()
	stopDone := make(chan error, 1)
	go func() { stopDone <- mgr.StopAndWaitTerminal(context.Background(), []int64{10}, 3*time.Second) }()
	waitRunCtxCanceled(t, strat.lastHandle())
	strat.release <- struct{}{}
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("整树停止失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("整树停止未返回")
	}
	if leaf1MT.GetState() != TaskStateFailed {
		t.Fatalf("整树停止后叶子 1 应达 Failed 终态: %s", taskStateName(leaf1MT.GetState()))
	}
}

// TestStopAndWaitTerminalTimeoutRefuses 超时拒绝：策略不理会取消（runCtx 取消后仍阻塞不出），
// 停止命令应答超时后任务仍处 Processing，等待按内存运行态轮询至超时返回 ErrStopWaitTimeout
// （调用方拒绝本次删除的信号）。应答上界注入缩短，避免挂满生产默认 35s
func TestStopAndWaitTerminalTimeoutRefuses(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := newScriptedStrategy("interrupt")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-strat.entered
	waitState(t, mgr, 1, TaskStateProcessing, 3*time.Second)

	mgr.mu.RLock()
	mt := mgr.taskMap[1]
	mgr.mu.RUnlock()
	if mt == nil {
		t.Fatal("运行中任务应在 taskMap")
	}
	mt.ackWaitTimeout = 250 * time.Millisecond

	start := time.Now()
	err := mgr.StopAndWaitTerminal(context.Background(), []int64{1}, 900*time.Millisecond)
	if !errors.Is(err, ErrStopWaitTimeout) {
		t.Fatalf("任务不可停止时应超时拒绝, 实际 %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("超时等待应以注入 timeout 为界（不含默认应答上界），实际耗时 %v", elapsed)
	}

	// 收场：释放策略（runCtx 已取消 → 不上报终态 → 待处理停止命令收口 Failed），防 goroutine 泄漏
	strat.release <- struct{}{}
	select {
	case <-mt.actorDone:
	case <-time.After(3 * time.Second):
		t.Fatal("释放后任务 actor 未退出")
	}
}
