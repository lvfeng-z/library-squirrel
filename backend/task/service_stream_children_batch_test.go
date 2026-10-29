package task

import (
	"context"
	"errors"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// 本文件为流式创建的子行批次块（children_batch，契约 21）聚合与协议违规测试：
//   - 指称聚合：批次块按 plugin_task_id 挂既有父，last=true 即收批；内联子行与批次子行都计入该父。
//   - 多任务交错：不同 PluginTaskId 的 task 块交错发送也各归其父（不依赖块位置）。
//   - 三类协议违规收流：悬空引用（父 ID 未出现过或指向叶块）、收批后再到批次、流结束缺 last——
//     计失败项并丢弃后续块，已到父行与子行保留（部分完成语义）。

// batchItem 构造子行批次块条目。
func batchItem(pluginTaskId string, last bool, children ...*sdkdto.TaskCreateChildResponse) *sdkdto.CreateStreamItem {
	return sdkdto.ChildrenBatchItem(pluginTaskId, children, last)
}

// childOf 构造单个子行声明。
func childOf(name, siteWorkId string) *sdkdto.TaskCreateChildResponse {
	return &sdkdto.TaskCreateChildResponse{TaskName: name, SiteWorkId: siteWorkId, Url: "http://x/" + siteWorkId, ResourceType: entity.ResourceTypeImage}
}

// parentBlockOf 构造携带内联子行的父任务块。
func parentBlockOf(pluginTaskId, name string, children ...*sdkdto.TaskCreateChildResponse) *sdkdto.TaskCreateResponse {
	return &sdkdto.TaskCreateResponse{
		PluginTaskId: pluginTaskId, TaskName: name, Url: "http://x/" + pluginTaskId, SiteKey: testSiteKey,
		Children: children,
	}
}

// drainStreamFull 耗尽 stream 输出通道，分别收集 Task 项、Parent 项与 Error 项。
func drainStreamFull(out <-chan *CreateTaskStreamChan) (tasks int, parents int, errs []error) {
	for item := range out {
		switch {
		case item.Task != nil:
			tasks++
		case item.Parent != nil:
			parents++
		case item.Error != nil:
			errs = append(errs, item.Error)
		}
	}
	return
}

// taskNamesInOrder 按落盘顺序取全部核心行任务名。
func taskNamesInOrder(ts []*entity.Task) []string {
	names := make([]string, 0, len(ts))
	for _, t := range ts {
		names = append(names, t.TaskName.String)
	}
	return names
}

// TestHandleCreateTaskStream_InterleavedParentsMergeByReference 多任务交错流：不同 PluginTaskId 的
// task 块交错发送，各归其父——A 的两个块（中间隔着 B 的块）合并为同一父行，子行总数全等且序保持。
func TestHandleCreateTaskStream_InterleavedParentsMergeByReference(t *testing.T) {
	svc, repo := newTestService(t)
	in := taskItemChan(
		parentBlockOf("work-A", "work-A", childOf("a-1", "c-a1")),
		parentBlockOf("work-B", "work-B", childOf("b-1", "c-b1")),
		parentBlockOf("work-A", "work-A", childOf("a-2", "c-a2"), childOf("a-3", "c-a3")),
	)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	tasks, parents, errs := drainStreamFull(out)
	if len(errs) != 0 {
		t.Fatalf("期望零错误项，得到 %v", errs)
	}
	if parents != 2 || tasks != 4 {
		t.Fatalf("期望 2 个 Parent 项 + 4 个 Task 项（A:3 子 + B:1 子），得到 parents=%d tasks=%d", parents, tasks)
	}
	if got := countParents(repo.tasks); got != 2 {
		t.Fatalf("期望 2 个父任务行（A 合并、B 独立），得到 %d", got)
	}
	if len(repo.tasks) != 6 {
		t.Fatalf("期望落盘 6 行（2 父 + 4 子），得到 %d", len(repo.tasks))
	}

	// 父行身份与子行归属：A 的 3 子指向同一父行，B 的 1 子指向另一父行
	parentOfChild := make(map[string]int64) // 子任务名 → 父行 ID
	for _, tk := range repo.tasks {
		if tk.HasChild.Valid && tk.HasChild.Bool {
			continue
		}
		if !tk.Pid.Valid {
			t.Fatalf("子任务 %s 缺少 Pid", tk.TaskName.String)
		}
		parentOfChild[tk.TaskName.String] = tk.Pid.Int64
	}
	if parentOfChild["a-1"] != parentOfChild["a-2"] || parentOfChild["a-1"] != parentOfChild["a-3"] {
		t.Fatalf("A 的三个子任务应指向同一父行，得到映射 %+v", parentOfChild)
	}
	if parentOfChild["b-1"] == parentOfChild["a-1"] {
		t.Fatalf("B 的子任务不应指向 A 的父行，得到映射 %+v", parentOfChild)
	}

	// 序保持：子行按到达顺序落盘（批量缓存按同序 flush），父行在各自块到达时即时落盘
	// （work-A 先于 work-B）
	childNames := make([]string, 0, 4)
	parentNames := make([]string, 0, 2)
	for _, tk := range repo.tasks {
		if tk.HasChild.Valid && tk.HasChild.Bool {
			parentNames = append(parentNames, tk.TaskName.String)
		} else {
			childNames = append(childNames, tk.TaskName.String)
		}
	}
	wantChildren := []string{"a-1", "b-1", "a-2", "a-3"}
	for i := range wantChildren {
		if childNames[i] != wantChildren[i] {
			t.Fatalf("子行落盘顺序应保持到达顺序：期望 %v，得到 %v", wantChildren, childNames)
		}
	}
	wantParents := []string{"work-A", "work-B"}
	for i := range wantParents {
		if parentNames[i] != wantParents[i] {
			t.Fatalf("父行落盘顺序应保持块到达顺序：期望 %v，得到 %v", wantParents, parentNames)
		}
	}
}

// TestHandleCreateTaskStream_InlineAndBatchMixedCounted 同一父的内联子行与批次子行都计入该父：
// 父块内联 1 子 + 批次块 2 子（末批 last=true）→ 父行单条、子行合计 3、全部指向该父。
func TestHandleCreateTaskStream_InlineAndBatchMixedCounted(t *testing.T) {
	svc, repo := newTestService(t)
	in := make(chan *sdkdto.CreateStreamItem, 2)
	in <- sdkdto.TaskItem(parentBlockOf("work-X", "work-X", childOf("inline-1", "c-1")))
	in <- batchItem("work-X", true, childOf("batch-1", "c-2"), childOf("batch-2", "c-3"))
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	tasks, parents, errs := drainStreamFull(out)
	if len(errs) != 0 {
		t.Fatalf("期望零错误项，得到 %v", errs)
	}
	if parents != 1 || tasks != 3 {
		t.Fatalf("期望 1 个 Parent 项 + 3 个 Task 项（内联 1 + 批次 2），得到 parents=%d tasks=%d", parents, tasks)
	}
	parent := findParent(repo.tasks)
	if parent == nil {
		t.Fatal("未找到 HasChild=true 的父任务")
	}
	assertChildrenPid(t, repo.tasks, parent.GetID(), 3)
	want := []string{"work-X", "inline-1", "batch-1", "batch-2"}
	got := taskNamesInOrder(repo.tasks)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("落盘顺序不符（内联先于批次）：期望 %v，得到 %v", want, got)
		}
	}
}

