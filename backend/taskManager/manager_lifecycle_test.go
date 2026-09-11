package taskManager

// 任务生命周期与命令可靠性测试：活跃插件计数（窄投影内存过滤）、命令通道满丢弃的应答
// 回写、应答有界等待（超时返回/actor 已退出立返/应答优先于退出信号）、actor 退出对任务
// ctx 取消与命令通道善后消费的联动、优雅关闭暂停阶段的并行性。

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	domain "github.com/library-squirrel/backend/base/model/entity"
)

// fakeWorkTaskProjector 作品任务领域行投影桩：按预置行表返回
type fakeWorkTaskProjector struct {
	rows map[int64]*domain.WorkTask
}

func (p *fakeWorkTaskProjector) ListByIds(ctx context.Context, ids []int64) (map[int64]*domain.WorkTask, error) {
	out := make(map[int64]*domain.WorkTask, len(ids))
	for _, id := range ids {
		if wt, ok := p.rows[id]; ok {
			out[id] = wt
		}
	}
	return out, nil
}

// TestCountActiveByPlugin 验证插件停用拦截判据：仅统计该插件名下运行中任务
// （Processing/Pausing/Stopping/WaitingForInput），Created/Paused/终态与其他插件不计，
// 无领域行投影（其他任务类型）不计
func TestCountActiveByPlugin(t *testing.T) {
	const target = "com.example.plugin"
	mkWorkTask := func(id int64, plugin string) *domain.WorkTask {
		wt := domain.NewWorkTask(id)
		wt.PluginPublicID = sql.NullString{String: plugin, Valid: true}
		return wt
	}
	rows := map[int64]*domain.WorkTask{
		1: mkWorkTask(1, target),
		2: mkWorkTask(2, target),
		3: mkWorkTask(3, target),
		4: mkWorkTask(4, target),
		5: mkWorkTask(5, target),
		6: mkWorkTask(6, target),
		7: mkWorkTask(7, target),
		8: mkWorkTask(8, "com.other.plugin"),
		// 任务 9：无领域行投影（其他任务类型），不计入任何插件
	}
	mgr := NewManager(2, nil, nil, nil, nil, &fakeWorkTaskProjector{rows: rows}, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	// 任务对象仅承载运行状态（插件身份在领域行投影）
	mk := func(id int64, state TaskState) *ManagedTask {
		mt := newTestManagedTask()
		mt.taskId = id
		mt.setState(state)
		return mt
	}

	mgr.mu.Lock()
	mgr.taskMap[1] = mk(1, TaskStateProcessing)
	mgr.taskMap[2] = mk(2, TaskStatePausing)
	mgr.taskMap[3] = mk(3, TaskStateStopping)
	mgr.taskMap[4] = mk(4, TaskStateWaitingForInput)
	mgr.taskMap[5] = mk(5, TaskStatePaused)
	mgr.taskMap[6] = mk(6, TaskStateCreated)
	mgr.taskMap[7] = mk(7, TaskStateFinished)
	mgr.taskMap[8] = mk(8, TaskStateProcessing)
	mgr.taskMap[9] = mk(9, TaskStateProcessing)
	mgr.mu.Unlock()

	if n := mgr.CountActiveByPlugin(target); n != 4 {
		t.Fatalf("运行中任务计数不符: 期望 4, 实际 %d", n)
	}
	if n := mgr.CountActiveByPlugin("com.other.plugin"); n != 1 {
		t.Fatalf("其他插件计数不符: 期望 1, 实际 %d", n)
	}
	if n := mgr.CountActiveByPlugin("com.absent.plugin"); n != 0 {
		t.Fatalf("无任务插件计数应为 0, 实际 %d", n)
	}
}

// TestAckQueueFullWriteBack 命令通道占满后投递:带应答命令被丢弃时立即在应答
// 通道回写队列满错误（等待方随即收到失败,不空等到应答超时）;无应答命令丢弃不 panic
func TestAckQueueFullWriteBack(t *testing.T) {
	m := newTestManagedTask() // 不启动 actor,队列占用可控
	for i := 0; i < cap(m.cmdCh); i++ {
		m.cmdCh <- taskCmd{kind: cmdPause}
	}

	ack := make(chan error, 1)
	m.postCmd(taskCmd{kind: cmdPause, ack: ack})
	select {
	case err := <-ack:
		if !errors.Is(err, ErrCmdQueueFull) {
			t.Fatalf("期望队列满错误, 实际 %v", err)
		}
	default:
		t.Fatal("队列满丢弃应立即回写应答错误")
	}

	// fire-and-forget 命令（无应答通道）丢弃不影响投递方
	m.postCmd(taskCmd{kind: cmdStop})
}

// TestAckWaitTimeout 应答超时:命令已投递但无处理方（actor 未启动）且 actor 未退出时,
// 应答等待在注入的缩短上界后返回超时错误
func TestAckWaitTimeout(t *testing.T) {
	m := newTestManagedTask()
	m.ackWaitTimeout = 50 * time.Millisecond
	ack := make(chan error, 1)
	m.postCmd(taskCmd{kind: cmdPause, ack: ack})

	start := time.Now()
	if err := m.waitAck(ack); !errors.Is(err, ErrTaskAckTimeout) {
		t.Fatalf("期望应答超时错误, 实际 %v", err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Fatalf("超时返回不应早于注入的等待上界, 实际 %v", elapsed)
	}
}

// TestAckActorExitedPromptReturn actor 退出后 ack 立返:任务 ctx 取消令 actor 退出后,
// 暂停命令的应答等待立即返回 actor 已退出错误（命令可投递成功但不再有处理方）,不等到
// 应答超时。ackWaitTimeout 注入放大值,与"立返"形成区分度（超时分支需等满注入值）
func TestAckActorExitedPromptReturn(t *testing.T) {
	mgr := NewManager(1, nil, nil, nil, nil, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	mt := NewManagedTask(1, 0, domain.NewTask(), nil, mgr, make(chan struct{}, 1))
	mt.ackWaitTimeout = 3 * time.Second
	mt.cancel()
	<-mt.actorDone

	start := time.Now()
	if err := mt.Pause(); !errors.Is(err, ErrTaskActorExited) {
		t.Fatalf("期望 actor 已退出错误, 实际 %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("actor 退出后应答等待应立返, 实际耗时 %v", elapsed)
	}
}

// TestAckPriorityOnActorTerminalExit 停止命令处理完毕即终态退出:应答与 actorDone 同时
// 就绪时应答优先（停止成功不被误报为 actor 已退出）;actor 退出时取消任务 ctx（命令通道
// 善后消费的退出条件）。经 NewManagedTask 构造（生产路径,actor 真实运转）
func TestAckPriorityOnActorTerminalExit(t *testing.T) {
	// 停止命令收口含内存清理（cleanupFinishedTask 推送前端移除），manager 须带推送器依赖
	mgr := NewManager(1, nil, NewNoopProgressPusher(), &TaskDeps{Pusher: NewNoopProgressPusher()}, nil, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	mt := NewManagedTask(1, 0, domain.NewTask(), nil, mgr, make(chan struct{}, 1))
	ack := make(chan error, 1)
	mt.postCmd(taskCmd{kind: cmdStop, ack: ack})
	<-mt.actorDone // actor 处理完停止（终态退出）,应答已写入缓冲

	if err := mt.waitAck(ack); err != nil {
		t.Fatalf("停止成功的应答应优先于 actor 退出信号: %v", err)
	}
	select {
	case <-mt.ctx.Done():
	default:
		t.Fatal("actor 退出后任务 ctx 应已取消")
	}
	if mt.GetState() != TaskStateFailed {
		t.Fatalf("停止应置 Failed 终态: %s", taskStateName(mt.GetState()))
	}
}

// TestDrainCmds_ExitsOnCtxCancelOrClose actor 退出后的命令通道善后消费:任务 ctx 取消或
// 通道关闭即退出,不随任务对象常驻
func TestDrainCmds_ExitsOnCtxCancelOrClose(t *testing.T) {
	// 任务 ctx 取消即退出（缓冲内残留命令为善后消费对象）
	m := newTestManagedTask()
	m.cmdCh <- taskCmd{kind: cmdPause}
	finished := make(chan struct{})
	go func() {
		m.drainCmds()
		close(finished)
	}()
	m.cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("任务 ctx 取消后善后消费应退出")
	}

	// 命令通道关闭即退出
	m2 := newTestManagedTask()
	close(m2.cmdCh)
	finished2 := make(chan struct{})
	go func() {
		m2.drainCmds()
		close(finished2)
	}()
	select {
	case <-finished2:
	case <-time.After(time.Second):
		t.Fatal("命令通道关闭后善后消费应退出")
	}
}

// slowNotifyStrategy 中断通知慢响应策略桩:暂停/停止命令处理经 NotifyInterrupt 阻塞
// notifyDelay,模拟插件暂停 RPC 慢响应（控制命令处理时长的构成主体）
type slowNotifyStrategy struct{ notifyDelay time.Duration }

func (s *slowNotifyStrategy) Execute(h StrategyHandle) {}

func (s *slowNotifyStrategy) NotifyInterrupt(ctx context.Context, taskID int64, stop bool) {
	select {
	case <-time.After(s.notifyDelay):
	case <-ctx.Done():
	}
}

// TestGracefulShutdown_ParallelPause 优雅关闭的暂停阶段并行:各 Processing 任务的暂停
// （含执行面中断通知等待）并行发起与等待,总耗时不随任务数线性叠加。串行暂停耗时约
// n×notifyDelay,并行约一个 notifyDelay,阈值取两者之间
func TestGracefulShutdown_ParallelPause(t *testing.T) {
	const n = 4
	mgr := NewManager(n, nil, nil, nil, nil, nil, nil)
	strat := &slowNotifyStrategy{notifyDelay: 300 * time.Millisecond}
	tasks := make([]*ManagedTask, 0, n)
	mgr.mu.Lock()
	for i := 1; i <= n; i++ {
		mt := NewManagedTask(int64(i), 0, domain.NewTask(), nil, mgr, mgr.semaphore)
		mt.strategy = strat
		mt.setState(TaskStateProcessing)
		mgr.taskMap[mt.taskId] = mt
		tasks = append(tasks, mt)
	}
	mgr.mu.Unlock()

	start := time.Now()
	if err := mgr.GracefulShutdown(context.Background()); err != nil {
		t.Fatalf("优雅关闭失败: %v", err)
	}
	elapsed := time.Since(start)
	for _, mt := range tasks {
		mt.cancel() // 收尾测试对象:退出驻留 actor
	}

	if elapsed > 900*time.Millisecond {
		t.Fatalf("并行暂停下优雅关闭耗时 %v,超过单任务中断通知时长的合理叠加界(串行约 %v)",
			elapsed, time.Duration(n)*strat.notifyDelay)
	}
	for _, mt := range tasks {
		if mt.GetState() != TaskStatePaused {
			t.Fatalf("任务 %d 应被暂停: %s", mt.taskId, taskStateName(mt.GetState()))
		}
	}
}
