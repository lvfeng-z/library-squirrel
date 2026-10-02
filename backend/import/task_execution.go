package importer

// import 任务的执行面策略（taskManager.ExecutionStrategy 实现，按 TaskTypeImport 注册进
// Manager 策略表，app.go 装配）。每子任务一次 Execute：
//
// 数据流：按任务 id 查 import_task 领域行（zip 路径 / 共享清单定位 / 本作品 ID）→ 读父作用域
// 共享 manifest → 构造只含本作品的子 manifest → 打开 zip 取文件字节流 → 回灌前置编排
// （ReplacePlanner：查重三分类 → 覆盖确认 → 逐文件内容判定 → 软删+回滚登记，共享能力同
// share-receive）→ ManifestIngestor 回灌导入（zip 条目流文件源 + 任务作用域暂存层 + 编排产物
// 注入 IngestOptions）→ 新建 store 行清单并入终态回滚登记 → Finish。与收件执行器的差异仅在
// 字节来源（zip 条目流 vs 网络拉取流）：暂存可随时重产、恢复全量重跑（zip 整包在手无续传
// 语义），退出路径经暂存层 Release 回收本子任务作用域。
//
// 进度：字节语义。子清单待解包条目声明总字节为 total，解包写暂存的字节经暂存层 writer 的
// 计数外包上报（不动 Ingest 接口签名）；已完整暂存的条目（内容判定拷入 / 恢复残留）计入
// 初始段位。RunCtx 取消（暂停/停止）不上报终态交控制面接管（导出执行器同款分流）。

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/task"
	"github.com/library-squirrel/backend/taskManager"
)

// ImportExecution import（zip 导入）任务的执行面策略。
type ImportExecution struct {
	workDirGetter func() string    // 库根读取器（导入暂存作用域在其下）
	taskStore     ImportTaskStore  // import_task 领域行查询（执行参数来源）
	ingestor      ManifestIngestor // 回灌导入能力（与 share-receive 执行器共用同一实例，app.go 装配）
	planner       *ReplacePlanner  // 回灌前置编排（查重三分类→确认→内容判定→软删+登记，本模块共享能力）
}

// NewImportExecution 创建 import 执行面策略（taskStore 为导入任务领域行查询能力；
// checker/replaceOps/mountReader 为回灌前置编排依赖，汇入 ReplacePlanner——异常装配缺项时
// 编排维持全新建/既有跳过旧语义）。
func NewImportExecution(workDirGetter func() string, taskStore ImportTaskStore, ingestor ManifestIngestor,
	checker duplicate.DuplicateChecker, replaceOps resource.ReplaceStoreOps,
	mountReader StoreMountReader) *ImportExecution {
	return &ImportExecution{
		workDirGetter: workDirGetter,
		taskStore:     taskStore,
		ingestor:      ingestor,
		planner:       NewReplacePlanner(checker, replaceOps, mountReader),
	}
}