// TestHandleCreateTaskStream_MultiBatchBlocksSameParent 多批次块同父合并：父块（首批内联）+
// 两个批次块（非末批 + 末批）→ 父行单条、子行总数全等（1+2+1）、序保持。
func TestHandleCreateTaskStream_MultiBatchBlocksSameParent(t *testing.T) {
	svc, repo := newTestService(t)
	in := make(chan *sdkdto.CreateStreamItem, 3)
	in <- sdkdto.TaskItem(parentBlockOf("work-X", "work-X", childOf("inline-1", "c-1")))
	in <- batchItem("work-X", false, childOf("batch-1", "c-2"), childOf("batch-2", "c-3"))
	in <- batchItem("work-X", true, childOf("batch-3", "c-4"))
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	tasks, parents, errs := drainStreamFull(out)
	if len(errs) != 0 {
		t.Fatalf("期望零错误项，得到 %v", errs)
	}
	if parents != 1 || tasks != 4 {
		t.Fatalf("期望 1 个 Parent 项 + 4 个 Task 项（1+2+1），得到 parents=%d tasks=%d", parents, tasks)
	}
	if got := countParents(repo.tasks); got != 1 {
		t.Fatalf("期望父行单条，得到 %d", got)
	}
	parent := findParent(repo.tasks)
	assertChildrenPid(t, repo.tasks, parent.GetID(), 4)
	want := []string{"work-X", "inline-1", "batch-1", "batch-2", "batch-3"}
	got := taskNamesInOrder(repo.tasks)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("落盘顺序不符：期望 %v，得到 %v", want, got)
		}
	}
}

