package taskManager

// 控制面核心机制单测：actor 命令通道与首派守卫、批量控制命令投递、命令监听的阶段感知暂停
// 分流（可排空阶段标志/软暂停广播/drain 超时兜底）与执行期信号重置。

import (
	"context"
	"database/sql"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"

	"go.uber.org/zap"
)

// TestMain 全局初始化 no-op logger,使所有测试中 setState/命令处理等日志调用安全(logger.Log 默认 nil)
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

// newTestManagedTask 构造最小可测 ManagedTask(ctx/cmdCh/state/done/task),不启动 actor
// (供命令监听/状态机单元测试)
func newTestManagedTask() *ManagedTask {
	ctx, cancel := context.WithCancel(context.Background())
	m := &ManagedTask{
		taskId:         1,
		ctx:            ctx,
		cancel:         cancel,
		runCtx:         ctx, // 单元测试用 runCtx(正式环境由 handleRunCmd 派生)
		runCancel:      cancel,
		cmdCh:          make(chan taskCmd, 8),
		actorDone:      make(chan struct{}),
		ackWaitTimeout: ackWaitTimeoutDefault,
		done:           make(chan struct{}),
		confirmCh:      make(chan replaceConfirmResult, 1),
		softPauseCh:    make(chan struct{}),
		task:           entity.NewTask(),
	}
	m.state.Store(int32(TaskStateProcessing))
	return m
}

// TestDispatch_Exclusive 回归:dispatch 的 actorStarted CAS 保证"一任务一 actor"。
// 首次 dispatch 成功(actorStarted false→true + 投 cmdStart);重复 dispatch 幂等返回 false。
func TestDispatch_Exclusive(t *testing.T) {
	mgr := NewManager(2, nil, nil, nil, nil, nil, nil)
	defer func() {
		close(mgr.closeCh)
		<-mgr.flushDone
	}()

	task := newTestManagedTask()
	// 不启动 actor:本测试只验证 actorStarted CAS 幂等,不实际消费 cmdCh

	if !mgr.dispatch(task) {
		t.Fatal("首次 dispatch 应 claim 成功")
	}
	if mgr.dispatch(task) {
		t.Fatal("已 dispatched(actorStarted=true 且非 Paused/Pausing),重复 dispatch 应返回 false(幂等)")
	}
}

