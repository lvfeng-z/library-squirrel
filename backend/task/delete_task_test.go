package task

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/migration"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 本文件为任务删除链回归测试：①外键前置义务——删任务前清 resource.task_id 引用（NULL=非任务产），
// 资源行保留、任务行（主+子）物理消亡，删除链编排在 Service.DeleteTask 事务内完成；
// ②暂存目录联动——被删任务（含子任务）的下载暂存目录随删除链即时移除。

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
// TestDeleteTaskCleansStagingDirs 删任务 → 被删任务（含子任务）的下载暂存目录一并移除、
// 对照组任务的暂存目录保留。暂存目录按任务 ID 派生（task-staging/{taskID}/），
// 生命周期与任务行一致——删除链在事务提交后即时清理，不等启动清扫兜底
func TestDeleteTaskCleansStagingDirs(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	workDir := t.TempDir()
	wtStore := newTestWorkTaskStore(db)
	repo := NewRepository(db, wtStore, wtStore)
	svc := NewService(repo, &testTransactor{db: db}, nil, nil, nil, func() string { return workDir })

	newTask := func(name string, pid int64) *domain.Task {
		tk := domain.NewTask()
		tk.TaskName = sql.NullString{String: name, Valid: true}
		if pid > 0 {
			tk.Pid = sql.NullInt64{Int64: pid, Valid: true}
		}
		if err := db.Create(tk).Error; err != nil {
			t.Fatalf("插任务 %s 失败: %v", name, err)
		}
		return tk
	}
	parent := newTask("主任务", 0)
	child := newTask("子任务", parent.GetID())
	other := newTask("对照组", 0)

	// 各任务暂存目录内置一个暂存文件（空目录无法区分「已清」与「从未建」）
	newStaging := func(taskId int64) string {
		dir := StagingPath(workDir, taskId)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("建暂存目录失败: %v", err)
		}
		file := filepath.Join(dir, StagingFileName("image", 0, ".jpg"))
		if err := os.WriteFile(file, []byte("staged"), 0o644); err != nil {
			t.Fatalf("写暂存文件失败: %v", err)
		}
		return file
	}
	parentFile := newStaging(parent.GetID())
	childFile := newStaging(child.GetID())
	otherFile := newStaging(other.GetID())

	if err := svc.DeleteTask(context.Background(), []int64{parent.GetID()}); err != nil {
		t.Fatalf("删除任务失败: %v", err)
	}

	// 父与子的暂存目录随删除链移除；对照组保留
	for path, name := range map[string]string{parentFile: "主任务", childFile: "子任务"} {
		if _, serr := os.Stat(path); !os.IsNotExist(serr) {
			t.Fatalf("%s的暂存目录应随删除链移除，文件仍存在: %s", name, path)
		}
	}
	if _, serr := os.Stat(otherFile); serr != nil {
		t.Fatalf("对照组任务的暂存目录应保留: %v", serr)
	}
}

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
	svc := NewService(repo, &testTransactor{db: db}, nil, nil, nil, nil)

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
		if err := db.Create(domain.NewExportTask(tk.GetID())).Error; err != nil {
			t.Fatalf("插任务 %s 的导出领域行失败: %v", name, err)
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

	// 被删任务的导出领域行随之消亡（同共享主键外键防线；删除链漏摘该行时删核心行直接 FK 违约）
	var exportTaskCount int64
	if err := db.Model(&domain.ExportTask{}).Where("id IN ?", []int64{parent.GetID(), child.GetID()}).Count(&exportTaskCount).Error; err != nil {
		t.Fatalf("统计导出领域行失败: %v", err)
	}
	if exportTaskCount != 0 {
		t.Fatalf("被删任务的导出领域行应随之消亡，剩余 %d 行", exportTaskCount)
	}
	var otherExportTask int64
	if err := db.Model(&domain.ExportTask{}).Where("id = ?", other.GetID()).Count(&otherExportTask).Error; err != nil {
		t.Fatalf("统计对照组导出领域行失败: %v", err)
	}
	if otherExportTask != 1 {
		t.Fatalf("对照组任务的导出领域行应保留，实际 %d 行", otherExportTask)
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

// —— 运行态删除编排测试（先停后删）：无条件停止等待 / 停止器报错拒绝且行保留 ——
// 运行态是瞬态、不落库（运行中任务 DB 行恒 Created），删除编排无运行态判定门：删除即无条件
// 经停止器收口，非运行任务由停止器内部快速直通；测试树统一用 Created 行（生产可达形态）

// fakeRunningStopper 运行态任务停止器桩：记录调用；err 模拟等待终态超时
type fakeRunningStopper struct {
	calls [][]int64
	err   error
}

func (f *fakeRunningStopper) StopAndWaitTerminal(_ context.Context, ids []int64, _ time.Duration) error {
	f.calls = append(f.calls, append([]int64{}, ids...))
	return f.err
}

// newDeleteOrchestrationEnv 建删除编排测试环境：内存库 + 全 Created 任务树（同建树回滚形态）
// + 服务（注入停止器桩）
func newDeleteOrchestrationEnv(t *testing.T) (*Service, *fakeRunningStopper, *domain.Task, *domain.Task, *gorm.DB) {
	t.Helper()
	db, err := migration.OpenTestDB()
	if err != nil {
		t.Skipf("环境无 CGO SQLite，跳过: %v", err)
	}
	wtStore := newTestWorkTaskStore(db)
	repo := NewRepository(db, wtStore, wtStore)
	svc := NewService(repo, &testTransactor{db: db}, nil, nil, nil, nil)
	stopper := &fakeRunningStopper{}
	svc.SetRunningStopper(stopper)

	newTask := func(name string, pid int64) *domain.Task {
		tk := domain.NewTask()
		tk.TaskName = sql.NullString{String: name, Valid: true}
		if pid > 0 {
			tk.Pid = sql.NullInt64{Int64: pid, Valid: true}
		}
		require.NoError(t, db.Create(tk).Error)
		return tk
	}
	parent := newTask("主任务", 0)
	child := newTask("子任务", parent.GetID())
	return svc, stopper, parent, child, db
}

// TestDeleteTaskAlwaysStopsBeforeDelete 删除无条件经停止器收口（含建树回滚删 Created 态树），
// 停止完成（桩返回 nil）后走既有删除链。
func TestDeleteTaskAlwaysStopsBeforeDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	svc, stopper, parent, child, db := newDeleteOrchestrationEnv(t)

	if err := svc.DeleteTask(context.Background(), []int64{parent.GetID()}); err != nil {
		t.Fatalf("删除任务失败: %v", err)
	}
	require.Len(t, stopper.calls, 1, "删除应无条件调用停止器")
	assert.Equal(t, []int64{parent.GetID()}, stopper.calls[0])

	var taskCount int64
	require.NoError(t, db.Model(&domain.Task{}).Where("id IN ?", []int64{parent.GetID(), child.GetID()}).Count(&taskCount).Error)
	assert.Equal(t, int64(0), taskCount, "停止完成后任务树应删除")
}

// TestDeleteTaskStopTimeoutRefuses 停止器报错（等待终态超时）拒绝删除：任务行保留不强行删除。
func TestDeleteTaskStopTimeoutRefuses(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	svc, stopper, parent, child, db := newDeleteOrchestrationEnv(t)
	stopper.err = errors.New("任务停止超时，请稍后重试删除")

	err := svc.DeleteTask(context.Background(), []int64{parent.GetID()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "任务停止超时")

	var taskCount int64
	require.NoError(t, db.Model(&domain.Task{}).Where("id IN ?", []int64{parent.GetID(), child.GetID()}).Count(&taskCount).Error)
	assert.Equal(t, int64(2), taskCount, "超时拒绝删除，任务树应保留")
}
