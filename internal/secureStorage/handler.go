package secureStorage

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
)

// Handler 安全存储 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建安全存储 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Set 设置值
func (h *Handler) Set(ctx context.Context, storageKey string, plainValue string, description string) *model.ApiResponse[int64] {
	return model.HandleResult(h.svc.Set(ctx, storageKey, plainValue, description))
}

// Delete 删除值
func (h *Handler) Delete(ctx context.Context, storageKey string) *model.ApiResponse[any] {
	_, err := h.svc.Remove(ctx, storageKey)
	if err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// Get 获取值
func (h *Handler) Get(ctx context.Context, storageKey string) *model.ApiResponse[string] {
	return model.HandleResult(h.svc.GetValue(ctx, storageKey))
}

// Keys 获取所有键
func (h *Handler) Keys(ctx context.Context) *model.ApiResponse[[]string] {
	return model.HandleResult(h.svc.ListKeys(ctx))
}
