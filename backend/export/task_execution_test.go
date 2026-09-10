package export

import (
	"archive/zip"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/download"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/task"
	"github.com/library-squirrel/backend/taskManager"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// —— export 执行面策略测试：fake StrategyHandle（终态/进度调用形态断言）+ 真实 Collector/Packer
// + 临时目录；服务层两步建任务测试用 fake TaskControl ——

// TestMain 全局初始化 no-op logger：策略表集成测试驱动真实 taskManager（状态变更/命令处理
// 全路径打点日志），logger.Log 默认 nil 会令其 panic
func TestMain(m *testing.M) {
	logger.Log = zap.NewNop().Sugar()
	os.Exit(m.Run())
}

// fakeStrategyHandle 导出执行面句柄桩：记录终态与进度上报。导出策略不使用确认等待/回滚
// 登记/软暂停/跳过收口等能力，对应方法为空实现
type fakeStrategyHandle struct {
	task       *entity.Task
	runCtx     context.Context
	mu         sync.Mutex
	finished   bool
	failed     bool
	errMsg     string
	progresses [][2]int64
}

func (h *fakeStrategyHandle) Task() *entity.Task      { return h.task }
func (h *fakeStrategyHandle) RunCtx() context.Context { return h.runCtx }
func (h *fakeStrategyHandle) Finish() {
	h.mu.Lock()
	h.finished = true
	h.mu.Unlock()
}
func (h *fakeStrategyHandle) Fail(errMsg string) {
	h.mu.Lock()
	h.failed = true
	h.errMsg = errMsg
	h.mu.Unlock()
}
func (h *fakeStrategyHandle) ReportProgress(total, finished int64) {
	h.mu.Lock()
	h.progresses = append(h.progresses, [2]int64{total, finished})
	h.mu.Unlock()
}
func (h *fakeStrategyHandle) WaitReplaceConfirm(conflicts []taskManager.ConflictInfo) (taskManager.ReplaceDecision, bool) {
	return taskManager.ReplaceDecisionSkip, false
}
func (h *fakeStrategyHandle) ConfirmMemo() *taskManager.ReplaceConfirmMemo              { return nil }
func (h *fakeStrategyHandle) SetTerminalRollback(rollback taskManager.TerminalRollback) {}
func (h *fakeStrategyHandle) MarkDrainPhase(in bool)                                    {}
func (h *fakeStrategyHandle) SoftPauseSignal() <-chan struct{}                          { return nil }
func (h *fakeStrategyHandle) Skip(errMsg string)                                        {}
func (h *fakeStrategyHandle) ResumeRequested() bool                                     { return false }

// blockingExporter 测试桩：Plan 返回基于模型文件的统计，Pack 创建临时文件后阻塞至 ctx 取消。
type blockingExporter struct {
	entered chan struct{}
}

func (p *blockingExporter) Plan(_ context.Context, _ string, model *ExportModel) (*PackStats, error) {
	stats := &PackStats{}
	for _, f := range model.Manifest.Files {
		stats.TotalFiles++
		stats.TotalBytes += f.Size
	}
	return stats, nil
}

func (p *blockingExporter) Pack(ctx context.Context, _ string, _ *ExportModel, targetPath string, _ *PackStats, _ ProgressFn) error {
	if err := os.WriteFile(targetPath, []byte("partial"), 0o644); err != nil {
		return err
	}
	close(p.entered)
	<-ctx.Done()
	return ctx.Err()
}

// seedExportSourceFile 在测试 workDir 下播种夹具作品的源文件（store 活行指向）。
func seedExportSourceFile(t *testing.T, workDir string) {
	t.Helper()
	rel := filepath.Join("store", "resource", "作品1.jpg")
	abs := filepath.Join(workDir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte("image-content"), 0o644))
}

// exportExecEnv 执行面测试环境：内存库 + 夹具数据 + 服务与策略
type exportExecEnv struct {
	db      *gorm.DB
	f       *exportFixture
	svc     *Service
	exec    *ExportExecution
	workDir string
}

