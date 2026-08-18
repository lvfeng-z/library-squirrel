package recycleBin

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/database"
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
// query: 查询条件（时间范围/站点/作者/标签 + 排序），见 RecycleQueryDTO
func (h *Handler) Page(ctx context.Context, page *model.Page[RecycleItemDTO], query RecycleQueryDTO) *model.ApiResponse[*model.Page[RecycleItemDTO]] {
	if page == nil {
		page = &model.Page[RecycleItemDTO]{}
	}
	opt := &database.PageOption{
		Page:     page.PageNumber,
		PageSize: page.PageSize,
	}
	result, err := h.svc.Page(ctx, opt, &query)
	if err != nil {
		return model.HandleError[*model.Page[RecycleItemDTO]](err)
	}
	return model.Success(result)
}

// Restore 从回收站复原作品
// overwrite: 检测到 (site_id, site_work_id) 冲突时是否覆盖已存在作品
func (h *Handler) Restore(ctx context.Context, id int64, overwrite bool) *model.ApiResponse[int64] {
	result, err := h.svc.RestoreWork(ctx, id, overwrite)
	if err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(result)
}

// Purge 彻底删除回收站条目（不可恢复）
func (h *Handler) Purge(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Purge(ctx, id))
}
