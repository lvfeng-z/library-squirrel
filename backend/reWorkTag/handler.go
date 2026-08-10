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

// Link 链接标签到作品。namespaces 与 tagIds 等长配对（local=用户自设 ns，空串=无 ns）；
// site 关联的 namespace 由后端按 site_tag.namespace 镜像，namespaces 传值被忽略。
func (h *Handler) Link(ctx context.Context, tagType int, tagIds []int64, namespaces []string, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.LinkBatchToWork(ctx, workId, tagType, tagIds, namespaces))
}

// Unlink 从作品移除标签
func (h *Handler) Unlink(ctx context.Context, tagType int, tagIds []int64, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.RemoveBatchFromWork(ctx, workId, tagType, tagIds))
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