// TestHandleCreateTaskStream_BatchDanglingReference 悬空引用收流：批次块指称的父 ID 未出现过
// → 协议违规计失败项，流上后续块丢弃（其后 task 块不落盘），违规前已到的父行与子行保留。
func TestHandleCreateTaskStream_BatchDanglingReference(t *testing.T) {
	svc, repo := newTestService(t)
	in := make(chan *sdkdto.CreateStreamItem, 3)
	in <- sdkdto.TaskItem(parentBlockOf("work-A", "work-A", childOf("a-1", "c-a1")))
	in <- batchItem("work-unknown", true, childOf("dangling-1", "c-d1"))
	in <- sdkdto.TaskItem(parentBlockOf("work-B", "work-B", childOf("b-1", "c-b1")))
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	tasks, parents, errs := drainStreamFull(out)
	if len(errs) != 1 || !errors.Is(errs[0], ErrChildrenBatchDanglingParent) {
		t.Fatalf("期望恰 1 个悬空引用错误项，得到 %v", errs)
	}
	if parents != 1 || tasks != 1 {
		t.Fatalf("违规前应透出 1 Parent + 1 Task，得到 parents=%d tasks=%d", parents, tasks)
	}
	// 部分完成语义：违规前的父行与子行保留，违规后的 work-B 不落盘
	if len(repo.tasks) != 2 {
		t.Fatalf("期望落盘 2 行（父A + a-1），得到 %d: %v", len(repo.tasks), taskNamesInOrder(repo.tasks))
	}
	if names := taskNamesInOrder(repo.tasks); names[0] != "work-A" || names[1] != "a-1" {
		t.Fatalf("期望仅保留违规前落盘行，得到 %v", names)
	}
}

// TestHandleCreateTaskStream_BatchReferencingLeafIsDangling 批次块指向独立叶任务（叶块不进指称表）
// → 同样按悬空引用收流。
func TestHandleCreateTaskStream_BatchReferencingLeafIsDangling(t *testing.T) {
	svc, repo := newTestService(t)
	leaf := &sdkdto.TaskCreateResponse{
		PluginTaskId: "leaf-1", TaskName: "leaf-1", SiteWorkId: "w-1", Url: "http://x/1",
		SiteKey: testSiteKey, ResourceType: entity.ResourceTypeImage,
	}
	in := make(chan *sdkdto.CreateStreamItem, 2)
	in <- sdkdto.TaskItem(leaf)
	in <- batchItem("leaf-1", true, childOf("child-1", "c-1"))
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	tasks, _, errs := drainStreamFull(out)
	if len(errs) != 1 || !errors.Is(errs[0], ErrChildrenBatchDanglingParent) {
		t.Fatalf("期望指向叶块按悬空引用收流，得到 %v", errs)
	}
	if tasks != 1 || len(repo.tasks) != 1 {
		t.Fatalf("叶块自身照常落盘（1 行），得到 tasks=%d 落盘=%d", tasks, len(repo.tasks))
	}
}

// TestHandleCreateTaskStream_BatchAfterLastViolates 收批后再到批次：同父 last 末批已到，其后
// 的批次块按协议违规收流，末批前的子行保留。
func TestHandleCreateTaskStream_BatchAfterLastViolates(t *testing.T) {
	svc, repo := newTestService(t)
	in := make(chan *sdkdto.CreateStreamItem, 3)
	in <- sdkdto.TaskItem(parentBlockOf("work-X", "work-X", childOf("inline-1", "c-0")))
	in <- batchItem("work-X", true, childOf("batch-1", "c-1"))
	in <- batchItem("work-X", true, childOf("late-1", "c-2"))
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	tasks, _, errs := drainStreamFull(out)
	if len(errs) != 1 || !errors.Is(errs[0], ErrChildrenBatchAfterLastBatch) {
		t.Fatalf("期望恰 1 个收批后重复批次错误项，得到 %v", errs)
	}
	if tasks != 2 || len(repo.tasks) != 3 {
		t.Fatalf("期望末批前子行保留（父+2 子），得到 tasks=%d 落盘=%d", tasks, len(repo.tasks))
	}
}

