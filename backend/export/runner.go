package export

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/library-squirrel/backend/base/logger"
)

// 导出临时文件命名常量：最终 zip 的临时版本写在同一目录（目标盘同级，保证 rename 原子），
// 启动清理据此识别崩溃残留（进程在 rename 前退出时的未清理文件）——对齐 merge 包 ls-merge- 先例。
const (
	exportZipPrefix  = "library-squirrel-export-"
	exportZipSuffix  = ".zip"
	exportTempSuffix = ".zip.tmp"
)

// 导出执行错误定义。
var (
	// ErrExportEmptySelection 导出选择为空（作品/作品集 id 列表均空）。
	ErrExportEmptySelection = errors.New("请至少选择一个作品或作品集")
	// ErrExportWorkDirEmpty 工作目录未配置，无法解析源文件与落盘产物。
	ErrExportWorkDirEmpty = errors.New("工作目录未配置，无法导出")
	// ErrExportDiskSpace 目标盘剩余空间不足（导出前预检，风险6）。
	ErrExportDiskSpace = errors.New("目标盘剩余空间不足")
)

// exportJob 一次进行中的导出：持有脱离 IPC handler ctx 的独立 ctx，供 CancelExport 主动中断。
type exportJob struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Exporter 打包执行能力（由 *Packer 实现；接口隔离，供测试替换桩验证取消/失败清理路径）。
type Exporter interface {
	Plan(ctx context.Context, workDir string, model *ExportModel) (*PackStats, error)
	Pack(ctx context.Context, workDir string, model *ExportModel, targetPath string, stats *PackStats, onProgress ProgressFn) error
}

// Runner 导出异步执行壳：StartExport 立即返回 exportID，后台 goroutine 执行 Collect→Plan→Pack→
// 原子重命名，进度/完成经 emitter 推送。临时文件治理（风险1 对治）：写目标盘同级临时名，
// 成功原子 rename 为最终 zip，失败/取消不留半成品，启动清理残留。
type Runner struct {
	collector   *Collector
	packer      Exporter
	emitter     ExportEventEmitter
	workDirFunc func() string
	freeSpaceFn func(dir string) (uint64, error) // 目标盘可用空间查询（测试可替换）
	jobsMu      sync.Mutex
	jobs        map[string]*exportJob // exportID → 进行中导出（in-flight 守卫 + cancel 锚点）
}

// NewRunner 创建导出执行壳。
func NewRunner(collector *Collector, packer Exporter, emitter ExportEventEmitter, workDirFunc func() string) *Runner {
	return &Runner{
		collector:   collector,
		packer:      packer,
		emitter:     emitter,
		workDirFunc: workDirFunc,
		freeSpaceFn: diskFreeSpace,
		jobs:        make(map[string]*exportJob),
	}
}

// Start 启动异步导出：前置校验（选择非空）通过即注册 in-flight job 并起独立 goroutine，
// 立即返回 exportID（不阻塞 IPC）。进度/完成经 emitter 推送。
// outputDir 为空时落盘到工作目录根，非空为自选输出目录（目标路径/磁盘预检/残留清扫均以它为准）。
func (r *Runner) Start(ctx context.Context, workIDs []int64, workSetIDs []int64, outputDir string) (string, error) {
	if len(workIDs) == 0 && len(workSetIDs) == 0 {
		return "", ErrExportEmptySelection
	}
	exportID := fmt.Sprintf("export-%d", time.Now().UnixNano())
	runCtx, cancel := context.WithCancel(context.Background()) // detached：handler 返回后导出仍跑
	r.jobsMu.Lock()
	r.jobs[exportID] = &exportJob{ctx: runCtx, cancel: cancel}
	r.jobsMu.Unlock()
	go r.run(runCtx, exportID, workIDs, workSetIDs, outputDir)
	return exportID, nil
}

// Cancel 取消指定导出（无进行中导出则 no-op）。
func (r *Runner) Cancel(exportID string) {
	r.jobsMu.Lock()
	if job, ok := r.jobs[exportID]; ok {
		job.cancel()
	}
	r.jobsMu.Unlock()
}

