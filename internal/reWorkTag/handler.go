package reWorkTag

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// Handler 作品-标签关联 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建作品-标签关联 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Save 保存关联
func (h *Handler) Save(ctx context.Context, workId int64, tagType int, tagId int64) *model.ApiResponse[any] {
	if err := h.svc.LinkTagToWork(ctx, workId, tagType, tagId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// SaveBatch 批量保存关联
func (h *Handler) SaveBatch(ctx context.Context, workId int64, tagType int, tagIds []int64) *model.ApiResponse[any] {
	if err := h.svc.LinkBatchToWork(ctx, workId, tagType, tagIds); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Delete 删除关联
func (h *Handler) Delete(ctx context.Context, workId int64, tagType int, tagId int64) *model.ApiResponse[any] {
	if err := h.svc.UnlinkTagFromWork(ctx, workId, tagType, tagId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// DeleteBatch 批量删除关联
func (h *Handler) DeleteBatch(ctx context.Context, workId int64, tagType int, tagIds []int64) *model.ApiResponse[any] {
	if err := h.svc.RemoveBatchFromWork(ctx, workId, tagType, tagIds); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ListByWorkId 查询作品关联的所有标签
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*domain.ReWorkTag] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*domain.ReWorkTag](err.Error())
	}
	return model.Success(result)
}

// ListLocalTagIdsByWorkId 查询作品关联的本地标签ID列表
func (h *Handler) ListLocalTagIdsByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]int64] {
	result, err := h.svc.ListLocalTagIdsByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]int64](err.Error())
	}
	return model.Success(result)
}

// ListSiteTagIdsByWorkId 查询作品关联的站点标签ID列表
func (h *Handler) ListSiteTagIdsByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]int64] {
	result, err := h.svc.ListSiteTagIdsByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]int64](err.Error())
	}
	return model.Success(result)
}