// Execute 导入子任务主体：读领域行 → workDir 守卫 → 读共享 manifest → 子 manifest → 打开
// zip → 确保任务暂存作用域 → 回灌前置编排 → 回灌导入 → 回滚登记并入 → Finish。错误分流：
// RunCtx 取消（暂停/停止）不上报终态交控制面接管；领域行缺失或 ManifestID==0（过时载荷）、
// zip 打开失败（包被移动/删除）按用户可读文案显式 Fail。
func (e *ImportExecution) Execute(h taskManager.StrategyHandle) {
	taskID := h.Task().GetID()
	ctx := h.RunCtx()
	it, err := e.taskStore.GetById(ctx, taskID)
	if err != nil || it == nil {
		// 领域行缺失（建树链崩溃窗口遗留）：按过时载荷显式 Fail
		h.Fail("请删除本任务后重新导入")
		return
	}
	workDir := e.workDirGetter()
	if workDir == "" {
		// 请求期导入拒绝：领域文案经任务终态呈现，同时经统一发射口通知前端引导配置
		settings.NotifyWorkDirUnconfigured("import")
		h.Fail("工作目录未配置，无法导入")
		return
	}
	if it.ManifestID == 0 {
		// 过时载荷（存量整体任务）：新代码不兼容存量，不做迁移或降级
		h.Fail("请删除本任务后重新导入")
		return
	}
	// 读父作用域共享 manifest（建树时从包内原字节落盘，子任务不重复解包 manifest 条目）
	manifest, err := readSharedManifest(workDir, it.ManifestRel)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	if manifest.SchemaVersion != export.SchemaVersion {
		h.Fail(fmt.Sprintf("导入清单版本不支持: %d（支持 %d）", manifest.SchemaVersion, export.SchemaVersion))
		return
	}
	// 构造只含本作品的子 manifest（按 ManifestID 定位本作品，查重/暂存/导入均收窄到本作品）
	sub, err := BuildSubManifest(manifest, it.ManifestID)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	logger.Log.Infof("[import] 任务 %d 执行开始 zip=%s manifest=%s", taskID, it.ZipPath, it.ManifestRel)
	reader, err := zip.OpenReader(it.ZipPath)
	if err != nil {
		// 导入中途包被移动/删除：manifest 建树时已解出落盘，仅文件字节仍依赖原包——文案指明位置
		h.Fail("导入包不存在或无法打开，请确认包文件仍在原位置")
		return
	}
	defer func() { _ = reader.Close() }()
	// 子任务暂存作用域：staging/import/{taskID}/（父作用域含共享 manifest，平级不嵌套）；
	// 恢复场景复用既有作用域（zip 解包可重产，残留经 Release/启动清扫回收）
	stagingDir, err := task.EnsureImportScope(ctx, workDir, taskID)
	if err != nil {
		h.Fail(fmt.Sprintf("创建导入暂存目录失败: %v", err))
		return
	}

	// 查重 → 确认 → 逐文件内容判定 → 软删 + 回滚登记（作用域为本作品子集；共享编排能力与
	// share-receive 同源，时序语义同整体路径）
	planStart := time.Now()
	plan, canceled, err := e.planner.PlanReplace(ctx, sub, stagingDir, workDir, h)
	logger.Log.Infof("[import] 任务 %d 查重+确认+内容判定+软删 完成 耗时=%s canceled=%v err=%v", taskID, time.Since(planStart), canceled, err)
	if err != nil {
		failUnlessImportCanceled(h, ctx, err)
		return
	}
	if canceled {
		return // 确认被取消（暂停/停止防御性打断）：不上报终态，交控制面接管
	}
	if ctx.Err() != nil {
		logger.Log.Infof("[import] 任务 %d 查重替换阶段被暂停/停止打断", taskID)
		return // 软删窗口内暂停/停止：回滚清单已登记（交 setFailed 单点），恢复全量重跑
	}

	// 回灌导入子 manifest：文件源=已暂存条目优先（内容判定拷入的匹配文件免解包重读）、
	// 未暂存回落 zip 条目流；暂存层=任务作用域 zip 解包轨（writer 计数外包上报进度、
	// 已完整暂存条目旁路免重写）。替换选项：确认替换与零交集并入注入；二者皆空（无命中
	// 替换）则保持全跳过旧语义（opts=nil）
	skipFiles := FileSkipSet(sub, plan.SkipWorks)
	progress := newImportByteProgress(h, sub, skipFiles, stagingDir)
	fileSource := stagedZipFileSource(&reader.Reader, progress)
	layer := progressStaging{
		inner:   stagedSkipStaging{inner: NewZipUnpackStaging(e.workDirGetter, taskID), complete: progress.stagedComplete},
		onBytes: progress.addBytes,
	}
	var opts *IngestOptions
	if len(plan.ConfirmedWorks) > 0 || len(plan.AutoMergeWorks) > 0 {
		opts = &IngestOptions{
			ReplaceWorks:   plan.ConfirmedWorks,
			AutoMergeWorks: plan.AutoMergeWorks,
		}
	}
	ingestStart := time.Now()
	imported, err := e.ingestor.Ingest(ctx, sub, fileSource, layer, opts)
	if err != nil {
		logger.Log.Debugf("[import] 任务 %d 导入失败 耗时=%s err=%v", taskID, time.Since(ingestStart), err)
		failUnlessImportCanceled(h, ctx, err)
		return
	}
	// 导入建行事务已提交：新建 store 行清单并入终态回滚登记——导入成功到 Finish 之间的
	// 停止/失败窗口内，控制面复活被软删受害者前先物理丢弃新建行（释放其占用的 file_path）
	if len(imported.CreatedStoreIDs) > 0 {
		h.SetTerminalRollback(taskManager.TerminalRollback{CreatedStoreIDs: imported.CreatedStoreIDs})
	}
	logger.Log.Infof("[import] 任务 %d 导入完成 耗时=%s 新建作品 %d 替换 %d 跳过 %d",
		taskID, time.Since(ingestStart), imported.CreatedWorks, imported.ReplacedWorks, imported.SkippedWorks)
	// 成功：本子任务暂存作用域经 Ingest 退出收尾（暂存层 Release）回收；Finish
	h.Finish()
	logger.Log.Infof("[import] 任务 %d 执行完成", taskID)
}