func newExportExecEnv(t *testing.T) *exportExecEnv {
	t.Helper()
	db, err := migration.OpenTestDB()
	require.NoError(t, err)
	f := seedExportFixture(t, db)
	workDir := t.TempDir()
	seedExportSourceFile(t, workDir)
	svc := NewService(NewRepository(db), NewExportTaskRepository(db),
		func() string { return "test-version" }, func() string { return workDir })
	exec := NewExportExecution(svc, NewPacker())
	exec.freeSpaceFn = func(string) (uint64, error) { return 1 << 40, nil }
	return &exportExecEnv{db: db, f: f, svc: svc, exec: exec, workDir: workDir}
}

// newExecTaskEntity 构造携带任务 ID 的任务实体（fake handle 载荷）
func newExecTaskEntity(id int64) *entity.Task {
	tk := entity.NewTask()
	tk.SetID(id)
	tk.TaskType = sql.NullString{String: TaskTypeExport, Valid: true}
	return tk
}

// idJSON 序列化选择 ID 集为 JSON 数组文本（测试夹具）
func idJSON(t *testing.T, ids []int64) string {
	t.Helper()
	s, err := marshalIDList(ids)
	require.NoError(t, err)
	return s
}

// seedExportTask 建任务核心行 + export_task 领域行（Created 状态），返回任务 ID
func seedExportTask(t *testing.T, db *gorm.DB, workIDsJSON, workSetIDsJSON, outputDir string) int64 {
	t.Helper()
	tk := entity.NewTask()
	tk.TaskName = sql.NullString{String: "导出（1 项）", Valid: true}
	tk.Status = int(task.TaskStatusCreated)
	tk.TaskType = sql.NullString{String: TaskTypeExport, Valid: true}
	tk.HasChild = sql.NullBool{Bool: false, Valid: true}
	require.NoError(t, db.Create(tk).Error)
	et := entity.NewExportTask(tk.GetID())
	et.WorkIDs = workIDsJSON
	et.WorkSetIDs = workSetIDsJSON
	et.OutputDir = outputDir
	require.NoError(t, NewExportTaskRepository(db).CreateForTask(context.Background(), tk.GetID(), et))
	return tk.GetID()
}

// findSingleExportZip 找出 dir 下唯一的导出最终 zip（排除临时文件），返回路径
func findSingleExportZip(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var zips []string
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) == ".zip" && len(name) > len(exportZipPrefix) &&
			name[:len(exportZipPrefix)] == exportZipPrefix {
			zips = append(zips, filepath.Join(dir, name))
		}
	}
	require.Len(t, zips, 1, "应恰有一个导出产物 zip，实际: %v", zips)
	return zips[0]
}

// TestExportExecutionSuccessFinish 成功收口：Finish 终态 + 产物 zip 结构正确 + 进度按字节
// 语义上报（末次 total=processed=源文件总字节）+ 无临时残留
func TestExportExecutionSuccessFinish(t *testing.T) {
	env := newExportExecEnv(t)
	taskID := seedExportTask(t, env.db, idJSON(t, []int64{env.f.w1ID}), idJSON(t, nil), "")

	h := &fakeStrategyHandle{task: newExecTaskEntity(taskID), runCtx: context.Background()}
	env.exec.Execute(h)

	h.mu.Lock()
	finished, failed := h.finished, h.failed
	progresses := append([][2]int64(nil), h.progresses...)
	h.mu.Unlock()
	require.True(t, finished, "应上报 Finish 终态")
	require.False(t, failed)

	zipPath := findSingleExportZip(t, env.workDir)
	zr, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer func() { _ = zr.Close() }()
	names := make([]string, 0, len(zr.File))
	for _, zf := range zr.File {
		names = append(names, zf.Name)
	}
	assert.Contains(t, names, "manifest.json")
	assert.Contains(t, names, "works/作品1/作品1.jpg")

	// 进度：单源文件 13 字节（"image-content"），末次上报 total=processed=13
	require.NotEmpty(t, progresses)
	last := progresses[len(progresses)-1]
	assert.Equal(t, int64(13), last[0])
	assert.Equal(t, int64(13), last[1])
}

