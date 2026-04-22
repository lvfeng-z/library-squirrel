package reWorkTag

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
	domain "github.com/library-squirrel/wails/pkg/model/entity"
)

// Handler 作品-标签关联 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建作品-标签关联 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Link 链接标签到作品
func (h *Handler) Link(ctx context.Context, tagType int, tagIds []int64, workId int64) *model.ApiResponse[any] {
	if err := h.svc.LinkBatchToWork(ctx, workId, tagType, tagIds); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Unlink 从作品移除标签
func (h *Handler) Unlink(ctx context.Context, tagType int, tagIds []int64, workId int64) *model.ApiResponse[any] {
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