// readSharedManifest 读父作用域共享 manifest：workDir 相对路径（relPath 域正斜杠，建树时
// 原字节落盘），现场 join 为绝对路径读取后反序列化（import 模块自有实现，不经 zip 重读）。
func readSharedManifest(workDir, relPath string) (*export.Manifest, error) {
	if relPath == "" {
		return nil, errors.New("导入任务缺少共享清单路径")
	}
	data, err := os.ReadFile(filepath.Join(workDir, filepath.FromSlash(relPath)))
	if err != nil {
		return nil, fmt.Errorf("读取共享清单失败: %w", err)
	}
	manifest, err := export.Deserialize(data)
	if err != nil {
		return nil, fmt.Errorf("解析共享清单失败: %w", err)
	}
	return manifest, nil
}

// failUnlessImportCanceled 错误分流：RunCtx 已取消（用户暂停/停止）→ 不上报终态交控制面
// 接管（暂停→Paused 可恢复全量重跑、停止→Failed「任务被用户停止」）；否则上报用户可读
// 失败文案。
func failUnlessImportCanceled(h taskManager.StrategyHandle, ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	h.Fail(importUserMessage(err))
}

// importUserMessage 错误 → 用户可读文案（导入链错误定义已自带中文文案，透传并截断防溢出）。
func importUserMessage(err error) string {
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

// ===== 进度（字节语义）与暂存层外包 =====

// importByteProgress 字节进度计数器：total=子清单待解包条目（可落盘且未被裁决跳过）声明
// 总字节；done=已完整暂存条目（初始段位——内容判定拷入/恢复残留）+ 本次解包经计数 writer
// 流入的字节。stagedComplete 兼作「已暂存条目免解包」判定（暂存大小==声明大小）。
type importByteProgress struct {
	h       taskManager.StrategyHandle // 进度上报句柄
	staging string                     // 任务暂存作用域绝对路径（absPath 域）
	sizes   map[string]int64           // 待解包条目路径 → 声明字节
	total   int64
	done    int64
}

// newImportByteProgress 构建进度计数器并上报初始段位（已完整暂存条目计入 done——含内容
// 判定拷入的匹配文件与恢复残留中的完整件；半成品不计，避免全量重写时双重计数）。
func newImportByteProgress(h taskManager.StrategyHandle, sub *export.Manifest, skipFiles map[int64]struct{}, stagingDir string) *importByteProgress {
	p := &importByteProgress{h: h, staging: stagingDir, sizes: make(map[string]int64)}
	for i := range sub.Files {
		entry := &sub.Files[i]
		if entry.Path == "" || entry.Missing {
			continue
		}
		if _, skip := skipFiles[entry.StoreID]; skip {
			continue
		}
		p.sizes[entry.Path] = entry.Size
		p.total += entry.Size
	}
	for entryPath, size := range p.sizes {
		if p.stagedComplete(entryPath) {
			p.done += size
		}
	}
	p.report()
	return p
}

// stagedComplete 条目是否已完整暂存（暂存文件存在且大小==声明大小）：文件源旁路（免解包
// 重读）与暂存层旁路（免重写）的同一判据，两处共用保证读写落点一致。
func (p *importByteProgress) stagedComplete(entryPath string) bool {
	want, ok := p.sizes[entryPath]
	if !ok || want <= 0 {
		return false
	}
	fi, err := os.Stat(StagingPath(p.staging, entryPath))
	return err == nil && fi.Size() == want
}

// addBytes 解包写入字节累计上报（计数 writer 回调；单 goroutine 串行解包，无需互斥）。
func (p *importByteProgress) addBytes(n int64) {
	if n <= 0 {
		return
	}
	p.done += n
	p.report()
}

func (p *importByteProgress) report() {
	if p.total > 0 {
		p.h.ReportProgress(p.total, p.done)
	}
}

// stagedZipFileSource 构建「已暂存条目优先」的 zip 文件源（FileSource 实现）：条目已完整
// 暂存（内容判定拷入/恢复残留——字节已经内容判定或历史解包校验）时读暂存副本免 zip 解压
// 重读；未暂存回落 zip 条目流；两处皆无按包缺文件报错。
func stagedZipFileSource(reader *zip.Reader, p *importByteProgress) FileSource {
	return func(entryPath string) (io.ReadCloser, error) {
		if !safeZipEntryPath(entryPath) {
			return nil, fmt.Errorf("包内路径不合法: %s", entryPath)
		}
		if p.stagedComplete(entryPath) {
			if f, err := os.Open(StagingPath(p.staging, entryPath)); err == nil {
				return f, nil
			}
		}
		for _, f := range reader.File {
			if f.Name == entryPath {
				return f.Open()
			}
		}
		return nil, fmt.Errorf("%w：%s", ErrPackageFileMissing, entryPath)
	}
}

// stagedSkipStaging 已暂存条目旁路暂存层（IngestStaging 装饰器）：complete 命中返回 nil
// writer（内容已在暂存落点——解包相位仅读流实测 sha，落位 rename 直接消费暂存副本），
// 否则委托任务作用域 zip 解包轨。与 stagedZipFileSource 共用同一 complete 判据。
type stagedSkipStaging struct {
	inner    *ZipUnpackStaging // 任务作用域 zip 解包轨（未旁路条目的写入落点）
	complete func(string) bool // 已完整暂存判定（nil=不旁路，恒走写入）
}

// Stage 已完整暂存条目返回 nil writer + 暂存落点登记路径；其余委托解包轨新建写入句柄。
func (s stagedSkipStaging) Stage(ctx context.Context, entryPath string) (io.WriteCloser, string, error) {
	if s.complete != nil && s.complete(entryPath) {
		return nil, s.inner.JournalRel(entryPath), nil
	}
	return s.inner.Stage(ctx, entryPath)
}

func (s stagedSkipStaging) AbortAction() entity.IngestAbortAction { return s.inner.AbortAction() }

func (s stagedSkipStaging) Release(ctx context.Context) { s.inner.Release(ctx) }

// progressStaging 进度计数暂存层（IngestStaging 装饰器）：把解包轨返回的 writer 包一层
// 计数 WriteCloser——解包写字节经回调上报（不动 Ingest/IngestStaging 接口签名，零侵入）。
type progressStaging struct {
	inner   IngestStaging
	onBytes func(int64)
}

func (p progressStaging) Stage(ctx context.Context, entryPath string) (io.WriteCloser, string, error) {
	w, rel, err := p.inner.Stage(ctx, entryPath)
	if err != nil || w == nil {
		return w, rel, err
	}
	return &countingWriteCloser{w: w, onBytes: p.onBytes}, rel, nil
}

func (p progressStaging) AbortAction() entity.IngestAbortAction { return p.inner.AbortAction() }

func (p progressStaging) Release(ctx context.Context) { p.inner.Release(ctx) }

// countingWriteCloser 计数 writer：写入字节经回调上报后透传底层 writer。
type countingWriteCloser struct {
	w       io.WriteCloser
	onBytes func(int64)
}

func (c *countingWriteCloser) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	if n > 0 && c.onBytes != nil {
		c.onBytes(int64(n))
	}
	return n, err
}

func (c *countingWriteCloser) Close() error { return c.w.Close() }

// safeZipEntryPath 包内路径白名单校验：非空、正斜杠相对路径、无穿越段（与 share 收件侧
// stagedFileSource 的同型守卫——文件源读暂存副本前的路径合法前置）。
func safeZipEntryPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}