// TestNewManagedTask_ActorStartedZero 回归:NewManagedTask 不得对 actorStarted 赋值。
// actorStarted 是 dispatch 的 CAS(false→true) 首派守卫(初值须为 false);创建期 Store(true)
// 会使守卫永远失败、cmdStart 不投递,新任务卡死在 Created 永不执行。
// 本测试经 NewManagedTask 构造(生产路径),区别于 newTestManagedTask 的字面量构造——
// 后者绕过 NewManagedTask,无法捕获创建期的错误赋值(正是此前 bug 的潜伏原因)。
func TestNewManagedTask_ActorStartedZero(t *testing.T) {
	mgr := NewManager(2, nil, nil, nil, nil, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	task := entity.NewTask()
	task.TaskName = sql.NullString{String: "t", Valid: true}

	mt := NewManagedTask(1, 0, task, nil, mgr, make(chan struct{}, 1))
	mt.cancel()
	<-mt.actorDone

	if mt.actorStarted.Load() {
		t.Fatal("NewManagedTask 后 actorStarted 必须为 false(零值):它是 dispatch 的 CAS 首派守卫,创建期赋 true 会令新任务永不启动")
	}
}

// TestPauseTaskTree_PostsCmdPause 回归:PauseTaskTree 对非终态子任务投 cmdPause(actor 命令队列保证 pause 覆盖陈旧 resume)。
// 本测试验证 cmdPause 被投递到子任务 cmdCh。
func TestPauseTaskTree_PostsCmdPause(t *testing.T) {
	mgr := NewManager(2, nil, nil, nil, nil, nil, nil)
	defer func() {
		close(mgr.closeCh)
		<-mgr.flushDone
	}()

	child := newTestManagedTask()
	child.setState(TaskStatePaused)

	parent := NewParentTask(254, "parent")
	parent.AddChild(child)
	mgr.mu.Lock()
	mgr.parentMap[254] = parent
	mgr.mu.Unlock()

	if err := mgr.PauseTaskTrees(context.Background(), []int64{254}); err != nil {
		t.Fatalf("PauseTaskTrees 失败: %v", err)
	}

	// cmdPause 应被投递到 child.cmdCh(不启动 actor,直接读 channel)
	select {
	case cmd := <-child.cmdCh:
		if cmd.kind != cmdPause {
			t.Fatalf("期望 cmdPause, 实际 %d", cmd.kind)
		}
	default:
		t.Fatal("PauseTaskTree 应投递 cmdPause 到子任务")
	}
}

// TestCmdWatcher_PauseStageAware 验证 cmdWatcher 对 cmdPause 的阶段感知分流:
// 可排空阶段(下载循环内,drainPhase=true)走优雅暂停(软暂停广播 + drainTimer,不取消 runCtx);
// 其余阶段立即 runCancel(无在途数据块,快速中断)。cmdStop 不论阶段都立即取消。
func TestCmdWatcher_PauseStageAware(t *testing.T) {
	// 可排空阶段:cmdPause 走优雅暂停——广播软暂停 + drainTimer,不取消 runCtx
	m := newTestManagedTask()
	m.drainPhase.Store(true) // 模拟执行面已上报进入下载循环
	stop := make(chan struct{})
	go m.cmdWatcher(stop)

	m.cmdCh <- taskCmd{kind: cmdPause}
	time.Sleep(50 * time.Millisecond) // 等 watcher 处理命令

	if !m.softPauseBroadcast() {
		t.Fatal("可排空阶段 cmdPause 应广播软暂停")
	}
	if m.runCtx.Err() != nil {
		t.Fatal("可排空阶段 cmdPause(优雅暂停)不应立即取消 runCtx")
	}
	if m.drainTimer == nil {
		t.Fatal("可排空阶段 cmdPause 应启动 drainTimer")
	}
	m.drainTimer.Stop() // 防 2s 后触发干扰后续断言
	close(stop)

	// 其余阶段:cmdPause 立即 runCancel(无在途数据块,快速中断)
	m2 := newTestManagedTask() // drainPhase 保持 false
	stop2 := make(chan struct{})
	go m2.cmdWatcher(stop2)
	m2.cmdCh <- taskCmd{kind: cmdPause}
	time.Sleep(50 * time.Millisecond)
	if m2.runCtx.Err() == nil {
		t.Fatal("其余阶段 cmdPause 应立即取消 runCtx(快速中断)")
	}
	if m2.softPauseBroadcast() {
		t.Fatal("其余阶段 cmdPause 不应广播软暂停(走 runCancel,非 drain)")
	}
	close(stop2)

	// cmdStop:不论阶段都立即取消 runCtx
	m3 := newTestManagedTask()
	m3.drainPhase.Store(true) // 即使可排空阶段,cmdStop 也立即取消
	stop3 := make(chan struct{})
	go m3.cmdWatcher(stop3)
	m3.cmdCh <- taskCmd{kind: cmdStop}
	time.Sleep(50 * time.Millisecond)
	if m3.runCtx.Err() == nil {
		t.Fatal("cmdStop 应立即取消 runCtx(不论阶段)")
	}
	close(stop3)
}

// TestForceCancelDrainStuck drain 超时兜底:软暂停已广播且在途迟迟不落盘时强制取消 runCtx
// (退化为有损立即暂停);软暂停未广播(误触)不取消
func TestForceCancelDrainStuck(t *testing.T) {
	// 软暂停已广播 → 强制取消
	m := newTestManagedTask()
	m.broadcastSoftPause()
	m.forceCancelDrainStuck()
	if m.runCtx.Err() == nil {
		t.Fatal("软暂停已广播时兜底应强制取消 runCtx")
	}

	// 软暂停未广播 → 不取消
	m2 := newTestManagedTask()
	m2.forceCancelDrainStuck()
	if m2.runCtx.Err() != nil {
		t.Fatal("软暂停未广播时兜底不应取消 runCtx(误触)")
	}
}

// TestResetRunSignals 执行期信号重置:排空阶段标志清零、软暂停广播通道重建(已 close 的通道
// 重建后恢复可广播)、drain 兜底定时器停用——上一轮执行遗留的信号不得影响新一轮执行
func TestResetRunSignals(t *testing.T) {
	m := newTestManagedTask()
	m.drainPhase.Store(true)
	m.broadcastSoftPause()
	timerFired := &atomic.Bool{}
	m.drainTimer = time.AfterFunc(time.Hour, func() { timerFired.Store(true) })

	m.resetRunSignals()

	if m.drainPhase.Load() {
		t.Fatal("重置应清零可排空阶段标志")
	}
	if m.softPauseBroadcast() {
		t.Fatal("重置应重建软暂停广播通道(未广播态)")
	}
	if m.drainTimer != nil {
		t.Fatal("重置应清空 drainTimer")
	}
	if timerFired.Load() {
		t.Fatal("重置应停用旧 drainTimer")
	}
}