// TestExportExecutionEmptySelectionFail 选择空（领域行两数组均空）：显式 Fail 且不产出 zip。
func TestExportExecutionEmptySelectionFail(t *testing.T) {
	env := newExportExecEnv(t)
	taskID := seedExportTask(t, env.db, idJSON(t, nil), idJSON(t, nil), "")

	h := &fakeStrategyHandle{task: newExecTaskEntity(taskID), runCtx: context.Background()}
	env.exec.Execute(h)

	h.mu.Lock()
	defer h.mu.Unlock()
	assert.True(t, h.failed)
	assert.Equal(t, ErrExportEmptySelection.Error(), h.errMsg)
	findNoExportArtifact(t, env.workDir)
}

// TestExportExecutionCorruptRowFail 领域行 JSON 损坏：按过时载荷显式 Fail（防御文案），不产出 zip。
func TestExportExecutionCorruptRowFail(t *testing.T) {
	env := newExportExecEnv(t)
	taskID := seedExportTask(t, env.db, "{bad json", "[]", "")

	h := &fakeStrategyHandle{task: newExecTaskEntity(taskID), runCtx: context.Background()}
	env.exec.Execute(h)

	h.mu.Lock()
	defer h.mu.Unlock()
	assert.True(t, h.failed)
	assert.Equal(t, "请删除本任务后重新导出", h.errMsg)
	findNoExportArtifact(t, env.workDir)
}

// TestExportExecutionRowMissingFail 领域行缺失（建任务链崩溃窗口遗留）：同防御文案 Fail。
func TestExportExecutionRowMissingFail(t *testing.T) {
	env := newExportExecEnv(t)
	tk := entity.NewTask()
	tk.TaskType = sql.NullString{String: TaskTypeExport, Valid: true}
	require.NoError(t, env.db.Create(tk).Error)

	h := &fakeStrategyHandle{task: newExecTaskEntity(tk.GetID()), runCtx: context.Background()}
	env.exec.Execute(h)

	h.mu.Lock()
	defer h.mu.Unlock()
	assert.True(t, h.failed)
	assert.Equal(t, "请删除本任务后重新导出", h.errMsg)
}

// TestExportExecutionDiskSpaceFail 磁盘预检不足：Fail 带容量文案，不产出 zip。
func TestExportExecutionDiskSpaceFail(t *testing.T) {
	env := newExportExecEnv(t)
	taskID := seedExportTask(t, env.db, idJSON(t, []int64{env.f.w1ID}), "[]", "")
	env.exec.freeSpaceFn = func(string) (uint64, error) { return 1, nil } // 仅 1 字节可用

	h := &fakeStrategyHandle{task: newExecTaskEntity(taskID), runCtx: context.Background()}
	env.exec.Execute(h)

	h.mu.Lock()
	defer h.mu.Unlock()
	assert.True(t, h.failed)
	assert.Contains(t, h.errMsg, "剩余空间不足")
	findNoExportArtifact(t, env.workDir)
}

// TestExportExecutionCancelKeepsTempThenRerun 取消语义：RunCtx 取消 → 不上报终态且 .zip.tmp
// 保留（重跑不续写）；恢复重跑 → 清扫先清自身残留再全量重写为最终 zip
func TestExportExecutionCancelKeepsTempThenRerun(t *testing.T) {
	env := newExportExecEnv(t)
	taskID := seedExportTask(t, env.db, idJSON(t, []int64{env.f.w1ID}), "[]", "")

	bp := &blockingExporter{entered: make(chan struct{})}
	exec := NewExportExecution(env.svc, bp)
	exec.freeSpaceFn = func(string) (uint64, error) { return 1 << 40, nil }

	ctx, cancel := context.WithCancel(context.Background())
	h := &fakeStrategyHandle{task: newExecTaskEntity(taskID), runCtx: ctx}
	done := make(chan struct{})
	go func() {
		defer close(done)
		exec.Execute(h)
	}()
	<-bp.entered // 等打包桩进入（临时文件已创建）
	cancel()
	<-done

	h.mu.Lock()
	finished, failed := h.finished, h.failed
	h.mu.Unlock()
	assert.False(t, finished, "取消退出不上报 Finish")
	assert.False(t, failed, "取消退出不上报 Fail（终态交控制面接管）")

	// .zip.tmp 保留（暂停/停止残留由重跑前清扫回收）
	entries, err := os.ReadDir(env.workDir)
	require.NoError(t, err)
	tmpCount := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			tmpCount++
		}
	}
	require.Equal(t, 1, tmpCount, "取消后应恰有一个 .zip.tmp 残留")

	// 恢复重跑（真实 Packer）：清扫先清残留，全量重写并成功收口
	h2 := &fakeStrategyHandle{task: newExecTaskEntity(taskID), runCtx: context.Background()}
	env.exec.Execute(h2)
	h2.mu.Lock()
	rerunFinished := h2.finished
	h2.mu.Unlock()
	require.True(t, rerunFinished, "重跑应成功收口")
	findSingleExportZip(t, env.workDir)

	entries, err = os.ReadDir(env.workDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), exportTempSuffix, "重跑后无临时残留")
	}
}

