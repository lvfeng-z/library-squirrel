package reWorkAuthor

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
)

// Handler 作品-作者关联 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建作品-作者关联 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 写入操作 ==========

// Link 链接作者到作品。roleNames 与 authorIds 等长配对（local/site 关联均用前端传值，
// 空数组=全无角色），空串角色落 NULL；冲突不翻转来源（插件先建的关联保持 PLUGIN）。
func (h *Handler) Link(ctx context.Context, authorType int, authorIds []int64, roleNames []string, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.LinkBatchToWork(ctx, workId, authorType, authorIds, roleNames))
}

// Unlink 从作品移除作者
func (h *Handler) Unlink(ctx context.Context, authorType int, authorIds []int64, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.RemoveBatchFromWork(ctx, workId, authorType, authorIds))
}

// ========== 查询操作 ==========

// ListByWorkId 获取单个作品的作者关联信息
// @Summary 获取单个作品的作者关联信息
// @Param workId path int true "作品ID"
// @Success 200 {object} model.ApiResponse[*dto.WorkAuthorDTO]
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[*dto.WorkAuthorDTO] {
	return model.HandleResult(h.svc.ListByWorkId(ctx, workId))
}

// ListByWorkIds 批量获取多个作品的作者关联信息
// @Summary 批量获取多个作品的作者关联信息
// @Param workIds body []int64 true "作品ID列表"
// @Success 200 {object} model.ApiResponse[[]*WorkAuthorsResultDTO]
func (h *Handler) ListByWorkIds(ctx context.Context, workIds []int64) *model.ApiResponse[[]*dto.WorkAuthorsResultDTO] {
	return model.HandleResult(h.svc.ListByWorkIds(ctx, workIds))
}

// ListLocalAuthorsByWorkId 查询作品关联的本地作者
// @Summary 查询作品关联的本地作者
// @Param workId path int true "作品ID"
// @Success 200 {object} model.ApiResponse[[]*model.RankedLocalAuthor]
func (h *Handler) ListLocalAuthorsByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto.RankedLocalAuthor] {
	return model.HandleResult(h.svc.ListLocalAuthorsByWorkId(ctx, workId))
}

// ListSiteAuthorsByWorkId 查询作品关联的站点作者
// @Summary 查询作品关联的站点作者
// @Param workId path int true "作品ID"
// @Success 200 {object} model.ApiResponse[[]*model.RankedSiteAuthor]
func (h *Handler) ListSiteAuthorsByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto.RankedSiteAuthor] {
	return model.HandleResult(h.svc.ListSiteAuthorsByWorkId(ctx, workId))
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表（带作品ID）
// @Summary 查询多个作品的本地作者列表（带作品ID）
// @Param workIds body []int64 true "作品ID列表"
// @Success 200 {object} model.ApiResponse[[]*model.RankedLocalAuthorWithWorkId]
func (h *Handler) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) *model.ApiResponse[[]*dto.RankedLocalAuthorWithWorkId] {
	return model.HandleResult(h.svc.ListRankedLocalAuthorWithWorkIdByWorkIds(ctx, workIds))
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表（带作品ID）
// @Summary 查询多个作品的站点作者列表（带作品ID）
// @Param workIds body []int64 true "作品ID列表"
// @Success 200 {object} model.ApiResponse[[]*model.RankedSiteAuthorWithWorkId]
func (h *Handler) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) *model.ApiResponse[[]*dto.RankedSiteAuthorWithWorkId] {
	return model.HandleResult(h.svc.ListRankedSiteAuthorWithWorkIdByWorkIds(ctx, workIds))
}
