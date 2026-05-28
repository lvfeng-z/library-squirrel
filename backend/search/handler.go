package search

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
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
func (h *Handler) QueryWorkPage(ctx context.Context, pageNumber, pageSize int, conditions []*sdkdto.SearchCondition) *model.ApiResponse[*model.Page[sdkdto.WorkFullDTO]] {
	if conditions == nil {
		conditions = []*sdkdto.SearchCondition{}
	}
	return model.HandleResult(h.svc.QueryWorkPage(ctx, pageNumber, pageSize, conditions))
}

// QueryWorkSetPage 查询作品集分页（通过搜索条件筛选）
func (h *Handler) QueryWorkSetPage(ctx context.Context, page *model.Page[sdkdto.WorkSetWithCoverDTO], conditions []*sdkdto.SearchCondition) *model.ApiResponse[*model.Page[sdkdto.WorkSetWithCoverDTO]] {
	if page == nil {
		page = &model.Page[sdkdto.WorkSetWithCoverDTO]{}
	}
	if conditions == nil {
		conditions = []*sdkdto.SearchCondition{}
	}
	return model.HandleResult(h.svc.QueryWorkSetPage(ctx, page.PageNumber, page.PageSize, conditions))
}

// QuerySearchConditionPage 查询搜索条件分页
func (h *Handler) QuerySearchConditionPage(ctx context.Context, page, pageSize int, query *sdkdto.SearchConditionQuery) *model.ApiResponse[*model.Page[sdkdto.SelectItem]] {
	return model.HandleResult(h.svc.QuerySearchConditionPage(ctx, page, pageSize, query))
}

// UpdateLastUsed 更新最后使用时间
func (h *Handler) UpdateLastUsed(ctx context.Context, used map[sdkdto.SearchType][]int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.UpdateLastUsed(ctx, used))
}