// TestHandleCreateTaskStream_BatchMissingLastViolatesAtStreamEnd 流结束缺末批：父已开批但至流
// 关闭未收到 last → 协议违规计失败项；已到子行照常落盘（部分完成语义）。
func TestHandleCreateTaskStream_BatchMissingLastViolatesAtStreamEnd(t *testing.T) {
	svc, repo := newTestService(t)
	in := make(chan *sdkdto.CreateStreamItem, 2)
	in <- sdkdto.TaskItem(parentBlockOf("work-X", "work-X", childOf("inline-1", "c-1")))
	in <- batchItem("work-X", false, childOf("batch-1", "c-2"))
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	tasks, _, errs := drainStreamFull(out)
	if len(errs) != 1 || !errors.Is(errs[0], ErrChildrenBatchMissingLastBatch) {
		t.Fatalf("期望恰 1 个缺末批错误项，得到 %v", errs)
	}
	if tasks != 2 {
		t.Fatalf("期望内联 1 子 + 批次 1 子均透出，得到 tasks=%d", tasks)
	}
	// 缺 last 不抹除已到行：父 + 2 子全部落盘
	if len(repo.tasks) != 3 {
		t.Fatalf("期望落盘 3 行（父 + 2 子），得到 %d: %v", len(repo.tasks), taskNamesInOrder(repo.tasks))
	}
	parent := findParent(repo.tasks)
	assertChildrenPid(t, repo.tasks, parent.GetID(), 2)
}

// TestHandleCreateTaskStream_EmptyPluginTaskIdParentBlocksIndependent 空 PluginTaskId 的父块
// 无可指称键：每个父块独立建行、互不合并（空 ID 与任何块都不构成同父）。
func TestHandleCreateTaskStream_EmptyPluginTaskIdParentBlocksIndependent(t *testing.T) {
	svc, repo := newTestService(t)
	in := taskItemChan(
		parentBlockOf("", "work-A", childOf("a-1", "c-a1")),
		parentBlockOf("", "work-B", childOf("b-1", "c-b1")),
	)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	tasks, parents, errs := drainStreamFull(out)
	if len(errs) != 0 {
		t.Fatalf("期望零错误项，得到 %v", errs)
	}
	if parents != 2 || tasks != 2 {
		t.Fatalf("期望 2 个独立 Parent + 2 个 Task，得到 parents=%d tasks=%d", parents, tasks)
	}
	if got := countParents(repo.tasks); got != 2 {
		t.Fatalf("空 PluginTaskId 的父块应各自独立建行（2 个），得到 %d", got)
	}
}

// TestHandleCreateTaskStream_BatchChildSiteKeyInherit 批次子行归属键两态：自带非空键按子行键
// 解析站点，空键继承父任务块声明的键（与内联子行同一判定）。
func TestHandleCreateTaskStream_BatchChildSiteKeyInherit(t *testing.T) {
	svc, repo := newCrossSiteTestService(t)
	in := make(chan *sdkdto.CreateStreamItem, 2)
	in <- sdkdto.TaskItem(parentBlockOf("work-X", "work-X", childOf("inline-1", "c-0")))
	in <- batchItem("work-X", true,
		&sdkdto.TaskCreateChildResponse{TaskName: "own", SiteWorkId: "c-1", Url: "http://x/1", SiteKey: testCrossSiteKey, ResourceType: entity.ResourceTypeImage},
		childOf("inh", "c-2"),
	)
	close(in)

	out, err := svc.handleCreateTaskStream(context.Background(), in, testListener(), 100)
	if err != nil {
		t.Fatalf("handleCreateTaskStream 返回错误: %v", err)
	}
	if _, _, errs := drainStreamFull(out); len(errs) != 0 {
		t.Fatalf("期望零错误项，得到 %v", errs)
	}
	for _, tk := range repo.tasks {
		if tk.HasChild.Valid && tk.HasChild.Bool {
			continue
		}
		wt := repo.workTaskOf(tk.GetID())
		if wt == nil || !wt.SiteID.Valid {
			t.Fatalf("子任务 %s 缺少有效 SiteID", tk.TaskName.String)
		}
		want := testSiteID // 空键继承父键（inh 与内联子行 inline-1 同态）
		if tk.TaskName.String == "own" {
			want = testCrossSiteID // own：自带子行键
		}
		if wt.SiteID.Int64 != want {
			t.Errorf("子任务 %s SiteID 应为 %d，得到 %d", tk.TaskName.String, want, wt.SiteID.Int64)
		}
	}
}
