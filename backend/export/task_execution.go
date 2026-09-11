package export

// export 任务的执行面策略（taskManager.ExecutionStrategy 实现，按 task_type 注册进
// Manager 策略表，app.go 装配）：
//   - ExportExecution：读 export_task 领域行取选择参数 → Collect 收集 → Plan 规划 →
//     磁盘预检 → Pack 写目标目录同级 .zip.tmp → 原子 rename 为最终 zip
//
// 进度/终态不经本模块推送：进度经 handle.ReportProgress 上报控制面（任务面板按比值展示），
// 成功/失败终态由任务模块统一通知承载。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/taskManager"
)

// 导出临时文件命名常量：最终 zip 的临时版本写在同一目录（目标盘同级，保证 rename 原子），
// 启动清理与导出前清扫据此识别残留（进程在 rename 前退出、暂停/停止退出的未清理文件）——
// 对齐 merge 包 ls-merge- 先例。导出暂存不进统一暂存根 task-staging/：输出目录可为任意
// 自选目录（含与工作目录异卷），统一暂存根固定在 workDir 卷，跨卷 rename 不可绕，故维持
// 「目标目录同级」私例。
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
	// ErrExportDiskSpace 目标盘剩余空间不足（导出前预检）。
	ErrExportDiskSpace = errors.New("目标盘剩余空间不足")
)

// Exporter 打包执行能力（由 *Packer 实现；接口隔离，供测试替换桩验证取消/失败清理路径）。
type Exporter interface {
	Plan(ctx context.Context, workDir string, model *ExportModel, fileNameFormat string) (*PackStats, error)
	Pack(ctx context.Context, workDir string, model *ExportModel, targetPath string, stats *PackStats, onProgress ProgressFn) error
}

// FileNameFormatProvider 导出文件名模板供给（由设置服务实现，app.go 装入；
// 空值回退默认模板由供给方负责）。
type FileNameFormatProvider interface {
	GetFileNameFormat() string
}

// ExportExecution 导出任务执行面策略：执行参数源为 export_task 领域行（核心行不承载领域
// 载荷，share_task 同形）。暂停/停止由 Pack 逐文件 ctx 检查点退出（.zip.tmp 保留）；恢复/
// 重试均走重新 Execute 全量重跑（zip 不支持续写，重跑前清扫目标目录残留临时文件）。
type ExportExecution struct {
	svc            *Service // 提供 collector（导出数据面）、exportTasks（领域行查询）与 workDir
	packer         Exporter
	fileNameFormat FileNameFormatProvider           // 导出文件名模板供给（nil/空回退默认模板）
	freeSpaceFn    func(dir string) (uint64, error) // 目标盘可用空间查询（测试可替换）
}

// NewExportExecution 创建导出执行面策略。
func NewExportExecution(svc *Service, packer Exporter, fileNameFormat FileNameFormatProvider) *ExportExecution {
	return &ExportExecution{
		svc:            svc,
		packer:         packer,
		fileNameFormat: fileNameFormat,
		freeSpaceFn:    diskFreeSpace,
	}
}

// template 取导出文件名模板：供给方为空回退默认模板。
func (e *ExportExecution) template() string {
	if e.fileNameFormat != nil {
		if tpl := e.fileNameFormat.GetFileNameFormat(); tpl != "" {
			return tpl
		}
	}
	return settings.DefaultFileNameFormat
}

