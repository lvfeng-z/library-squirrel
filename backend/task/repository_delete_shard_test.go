package task

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/util"
)

// 本文件为任务删除链 id 列表分片回归（live-test 缺陷 7：净室 C 清理 68,389 子行删除
// 「too many SQL variables」整链失败——SQLite SQLITE_MAX_VARIABLE_NUMBER=32766 上限下，
// 全 id IN 列表超限即语句编译失败。修复=全 IN 圈定按 deleteIDShardSize 分片循环执行，
// 事务边界不变）。两形态：
//   - 真上限规模：3.3 万子行（>32766）按生产分片 4000 删除零残留（修复前该规模必红）
//   - 小上限注入：分片缩至 7 删 50 子行零残留（跨分片边界含余数片的循环正确性）

// TestShardInt64IDs 分片纯函数边界：空表、不足一片、恰好整片、整片+余数、size 非法退化单元素片
func TestShardInt64IDs(t *testing.T) {
	ids := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	cases := []struct {
		size  int
		want  [][]int64
		descr string
	}{
		{3, [][]int64{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}, {10}}, "整片+余数片"},
		{10, [][]int64{{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}, "恰好一片"},
		{11, [][]int64{{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}, "不足一片"},
		{0, [][]int64{{1}, {2}, {3}, {4}, {5}, {6}, {7}, {8}, {9}, {10}}, "size 非法退化单元素片"},
	}
	for _, c := range cases {
		got := shardInt64IDs(ids, c.size)
		if len(got) != len(c.want) {
			t.Fatalf("%s：期望 %d 片，得到 %d 片", c.descr, len(c.want), len(got))
		}
		for i := range got {
			if len(got[i]) != len(c.want[i]) {
				t.Fatalf("%s：第 %d 片期望 %d 元素，得到 %d", c.descr, i, len(c.want[i]), len(got[i]))
			}
		}
	}
	if got := shardInt64IDs(nil, 4); len(got) != 0 {
		t.Fatalf("空表应返回零片，得到 %d 片", len(got))
	}
}

// TestDeleteTaskBeyondSQLiteVariableLimit 真上限规模回归（缺陷 7 锚）：父+33,000 子
//（>SQLITE_MAX_VARIABLE_NUMBER=32766）按生产分片 4000 循环删除——UI 全选子行批量删除的
// 实测形态（净室 C：68,389 子行 IN 列表超限失败）。断言 task/work_task 零残留；
// 修复前该规模在 work_task 领域行删除语句即报「too many SQL variables」必红。
func TestDeleteTaskBeyondSQLiteVariableLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	wtStore := newTestWorkTaskStore(db)
	repo := NewRepository(db, wtStore, wtStore)
	svc := NewService(repo, &testTransactor{db: db}, nil, nil, nil, func() string { return t.TempDir() }, nil)

	parent := domain.NewTask()
	parent.TaskName = sql.NullString{String: "大选择集 · 分片删除真上限规模", Valid: true}
	parent.HasChild = sql.NullBool{Bool: true, Valid: true}
	if err := db.Create(parent).Error; err != nil {
		t.Fatalf("插父任务失败: %v", err)
	}
	parentWt := domain.NewWorkTask(parent.GetID())
	parentWt.SetCreateTime(util.GetCurrentTimestamp())
	if err := db.Create(parentWt).Error; err != nil {
		t.Fatalf("插父领域行失败: %v", err)
	}

	const childCount = 33000 // > 32766：未分片的 IN 列表必超 SQLite 绑定变量上限
	tasks := make([]*domain.Task, childCount)
	for i := range tasks {
		tasks[i] = domain.NewTask()
		tasks[i].TaskName = sql.NullString{String: fmt.Sprintf("子任务-%06d", i), Valid: true}
		tasks[i].Pid = sql.NullInt64{Int64: parent.GetID(), Valid: true}
	}
	// 核心行分批插入拿自增 id，再按共享主键建领域行
	for start := 0; start < childCount; start += 2000 {
		end := start + 2000
		if end > childCount {
			end = childCount
		}
		if err := db.CreateInBatches(tasks[start:end], 2000).Error; err != nil {
			t.Fatalf("批量插子任务失败（%d..%d）: %v", start, end, err)
		}
	}
	wts := make([]*domain.WorkTask, childCount)
	for i := range tasks {
		wts[i] = domain.NewWorkTask(tasks[i].GetID())
		wts[i].SetCreateTime(util.GetCurrentTimestamp())
	}
	for start := 0; start < childCount; start += 2000 {
		end := start + 2000
		if end > childCount {
			end = childCount
		}
		if err := db.CreateInBatches(wts[start:end], 2000).Error; err != nil {
			t.Fatalf("批量插子领域行失败（%d..%d）: %v", start, end, err)
		}
	}

	// UI 全选子行批量删除形态：入参即全量子行 id（33,000 个）+父——ClearResourceTaskId 与
	// DeleteTask 的全 IN 圈定均超上限，须各自分片
	allIDs := make([]int64, 0, childCount+1)
	allIDs = append(allIDs, parent.GetID())
	for _, tk := range tasks {
		allIDs = append(allIDs, tk.GetID())
	}
	if err := svc.DeleteTask(context.Background(), allIDs); err != nil {
		t.Fatalf("删除 %d 行任务失败（分片前此规模报 too many SQL variables）: %v", len(allIDs), err)
	}

	assertTableCount(t, db, "task", 0)
	assertTableCount(t, db, "work_task", 0)
}

// TestDeleteTaskShardInjectionSmall 小上限注入：deleteIDShardSize 缩至 7（50 子行=8 片含余数片），
// 验证分片循环的边界正确性（首片/中间片/余数片全覆盖、零残留）
func TestDeleteTaskShardInjectionSmall(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	origShard := deleteIDShardSize
	deleteIDShardSize = 7
	t.Cleanup(func() { deleteIDShardSize = origShard })

	wtStore := newTestWorkTaskStore(db)
	repo := NewRepository(db, wtStore, wtStore)
	svc := NewService(repo, &testTransactor{db: db}, nil, nil, nil, func() string { return t.TempDir() }, nil)

	parent := domain.NewTask()
	parent.TaskName = sql.NullString{String: "分片注入 · 父", Valid: true}
	parent.HasChild = sql.NullBool{Bool: true, Valid: true}
	if err := db.Create(parent).Error; err != nil {
		t.Fatalf("插父任务失败: %v", err)
	}
	const childCount = 50 // 7 片整 + 1 余数片（50/7=7 余 1）
	for i := 0; i < childCount; i++ {
		tk := domain.NewTask()
		tk.TaskName = sql.NullString{String: fmt.Sprintf("子-%02d", i), Valid: true}
		tk.Pid = sql.NullInt64{Int64: parent.GetID(), Valid: true}
		if err := db.Create(tk).Error; err != nil {
			t.Fatalf("插子任务失败: %v", err)
		}
		wt := domain.NewWorkTask(tk.GetID())
		wt.SetCreateTime(util.GetCurrentTimestamp())
		if err := db.Create(wt).Error; err != nil {
			t.Fatalf("插子领域行失败: %v", err)
		}
	}

	// 删父（子行随链消亡）：allIds=51 → 8 片循环
	if err := svc.DeleteTask(context.Background(), []int64{parent.GetID()}); err != nil {
		t.Fatalf("分片删除失败: %v", err)
	}
	assertTableCount(t, db, "task", 0)
	assertTableCount(t, db, "work_task", 0)
}
