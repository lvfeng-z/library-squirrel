package importer

// import 执行面策略（ImportExecution）测试：fake StrategyHandle 断言执行面契约——成功
// Finish＋字节进度上报、已暂存条目旁路免解包、编排产物注入 IngestOptions、CreatedStoreIDs
// 并入终态回滚登记、RunCtx 取消/确认取消不上报终态、领域行缺失/过时载荷/zip 缺失/工作目录
// 未配置的显式 Fail 文案。回灌本体经 fake ManifestIngestor 驱动（真实 ingest 链路由
// ingest_test.go 锚定），解包相位经 fake ingestor 驱动真实暂存层外包（计数/旁路/回收）。

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/task"
	"github.com/library-squirrel/backend/taskManager"
)

// ===== fake StrategyHandle =====

// execProgressPoint 一次进度上报快照（total/done 字节语义）。
type execProgressPoint struct {
	total int64
	done  int64
}

// fakeExecHandle 执行句柄桩：记录终态/进度/回滚登记/确认交互，confirmDecision 与
// confirmCanceled 控制覆盖确认答复。
type fakeExecHandle struct {
	mu              sync.Mutex
	task            *entity.Task
	ctx             context.Context
	finished        bool
	failed          string
	progress        []execProgressPoint
	rollbacks       []taskManager.TerminalRollback
	waitConflicts   []taskManager.ConflictInfo
	confirmDecision taskManager.ReplaceDecision
	confirmCanceled bool
	skipMsg         *string
}

func (h *fakeExecHandle) Task() *entity.Task               { return h.task }
func (h *fakeExecHandle) RunCtx() context.Context          { return h.ctx }
func (h *fakeExecHandle) MarkDrainPhase(in bool)           {}
func (h *fakeExecHandle) SoftPauseSignal() <-chan struct{} { return nil }
func (h *fakeExecHandle) ResumeRequested() bool            { return false }

func (h *fakeExecHandle) Finish() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finished = true
}

func (h *fakeExecHandle) Fail(errMsg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failed = errMsg
}

func (h *fakeExecHandle) ReportProgress(total, finished int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.progress = append(h.progress, execProgressPoint{total: total, done: finished})
}

func (h *fakeExecHandle) SetTerminalRollback(rollback taskManager.TerminalRollback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rollbacks = append(h.rollbacks, rollback)
}

func (h *fakeExecHandle) WaitReplaceConfirm(conflicts []taskManager.ConflictInfo) (taskManager.ReplaceDecision, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.waitConflicts = append(h.waitConflicts, conflicts...)
	return h.confirmDecision, h.confirmCanceled
}

func (h *fakeExecHandle) ConfirmMemo() *taskManager.ReplaceConfirmMemo { return nil }

func (h *fakeExecHandle) Skip(errMsg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.skipMsg = &errMsg
}

// hasRollbackPayload 判定已登记回滚是否含指定新建行清单（终态回滚登记断言）。
func (h *fakeExecHandle) hasRollbackPayload(created []int64, victims []resource.StoreRef) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, rb := range h.rollbacks {
		if len(rb.CreatedStoreIDs) != len(created) || len(rb.Victims) != len(victims) {
			continue
		}
		sameC, sameV := true, true
		for i := range rb.CreatedStoreIDs {
			if rb.CreatedStoreIDs[i] != created[i] {
				sameC = false
			}
		}
		for i := range rb.Victims {
			if rb.Victims[i] != victims[i] {
				sameV = false
			}
		}
		if sameC && sameV {
			return true
		}
	}
	return false
}

// ===== fake 回灌/编排依赖 =====

// fakeExecIngestor 回灌导入桩：记录调用载荷（子 manifest/opts），并对可落盘条目驱动真实
// 暂存层（解包写入或已暂存旁路）+ 真实文件源读取——锚定执行面的进度计数、旁路与回收外包。
type fakeExecIngestor struct {
	mu          sync.Mutex
	called      bool
	gotManifest *export.Manifest
	gotOpts     *IngestOptions
	written     []string // 经 writer 解包写入的条目
	bypassed    []string // 已暂存旁路（writer=nil）的条目
	result      *ImportResult
	err         error
}