// Execute 导出任务主体至终态：读领域行 → Collect → 工作目录守卫 → Plan → 输出目录解析 →
// 残留清扫 → 磁盘预检 → Pack → 原子 rename → Finish。错误分流：RunCtx 取消（用户暂停/停止）
// 直接返回不上报终态（控制面接管：暂停→Paused、停止→Failed）；其余错误上报用户可读文案。
// 领域行缺失或载荷损坏按过时载荷显式 Fail（建任务链崩溃窗口遗留，重跑无法自愈）。
func (e *ExportExecution) Execute(h taskManager.StrategyHandle) {
	ctx := h.RunCtx()
	taskID := h.Task().GetID()
	et, err := e.svc.exportTasks.GetById(ctx, taskID)
	if err != nil || et == nil {
		h.Fail("请删除本任务后重新导出")
		return
	}
	workIDs, workSetIDs, err := parseExportSelection(et)
	if err != nil {
		h.Fail("请删除本任务后重新导出")
		return
	}
	if len(workIDs) == 0 && len(workSetIDs) == 0 {
		h.Fail(ErrExportEmptySelection.Error())
		return
	}

	model, err := e.svc.collector.Collect(ctx, workIDs, workSetIDs)
	if err != nil {
		failUnlessCanceled(h, ctx, err)
		return
	}

	workDir := e.svc.workDir()
	if workDir == "" {
		// 请求期导出拒绝：领域文案经任务终态呈现，同时经统一发射口通知前端引导配置
		settings.NotifyWorkDirUnconfigured("export")
		h.Fail(ErrExportWorkDirEmpty.Error())
		return
	}

	stats, err := e.packer.Plan(ctx, workDir, model, e.template())
	if err != nil {
		failUnlessCanceled(h, ctx, err)
		return
	}

	outDir := et.OutputDir
	if outDir == "" {
		outDir = workDir
	} else if outDir != workDir {
		// 自选输出目录：确保存在（持久化的路径可能在选后已被删除），创建失败即中止
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			h.Fail(fmt.Sprintf("创建输出目录失败: %v", err))
			return
		}
	}

	// 目标目录残留临时文件清扫：自选目录不在启动清理范围（只扫工作目录），导出前统一清扫
	// 兜底——崩溃残留与暂停/停止残留均在此清除（全量重跑不续写临时文件）。并发导出同目录
	// 写入期句柄占用致删除失败仅告警不中断
	if err := sweepStaleTemp(outDir); err != nil {
		h.Fail(fmt.Sprintf("清理目标目录残留失败: %v", err))
		return
	}

	// 导出前预检目标盘剩余空间：store 模式 zip≈源文件总量，源已占用既有空间，预检的是新增
	// zip 的容量，留 1/10 余量覆盖 zip 目录结构与头部开销；自选目录按所在卷预检
	if err := e.checkDiskSpace(outDir, stats.TotalBytes); err != nil {
		h.Fail(err.Error())
		return
	}

	targetPath := buildExportTargetPath(outDir)
	tempPath := targetPath + exportTempSuffix

	err = e.packer.Pack(ctx, workDir, model, tempPath, stats, func(processedFiles, processedBytes, totalFiles, totalBytes int64) {
		// 字节语义映射到控制面两字段比值模型（前端按比值展示，文件数维度不透出）
		h.ReportProgress(totalBytes, processedBytes)
	})
	if err != nil {
		if ctx.Err() != nil {
			// 暂停/停止：检查点退出保留 .zip.tmp（重跑前清扫回收），终态交控制面接管
			return
		}
		_ = os.Remove(tempPath) // 失败：不留下半成品 zip
		h.Fail(err.Error())
		return
	}

	// 原子替换：临时名 rename 为最终 zip（同目录保证同文件系统，rename 原子）
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		h.Fail(fmt.Sprintf("移动导出产物失败: %v", err))
		return
	}
	h.Finish()
}

// failUnlessCanceled 错误分流：RunCtx 已取消（用户暂停/停止）→ 不上报终态交控制面接管
// （暂停→Paused 可恢复重跑、停止→Failed「任务被用户停止」）；否则上报用户可读失败文案
func failUnlessCanceled(h taskManager.StrategyHandle, ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	h.Fail(err.Error())
}

// checkDiskSpace 预检 dir 所在磁盘是否足够容纳导出 zip；totalBytes<=0（全缺失/空导出）无需预检。
func (e *ExportExecution) checkDiskSpace(dir string, totalBytes int64) error {
	if totalBytes <= 0 {
		return nil
	}
	free, err := e.freeSpaceFn(dir)
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
// 幂等——无残留无副作用；单个删除失败不中断整体清理（残留可能被占用，下次再清）。
func sweepStaleTemp(dir string) error {
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

// parseExportSelection 解析导出任务领域行的选择参数（JSON 数组文本）。载荷损坏（非法 JSON）
// 由调用方按领域行过时显式 Fail。
func parseExportSelection(et *entity.ExportTask) (workIDs, workSetIDs []int64, err error) {
	if err = json.Unmarshal([]byte(et.WorkIDs), &workIDs); err != nil {
		return nil, nil, err
	}
	if err = json.Unmarshal([]byte(et.WorkSetIDs), &workSetIDs); err != nil {
		return nil, nil, err
	}
	return workIDs, workSetIDs, nil
}

// marshalIDList 序列化选择 ID 集为 JSON 数组文本（恒数组形态，空集存 [] 不存 null）。
func marshalIDList(ids []int64) (string, error) {
	if ids == nil {
		ids = []int64{}
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("序列化导出选择失败: %w", err)
	}
	return string(b), nil
}

// exportTaskName 导出任务名：N 为创建时点的选择计数（作品数 + 作品集数）。
func exportTaskName(count int) string {
	return fmt.Sprintf("导出（%d 项）", count)
}
