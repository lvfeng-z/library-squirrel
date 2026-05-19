package search

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
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
func (h *Handler) QueryWorkPage(ctx context.Context, pageNumber, pageSize int, conditions []*dto2.SearchCondition) *model.ApiResponse[*model.Page[dto2.WorkFullDTO]] {
	if conditions == nil {
		conditions = []*dto2.SearchCondition{}
	}
	return model.HandleResult(h.svc.QueryWorkPage(ctx, pageNumber, pageSize, conditions))
}

// QueryWorkSetPage 查询作品集分页（通过搜索条件筛选）
func (h *Handler) QueryWorkSetPage(ctx context.Context, page *model.Page[dto2.WorkSetWithCoverDTO], conditions []*dto2.SearchCondition) *model.ApiResponse[*model.Page[dto2.WorkSetWithCoverDTO]] {
	if page == nil {
		page = &model.Page[dto2.WorkSetWithCoverDTO]{}
	}
	if conditions == nil {
		conditions = []*dto2.SearchCondition{}
	}
	return model.HandleResult(h.svc.QueryWorkSetPage(ctx, page.PageNumber, page.PageSize, conditions))
}

// QuerySearchConditionPage 查询搜索条件分页
func (h *Handler) QuerySearchConditionPage(ctx context.Context, page, pageSize int, query *dto2.SearchConditionQuery) *model.ApiResponse[*model.Page[dto2.SelectItem]] {
	return model.HandleResult(h.svc.QuerySearchConditionPage(ctx, page, pageSize, query))
}

// UpdateLastUsed 更新最后使用时间
func (h *Handler) UpdateLastUsed(ctx context.Context, used map[dto2.SearchType][]int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.UpdateLastUsed(ctx, used))
}
