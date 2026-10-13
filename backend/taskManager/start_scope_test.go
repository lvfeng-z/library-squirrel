package taskManager

// 树启动派发范围回归测试：父任务显式入选时派发整树（不被显式子集收窄）、
// 已加载父单元内从未派发的兄弟在后续 start 请求（叶子或父任务）时补派。
// 锚定缺陷形态：start([父,部分子]) 漏派未入选兄弟；叶子单启后再 start
// 兄弟/父任务零派发，未派发兄弟永久滞留 Created。

import (
	"context"
	"database/sql"
	"testing"
	"time"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/task"
)

// newParentTaskRow 构造父任务实体（has_child=true、根级）
func newParentTaskRow(id int64, taskType string) *domain.Task {
	t := domain.NewTask()
	t.SetID(id)
	t.TaskType = sql.NullString{String: taskType, Valid: true}
	t.TaskName = sql.NullString{String: "父任务样例", Valid: true}
	t.Status = int(TaskStateCreated)
	t.HasChild = sql.NullBool{Bool: true, Valid: true}
	return t
}

// newLeafTaskRow 构造子任务实体（pid 指向父）
func newLeafTaskRow(id int64, parentId int64, taskType string) *domain.Task {
	t := domain.NewTask()
	t.SetID(id)
	t.TaskType = sql.NullString{String: taskType, Valid: true}
	t.TaskName = sql.NullString{String: "子任务样例", Valid: true}
	t.Status = int(TaskStateCreated)
	t.Pid = sql.NullInt64{Int64: parentId, Valid: true}
	return t
}

// waitRepoStatus 轮询等待任务状态落盘（终态即时落盘路径）
func waitRepoStatus(t *testing.T, repo *fakeBuiltinRepo, taskId int64, want task.TaskStatusEnum, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if u, ok := repo.statusOf(taskId); ok && u.Status == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等待任务 %d 落盘 %v 超时", taskId, want)
}

// TestStartParentWithSubsetLeavesDispatchesWholeTree 父任务与部分子任务同时入选启动时
// 派发范围为整树——操作父任务即作用于全部子任务，显式列入的子集不得收窄派发面
// （未入选兄弟被整树加载进聚合后漏派，将永久滞留 Created 无任何途径再派发）。
func TestStartParentWithSubsetLeavesDispatchesWholeTree(t *testing.T) {
	repo := newFakeBuiltinRepo(
		newParentTaskRow(10, "demo"),
		newLeafTaskRow(1, 10, "demo"),
		newLeafTaskRow(2, 10, "demo"),
		newLeafTaskRow(3, 10, "demo"),
	)
	strat := newScriptedStrategy("finish")
	mgr := NewManager(3, repo, NewNoopEventPusher(), &TaskDeps{Pusher: NewNoopEventPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	// 父任务 + 3 子中的 2 子入选（净室实机事故同形请求）
	if err := mgr.StartTaskTrees(context.Background(), []int64{10, 1, 2}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	// 全部 3 子都应进入执行：未入选的第三子不得漏派
	for i := 0; i < 3; i++ {
		select {
		case <-strat.entered:
		case <-time.After(3 * time.Second):
			t.Fatalf("仅 %d 个子任务进入执行，整树派发被显式子集收窄（应有 3 个）", i)
		}
	}
	for i := 0; i < 3; i++ {
		strat.release <- struct{}{}
	}
	waitIdle(t, mgr, 3*time.Second)
	for _, id := range []int64{1, 2, 3} {
		if u, ok := repo.statusOf(id); !ok || u.Status != task.TaskStatusFinished {
			t.Fatalf("子任务 %d 应完成落盘: %+v ok=%v", id, u, ok)
		}
	}
}

// TestStartLeafAfterLeafOnlyStartDispatchesSibling 单独启动叶子（整树加载驻留、兄弟
// Created 未派发为既定设计）后，再次单独启动兄弟叶子应正常派发：请求叶子从未派发时
// 补投启动，而非按运行中任务跳过（滞留兄弟无任何派发途径即成死任务）。
func TestStartLeafAfterLeafOnlyStartDispatchesSibling(t *testing.T) {
	repo := newFakeBuiltinRepo(
		newParentTaskRow(10, "demo"),
		newLeafTaskRow(1, 10, "demo"),
		newLeafTaskRow(2, 10, "demo"),
	)
	strat := newScriptedStrategy("finish")
	mgr := NewManager(2, repo, NewNoopEventPusher(), &TaskDeps{Pusher: NewNoopEventPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	// 单独启动叶子 1：整树加载（兄弟 2 驻留内存 Created 未派发）
	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-strat.entered
	strat.release <- struct{}{}
	waitRepoStatus(t, repo, 1, task.TaskStatusFinished, 3*time.Second)

	// 再次单独启动从未派发的兄弟 2：应派发执行
	if err := mgr.StartTaskTrees(context.Background(), []int64{2}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	select {
	case <-strat.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("从未派发的兄弟叶子未被派发（已加载父单元的请求叶子被按运行中任务跳过）")
	}
	strat.release <- struct{}{}
	waitIdle(t, mgr, 3*time.Second)
	if u, ok := repo.statusOf(2); !ok || u.Status != task.TaskStatusFinished {
		t.Fatalf("叶子 2 应完成落盘: %+v ok=%v", u, ok)
	}
}

// TestStartParentAfterLeafOnlyStartDispatchesRemaining 单独启动叶子后再次启动父任务
// （整树请求）：已加载父单元内从未派发的兄弟应补派，不得因单元已加载而整体跳过——
// 整树语义要求全部子任务进入执行。
func TestStartParentAfterLeafOnlyStartDispatchesRemaining(t *testing.T) {
	repo := newFakeBuiltinRepo(
		newParentTaskRow(10, "demo"),
		newLeafTaskRow(1, 10, "demo"),
		newLeafTaskRow(2, 10, "demo"),
		newLeafTaskRow(3, 10, "demo"),
	)
	strat := newScriptedStrategy("finish")
	mgr := NewManager(3, repo, NewNoopEventPusher(), &TaskDeps{Pusher: NewNoopEventPusher()},
		map[string]ExecutionStrategy{"demo": strat}, nil, nil)
	defer func() { close(mgr.closeCh); <-mgr.flushDone }()

	// 单独启动叶子 1 并完成（兄弟 2/3 驻留 Created 未派发）
	if err := mgr.StartTaskTrees(context.Background(), []int64{1}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	<-strat.entered
	strat.release <- struct{}{}
	waitRepoStatus(t, repo, 1, task.TaskStatusFinished, 3*time.Second)

	// 启动父任务（整树请求）：未派发兄弟 2/3 应补派
	if err := mgr.StartTaskTrees(context.Background(), []int64{10}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-strat.entered:
		case <-time.After(3 * time.Second):
			t.Fatalf("仅 %d 个未派发兄弟被补派（整树 start 被已加载单元整体跳过，应有 2 个）", i)
		}
	}
	for i := 0; i < 2; i++ {
		strat.release <- struct{}{}
	}
	waitIdle(t, mgr, 3*time.Second)
	for _, id := range []int64{2, 3} {
		if u, ok := repo.statusOf(id); !ok || u.Status != task.TaskStatusFinished {
			t.Fatalf("子任务 %d 应完成落盘: %+v ok=%v", id, u, ok)
		}
	}
}
