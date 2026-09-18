package export

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/download"
	"github.com/library-squirrel/backend/migration"
	"github.com/library-squirrel/backend/staging"
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

func (p *blockingExporter) Plan(_ context.Context, _ string, model *ExportModel, _ string) (*PackStats, error) {
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

// fixedFormatProvider 固定模板供给桩：空串经 ExportExecution.template() 回退默认模板
// （本文件锚定执行面流程，zip 内文件名按默认模板渲染锚定）。
type fixedFormatProvider struct{}

func (fixedFormatProvider) GetFileNameFormat() string { return "" }

func newExportExecEnv(t *testing.T) *exportExecEnv {
	t.Helper()
	db, err := migration.OpenTestDB()
	require.NoError(t, err)
	f := seedExportFixture(t, db)
	workDir := t.TempDir()
	seedExportSourceFile(t, workDir)
	svc := NewService(NewRepository(db), NewExportTaskRepository(db),
		func() string { return "test-version" }, func() string { return workDir })
	exec := NewExportExecution(svc, NewPacker(), fixedFormatProvider{})
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
	// 目录与文件名同模板渲染（同基底）：作者（作品关联本地作者）+ 站点作品 ID + 作品名 + 源文件扩展名
	assert.Contains(t, names, "works/[画师A]_[w-1]_作品1/[画师A]_[w-1]_作品1.jpg")

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

// TestExportExecutionCancelCleansTempAndScope 取消语义：RunCtx 取消 → 不上报终态、显式清理
// 目标目录临时文件与暂存作用域（zip 不支持续写，恢复全量重跑）；在途账本登记目标目录与
// 实际临时文件名；恢复重跑全量重写为最终 zip 且无作用域残留
func TestExportExecutionCancelCleansTempAndScope(t *testing.T) {
	env := newExportExecEnv(t)
	outDir := t.TempDir() // 自选输出目录（区别于 workDir，锚定账本登记的目标位置）
	taskID := seedExportTask(t, env.db, idJSON(t, []int64{env.f.w1ID}), "[]", outDir)

	bp := &blockingExporter{entered: make(chan struct{})}
	exec := NewExportExecution(env.svc, bp, fixedFormatProvider{})
	exec.freeSpaceFn = func(string) (uint64, error) { return 1 << 40, nil }

	ctx, cancel := context.WithCancel(context.Background())
	h := &fakeStrategyHandle{task: newExecTaskEntity(taskID), runCtx: ctx}
	done := make(chan struct{})
	go func() {
		defer close(done)
		exec.Execute(h)
	}()
	<-bp.entered // 等打包桩进入（作用域已建、临时文件已写）

	// 在途账本：作用域描述登记目标目录与实际在写的临时文件名
	ledgers := readExportScopeLedgers(t, env.workDir)
	require.Len(t, ledgers, 1, "应恰有一个在途导出作用域")
	assert.Equal(t, outDir, ledgers[0].TargetDir)
	require.Len(t, ledgers[0].TempFiles, 1)
	assert.Equal(t, soleTempFileName(t, outDir), ledgers[0].TempFiles[0])

	cancel()
	<-done

	h.mu.Lock()
	finished, failed := h.finished, h.failed
	h.mu.Unlock()
	assert.False(t, finished, "取消退出不上报 Finish")
	assert.False(t, failed, "取消退出不上报 Fail（终态交控制面接管）")

	// 取消清理：目标目录临时文件与暂存作用域均回收
	assertExportRootEmpty(t, env.workDir)
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), exportTempSuffix, "取消后目标目录应无临时文件残留")
	}

	// 恢复重跑（真实 Packer）：全量重写并成功收口
	h2 := &fakeStrategyHandle{task: newExecTaskEntity(taskID), runCtx: context.Background()}
	env.exec.Execute(h2)
	h2.mu.Lock()
	rerunFinished := h2.finished
	h2.mu.Unlock()
	require.True(t, rerunFinished, "重跑应成功收口")
	findSingleExportZip(t, outDir)
	assertExportRootEmpty(t, env.workDir)
}

// TestExportCrashResidueRecoveredByStartupSweep 崩溃残留回收：作用域账本与目标目录临时文件
// 留盘（进程在导出中途崩溃形态）→ 启动清扫按账本删目标侧临时文件并回收作用域；账外文件
// 不动（描述即权威清单，不扫描目标目录）
func TestExportCrashResidueRecoveredByStartupSweep(t *testing.T) {
	workDir := t.TempDir()
	target := t.TempDir()
	tempName := "library-squirrel-export-1690000000000.zip.zip.tmp"
	require.NoError(t, os.WriteFile(filepath.Join(target, tempName), []byte("x"), 0o644))
	keep := filepath.Join(target, "unrelated.txt")
	require.NoError(t, os.WriteFile(keep, []byte("k"), 0o644))
	scopeKey, err := staging.MintScopeKey()
	require.NoError(t, err)
	scopeDir, err := staging.CreateExportScope(context.Background(), workDir, scopeKey,
		exportScopeContentShape, staging.ExportLedger{
			TargetDir: target,
			TempFiles: []string{tempName},
		})
	require.NoError(t, err)

	require.NoError(t, staging.SweepAtStartup(context.Background(), workDir, nil))

	assert.NoFileExists(t, filepath.Join(target, tempName), "账内临时文件未被删除")
	assert.FileExists(t, keep, "账外文件被误删")
	assert.NoDirExists(t, scopeDir, "作用域未被回收")
}

