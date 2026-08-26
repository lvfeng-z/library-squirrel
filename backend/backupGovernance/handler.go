package backupGovernance

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
)

// Handler 备份治理 Handler（备份管理面板数据面）。
// 归属本模块而非 backup：「有主/无主」引用状态是业务知识（哪些业务行引用了备份），
// backup 纯保管能力包按 MODULE_BOUNDARY_PURITY 不得感知，数据面经本模块编排提供
type Handler struct {
	svc *Service
}

// NewHandler 创建备份治理 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// PageBackups 分页查询备份保管清单（create_time 倒序）
// page: 分页参数（pageNumber/pageSize）；referenced: 引用态过滤（nil=全部 / true=有主 / false=无主）
func (h *Handler) PageBackups(ctx context.Context, page *model.Page[BackupDTO], referenced *bool) *model.ApiResponse[*model.Page[BackupDTO]] {
	if page == nil {
		page = &model.Page[BackupDTO]{}
	}
	result, err := h.svc.PageBackups(ctx, page.PageNumber, page.PageSize, referenced)
	if err != nil {
		return model.HandleError[*model.Page[BackupDTO]](err)
	}
	return model.Success(result)
}

// DeleteBackups 批量删除备份（磁盘文件与清单行）。任一 id 被业务行引用即整体拒绝；
// 「清理全部无主」的批量圈定取 GetBackupStats 的 expiredOrphanIds（超保留期）
func (h *Handler) DeleteBackups(ctx context.Context, ids []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.DeleteBackups(ctx, ids))
}

// DeleteBackupRecords 仅删除备份清单行（不动磁盘文件）——文件删除失败（被占用/只读等）
// 后用户明确选择「仅删记录」的降级路径；引用检查与 DeleteBackups 同源
func (h *Handler) DeleteBackupRecords(ctx context.Context, ids []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.DeleteBackupRecords(ctx, ids))
}

// RunReconciliationNow 手动触发一轮双向对账（与定时巡检互斥），返回清理统计
func (h *Handler) RunReconciliationNow(ctx context.Context) *model.ApiResponse[ReconciliationResult] {
	return model.Success(h.svc.RunReconciliationNow(ctx))
}

// GetBackupStats 备份占用统计：总占用 / 有主·无主拆分 / 按引用方分组 / 无主超期圈定（短 TTL 缓存）
func (h *Handler) GetBackupStats(ctx context.Context) *model.ApiResponse[*BackupStatsDTO] {
	result, err := h.svc.GetBackupStats(ctx)
	if err != nil {
		return model.HandleError[*BackupStatsDTO](err)
	}
	return model.Success(result)
}
