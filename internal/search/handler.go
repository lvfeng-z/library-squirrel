package search

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
	dto2 "github.com/library-squirrel/wails/pkg/model/dto"
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
func (h *Handler) QueryWorkPage(ctx context.Context, page *model.Page[dto2.SearchCondition, dto2.SearchCondition]) *model.ApiResponse[*model.Page[dto2.WorkFullDTO, dto2.SearchCondition]] {
	if page == nil {
		page = &model.Page[dto2.SearchCondition, dto2.SearchCondition]{}
	}
	if page.Data == nil {
		page.Data = []*dto2.SearchCondition{}
	}
	result, err := h.svc.QueryWorkPage(ctx, page.PageNumber, page.PageSize, page.Data)
	if err != nil {
		return model.Error[*model.Page[dto2.WorkFullDTO, dto2.SearchCondition]](err.Error())
	}
	return model.Success(result)
}

// QueryWorkSetPage 查询作品集分页
func (h *Handler) QueryWorkSetPage(ctx context.Context, page *model.Page[dto2.SelectItem, WorkSetQueryDTO], keyword string, siteId int64) *model.ApiResponse[*model.Page[dto2.SelectItem, WorkSetQueryDTO]] {
	if page == nil {
		page = &model.Page[dto2.SelectItem, WorkSetQueryDTO]{}
	}
	result, err := h.svc.QueryWorkSetPage(ctx, page.PageNumber, page.PageSize, keyword, siteId)
	if err != nil {
		return model.Error[*model.Page[dto2.SelectItem, WorkSetQueryDTO]](err.Error())
	}
	return model.Success(result)
}

// QuerySearchConditionPage 查询搜索条件分页
func (h *Handler) QuerySearchConditionPage(ctx context.Context, page, pageSize int, query *dto2.SearchConditionQuery) *model.ApiResponse[*model.Page[dto2.SelectItem, dto2.SearchConditionQuery]] {
	result, err := h.svc.QuerySearchConditionPage(ctx, page, pageSize, query)
	if err != nil {
		return model.Error[*model.Page[dto2.SelectItem, dto2.SearchConditionQuery]](err.Error())
	}
	return model.Success(result)
}

// UpdateLastUsed 更新最后使用时间
func (h *Handler) UpdateLastUsed(ctx context.Context, used map[dto2.SearchType][]int64) *model.ApiResponse[any] {
	if err := h.svc.UpdateLastUsed(ctx, used); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== DTO 定义 ==========

// WorkSetQueryDTO 作品集查询条件
type WorkSetQueryDTO struct {
	Keyword string `json:"keyword"` // 关键词
	SiteID  *int64 `json:"siteId"`  // 站点ID
}
