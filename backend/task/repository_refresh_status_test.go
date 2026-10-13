package task

// RefreshTaskStatus 聚合刷新回归锚定：刷新仅对有子行的父任务生效（按子任务状态聚合），
// 叶子/独立任务（无子行）与全未启动子树的父任务为无操作——聚合 CASE 的
// finished=total 分支在 total=0 时退化为真，曾把任意叶子刷新成 Finished（假完成）。

import (
	"context"
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/migration"

	"gorm.io/gorm"
)

func newRefreshTaskRow(id int64, pid int64, hasChild bool, status TaskStatusEnum) *entity.Task {
	t := entity.NewTask()
	t.SetID(id)
	t.TaskName = sql.NullString{String: "刷新样例", Valid: true}
	t.TaskType = sql.NullString{String: entity.TaskTypePluginDownload, Valid: true}
	t.HasChild = sql.NullBool{Bool: hasChild, Valid: true}
	t.Status = int(status)
	if pid > 0 {
		t.Pid = sql.NullInt64{Int64: pid, Valid: true}
	}
	return t
}

func statusOrNull(t *testing.T, db *gorm.DB, id int64) sql.NullInt64 {
	t.Helper()
	var st sql.NullInt64
	if err := db.Raw("SELECT status FROM task WHERE id = ?", id).Scan(&st).Error; err != nil {
		t.Fatalf("读取任务 %d 状态失败: %v", id, err)
	}
	return st
}

// TestRefreshTaskStatusLeafNoop 叶子/独立任务无子行：刷新为无操作（0 行受影响、
// 状态原样），不得因聚合分支退化（0 完成=0 总数）被标 Finished 假完成
func TestRefreshTaskStatusLeafNoop(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	repo := NewRepository(db, nil, nil)
	ctx := context.Background()

	leaf := newRefreshTaskRow(5, 0, false, TaskStatusCreated)
	if err := db.Create(leaf).Error; err != nil {
		t.Fatalf("种叶子行失败: %v", err)
	}

	rows, err := repo.RefreshTaskStatus(ctx, 5)
	if err != nil {
		t.Fatalf("刷新叶子失败: %v", err)
	}
	if rows != 0 {
		t.Fatalf("叶子刷新应无行受影响（无子行不可聚合），实际 %d 行", rows)
	}
	if st := statusOrNull(t, db, 5); !st.Valid || TaskStatusEnum(st.Int64) != TaskStatusCreated {
		t.Fatalf("叶子状态应保持 Created 不被误标终态: %+v", st)
	}
}

// TestRefreshTaskStatusParentAggregatesChildren 有子行父任务：按子任务状态聚合
// （全完成=Finished、含失败=PartlyFinished）
func TestRefreshTaskStatusParentAggregatesChildren(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	repo := NewRepository(db, nil, nil)
	ctx := context.Background()

	rows := []*entity.Task{
		newRefreshTaskRow(1, 0, true, TaskStatusCreated),
		newRefreshTaskRow(2, 1, false, TaskStatusFinished),
		newRefreshTaskRow(3, 1, false, TaskStatusFinished),
		newRefreshTaskRow(4, 1, false, TaskStatusFailed),
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("种树行失败: %v", err)
	}

	if n, err := repo.RefreshTaskStatus(ctx, 1); err != nil || n != 1 {
		t.Fatalf("父任务刷新应命中 1 行: n=%d err=%v", n, err)
	}
	if st := statusOrNull(t, db, 1); !st.Valid || TaskStatusEnum(st.Int64) != TaskStatusPartlyFinished {
		t.Fatalf("2 完成 1 失败应聚合 PartlyFinished: %+v", st)
	}

	// 末子改完成后全量完成
	if err := db.Exec("UPDATE task SET status = ? WHERE id = 4", TaskStatusFinished).Error; err != nil {
		t.Fatalf("改子状态失败: %v", err)
	}
	if _, err := repo.RefreshTaskStatus(ctx, 1); err != nil {
		t.Fatalf("刷新失败: %v", err)
	}
	if st := statusOrNull(t, db, 1); !st.Valid || TaskStatusEnum(st.Int64) != TaskStatusFinished {
		t.Fatalf("3 完成应聚合 Finished: %+v", st)
	}
}

// TestRefreshTaskStatusAllCreatedChildrenKeepsStatus 全未启动子树（子行全 Created）：
// 无聚合分支命中，父任务状态保持原值——聚合 CASE 不得写出 NULL 破坏状态列
func TestRefreshTaskStatusAllCreatedChildrenKeepsStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	repo := NewRepository(db, nil, nil)
	ctx := context.Background()

	rows := []*entity.Task{
		newRefreshTaskRow(1, 0, true, TaskStatusCreated),
		newRefreshTaskRow(2, 1, false, TaskStatusCreated),
		newRefreshTaskRow(3, 1, false, TaskStatusCreated),
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("种树行失败: %v", err)
	}

	if _, err := repo.RefreshTaskStatus(ctx, 1); err != nil {
		t.Fatalf("刷新失败: %v", err)
	}
	st := statusOrNull(t, db, 1)
	if !st.Valid {
		t.Fatal("全 Created 子树刷新后父状态被写成 NULL")
	}
	if TaskStatusEnum(st.Int64) != TaskStatusCreated {
		t.Fatalf("无分支命中应保持原状态 Created: %+v", st)
	}
}
