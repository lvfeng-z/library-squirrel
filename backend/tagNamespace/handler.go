package tagNamespace

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
)

// Handler 标签 namespace 维度清单 Handler（前端候选选择器的清单拉取面）
type Handler struct {
	svc *Service
}

// NewHandler 创建标签 namespace 维度清单 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// List 拉取全量 ns 清单（value/label/origin/lastUse 全字段返回，value 升序）；
// 分组（内置/用户/插件）与 last_use 排序归前端
func (h *Handler) List(ctx context.Context) *model.ApiResponse[[]*entity.TagNamespace] {
	return model.HandleResult(h.svc.ListAll(ctx))
}
