package recycleBin

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/shareLock"
)

// Handler 回收站 Handler（方法名显式表达条目实体归属：Works=作品条目、Stores=文件条目，
// 为作品集条目（WorkSets）的并列扩展留语义空间）
type Handler struct {
	svc      *Service
	workLock shareLock.ShareLockRegistry // 作品锁注册中心（强制解锁 IPC 直通；守卫查询在 Service 内经窄接口）
}

// NewHandler 创建回收站 Handler
func NewHandler(svc *Service, workLock shareLock.ShareLockRegistry) *Handler {
	return &Handler{svc: svc, workLock: workLock}
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

// PageWorkSets 分页查询回收站作品集条目（work_set 已删行）
// query: 作品集域平铺条件体系，见 dto.RecycleWorkSetPageQuery
func (h *Handler) PageWorkSets(ctx context.Context, page *model.Page[dto.RecycleWorkSetDTO], query *dto.RecycleWorkSetPageQuery) *model.ApiResponse[*model.Page[dto.RecycleWorkSetDTO]] {
	if page == nil {
		page = &model.Page[dto.RecycleWorkSetDTO]{}
	}
	result, err := h.svc.PageWorkSets(ctx, page.PageNumber, page.PageSize, query)
	if err != nil {
		return model.HandleError[*model.Page[dto.RecycleWorkSetDTO]](err)
	}
	return model.Success(result)
}

// RestoreWorkSet 从回收站复原作品集条目
// workSetId: 已软删作品集 ID；overwrite: 检测到业务键被占位时是否将占位作品集转入回收站
func (h *Handler) RestoreWorkSet(ctx context.Context, workSetId int64, overwrite bool) *model.ApiResponse[int64] {
	result, err := h.svc.RestoreWorkSet(ctx, workSetId, overwrite)
	if err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(result)
}

// PurgeWorkSet 彻底删除回收站作品集条目（不可恢复，级联清成员与父子关联行）
// workSetId: 已软删作品集 ID
func (h *Handler) PurgeWorkSet(ctx context.Context, workSetId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.PurgeWorkSet(ctx, workSetId))
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

// PurgeWork 彻底删除回收站作品条目（不可恢复，级联清从属行与备份）。
// 文件删除失败（被占用/只读等）时保留记录并返回错误——前端据此询问「仅删记录或放弃」（PurgeWorkRecords）
// workId: 已软删作品 ID
func (h *Handler) PurgeWork(ctx context.Context, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.PurgeWork(ctx, workId))
}

// PurgeWorkRecords 仅删除回收站作品条目记录（不动磁盘文件）——文件删除失败后用户明确选择「仅删记录」的降级路径
// workId: 已软删作品 ID
func (h *Handler) PurgeWorkRecords(ctx context.Context, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.PurgeWorkRecords(ctx, workId))
}

// PurgeStore 彻底删除回收站文件条目（不可恢复，条目单位=store 行）。
// 文件删除失败（被占用/只读等）时保留记录并返回错误——前端据此询问「仅删记录或放弃」（PurgeStoreRecords）
// storeId: 已软删 persistent_store 行 ID
func (h *Handler) PurgeStore(ctx context.Context, storeId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.PurgeStore(ctx, storeId))
}

// PurgeStoreRecords 仅删除回收站文件条目记录（不动磁盘文件）——文件删除失败后用户明确选择「仅删记录」的降级路径
// storeId: 已软删 persistent_store 行 ID
func (h *Handler) PurgeStoreRecords(ctx context.Context, storeId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.PurgeStoreRecords(ctx, storeId))
}

// RestoreStore 复原文件条目（版本回滚置换：行内备份还原为当前版本，被置换的当前活行转入回收站）
// storeId: 已软删 persistent_store 行 ID（须有备份且挂载活作品）
func (h *Handler) RestoreStore(ctx context.Context, storeId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.RestoreStore(ctx, storeId))
}

// ForceUnlockWork 强制解锁作品锁。作品正被分享拉取持有时，触碰其活行 store 文件的操作
// （替换软删、复原置换、覆盖转移）会被拒并返回 shareLock.ErrWorkLocked；用户知情接受
// 在途拉取可能失败后调用本方法清除该作品的全部会话引用，再重试原操作即可放行
// workId: 被锁作品 ID
func (h *Handler) ForceUnlockWork(ctx context.Context, workId int64) *model.ApiResponse[any] {
	h.workLock.ForceUnlock(ctx, workId)
	return model.Success[any](nil)
}
