package taskManager

// 内置任务类型（执行面策略）路径测试：策略解析、控制面集成（启动/暂停/停止/终态落盘）、
// 执行器违约防御与替换回滚守卫。

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/task"
)

// fakeBuiltinRepo 内置任务路径测试仓储桩：仅 ListTaskTree 有值，其余记录调用
type fakeBuiltinRepo struct {
	mu             sync.Mutex
	tasks          []*domain.Task
	statuses       map[int64]task.StatusUpdate
	sectionUpdates []sectionUpdateRecord
}

// sectionUpdateRecord 板块重执行选择写入记录（终态清空回归锚断言用）
type sectionUpdateRecord struct {
	taskIds         []int64
	storeRoles      sql.NullString
	includeWorkInfo bool
}

func newFakeBuiltinRepo(tasks ...*domain.Task) *fakeBuiltinRepo {
	return &fakeBuiltinRepo{tasks: tasks, statuses: make(map[int64]task.StatusUpdate)}
}

func (r *fakeBuiltinRepo) ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...task.TaskStatusEnum) ([]*domain.Task, error) {
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

func (r *fakeBuiltinRepo) BatchUpdatePendingResourceID(ctx context.Context, updates map[int64]sql.NullInt64) error {
	return nil
}

func (r *fakeBuiltinRepo) UpdateRedownloadSections(ctx context.Context, taskIds []int64, storeRoles sql.NullString, includeWorkInfo bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sectionUpdates = append(r.sectionUpdates, sectionUpdateRecord{taskIds: taskIds, storeRoles: storeRoles, includeWorkInfo: includeWorkInfo})
	return nil
}

// sectionUpdatesOf 取指定任务的板块选择写入记录
func (r *fakeBuiltinRepo) sectionUpdatesOf(id int64) []sectionUpdateRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []sectionUpdateRecord
	for _, u := range r.sectionUpdates {
		for _, tid := range u.taskIds {
			if tid == id {
				out = append(out, u)
				break
			}
		}
	}
	return out
}

func (r *fakeBuiltinRepo) ListBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) ([]*domain.Task, error) {
	return nil, nil
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

// newBuiltinTask 构造内置类型任务实体（Created、根级）
func newBuiltinTask(id int64, taskType string) *domain.Task {
	t := domain.NewTask()
	t.SetID(id)
	t.TaskType = sql.NullString{String: taskType, Valid: true}
	t.TaskName = sql.NullString{String: "内置任务样例", Valid: true}
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

// TestBuiltinTaskFinishLifecycle 内置任务经 Manager 全链执行：启动 → 策略 Execute →
// Finish 终态即时落盘 → taskMap 清理
func TestBuiltinTaskFinishLifecycle(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := newScriptedStrategy("finish")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), nil, &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat})
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

// TestFailedTerminalKeepsRecordedSections 失败重试板块选择保留锚：终态不清空执行模式持久化——
// 子集重下失败后 StoreRoles 留存，重试(RetryTaskTrees 不重记模式)按原子集再来一次。
// 修复前终态即时清空板块选择，失败重试退化为空 universe 派生(丢子集选择)
func TestFailedTerminalKeepsRecordedSections(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := newScriptedStrategy("fail")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), nil, &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat})
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	// 子集重下：记录 thumbnail 板块选择后启动
	if err := mgr.Redownload(context.Background(), []int64{1}, []string{domain.StoreTypeThumbnail}, false); err != nil {
		t.Fatalf("重下启动失败: %v", err)
	}
	<-strat.entered
	strat.release <- struct{}{}
	waitIdle(t, mgr, 3*time.Second)

	updates := repo.sectionUpdatesOf(1)
	if len(updates) == 0 {
		t.Fatal("重下启动应记录板块选择")
	}
	for _, u := range updates {
		if !u.storeRoles.Valid || u.storeRoles.String != domain.StoreTypeThumbnail || u.includeWorkInfo {
			t.Fatalf("任务 1 的板块记录应保持 thumbnail 子集(终态不清空)，实际 %+v", u)
		}
	}
	if u, ok := repo.statusOf(1); !ok || u.Status != task.TaskStatusFailed {
		t.Fatalf("任务应已失败落盘: %+v ok=%v", u, ok)
	}
}

// TestBuiltinTaskPauseStopResume 暂停/停止走标准控制语义：策略阻塞 → Pause 置 Paused、
// 恢复重新进入 Execute；Stop 置 Failed
func TestBuiltinTaskPauseStopResume(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	strat := newScriptedStrategy("interrupt")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), nil, &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat})
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-strat.entered
	h := strat.lastHandle()
	waitState(t, mgr, 1, TaskStateProcessing, 3*time.Second)

	// 暂停：watcher runCancel → 等策略 runCtx 取消后释放（interrupt 模式不上报）→ Paused
	if err := mgr.PauseTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("暂停失败: %v", err)
	}
	waitRunCtxCanceled(t, h)
	strat.release <- struct{}{}
	waitState(t, mgr, 1, TaskStatePaused, 3*time.Second)

	// 恢复：重新进入 Execute
	if err := mgr.ResumeTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	<-strat.entered
	if strat.execCount() != 2 {
		t.Fatalf("恢复应重新执行策略: %d", strat.execCount())
	}

	// 停止：置 Failed（"任务被用户停止"）；StopTaskTrees 带 ack 等待终态，须先取消后释放
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
	// 停止中途终态与插件任务同路径：Failed 落盘、内存对象保留至进程内后续清理
	waitState(t, mgr, 1, TaskStateFailed, 3*time.Second)
	if u, ok := repo.statusOf(1); !ok || u.Status != task.TaskStatusFailed {
		t.Fatalf("停止终态应落盘: %+v ok=%v", u, ok)
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

// TestBuiltinTaskFailTerminal 策略上报失败终态：Failed 即时落盘并携带错误信息
func TestBuiltinTaskFailTerminal(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(7, "demo"))
	strat := newScriptedStrategy("fail")
	mgr := NewManager(2, repo, NewNoopProgressPusher(), nil, &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat})
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

// TestBuiltinTaskUnknownTypeStrategy 未注册策略的内置类型任务不可构建（无执行面）
func TestBuiltinTaskUnknownTypeStrategy(t *testing.T) {
	mgr := NewManager(2, nil, nil, nil, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()
	if mt := mgr.newManagedTask(newBuiltinTask(1, "ghost-type")); mt != nil {
		t.Fatal("未注册策略的内置类型不应构建 ManagedTask")
	}
	// 已注册类型正常构建且不要求 PluginPublicID
	mgr2 := NewManager(2, nil, nil, nil, nil, map[string]ExecutionStrategy{"demo": newScriptedStrategy("finish")})
	defer func() { close(mgr2.closeCh); <-mgr2.flushDone }()
	if mt := mgr2.newManagedTask(newBuiltinTask(1, "demo")); mt == nil || mt.strategy == nil {
		t.Fatal("已注册内置类型应构建 ManagedTask 并绑定策略")
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
