package frontendLog

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
)

// Handler 前端日志 Handler，暴露给前端用于批量上报 console 日志
type Handler struct {
	svc *Service
}

// NewHandler 创建前端日志 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Write 接收前端批量日志并落盘到 frontend.log
func (h *Handler) Write(ctx context.Context, entries []FrontendLogEntry) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Write(ctx, entries))
}
