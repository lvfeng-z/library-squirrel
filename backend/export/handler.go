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

// StartExport 启动导出（异步：立即返回 exportID，进度/完成经 export-events 事件推送）。
// outputDir 为空时落盘到工作目录根（默认）；非空为自选输出目录（前端经文件选择器挑选并持久化）。
func (h *Handler) StartExport(ctx context.Context, workIDs []int64, workSetIDs []int64, outputDir string) *model.ApiResponse[string] {
	id, err := h.svc.StartExport(ctx, workIDs, workSetIDs, outputDir)
	if err != nil {
		return model.HandleError[string](err)
	}
	return model.Success(id)
}

// CancelExport 取消指定导出（无进行中导出则 no-op）。
func (h *Handler) CancelExport(ctx context.Context, exportID string) *model.ApiResponse[any] {
	h.svc.CancelExport(exportID)
	return model.Success[any](nil)
}
