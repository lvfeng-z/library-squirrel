package pluginTaskUrlListener

import (
	"context"

	"github.com/library-squirrel/wails/backend/base/model"
)

// Handler 插件任务URL监听器 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建插件任务URL监听器 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ListListener 根据URL获取监听此链接的插件列表
func (h *Handler) ListListener(ctx context.Context, url string) *model.ApiResponse[[]*PluginWithContribution] {
	result := h.svc.ListListener(url)
	return model.Success(result)
}