func (f *fakeExecIngestor) Ingest(ctx context.Context, manifest *export.Manifest, fileSource FileSource,
	staging IngestStaging, opts *IngestOptions) (*ImportResult, error) {
	f.mu.Lock()
	f.called = true
	f.gotManifest = manifest
	f.gotOpts = opts
	f.mu.Unlock()
	defer staging.Release(ctx) // 对齐真实 ingest 的退出收尾（成功/失败统一回收暂存作用域）
	if f.err != nil {
		return nil, f.err
	}
	for i := range manifest.Files {
		entry := &manifest.Files[i]
		if entry.Path == "" || entry.Missing {
			continue
		}
		w, _, err := staging.Stage(ctx, entry.Path)
		if err != nil {
			return nil, err
		}
		src, err := fileSource(entry.Path)
		if err != nil {
			if w != nil {
				_ = w.Close()
			}
			_ = src.Close()
			return nil, err
		}
		var dst io.Writer = io.Discard
		if w != nil {
			dst = w
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := src.Close()
		if w != nil {
			if werr := w.Close(); werr != nil && copyErr == nil {
				copyErr = werr
			}
		}
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		f.mu.Lock()
		if w == nil {
			f.bypassed = append(f.bypassed, entry.Path)
		} else {
			f.written = append(f.written, entry.Path)
		}
		f.mu.Unlock()
	}
	if f.result != nil {
		return f.result, nil
	}
	return &ImportResult{CreatedWorks: 1}, nil
}

// fakeExecChecker 查重判定桩（编排依赖）：全部条目返回同一分类结果。
type fakeExecChecker struct {
	result duplicate.DuplicateCheckResult
}

func (c *fakeExecChecker) Check(ctx context.Context, items []duplicate.DuplicateCheckItem) ([]duplicate.DuplicateCheckResult, error) {
	out := make([]duplicate.DuplicateCheckResult, len(items))
	for i := range out {
		out[i] = c.result
	}
	return out, nil
}

// fakeExecReplaceOps 软删替换桩（编排依赖）：记录调用并返回预置受害者清单。
type fakeExecReplaceOps struct {
	mu    sync.Mutex
	calls []int64
	refs  []resource.StoreRef
}

func (o *fakeExecReplaceOps) SoftDeleteWorkStoreRoles(ctx context.Context, workId int64, roles []string) ([]resource.StoreRef, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls = append(o.calls, workId)
	return o.refs, nil
}

func (o *fakeExecReplaceOps) RestoreReplacedStores(ctx context.Context, scope resource.RestoreScope) error {
	return nil
}

// ===== 夹具 =====

// execFixture 执行面测试夹具：真实 zip 包（manifest + 文件条目）＋父作用域共享 manifest 原字节
// 落盘＋import_task 领域行（fakeImportTaskStore，复用 start_import_test 定义）。
type execFixture struct {
	workDir  string
	zipPath  string
	manifest *export.Manifest
	files    map[string]string
	store    *fakeImportTaskStore
	parentID int64
	taskID   int64
}

func newExecFixture(t *testing.T) *execFixture {
	t.Helper()
	manifest, files := buildFixture()
	raw, err := manifest.Serialize()
	if err != nil {
		t.Fatalf("序列化 manifest 失败: %v", err)
	}
	entries := map[string][]byte{"manifest.json": raw}
	for p, c := range files {
		entries[p] = []byte(c)
	}
	zipPath := writeZip(t, entries)
	workDir := t.TempDir()
	const parentID, taskID = int64(6001), int64(6002)
	ctx := context.Background()
	if _, err := task.EnsureImportScope(ctx, workDir, parentID); err != nil {
		t.Fatalf("建父作用域失败: %v", err)
	}
	manifestRel := task.ImportManifestRelPath(parentID)
	if err := writeRawManifestFile(workDir, manifestRel, raw); err != nil {
		t.Fatalf("落盘共享 manifest 失败: %v", err)
	}
	store := &fakeImportTaskStore{}
	it := entity.NewImportTask(taskID)
	it.ZipPath = zipPath
	it.ManifestRel = manifestRel
	it.ManifestID = manifest.Works[0].ID
	if err := store.CreateForTask(ctx, taskID, it); err != nil {
		t.Fatalf("写领域行失败: %v", err)
	}
	return &execFixture{workDir: workDir, zipPath: zipPath, manifest: manifest, files: files,
		store: store, parentID: parentID, taskID: taskID}
}

// handle 构建绑定夹具任务的执行句柄（默认确认决策=替换）。
func (f *execFixture) handle(ctx context.Context) *fakeExecHandle {
	tk := entity.NewTask()
	tk.ID = f.taskID
	tk.TaskType = sql.NullString{String: TaskTypeImport, Valid: true}
	return &fakeExecHandle{task: tk, ctx: ctx, confirmDecision: taskManager.ReplaceDecisionReplace}
}

// declaredBytes 夹具两条可落盘条目的声明字节和（进度 total 期望值；missing 条目不计）。
func (f *execFixture) declaredBytes(entryPaths ...string) int64 {
	var total int64
	for _, p := range entryPaths {
		total += int64(len(f.files[p]))
	}
	return total
}

// ===== 用例 =====

// TestImportExecutionFinishWithByteProgress 成功主线：Finish＋字节进度（total=待解包条目声明
// 总字节、终报 done==total）＋编排产物皆空注入 opts=nil＋CreatedStoreIDs 并入终态回滚登记＋
// 回灌收到的 manifest 为本作品子 manifest＋成功后子任务暂存作用域回收。
func TestImportExecutionFinishWithByteProgress(t *testing.T) {
	fx := newExecFixture(t)
	h := fx.handle(context.Background())
	ing := &fakeExecIngestor{result: &ImportResult{CreatedWorks: 1, CreatedStoreIDs: []int64{11, 12}}}
	e := NewImportExecution(func() string { return fx.workDir }, fx.store, ing, nil, nil, nil)

	e.Execute(h)

	if !h.finished {
		t.Fatalf("应 Finish，实际 failed=%q", h.failed)
	}
	if len(ing.written) != 2 || len(ing.bypassed) != 0 {
		t.Fatalf("两条可落盘条目应经 writer 解包写入，实际 written=%v bypassed=%v", ing.written, ing.bypassed)
	}
	// 回灌 manifest 收窄到本作品（子 manifest 构造经领域行 ManifestID 定位）
	if len(ing.gotManifest.Works) != 1 || ing.gotManifest.Works[0].ID != fx.manifest.Works[0].ID {
		t.Fatalf("回灌 manifest 应为本作品子集，实际作品数=%d", len(ing.gotManifest.Works))
	}
	// 编排产物皆空（无查重依赖）：opts=nil 维持全跳过旧语义
	if ing.gotOpts != nil {
		t.Fatalf("无替换集时 opts 应为 nil，实际 %+v", ing.gotOpts)
	}
	// 进度：total=声明总字节，终报 done==total（解包字节全经计数 writer 上报）
	total := fx.declaredBytes("works/作品一/pic.jpg", "works/作品一/pic_thumb.jpg")
	if len(h.progress) == 0 {
		t.Fatalf("应上报进度")
	}
	last := h.progress[len(h.progress)-1]
	if last.total != total || last.done != total {
		t.Fatalf("终报进度应 done==total==%d，实际 total=%d done=%d", total, last.total, last.done)
	}
	// 新建 store 行清单并入终态回滚登记（导入成功到 Finish 间中断窗口的丢弃载荷）
	if !h.hasRollbackPayload([]int64{11, 12}, nil) {
		t.Fatalf("CreatedStoreIDs 应并入终态回滚登记，实际 %+v", h.rollbacks)
	}
	// 成功后子任务暂存作用域回收（zip 解包可随时重产，无续传语义）
	if _, err := os.Stat(task.ImportStagingPath(fx.workDir, fx.taskID)); !os.IsNotExist(err) {
		t.Fatalf("成功后子任务暂存作用域应回收")
	}
}

// TestImportExecutionStagedBypassSkipsUnpack 已完整暂存条目（内容判定拷入/恢复残留）旁路：
// 文件源读暂存副本（免 zip 解压重读）、暂存层 writer=nil（免重写）、初始进度计入其字节。
func TestImportExecutionStagedBypassSkipsUnpack(t *testing.T) {
	fx := newExecFixture(t)
	h := fx.handle(context.Background())
	const bypass = "works/作品一/pic.jpg"
	stagingDir := task.ImportStagingPath(fx.workDir, fx.taskID)
	if err := os.MkdirAll(filepath.Dir(StagingPath(stagingDir, bypass)), 0o755); err != nil {
		t.Fatalf("建预暂存目录失败: %v", err)
	}
	if err := os.WriteFile(StagingPath(stagingDir, bypass), []byte(fx.files[bypass]), 0o644); err != nil {
		t.Fatalf("写预暂存文件失败: %v", err)
	}
	ing := &fakeExecIngestor{}
	e := NewImportExecution(func() string { return fx.workDir }, fx.store, ing, nil, nil, nil)

	e.Execute(h)

	if !h.finished {
		t.Fatalf("应 Finish，实际 failed=%q", h.failed)
	}
	if len(ing.bypassed) != 1 || ing.bypassed[0] != bypass {
		t.Fatalf("已暂存条目应旁路免解包，实际 bypassed=%v", ing.bypassed)
	}
	if len(ing.written) != 1 || ing.written[0] != "works/作品一/pic_thumb.jpg" {
		t.Fatalf("未暂存条目应经 writer 解包，实际 written=%v", ing.written)
	}
	// 初始段位已含旁路条目字节（免解包重读不计新进度）
	total := fx.declaredBytes("works/作品一/pic.jpg", "works/作品一/pic_thumb.jpg")
	first := h.progress[0]
	if first.total != total || first.done != int64(len(fx.files[bypass])) {
		t.Fatalf("初始进度应计入已暂存条目字节，实际 total=%d done=%d", first.total, first.done)
	}
}

// TestImportExecutionInjectsReplaceOptions 编排产物注入：查重命中冲突 → 确认弹窗（整体决策
// 替换）→ 软删本库作品并登记受害者清单 → IngestOptions 携带确认替换集 → CreatedStoreIDs
// 并入登记（受害者/新建行两载荷分次登记）。
func TestImportExecutionInjectsReplaceOptions(t *testing.T) {
	fx := newExecFixture(t)
	h := fx.handle(context.Background())
	checker := &fakeExecChecker{result: duplicate.DuplicateCheckResult{
		Class: duplicate.DuplicateHitConflict, WorkID: 500, WorkName: "已有作品",
		ConflictRoles: []string{entity.StoreTypeImage},
	}}
	victims := []resource.StoreRef{{StoreID: 77, ResourceID: 8801}}
	ops := &fakeExecReplaceOps{refs: victims}
	ing := &fakeExecIngestor{result: &ImportResult{ReplacedConfirmed: 1, CreatedStoreIDs: []int64{11}}}
	e := NewImportExecution(func() string { return fx.workDir }, fx.store, ing, checker, ops, nil)

	e.Execute(h)

	if !h.finished {
		t.Fatalf("应 Finish，实际 failed=%q", h.failed)
	}
	if len(h.waitConflicts) != 1 || h.waitConflicts[0].WorkID != 500 {
		t.Fatalf("命中冲突应弹整体确认，实际 conflicts=%v", h.waitConflicts)
	}
	if len(ops.calls) != 1 || ops.calls[0] != 500 {
		t.Fatalf("确认替换应软删本库作品 500，实际 calls=%v", ops.calls)
	}
	if ing.gotOpts == nil || len(ing.gotOpts.ReplaceWorks) != 1 {
		t.Fatalf("确认替换集应注入 IngestOptions，实际 %+v", ing.gotOpts)
	}
	if _, ok := ing.gotOpts.ReplaceWorks[fx.manifest.Works[0].ID]; !ok {
		t.Fatalf("替换集应含本作品 manifest ID %d", fx.manifest.Works[0].ID)
	}
	if len(ing.gotOpts.AutoMergeWorks) != 0 {
		t.Fatalf("无零交集命中时自动增补集应空，实际 %+v", ing.gotOpts.AutoMergeWorks)
	}
	if !h.hasRollbackPayload(nil, victims) {
		t.Fatalf("软删受害者清单应登记终态回滚，实际 %+v", h.rollbacks)
	}
	if !h.hasRollbackPayload([]int64{11}, nil) {
		t.Fatalf("导入新建行清单应并入终态回滚登记，实际 %+v", h.rollbacks)
	}
}

// TestImportExecutionCanceledRunReportsNoTerminal RunCtx 取消（暂停/停止）不上报终态：回灌
// 错误经取消分流静默返回，交控制面接管。
func TestImportExecutionCanceledRunReportsNoTerminal(t *testing.T) {
	fx := newExecFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	h := fx.handle(ctx)
	ing := &fakeExecIngestor{err: errors.New("解包被暂停/停止中断")}
	e := NewImportExecution(func() string { return fx.workDir }, fx.store, ing, nil, nil, nil)

	cancel()
	e.Execute(h)

	if h.finished || h.failed != "" || h.skipMsg != nil {
		t.Fatalf("取消路径不应上报终态，实际 finished=%v failed=%q skip=%v", h.finished, h.failed, h.skipMsg)
	}
}

// TestImportExecutionConfirmCanceledNoTerminal 覆盖确认等待被取消（暂停/停止打断）：不上报
// 终态且不进入回灌。
func TestImportExecutionConfirmCanceledNoTerminal(t *testing.T) {
	fx := newExecFixture(t)
	h := fx.handle(context.Background())
	h.confirmCanceled = true
	checker := &fakeExecChecker{result: duplicate.DuplicateCheckResult{
		Class: duplicate.DuplicateHitConflict, WorkID: 500, WorkName: "已有作品",
		ConflictRoles: []string{entity.StoreTypeImage},
	}}
	ing := &fakeExecIngestor{}
	e := NewImportExecution(func() string { return fx.workDir }, fx.store, ing, checker, &fakeExecReplaceOps{}, nil)

	e.Execute(h)

	if h.finished || h.failed != "" || h.skipMsg != nil {
		t.Fatalf("确认取消不应上报终态，实际 finished=%v failed=%q", h.finished, h.failed)
	}
	if ing.called {
		t.Fatalf("确认取消不应进入回灌导入")
	}
}

// TestImportExecutionFailPaths 显式失败文案：领域行缺失、过时载荷（ManifestID==0）、zip 缺失
// （包被移动/删除）、工作目录未配置。
func TestImportExecutionFailPaths(t *testing.T) {
	newExec := func(fx *execFixture, ing *fakeExecIngestor, workDir func() string) *ImportExecution {
		return NewImportExecution(workDir, fx.store, ing, nil, nil, nil)
	}

	t.Run("领域行缺失", func(t *testing.T) {
		fx := newExecFixture(t)
		h := fx.handle(context.Background())
		another := entity.NewTask()
		another.ID = 9999 // 无领域行
		h.task = another
		newExec(fx, &fakeExecIngestor{}, func() string { return fx.workDir }).Execute(h)
		if h.finished || h.failed != "请删除本任务后重新导入" {
			t.Fatalf("领域行缺失应显式 Fail，实际 failed=%q", h.failed)
		}
	})

	t.Run("过时载荷", func(t *testing.T) {
		fx := newExecFixture(t)
		fx.store.rows[fx.taskID].ManifestID = 0
		h := fx.handle(context.Background())
		newExec(fx, &fakeExecIngestor{}, func() string { return fx.workDir }).Execute(h)
		if h.finished || h.failed != "请删除本任务后重新导入" {
			t.Fatalf("过时载荷应显式 Fail，实际 failed=%q", h.failed)
		}
	})

	t.Run("zip 缺失", func(t *testing.T) {
		fx := newExecFixture(t)
		fx.store.rows[fx.taskID].ZipPath = filepath.Join(t.TempDir(), "moved-away.zip")
		h := fx.handle(context.Background())
		ing := &fakeExecIngestor{}
		newExec(fx, ing, func() string { return fx.workDir }).Execute(h)
		if h.finished || h.failed != "导入包不存在或无法打开，请确认包文件仍在原位置" {
			t.Fatalf("zip 缺失应显式 Fail 指明包位置，实际 failed=%q", h.failed)
		}
		if ing.called {
			t.Fatalf("zip 打开失败不应进入回灌")
		}
	})

	t.Run("工作目录未配置", func(t *testing.T) {
		fx := newExecFixture(t)
		h := fx.handle(context.Background())
		newExec(fx, &fakeExecIngestor{}, func() string { return "" }).Execute(h)
		if h.finished || h.failed != "工作目录未配置，无法导入" {
			t.Fatalf("未配置应显式 Fail，实际 failed=%q", h.failed)
		}
	})
}

// TestExtractFilesCancelCheckpoint 解包相位逐条目取消检查点（风险4 落定）：RunCtx 取消后
// 首个条目边界即打断——不读文件源、不建暂存作用域、错误携带取消语义交调用方分流。
func TestExtractFilesCancelCheckpoint(t *testing.T) {
	ing, _, _, workDir := newTestSetup(t)
	manifest, files := buildFixture()
	works := make([]*export.WorkRecord, len(manifest.Works))
	for i := range manifest.Works {
		works[i] = &manifest.Works[i]
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := ing.(*ingestor).extractFiles(ctx, manifest, works, mapFileSource(files), zipStagingFor(workDir))
	if err == nil || !strings.Contains(err.Error(), "解包被暂停/停止中断") {
		t.Fatalf("取消应在条目边界打断解包，实际 err=%v", err)
	}
	if res == nil || len(res.staged) != 0 || res.extractedFiles != 0 {
		t.Fatalf("取消打断不应残留已解包条目，实际 staged=%d extracted=%d", len(res.staged), res.extractedFiles)
	}
	// 检查点先于暂存层调用：作用域未建
	if _, serr := os.Stat(filepath.Join(workDir, "staging", "import")); !os.IsNotExist(serr) {
		t.Fatalf("取消打断不应创建解包暂存作用域")
	}
}
