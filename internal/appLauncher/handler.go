package appLauncher

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
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
	if err := h.svc.Open(app, filePath); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// OpenPath 打开路径
func (h *Handler) OpenPath(ctx context.Context, filePath string) *model.ApiResponse[any] {
	if err := h.svc.OpenPath(filePath); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// OpenExternal 打开外部链接
func (h *Handler) OpenExternal(ctx context.Context, url string) *model.ApiResponse[any] {
	if err := h.svc.OpenExternal(url); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}
