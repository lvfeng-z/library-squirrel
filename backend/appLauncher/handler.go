package appLauncher

import (
	"context"

	"github.com/library-squirrel/wails/backend/base/model"
)

// Handler 应用启动器 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建应用启动器 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Open 打开文件
func (h *Handler) Open(ctx context.Context, app ExternalAppEnum, filePath string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Open(app, filePath))
}

// OpenPath 打开路径
func (h *Handler) OpenPath(ctx context.Context, filePath string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.OpenPath(filePath))
}

// OpenExternal 打开外部链接
func (h *Handler) OpenExternal(ctx context.Context, url string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.OpenExternal(url))
}
