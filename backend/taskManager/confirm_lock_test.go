package taskManager

// 替换确认投递前置作品锁预检测试：replace 答复遇涉及作品被分享拉取持有时同步返回
// shareLock.ErrWorkLocked、不摘确认条目不投递；skip 答复不查锁；强制解锁后重发答复放行。
// 涉及作品=插件任务的替换定位作品（existingWorkId）与策略任务等待时记录的冲突作品集合

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/library-squirrel/backend/shareLock"
)

// newPluginConfirmTask 构造等待确认的插件型任务（strategy 为 nil，确认走命令通道投递），
// 不启动 actor——命令缓冲在 cmdCh 内，此处只验证确认面的查锁与条目增删
func newPluginConfirmTask(mgr *Manager, taskId, existingWorkId int64) *ManagedTask {
	m := newTestManagedTask()
	m.taskId = taskId
	m.manager = mgr
	m.existingWorkId = existingWorkId
	mgr.enqueueWaitingForInput(m)
	return m
}

// inWaitingTable 查询任务是否仍在等待确认表
func inWaitingTable(mgr *Manager, taskId int64) bool {
	mgr.waitingForInputMu.Lock()
	defer mgr.waitingForInputMu.Unlock()
	_, ok := mgr.waitingForInputMap[taskId]
	return ok
}

// TestConfirmReplaceRejectsLockedWork 插件任务替换定位作品被锁：replace 拒绝且任务留在确认表；
// skip 不查锁直接放行；强制解锁后 replace 放行并摘条目
func TestConfirmReplaceRejectsLockedWork(t *testing.T) {
	lock := shareLock.NewShareLockRegistry()
	mgr := NewManager(2, nil, nil, nil, &TaskDeps{Pusher: &fakePusher{}, WorkLockChecker: lock}, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()
	newPluginConfirmTask(mgr, 1, 500)

	lock.Register(context.Background(), []int64{500}, "session-a")
	if err := mgr.ConfirmReplace(1, "replace"); !errors.Is(err, shareLock.ErrWorkLocked) {
		t.Fatalf("锁命中应返回 shareLock.ErrWorkLocked，实际 %v", err)
	}
	if !inWaitingTable(mgr, 1) {
		t.Fatal("锁命中任务应留在等待确认表（不摘条目、不投递）")
	}

	// skip 答复不动作品资源，锁命中也直接放行
	if err := mgr.ConfirmReplace(2, "skip"); !errors.Is(err, ErrTaskTreeNotFound) {
		t.Fatalf("未在确认表的任务应返回 ErrTaskTreeNotFound，实际 %v", err)
	}
	if err := mgr.ConfirmReplace(1, "skip"); err != nil {
		t.Fatalf("skip 答复不应查锁，实际失败: %v", err)
	}
	if inWaitingTable(mgr, 1) {
		t.Fatal("skip 放行后应摘确认条目")
	}

	// 强制解锁后重新入表重发 replace：放行
	newPluginConfirmTask(mgr, 1, 500)
	lock.Register(context.Background(), []int64{500}, "session-b")
	if err := mgr.ConfirmReplace(1, "replace"); !errors.Is(err, shareLock.ErrWorkLocked) {
		t.Fatalf("再次锁命中应返回 shareLock.ErrWorkLocked，实际 %v", err)
	}
	lock.ForceUnlock(context.Background(), 500)
	if err := mgr.ConfirmReplace(1, "replace"); err != nil {
		t.Fatalf("解锁后 replace 应放行，实际失败: %v", err)
	}
	if inWaitingTable(mgr, 1) {
		t.Fatal("解锁后 replace 放行应摘确认条目")
	}
}

// TestConfirmReplaceRejectsLockedConflictWork 策略任务经真实 WaitReplaceConfirm 记录冲突作品
// 集合：冲突作品被锁时 replace 拒绝且等待不解除；强制解锁后重发答复，等待返回替换决策
func TestConfirmReplaceRejectsLockedConflictWork(t *testing.T) {
	lock := shareLock.NewShareLockRegistry()
	mgr := NewManager(2, nil, nil, nil, &TaskDeps{Pusher: &fakePusher{}, WorkLockChecker: lock}, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	m := newTestManagedTask()
	m.task = newBuiltinTask(1, "demo")
	m.manager = mgr
	m.semaphore = mgr.semaphore
	m.confirmCh = make(chan replaceConfirmResult, 1)
	m.strategy = &stubStrategy{}
	m.deps = &TaskDeps{Pusher: &fakePusher{}, WorkLockChecker: lock}
	h := newStrategyHandle(m)

	var decision ReplaceDecision
	done := make(chan struct{})
	go func() {
		decision, _ = h.WaitReplaceConfirm([]ConflictInfo{{WorkID: 300, WorkName: "作品A"}})
		close(done)
	}()
	waitRegistered(t, mgr, 1)

	lock.Register(context.Background(), []int64{300}, "session-a")
	if err := mgr.ConfirmReplace(1, "replace"); !errors.Is(err, shareLock.ErrWorkLocked) {
		t.Fatalf("冲突作品锁命中应返回 shareLock.ErrWorkLocked，实际 %v", err)
	}
	if !inWaitingTable(mgr, 1) {
		t.Fatal("锁命中任务应留在等待确认表，等待不解除")
	}
	select {
	case <-done:
		t.Fatal("锁命中不应投递答复，等待不应返回")
	case <-time.After(100 * time.Millisecond):
	}

	// 强制解锁后重发答复：等待被唤醒并返回替换决策
	lock.ForceUnlock(context.Background(), 300)
	if err := mgr.ConfirmReplace(1, "replace"); err != nil {
		t.Fatalf("解锁后 replace 应放行，实际失败: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("解锁后确认应唤醒等待")
	}
	if decision != ReplaceDecisionReplace {
		t.Fatalf("等待应返回替换决策，实际 %v", decision)
	}
}

// TestConfirmReplaceBatchRejectsLockedWork 批量 replace 任一涉及作品被锁：整体不投递
// （全部留在确认表）；强制解锁后重发整批放行
func TestConfirmReplaceBatchRejectsLockedWork(t *testing.T) {
	lock := shareLock.NewShareLockRegistry()
	mgr := NewManager(2, nil, nil, nil, &TaskDeps{Pusher: &fakePusher{}, WorkLockChecker: lock}, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()
	newPluginConfirmTask(mgr, 1, 500)
	newPluginConfirmTask(mgr, 2, 600)

	lock.Register(context.Background(), []int64{600}, "session-a")
	if err := mgr.ConfirmReplaceBatch([]int64{1, 2}, "replace"); !errors.Is(err, shareLock.ErrWorkLocked) {
		t.Fatalf("整批含被锁作品应返回 shareLock.ErrWorkLocked，实际 %v", err)
	}
	if !inWaitingTable(mgr, 1) || !inWaitingTable(mgr, 2) {
		t.Fatal("整批拒绝时全部任务应留在等待确认表")
	}

	lock.ForceUnlock(context.Background(), 600)
	if err := mgr.ConfirmReplaceBatch([]int64{1, 2}, "replace"); err != nil {
		t.Fatalf("解锁后整批 replace 应放行，实际失败: %v", err)
	}
	if inWaitingTable(mgr, 1) || inWaitingTable(mgr, 2) {
		t.Fatal("解锁后整批放行应摘全部确认条目")
	}
}
