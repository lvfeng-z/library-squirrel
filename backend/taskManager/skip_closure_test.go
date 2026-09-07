package taskManager

// 跳过收口测试：StrategyHandle.Skip 上报的控制面处置（回执行前状态/释放槽位/置收口标志/
// 单次清理防前端双推）与 runStrategy 的 runResultSkipped 分流（不走「未上报终态」防御）。
// 父聚合按子任务回退态聚合两例：原 Created 树全跳过父保持 Created、原 Finished 重下全跳过
// 父落 Finished（RefreshState 语义，fake 策略驱动）。

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/library-squirrel/backend/shareLock"
	"github.com/library-squirrel/backend/task"
)

// skipPusher 跳过收口测试推送桩：记录错误通知与前端移除（防双推断言用）
type skipPusher struct {
	mu            sync.Mutex
	errors        []string
	removed       []int64
	parentRemoved []int64
}

func (p *skipPusher) PushStateChange(int64, string, TaskState)       {}
func (p *skipPusher) PushParentStateChange(int64, string, TaskState) {}
func (p *skipPusher) PushProgress(int64, int64, int64)               {}
func (p *skipPusher) PushProgressBatch([]*taskScheduleDTO)           {}
func (p *skipPusher) PushParentProgress(int64, int64, int64)         {}
func (p *skipPusher) PushError(taskId int64, err string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errors = append(p.errors, err)
}
func (p *skipPusher) PushTaskRemove(ids []int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.removed = append(p.removed, ids...)
}
func (p *skipPusher) PushParentTaskRemove(ids []int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.parentRemoved = append(p.parentRemoved, ids...)
}
func (p *skipPusher) PushDuplicateDetected(taskId int64, taskName string, existingWorkId int64, existingWorkName string, conflictRoles []string) {
}

func (p *skipPusher) removedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.removed)
}

func (p *skipPusher) errorCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.errors)
}

// skipStrategy 跳过收口策略桩：Execute 内按预设错误说明调 Skip
type skipStrategy struct {
	errMsg string
}

func (s *skipStrategy) Execute(h StrategyHandle) {
	h.Skip(s.errMsg)
}

// newSkipClosureTask 构造跳过收口测试任务（挂入管理器、真实持槽），taskStatus 为任务行
// 加载时 DB 快照（回退目标态）
func newSkipClosureTask(mgr *Manager, taskId int64, taskStatus TaskState) *ManagedTask {
	m := newTestManagedTask()
	m.taskId = taskId
	m.task.Status = int(taskStatus)
	m.manager = mgr
	m.semaphore = mgr.semaphore
	m.deps = mgr.deps
	// 模拟 handleRunCmd 已取槽后进入策略
	m.slotHeld = true
	m.semaphore <- struct{}{}
	mgr.mu.Lock()
	mgr.taskMap[taskId] = m
	mgr.mu.Unlock()
	return m
}

