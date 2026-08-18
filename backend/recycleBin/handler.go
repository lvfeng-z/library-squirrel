package recycleBin

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
)

// Handler 回收站 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建回收站 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Page 分页查询回收站列表
// query: 查询条件（SearchCondition 条件体系 + 排序），见 RecyclePageQuery
func (h *Handler) Page(ctx context.Context, page *model.Page[dto.RecycleWorkDTO], query RecyclePageQuery) *model.ApiResponse[*model.Page[dto.RecycleWorkDTO]] {
	if page == nil {
		page = &model.Page[dto.RecycleWorkDTO]{}
	}
	result, err := h.svc.Page(ctx, page.PageNumber, page.PageSize, query.Conditions, query.SortBy, query.SortOrder)
	if err != nil {
		return model.HandleError[*model.Page[dto.RecycleWorkDTO]](err)
	}
	return model.Success(result)
}

// Restore 从回收站复原作品
// workId: 已软删作品 ID；overwrite: 检测到业务键被占位时是否将占位作品转入回收站
func (h *Handler) Restore(ctx context.Context, workId int64, overwrite bool) *model.ApiResponse[int64] {
	result, err := h.svc.RestoreWork(ctx, workId, overwrite)
	if err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(result)
}

// Purge 彻底删除回收站条目（不可恢复）
// workId: 已软删作品 ID
func (h *Handler) Purge(ctx context.Context, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Purge(ctx, workId))
}
