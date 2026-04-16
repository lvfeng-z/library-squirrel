package reWorkAuthor

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
)

// Handler 作品-作者关联 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建作品-作者关联 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 查询操作 ==========

// ListByWorkId 获取单个作品的作者关联信息
// @Summary 获取单个作品的作者关联信息
// @Param workId path int true "作品ID"
// @Success 200 {object} model.ApiResponse[*WorkAuthorDTO]
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[*WorkAuthorDTO] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.Error[*WorkAuthorDTO](err.Error())
	}
	return model.Success(result)
}

// ListByWorkIds 批量获取多个作品的作者关联信息
// @Summary 批量获取多个作品的作者关联信息
// @Param workIds body []int64 true "作品ID列表"
// @Success 200 {object} model.ApiResponse[[]*WorkAuthorsResultDTO]
func (h *Handler) ListByWorkIds(ctx context.Context, workIds []int64) *model.ApiResponse[[]*WorkAuthorsResultDTO] {
	result, err := h.svc.ListByWorkIds(ctx, workIds)
	if err != nil {
		return model.Error[[]*WorkAuthorsResultDTO](err.Error())
	}
	return model.Success(result)
}

// ListLocalAuthorsByWorkId 查询作品关联的本地作者
// @Summary 查询作品关联的本地作者
// @Param workId path int true "作品ID"
// @Success 200 {object} model.ApiResponse[[]*model.RankedLocalAuthor]
func (h *Handler) ListLocalAuthorsByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*model.RankedLocalAuthor] {
	result, err := h.svc.ListLocalAuthorsByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*model.RankedLocalAuthor](err.Error())
	}
	return model.Success(result)
}

// ListSiteAuthorsByWorkId 查询作品关联的站点作者
// @Summary 查询作品关联的站点作者
// @Param workId path int true "作品ID"
// @Success 200 {object} model.ApiResponse[[]*model.RankedSiteAuthor]
func (h *Handler) ListSiteAuthorsByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*model.RankedSiteAuthor] {
	result, err := h.svc.ListSiteAuthorsByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*model.RankedSiteAuthor](err.Error())
	}
	return model.Success(result)
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表（带作品ID）
// @Summary 查询多个作品的本地作者列表（带作品ID）
// @Param workIds body []int64 true "作品ID列表"
// @Success 200 {object} model.ApiResponse[[]*model.RankedLocalAuthorWithWorkId]
func (h *Handler) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) *model.ApiResponse[[]*model.RankedLocalAuthorWithWorkId] {
	result, err := h.svc.ListRankedLocalAuthorWithWorkIdByWorkIds(ctx, workIds)
	if err != nil {
		return model.Error[[]*model.RankedLocalAuthorWithWorkId](err.Error())
	}
	return model.Success(result)
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表（带作品ID）
// @Summary 查询多个作品的站点作者列表（带作品ID）
// @Param workIds body []int64 true "作品ID列表"
// @Success 200 {object} model.ApiResponse[[]*model.RankedSiteAuthorWithWorkId]
func (h *Handler) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) *model.ApiResponse[[]*model.RankedSiteAuthorWithWorkId] {
	result, err := h.svc.ListRankedSiteAuthorWithWorkIdByWorkIds(ctx, workIds)
	if err != nil {
		return model.Error[[]*model.RankedSiteAuthorWithWorkId](err.Error())
	}
	return model.Success(result)
}