// run 后台导出全流程（Collect→Plan→磁盘预检→Pack→原子重命名），任何退出路径推送 complete。
// workDir 恒为源文件根（collect/plan/pack 读源）；落盘目标 outDir = outputDir 或 workDir 缺省。
func (r *Runner) run(ctx context.Context, exportID string, workIDs, workSetIDs []int64, outputDir string) {
	defer r.removeJob(exportID)
	emitComplete := func(success bool, targetPath, errMsg string) {
		r.emitter.PushComplete(exportID, success, targetPath, errMsg)
	}

	model, err := r.collector.Collect(ctx, workIDs, workSetIDs)
	if err != nil {
		emitComplete(false, "", ctxErrorMessage(ctx, err))
		return
	}

	workDir := r.workDirFunc()
	if workDir == "" {
		emitComplete(false, "", ErrExportWorkDirEmpty.Error())
		return
	}

	stats, err := r.packer.Plan(ctx, workDir, model)
	if err != nil {
		emitComplete(false, "", ctxErrorMessage(ctx, err))
		return
	}

	outDir := outputDir
	if outDir == "" {
		outDir = workDir
	} else if outDir != workDir {
		// 自选输出目录：确保存在（持久化的路径可能在选后已被删除），创建失败即中止
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			emitComplete(false, "", fmt.Sprintf("创建输出目录失败: %v", err))
			return
		}
	}

	// 目标目录残留临时文件清扫：自选目录不在启动清理范围（只扫工作目录），
	// 导出前统一清扫兜底（崩溃残留的 .zip.tmp 不会与本次产物混淆）
	if err := r.sweepStaleTemp(outDir); err != nil {
		emitComplete(false, "", fmt.Sprintf("清理目标目录残留失败: %v", err))
		return
	}

	// 导出前预检目标盘剩余空间（风险6）：store 模式 zip≈源文件总量，源已占用既有空间，
	// 预检的是新增 zip 的容量，留 1/10 余量覆盖 zip 目录结构与头部开销；自选目录按所在卷预检
	if err := r.checkDiskSpace(outDir, stats.TotalBytes); err != nil {
		emitComplete(false, "", err.Error())
		return
	}

	targetPath := buildExportTargetPath(outDir)
	tempPath := targetPath + exportTempSuffix

	err = r.packer.Pack(ctx, workDir, model, tempPath, stats, func(processedFiles, processedBytes, totalFiles, totalBytes int64) {
		r.emitter.PushProgress(ExportProgressData{
			ExportID:       exportID,
			TotalFiles:     totalFiles,
			ProcessedFiles: processedFiles,
			TotalBytes:     totalBytes,
			ProcessedBytes: processedBytes,
		})
	})
	if err != nil {
		_ = os.Remove(tempPath) // 失败/取消：不留下半成品 zip
		emitComplete(false, "", ctxErrorMessage(ctx, err))
		return
	}

	// 原子替换：临时名 rename 为最终 zip（同目录保证同文件系统，rename 原子）
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		emitComplete(false, "", fmt.Sprintf("移动导出产物失败: %v", err))
		return
	}

	emitComplete(true, targetPath, "")
}

// ctxErrorMessage 生成用户可读的失败信息：ctx 已取消（用户点 [取消]）时报「已取消」，
// 否则透传原始错误——取消的语义对用户是主动中止，非缺陷。
func ctxErrorMessage(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		return "已取消"
	}
	return err.Error()
}

// checkDiskSpace 预检 dir 所在磁盘是否足够容纳导出 zip；totalBytes<=0（全缺失/空导出）无需预检。
func (r *Runner) checkDiskSpace(dir string, totalBytes int64) error {
	if totalBytes <= 0 {
		return nil
	}
	free, err := r.freeSpaceFn(dir)
	if err != nil {
		return fmt.Errorf("检查磁盘空间失败: %w", err)
	}
	required := uint64(totalBytes) + uint64(totalBytes)/10
	if free < required {
		return fmt.Errorf("%w：需要约 %.2f GB，剩余 %.2f GB",
			ErrExportDiskSpace, float64(required)/(1<<30), float64(free)/(1<<30))
	}
	return nil
}

// buildExportTargetPath 生成最终 zip 路径：dir 根下 library-squirrel-export-<毫秒时间戳>.zip。
func buildExportTargetPath(dir string) string {
	return filepath.Join(dir, fmt.Sprintf("%s%d%s", exportZipPrefix, time.Now().UnixMilli(), exportZipSuffix))
}

// sweepStaleTemp 清理 dir 下导出临时文件残留（exportZipPrefix 前缀 + exportTempSuffix 后缀文件）。
// 残留场景：导出写入临时文件后、rename 之前进程崩溃。幂等——无残留无副作用；
// 单个删除失败不中断整体清理（残留可能被占用，下次再清）。
func (r *Runner) sweepStaleTemp(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取目录失败: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, exportZipPrefix) && strings.HasSuffix(name, exportTempSuffix) {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				logger.Log.Warnf("清理导出临时文件失败 %s: %v", name, err)
			}
		}
	}
	return nil
}

// CleanupResidualTempFiles 清理工作目录下导出临时文件残留（应用启动时调用，自选目录不在扫描范围——
// 自选目录残留由每次导出前的 sweepStaleTemp(outDir) 兜底）。
func (r *Runner) CleanupResidualTempFiles() error {
	workDir := r.workDirFunc()
	if workDir == "" {
		return nil
	}
	return r.sweepStaleTemp(workDir)
}

// removeJob 从 in-flight 注册表删除指定导出（run 退出时调用）。
func (r *Runner) removeJob(exportID string) {
	r.jobsMu.Lock()
	delete(r.jobs, exportID)
	r.jobsMu.Unlock()
}
