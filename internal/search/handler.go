package search

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// Handler 搜索 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建搜索 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 查询操作 ==========

// QueryWorkPage 查询作品分页
func (h *Handler) QueryWorkPage(ctx context.Context, page, pageSize int, conditions []*domain.SearchCondition) *model.ApiResponse[*model.Page[domain.WorkFullDTO]] {
	result, err := h.svc.QueryWorkPage(ctx, page, pageSize, conditions)
	if err != nil {
		return model.Error[*model.Page[domain.WorkFullDTO]](err.Error())
	}
	return model.Success(result)
}

// QueryWorkSetPage 查询作品集分页
func (h *Handler) QueryWorkSetPage(ctx context.Context, page, pageSize int, keyword string, siteId int64) *model.ApiResponse[*model.Page[domain.SelectItem]] {
	result, err := h.svc.QueryWorkSetPage(ctx, page, pageSize, keyword, siteId)
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem]](err.Error())
	}
	return model.Success(result)
}

// QuerySearchConditionPage 查询搜索条件分页
func (h *Handler) QuerySearchConditionPage(ctx context.Context, page, pageSize int, query *domain.SearchConditionQuery) *model.ApiResponse[*model.Page[domain.SelectItem]] {
	result, err := h.svc.QuerySearchConditionPage(ctx, page, pageSize, query)
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem]](err.Error())
	}
	return model.Success(result)
}

// UpdateLastUsed 更新最后使用时间
func (h *Handler) UpdateLastUsed(ctx context.Context, used map[domain.SearchType][]int64) *model.ApiResponse[any] {
	if err := h.svc.UpdateLastUsed(ctx, used); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}
