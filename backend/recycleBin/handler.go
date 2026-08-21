package recycleBin

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
)

// Handler 回收站 Handler（方法名显式表达条目实体归属：Works=作品条目、Stores=文件条目，
// 为作品集条目（WorkSets）的并列扩展留语义空间）
type Handler struct {
	svc *Service
}

// NewHandler 创建回收站 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// PageWorks 分页查询回收站作品条目
// query: 查询条件（SearchCondition 条件体系 + 排序），见 RecyclePageQuery
func (h *Handler) PageWorks(ctx context.Context, page *model.Page[dto.RecycleWorkDTO], query RecyclePageQuery) *model.ApiResponse[*model.Page[dto.RecycleWorkDTO]] {
	if page == nil {
		page = &model.Page[dto.RecycleWorkDTO]{}
	}
	result, err := h.svc.PageWorks(ctx, page.PageNumber, page.PageSize, query.Conditions, query.SortBy, query.SortOrder)
	if err != nil {
		return model.HandleError[*model.Page[dto.RecycleWorkDTO]](err)
	}
	return model.Success(result)
}

// PageStores 分页查询回收站文件条目（persistent_store 已删行，非「作品已删」聚合形态）
// query: 文件域条件体系，见 dto.RecycleStorePageQuery
func (h *Handler) PageStores(ctx context.Context, page *model.Page[dto.RecycleStoreDTO], query *dto.RecycleStorePageQuery) *model.ApiResponse[*model.Page[dto.RecycleStoreDTO]] {
	if page == nil {
		page = &model.Page[dto.RecycleStoreDTO]{}
	}
	result, err := h.svc.PageStores(ctx, page.PageNumber, page.PageSize, query)
	if err != nil {
		return model.HandleError[*model.Page[dto.RecycleStoreDTO]](err)
	}
	return model.Success(result)
}

// RestoreWork 从回收站复原作品条目
// workId: 已软删作品 ID；overwrite: 检测到业务键被占位时是否将占位作品转入回收站
func (h *Handler) RestoreWork(ctx context.Context, workId int64, overwrite bool) *model.ApiResponse[int64] {
	result, err := h.svc.RestoreWork(ctx, workId, overwrite)
	if err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(result)
}

// PurgeWork 彻底删除回收站作品条目（不可恢复，级联清从属行与备份）
// workId: 已软删作品 ID
func (h *Handler) PurgeWork(ctx context.Context, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.PurgeWork(ctx, workId))
}

// PurgeStore 彻底删除回收站文件条目（不可恢复，条目单位=store 行）
// storeId: 已软删 persistent_store 行 ID
func (h *Handler) PurgeStore(ctx context.Context, storeId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.PurgeStore(ctx, storeId))
}
