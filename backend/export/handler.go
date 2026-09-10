package export

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
)

// Handler 导出 Handler（Wails Bind 方法，经 IPC 暴露给前端）。
type Handler struct {
	svc *Service
}

// NewHandler 创建导出 Handler。
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Collect 收集导出数据模型（决策5：id 列表透传——前端把选中 work/workSet id 列表传给后端）。
func (h *Handler) Collect(ctx context.Context, workIDs []int64, workSetIDs []int64) *model.ApiResponse[*ExportModel] {
	result, err := h.svc.Collect(ctx, workIDs, workSetIDs)
	if err != nil {
		return model.HandleError[*ExportModel](err)
	}
	return model.Success(result)
}

// StartExport 创建导出任务并启动（两步建任务：前置校验 → 建任务核心行 + export_task 领域行 →
// 启动执行）。返回新建任务 ID——执行进度与终态经任务面板统一承载。
// outputDir 为空时落盘到工作目录根（默认）；非空为自选输出目录（前端经文件选择器挑选并持久化）。
func (h *Handler) StartExport(ctx context.Context, workIDs []int64, workSetIDs []int64, outputDir string) *model.ApiResponse[*ExportTaskResult] {
	result, err := h.svc.StartExport(ctx, workIDs, workSetIDs, outputDir)
	if err != nil {
		return model.HandleError[*ExportTaskResult](err)
	}
	return model.Success(result)
}
