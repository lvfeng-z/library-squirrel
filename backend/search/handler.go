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

// QueryWorkSetPage 查询作品集分页
func (h *Handler) QueryWorkSetPage(ctx context.Context, pageNumber, pageSize int, keyword string, siteId int64) *model.ApiResponse[*model.Page[dto2.SelectItem]] {
	return model.HandleResult(h.svc.QueryWorkSetPage(ctx, pageNumber, pageSize, keyword, siteId))
}

// QuerySearchConditionPage 查询搜索条件分页
func (h *Handler) QuerySearchConditionPage(ctx context.Context, page, pageSize int, query *dto2.SearchConditionQuery) *model.ApiResponse[*model.Page[dto2.SelectItem]] {
	return model.HandleResult(h.svc.QuerySearchConditionPage(ctx, page, pageSize, query))
}

// UpdateLastUsed 更新最后使用时间
func (h *Handler) UpdateLastUsed(ctx context.Context, used map[dto2.SearchType][]int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.UpdateLastUsed(ctx, used))
}

// ========== DTO 定义 ==========

// WorkSetQueryDTO 作品集查询条件
type WorkSetQueryDTO struct {
	Keyword string `json:"keyword"` // 关键词
	SiteID  *int64 `json:"siteId"`  // 站点ID
}