// TestStrategySkipClosure Skip 上报控制面处置：状态回任务行 DB 快照、槽位释放不泄漏、
// 置收口标志（全子终态判定视为终态）、清理恰一次（前端移除单推）、幂等
func TestStrategySkipClosure(t *testing.T) {
	repo := newFakeBuiltinRepo()
	pusher := &skipPusher{}
	mgr := NewManager(2, repo, pusher, &TaskDeps{Pusher: pusher, WorkLockChecker: shareLock.NewShareLockRegistry()}, nil, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	m := newSkipClosureTask(mgr, 1, TaskStateCreated)
	h := newStrategyHandle(m)

	h.Skip("")

	if m.GetState() != TaskStateCreated {
		t.Fatalf("跳过应回执行前状态(Created), 实际 %s", taskStateName(m.GetState()))
	}
	if m.slotHeld || len(m.semaphore) != 0 {
		t.Fatalf("跳过应释放信号量槽位不泄漏, 实际 slotHeld=%v 槽占用=%d", m.slotHeld, len(m.semaphore))
	}
	if !m.skipped {
		t.Fatal("跳过应置收口标志(父任务全子终态判定视为终态)")
	}
	if m.GetState() == TaskStateFailed || m.GetState() == TaskStateFinished {
		t.Fatalf("跳过不应产生终态, 实际 %s", taskStateName(m.GetState()))
	}
	select {
	case <-m.ctx.Done():
	default:
		t.Fatal("跳过应取消 actor 主 ctx")
	}
	if _, ok := mgr.taskMap[1]; ok {
		t.Fatal("跳过清理应将任务移出 taskMap")
	}
	if pusher.removedCount() != 1 {
		t.Fatalf("前端移除应恰推送一次, 实际 %d 次", pusher.removedCount())
	}

	// 幂等：二次 Skip 为 no-op（不再推送移除/错误）
	h.Skip("重复上报")
	if pusher.removedCount() != 1 || pusher.errorCount() != 0 {
		t.Fatalf("重复 Skip 应 no-op, 实际 移除 %d 次 错误 %d 次", pusher.removedCount(), pusher.errorCount())
	}
}

// TestStrategySkipErrMsgPushed 非终态板块失败的错误说明推送：errMsg 非空时推送错误通知
// （回退态不落库，错误通知是对用户可见的唯一通道）
func TestStrategySkipErrMsgPushed(t *testing.T) {
	repo := newFakeBuiltinRepo()
	pusher := &skipPusher{}
	mgr := NewManager(2, repo, pusher, &TaskDeps{Pusher: pusher}, nil, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	m := newSkipClosureTask(mgr, 1, TaskStateCreated)
	m.deps = mgr.deps
	h := newStrategyHandle(m)

	h.Skip("创建作品信息失败: 插件不可用")

	if pusher.errorCount() != 1 {
		t.Fatalf("错误说明应推送一次, 实际 %d 次", pusher.errorCount())
	}
	if m.GetState() != TaskStateCreated {
		t.Fatalf("携带错误说明的跳过同样回执行前状态, 实际 %s", taskStateName(m.GetState()))
	}
}

// TestRunStrategySkippedResult runStrategy 分流：Skip 上报后返回跳过收口结果（不走
// 「未上报终态」防御置失败、不产生终态）
func TestRunStrategySkippedResult(t *testing.T) {
	repo := newFakeBuiltinRepo()
	pusher := &skipPusher{}
	mgr := NewManager(2, repo, pusher, &TaskDeps{Pusher: pusher}, nil, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	m := newSkipClosureTask(mgr, 1, TaskStateCreated)
	m.strategy = &skipStrategy{}
	// runStrategy 的中断检查基线：派生未取消的 runCtx
	m.runCtx, m.runCancel = context.WithCancel(m.ctx)

	res := m.runStrategy()

	if res != runResultSkipped {
		t.Fatalf("Skip 上报后 runStrategy 应返回跳过收口结果, 实际 %d", res)
	}
	if m.GetState() != TaskStateCreated {
		t.Fatalf("跳过收口应回执行前状态, 实际 %s", taskStateName(m.GetState()))
	}
	if pusher.errorCount() != 0 {
		t.Fatalf("正常跳过不应推送错误, 实际 %d 次", pusher.errorCount())
	}
}

// TestBuiltinTaskSkipLifecycle 经 Manager 全链的跳过收口：策略执行内 Skip →
// 不产生终态落盘（任务行停留执行前值）、任务清理退出、前端移除恰一次
func TestBuiltinTaskSkipLifecycle(t *testing.T) {
	repo := newFakeBuiltinRepo(newBuiltinTask(1, "demo"))
	pusher := &skipPusher{}
	mgr := NewManager(2, repo, pusher, &TaskDeps{Pusher: pusher},
		map[string]ExecutionStrategy{"demo": &skipStrategy{}}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	waitIdle(t, mgr, 3*time.Second)

	if u, ok := repo.statusOf(1); ok {
		t.Fatalf("跳过收口不应产生终态落盘, 实际 %+v", u)
	}
	if pusher.removedCount() != 1 {
		t.Fatalf("前端移除应恰推送一次(Skip 内单次清理), 实际 %d 次", pusher.removedCount())
	}
	if _, err := mgr.GetTaskState(1); err == nil {
		t.Fatal("跳过收口后任务应已清理")
	}
}

// TestStrategySkipParentAggregation 父聚合按子任务回退态聚合两例：
// 全子跳过后 AllChildrenTerminal 成立，父状态经 RefreshState 按回退态聚合——
// 原 Created 树父保持 Created（不终态落盘）、原 Finished 重下全跳过父落 Finished（终态落盘）
func TestStrategySkipParentAggregation(t *testing.T) {
	run := func(t *testing.T, childPreExec TaskState, wantParent TaskState, wantPersisted bool) {
		repo := newFakeBuiltinRepo()
		pusher := &skipPusher{}
		mgr := NewManager(2, repo, pusher, &TaskDeps{Pusher: pusher}, nil, nil, nil)
		defer func() { close(mgr.closeCh); <-mgr.flushDone }()

		parent := NewParentTask(9, "父任务")
		mgr.mu.Lock()
		mgr.parentMap[9] = parent
		mgr.mu.Unlock()

		for i := int64(1); i <= 2; i++ {
			m := newSkipClosureTask(mgr, i, childPreExec)
			m.parentId = 9
			parent.AddChild(m)
			h := newStrategyHandle(m)
			h.Skip("")
		}

		if !parent.AllChildrenTerminal() {
			t.Fatal("全子跳过后父任务全子终态判定应成立(收口标志视为终态)")
		}
		_, newParentState, _, total := parent.RefreshState()
		if newParentState != wantParent {
			t.Fatalf("父状态应按子任务回退态聚合为 %s, 实际 %s", taskStateName(wantParent), taskStateName(newParentState))
		}
		if total != 2 {
			t.Fatalf("子任务总数期望 2, 实际 %d", total)
		}
		if wantPersisted {
			if u, ok := repo.statusOf(9); !ok || u.Status != task.TaskStatusEnum(wantParent) {
				t.Fatalf("父终态应落盘, 实际 %+v ok=%v", u, ok)
			}
		} else if _, ok := repo.statusOf(9); ok {
			t.Fatal("父非终态不应落盘")
		}
	}
	t.Run("原Created树全跳过_父保持Created", func(t *testing.T) {
		run(t, TaskStateCreated, TaskStateCreated, false)
	})
	t.Run("原Finished重下全跳过_父落Finished", func(t *testing.T) {
		run(t, TaskStateFinished, TaskStateFinished, true)
	})
}