// TestExportSweepUnreachableTargetTolerated 目标目录不存在（目标侧本无临时文件，IsNotExist
// 按已清理落定）：清扫容忍不报错、作用域照常回收；重复清扫幂等无副作用
func TestExportSweepUnreachableTargetTolerated(t *testing.T) {
	workDir := t.TempDir()
	scopeKey, err := staging.MintScopeKey()
	require.NoError(t, err)
	scopeDir, err := staging.CreateExportScope(context.Background(), workDir, scopeKey,
		exportScopeContentShape, staging.ExportLedger{
			TargetDir: filepath.Join(workDir, "已卸载的目标盘"),
			TempFiles: []string{"library-squirrel-export-1690000000001.zip.zip.tmp"},
		})
	require.NoError(t, err)

	for i := 0; i < 2; i++ { // 首次清扫 + 幂等复扫
		require.NoError(t, staging.SweepAtStartup(context.Background(), workDir, nil),
			"目标盘不可达不应令清扫报错")
	}
	assert.NoDirExists(t, scopeDir, "作用域未被回收")
}

// TestExportSweepTargetDeletionFailureKeepsScopeForRetry 目标侧删除真失败（以非空目录占据
// 账本临时文件位模拟文件被占用/目标盘不可达——os.Remove 对非空目录报错且非 IsNotExist，
// 区别于目标目录不存在的已清理落定分支）：本轮不回收作用域（账本仍在）、目标路径仍在；
// 占用解除后再次清扫，两者皆清——下次启动凭账本重试的完整语义
func TestExportSweepTargetDeletionFailureKeepsScopeForRetry(t *testing.T) {
	workDir := t.TempDir()
	target := t.TempDir()
	tempName := "library-squirrel-export-1690000000002.zip.zip.tmp"
	tempPath := filepath.Join(target, tempName)
	require.NoError(t, os.MkdirAll(tempPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempPath, "held.txt"), []byte("x"), 0o644))
	scopeKey, err := staging.MintScopeKey()
	require.NoError(t, err)
	scopeDir, err := staging.CreateExportScope(context.Background(), workDir, scopeKey,
		exportScopeContentShape, staging.ExportLedger{
			TargetDir: target,
			TempFiles: []string{tempName},
		})
	require.NoError(t, err)

	require.NoError(t, staging.SweepAtStartup(context.Background(), workDir, nil))
	assert.DirExists(t, scopeDir, "目标侧清理未落定，作用域应保留供下次重试")
	assert.DirExists(t, tempPath, "被占用的目标路径不应被删除")

	// 占用解除（占位目录清空）：再次清扫凭账本重试，目标路径删除、作用域回收
	require.NoError(t, os.Remove(filepath.Join(tempPath, "held.txt")))
	require.NoError(t, staging.SweepAtStartup(context.Background(), workDir, nil))
	assert.NoDirExists(t, tempPath, "重试应删除目标临时文件")
	assert.NoDirExists(t, scopeDir, "清理落定后作用域应回收")
}

// TestExportSweepReclaimsScopeWithoutValidDesc 描述缺失/描述损坏（外部改动形态）：作用域按
// 无主数据回收
func TestExportSweepReclaimsScopeWithoutValidDesc(t *testing.T) {
	workDir := t.TempDir()
	exportRoot := filepath.Join(workDir, staging.RootName, string(staging.OwnerExport))
	noDesc := filepath.Join(exportRoot, "aaa")
	require.NoError(t, os.MkdirAll(noDesc, 0o755))
	corrupt := filepath.Join(exportRoot, "bbb")
	require.NoError(t, os.MkdirAll(corrupt, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(corrupt, "scope.json"), []byte("{bad"), 0o644))

	require.NoError(t, staging.SweepAtStartup(context.Background(), workDir, nil))

	assert.NoDirExists(t, noDesc, "无描述作用域未被回收")
	assert.NoDirExists(t, corrupt, "描述损坏作用域未被回收")
}

// readExportScopeLedgers 读取 workDir 暂存总根 export 属主根下全部作用域描述的账本。
func readExportScopeLedgers(t *testing.T, workDir string) []staging.ExportLedger {
	t.Helper()
	root := filepath.Join(workDir, staging.RootName, string(staging.OwnerExport))
	entries, err := os.ReadDir(root)
	if err != nil {
		require.True(t, os.IsNotExist(err), "读取导出属主根失败: %v", err)
		return nil
	}
	var ledgers []staging.ExportLedger
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(root, ent.Name(), "scope.json"))
		require.NoError(t, rerr)
		var desc staging.ScopeDescription
		require.NoError(t, json.Unmarshal(data, &desc))
		require.NotNil(t, desc.Export, "导出作用域描述应含账本: %s", ent.Name())
		ledgers = append(ledgers, *desc.Export)
	}
	return ledgers
}

// assertExportRootEmpty 断言暂存总根 export 属主根下无作用域残留（目录不存在或为空）。
func assertExportRootEmpty(t *testing.T, workDir string) {
	t.Helper()
	root := filepath.Join(workDir, staging.RootName, string(staging.OwnerExport))
	entries, err := os.ReadDir(root)
	if err != nil {
		require.True(t, os.IsNotExist(err), "读取导出属主根失败: %v", err)
		return
	}
	assert.Empty(t, entries, "导出属主根应无作用域残留")
}

// soleTempFileName 找出 dir 下唯一的导出临时文件名。
func soleTempFileName(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), exportTempSuffix) {
			names = append(names, e.Name())
		}
	}
	require.Len(t, names, 1, "应恰有一个在途临时文件，实际: %v", names)
	return names[0]
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
