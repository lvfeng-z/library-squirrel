package taskManager

// 程序退出阻断点（ShutdownGate）测试：注册项按序等待、单项失败不中断、ctx 超时传导；
// 任务模块自注册的优雅关闭阻断项——WaitAll 等待全部 Processing 任务暂停完成且 Paused
// 终刷落盘后返回。

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/library-squirrel/backend/task"
)

// TestShutdownGateWaitAllRunsAllRegistered 阻断点注册与等待：全部注册项按序执行，单项错误
// 不中断其余项并聚合返回；无注册项直通
func TestShutdownGateWaitAllRunsAllRegistered(t *testing.T) {
	g := NewShutdownGate()
	if err := g.WaitAll(context.Background()); err != nil {
		t.Fatalf("无注册项应直通返回 nil, 实际 %v", err)
	}

	var order []string
	g.Register("first", func(ctx context.Context) error {
		order = append(order, "first")
		return nil
	})
	g.Register("second", func(ctx context.Context) error {
		order = append(order, "second")
		return errors.New("样例失败")
	})
	g.Register("third", func(ctx context.Context) error {
		order = append(order, "third")
		return nil
	})

	err := g.WaitAll(context.Background())
	if err == nil {
		t.Fatal("含失败项应返回聚合错误")
	}
	if len(order) != 3 || order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Fatalf("全部注册项应按序执行且失败不中断, 实际 %v", order)
	}
}

// TestShutdownGateWaitAllTimeoutPropagates 整体等待有界：ctx 到期传导给等待中的注册项，
// 注册项按取消返回、WaitAll 聚合上报
func TestShutdownGateWaitAllTimeoutPropagates(t *testing.T) {
	g := NewShutdownGate()
	g.Register("blocked", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := g.WaitAll(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("应聚合上报 ctx 超时, 实际 %v", err)
	}
}

// TestManagerShutdownGateWaitsTasksPaused 任务模块自注册的退出阻断项：WaitAll 等待全部
// Processing 任务暂停完成且 Paused 终刷落盘（终刷先于返回）。经 newManagedTask 生产构造
// 接线状态回调（Paused 进批量通道，由关闭终刷落盘）
func TestManagerShutdownGateWaitsTasksPaused(t *testing.T) {
	repo := newFakeBuiltinRepo()
	strat := &slowNotifyStrategy{notifyDelay: 100 * time.Millisecond}
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	// 优雅关闭经 gate 触发，closeCh 由其内部收口关闭，测试不再补 close

	tasks := make([]*ManagedTask, 0, 2)
	for i := int64(1); i <= 2; i++ {
		mt := mgr.newManagedTask(newBuiltinTask(i, "demo"))
		if mt == nil {
			t.Fatalf("任务 %d 构建失败", i)
		}
		mt.setState(TaskStateProcessing)
		mgr.mu.Lock()
		mgr.taskMap[i] = mt
		mgr.mu.Unlock()
		tasks = append(tasks, mt)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := mgr.ShutdownGate().WaitAll(ctx); err != nil {
		t.Fatalf("退出阻断等待失败: %v", err)
	}
	for i, mt := range tasks {
		if mt.GetState() != TaskStatePaused {
			t.Fatalf("任务 %d 应暂停, 实际 %s", i+1, taskStateName(mt.GetState()))
		}
		u, ok := repo.statusOf(mt.taskId)
		if !ok || u.Status != task.TaskStatusPaused {
			t.Fatalf("任务 %d 暂停应终刷落盘, 实际 %+v ok=%v", mt.taskId, u, ok)
		}
	}
	// 收尾测试对象：退出驻留 actor
	for _, mt := range tasks {
		mt.cancel()
	}
}

// TestGracefulShutdownSettlesParkedTasks 关闭收口判定含无在途执行的驻留态：等待队列的
// Waiting（关闭起始已取消）与未派发的 Created 不阻塞收口——按稳定态等待会耗尽全部
// 等待预算（进程滞留到超时），有界 ctx 内应即刻收口返回
func TestGracefulShutdownSettlesParkedTasks(t *testing.T) {
	repo := newFakeBuiltinRepo()
	strat := &slowNotifyStrategy{notifyDelay: 50 * time.Millisecond}
	mgr := NewManager(2, repo, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)

	// running：Processing 任务（被暂停收口）
	running := mgr.newManagedTask(newBuiltinTask(1, "demo"))
	running.setState(TaskStateProcessing)
	// queued：等待队列中的 Waiting 任务（关闭起始被取消，状态停留 Waiting）
	queued := mgr.newManagedTask(newBuiltinTask(2, "demo"))
	queued.setState(TaskStateWaiting)
	// idle：整树加载未派发的 Created 兄弟
	idle := mgr.newManagedTask(newBuiltinTask(3, "demo"))
	mgr.mu.Lock()
	mgr.taskMap[1] = running
	mgr.taskMap[2] = queued
	mgr.taskMap[3] = idle
	mgr.waitingQueue = []*ManagedTask{queued}
	mgr.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := mgr.GracefulShutdown(ctx); err != nil {
		t.Fatalf("优雅关闭应在有界预算内收口: %v", err)
	}
	if running.GetState() != TaskStatePaused {
		t.Fatalf("Processing 任务应暂停, 实际 %s", taskStateName(running.GetState()))
	}
	if queued.GetState() != TaskStateWaiting || idle.GetState() != TaskStateCreated {
		t.Fatalf("驻留任务状态不应被改写, 实际 queued=%s idle=%s",
			taskStateName(queued.GetState()), taskStateName(idle.GetState()))
	}
	for _, mt := range []*ManagedTask{running, queued, idle} {
		mt.cancel()
	}
}