// TestExportExecutionSweepsStaleResidue 历史残留清扫：导出前清扫目标目录内崩溃/暂停残留的
// .zip.tmp（保留最终 zip 与无关文件）
func TestExportExecutionSweepsStaleResidue(t *testing.T) {
	env := newExportExecEnv(t)
	stale := filepath.Join(env.workDir, "library-squirrel-export-123.zip.tmp")
	require.NoError(t, os.WriteFile(stale, []byte("x"), 0o644))
	taskID := seedExportTask(t, env.db, idJSON(t, []int64{env.f.w1ID}), "[]", "")

	h := &fakeStrategyHandle{task: newExecTaskEntity(taskID), runCtx: context.Background()}
	env.exec.Execute(h)

	h.mu.Lock()
	defer h.mu.Unlock()
	require.True(t, h.finished)
	assert.NoFileExists(t, stale, "历史残留临时文件应被清扫")
	findSingleExportZip(t, env.workDir)
}

// findNoExportArtifact 断言目录内无导出产物（最终 zip 与临时文件均不存在）
func findNoExportArtifact(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), exportZipPrefix, "不应产出导出产物，实际存在: %s", e.Name())
	}
}

// —— 服务层两步建任务测试（fake TaskControl）——

// fakeTaskControl 导出任务控制桩：建任务落真实行（export_task 外键要求核心行存在），
// 记录建任务/启动/删除调用；fixedNextID 非零时直接返回该 ID 不建行（回滚测试预占主键场景）
type fakeTaskControl struct {
	db          *gorm.DB
	fixedNextID int64
	taskType    string
	taskName    string
	startedIDs  []int64
	deletedIDs  []int64
}

func (f *fakeTaskControl) CreateBuiltinTask(_ context.Context, taskType string, taskName string) (int64, error) {
	if f.fixedNextID != 0 {
		id := f.fixedNextID
		f.fixedNextID = 0
		f.taskType, f.taskName = taskType, taskName
		return id, nil
	}
	tk := entity.NewTask()
	tk.TaskName = sql.NullString{String: taskName, Valid: true}
	tk.Status = int(task.TaskStatusCreated)
	tk.TaskType = sql.NullString{String: taskType, Valid: true}
	tk.HasChild = sql.NullBool{Bool: false, Valid: true}
	if err := f.db.Create(tk).Error; err != nil {
		return 0, err
	}
	f.taskType, f.taskName = taskType, taskName
	return tk.GetID(), nil
}

func (f *fakeTaskControl) StartTasks(_ context.Context, taskIds []int64) error {
	f.startedIDs = append(f.startedIDs, taskIds...)
	return nil
}

func (f *fakeTaskControl) DeleteTask(_ context.Context, ids []int64) error {
	f.deletedIDs = append(f.deletedIDs, ids...)
	return nil
}

// TestStartExportTwoStepTaskCreation 两步建任务：建任务核心行（类型/任务名）→ 领域行（数组恒
// 序列化，空集存 []）→ 启动；返回任务 ID
func TestStartExportTwoStepTaskCreation(t *testing.T) {
	env := newExportExecEnv(t)
	ctl := &fakeTaskControl{db: env.db}
	env.svc.SetTaskControl(ctl)

	res, err := env.svc.StartExport(context.Background(), []int64{env.f.w1ID}, []int64{env.f.wsAID}, "D:/exports")
	require.NoError(t, err)

	assert.Equal(t, TaskTypeExport, ctl.taskType)
	assert.Equal(t, "导出（2 项）", ctl.taskName)
	assert.Equal(t, res.TaskID, ctl.startedIDs[0], "应启动新建任务")
	assert.Empty(t, ctl.deletedIDs)

	et, err := env.svc.exportTasks.GetById(context.Background(), res.TaskID)
	require.NoError(t, err)
	assert.Equal(t, idJSON(t, []int64{env.f.w1ID}), et.WorkIDs)
	assert.Equal(t, idJSON(t, []int64{env.f.wsAID}), et.WorkSetIDs)
	assert.Equal(t, "D:/exports", et.OutputDir)
}

