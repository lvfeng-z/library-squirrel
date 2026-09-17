package reWorkTag

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	domain "github.com/library-squirrel/backend/base/model/entity"
)

// Handler 作品-标签关联 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建作品-标签关联 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Link 链接标签到作品。namespaces 与 tagIds 等长配对（local/site 关联均用调用方传值，
// 空串=无 namespace——namespace 是关联级开放维度，site 关联同样开放用户自设）
func (h *Handler) Link(ctx context.Context, tagType int, tagIds []int64, namespaces []string, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.LinkBatchToWork(ctx, workId, tagType, tagIds, namespaces))
}

// Unlink 从作品移除标签（该标签的全部 ns 关联行）
func (h *Handler) Unlink(ctx context.Context, tagType int, tagIds []int64, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.RemoveBatchFromWork(ctx, workId, tagType, tagIds))
}

// UnlinkDimension 精确摘除维度关联行：namespaces 与 tagIds 等长配对，只删 (work, tag, ns) 命中行，
// 不波及同标签其他 ns 行（改 ns 的旧值行删除入口——新值行走 Link）
func (h *Handler) UnlinkDimension(ctx context.Context, tagType int, tagIds []int64, namespaces []string, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.RemoveDimensionFromWork(ctx, workId, tagType, tagIds, namespaces))
}

// ListByWorkId 查询作品关联的所有标签
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*domain.ReWorkTag] {
	return model.HandleResult(h.svc.ListByWorkId(ctx, workId))
}

// ListLocalTagIdsByWorkId 查询作品关联的本地标签ID列表
func (h *Handler) ListLocalTagIdsByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]int64] {
	return model.HandleResult(h.svc.ListLocalTagIdsByWorkId(ctx, workId))
}

// ListSiteTagIdsByWorkId 查询作品关联的站点标签ID列表
func (h *Handler) ListSiteTagIdsByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]int64] {
	return model.HandleResult(h.svc.ListSiteTagIdsByWorkId(ctx, workId))
}
