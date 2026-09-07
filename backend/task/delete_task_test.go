package task

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"

	"gorm.io/gorm"
)

// 本文件为任务删除链外键前置义务回归测试：删任务前清 resource.task_id 引用（NULL=非任务产），
// 资源行保留；任务行（主+子）物理消亡。删除链编排在 Service.DeleteTask 事务内完成。

// testTransactor 真事务执行器（事务 DB 经 ctx 传递，仓储 dbFromCtx 感知）
type testTransactor struct{ db *gorm.DB }

func (t *testTransactor) ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return database.WithTransactionContext(ctx, t.db, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, database.TxKey, tx)
		return fn(txCtx)
	})
}

// TestDeleteTaskClearsResourceTaskId 删任务 → 引用该任务（及其子任务）的 resource 行 task_id 置 NULL、
// resource 行保留、对照组任务的引用不受影响。外键强制库下删除成功本身即「先清引用后删任务行」
// 顺序的证明（引用未清即删任务直接 FK 违约报错）。
// resource.task_id 引用 work_task（同值共享主键），fixture 为各任务建对应领域行；
// 删除链同时摘除被删任务的领域行（对照组领域行保留）
func TestDeleteTaskClearsResourceTaskId(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	// 作品行种子（resource.work_id 外键防线，fixture 统一 WorkID=1）
	if err := db.Exec("INSERT INTO work (id, create_time, update_time, deleted_at) VALUES (1, 0, 0, 0)").Error; err != nil {
		t.Fatalf("建作品种子失败: %v", err)
	}
	wtStore := newTestWorkTaskStore(db)
	repo := NewRepository(db, wtStore, wtStore)
	svc := NewService(repo, &testTransactor{db: db}, nil, nil, nil)

	// 主任务 + 子任务 + 对照组任务（各配同 id 作品领域行——resource.task_id 引用防线）
	newSeededTask := func(name string, pid int64) *domain.Task {
		tk := domain.NewTask()
		tk.TaskName = sql.NullString{String: name, Valid: true}
		if pid > 0 {
			tk.Pid = sql.NullInt64{Int64: pid, Valid: true}
		}
		if err := db.Create(tk).Error; err != nil {
			t.Fatalf("插任务 %s 失败: %v", name, err)
		}
		if err := db.Create(domain.NewWorkTask(tk.GetID())).Error; err != nil {
			t.Fatalf("插任务 %s 的作品领域行失败: %v", name, err)
		}
		return tk
	}
	parent := newSeededTask("主任务", 0)
	child := newSeededTask("子任务", parent.GetID())
	other := newSeededTask("对照组", 0)

	// 资源行：引用主任务、引用子任务、无引用（NULL）、引用对照组（引用应保留）
	newRes := func(taskId int64) *domain.Resource {
		r := domain.NewResource()
		r.WorkID = 1
		r.ResourceType = "image"
		if taskId > 0 {
			r.TaskID = sql.NullInt64{Int64: taskId, Valid: true}
		}
		return r
	}
	for _, r := range []*domain.Resource{newRes(parent.GetID()), newRes(child.GetID()), newRes(0), newRes(other.GetID())} {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("插资源失败: %v", err)
		}
	}

	if err := svc.DeleteTask(context.Background(), []int64{parent.GetID()}); err != nil {
		t.Fatalf("删除任务失败: %v", err)
	}

	// 主任务与子任务行物理消亡
	var taskCount int64
	if err := db.Model(&domain.Task{}).Where("id IN ?", []int64{parent.GetID(), child.GetID()}).Count(&taskCount).Error; err != nil {
		t.Fatalf("统计任务行失败: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("主任务与子任务行应物理消亡，剩余 %d 行", taskCount)
	}

	// 被删任务的作品领域行随之消亡（领域行先于核心行删除——id→task 外键下顺序错误的删除直接违约）
	var workTaskCount int64
	if err := db.Model(&domain.WorkTask{}).Where("id IN ?", []int64{parent.GetID(), child.GetID()}).Count(&workTaskCount).Error; err != nil {
		t.Fatalf("统计作品领域行失败: %v", err)
	}
	if workTaskCount != 0 {
		t.Fatalf("被删任务的作品领域行应随之消亡，剩余 %d 行", workTaskCount)
	}
	var otherWorkTask int64
	if err := db.Model(&domain.WorkTask{}).Where("id = ?", other.GetID()).Count(&otherWorkTask).Error; err != nil {
		t.Fatalf("统计对照组领域行失败: %v", err)
	}
	if otherWorkTask != 1 {
		t.Fatalf("对照组任务的作品领域行应保留，实际 %d 行", otherWorkTask)
	}

	// 资源行全部保留
	var resCount int64
	if err := db.Model(&domain.Resource{}).Count(&resCount).Error; err != nil {
		t.Fatalf("统计资源行失败: %v", err)
	}
	if resCount != 4 {
		t.Fatalf("资源行应全部保留，实际剩余 %d 行", resCount)
	}

	// 引用被删任务的资源行 task_id 置 NULL；对照组引用保留
	var keptRef int64
	if err := db.Model(&domain.Resource{}).Where("task_id IN ?", []int64{parent.GetID(), child.GetID()}).Count(&keptRef).Error; err != nil {
		t.Fatalf("统计残留引用失败: %v", err)
	}
	if keptRef != 0 {
		t.Fatalf("被删任务的资源引用应全部置 NULL，残留 %d 行", keptRef)
	}
	var otherRef int64
	if err := db.Model(&domain.Resource{}).Where("task_id = ?", other.GetID()).Count(&otherRef).Error; err != nil {
		t.Fatalf("统计对照组引用失败: %v", err)
	}
	if otherRef != 1 {
		t.Fatalf("对照组任务的资源引用应保留，实际 %d 行", otherRef)
	}
}