// TestStartExportEmptySelectionNoTask 空选择前置校验：同步报错且不建任务行。
func TestStartExportEmptySelectionNoTask(t *testing.T) {
	env := newExportExecEnv(t)
	ctl := &fakeTaskControl{db: env.db}
	env.svc.SetTaskControl(ctl)

	_, err := env.svc.StartExport(context.Background(), nil, []int64{}, "")
	assert.ErrorIs(t, err, ErrExportEmptySelection)
	assert.Empty(t, ctl.startedIDs, "空选择不应建任务行")
	assert.Empty(t, ctl.deletedIDs)
	var taskCount int64
	require.NoError(t, env.db.Model(&entity.Task{}).Count(&taskCount).Error)
	assert.Equal(t, int64(0), taskCount, "空选择不应落任务核心行")
}

// TestStartExportRollbackOnDomainRowFailure 领域行写入失败（主键被预占）：显式删除任务行回滚。
func TestStartExportRollbackOnDomainRowFailure(t *testing.T) {
	env := newExportExecEnv(t)
	// 预建任务核心行并预占其 export_task 领域行主键 → CreateForTask 主键冲突
	pre := entity.NewTask()
	pre.TaskType = sql.NullString{String: TaskTypeExport, Valid: true}
	require.NoError(t, env.db.Create(pre).Error)
	occupied := entity.NewExportTask(pre.GetID())
	occupied.WorkIDs = "[]"
	occupied.WorkSetIDs = "[]"
	require.NoError(t, env.svc.exportTasks.CreateForTask(context.Background(), pre.GetID(), occupied))

	ctl := &fakeTaskControl{db: env.db, fixedNextID: pre.GetID()}
	env.svc.SetTaskControl(ctl)

	_, err := env.svc.StartExport(context.Background(), []int64{env.f.w1ID}, nil, "")
	require.Error(t, err)
	assert.Equal(t, []int64{pre.GetID()}, ctl.deletedIDs, "领域行写入失败应回滚删除任务行")
	assert.Empty(t, ctl.startedIDs)
}

// —— 策略表注册路径集成：任务行经真实 taskManager 按类型分发到 ExportExecution 直至终态 ——

// TestExportExecutionViaManager 建任务行（task_type=export）+ 领域行 → 真实 Manager 策略表
// 分发执行 → Finished 终态即时落盘 + 产物产出
func TestExportExecutionViaManager(t *testing.T) {
	env := newExportExecEnv(t)
	taskID := seedExportTask(t, env.db, idJSON(t, []int64{env.f.w1ID}), "[]", "")

	wtRepo := download.NewWorkTaskRepository(env.db)
	mgrRepo := task.NewRepository(env.db, wtRepo, wtRepo)
	mgr := taskManager.NewManager(2, mgrRepo, taskManager.NewNoopProgressPusher(),
		&taskManager.TaskDeps{Pusher: taskManager.NewNoopProgressPusher()},
		map[string]taskManager.ExecutionStrategy{TaskTypeExport: env.exec}, nil, nil)
	defer func() { _ = mgr.GracefulShutdown(context.Background()) }()

	require.NoError(t, mgr.StartTaskTrees(context.Background(), []int64{taskID}))

	// 等终态即时落盘（Finished）
	deadline := time.Now().Add(5 * time.Second)
	for {
		var tk entity.Task
		require.NoError(t, env.db.Where("id = ?", taskID).First(&tk).Error)
		if tk.Status == int(task.TaskStatusFinished) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("任务未到达 Finished 终态，当前状态 %d", tk.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	findSingleExportZip(t, env.workDir)
}
