package appLauncher

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
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

// OpenImage 打开图片资源
func (h *Handler) OpenImage(ctx context.Context, url string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.OpenImage(url))
}

// OpenPath 打开路径
func (h *Handler) OpenPath(ctx context.Context, filePath string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.OpenPath(filePath))
}

// OpenAbsolutePath 使用系统默认应用打开绝对路径（文件或目录），不做 workdir 拼接。
func (h *Handler) OpenAbsolutePath(ctx context.Context, path string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.OpenAbsolutePath(path))
}

// OpenExternal 打开外部链接
func (h *Handler) OpenExternal(ctx context.Context, url string) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.OpenExternal(url))
}
