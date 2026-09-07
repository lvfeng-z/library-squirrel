package download

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkTaskRepository 作品任务领域行仓储。BaseRepository 以私有字段组合——
// 通用写方法（可直写任意主键的 Create/Save/Updates）不外漏，写路径收口到 ForTask 后缀方法族
type WorkTaskRepository struct {
	base *database.BaseRepository[entity.WorkTask]
}

// NewWorkTaskRepository 创建作品任务领域行仓储
func NewWorkTaskRepository(db *gorm.DB) *WorkTaskRepository {
	return &WorkTaskRepository{
		base: database.NewBaseRepository[entity.WorkTask](db),
	}
}

// dbFromCtx 从 context 获取事务 DB，无事务时返回默认 DB
func (r *WorkTaskRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.base.GORM())
}

// CreateForTask 为任务 taskID 建作品任务领域行。领域行主键与核心行共享值：
// 入参对象携带的 id 一律覆写为 taskID（不信任调用方赋值）；非正任务 id 拒绝写入
func (r *WorkTaskRepository) CreateForTask(ctx context.Context, taskID int64, wt *entity.WorkTask) error {
	if taskID <= 0 {
		return fmt.Errorf("创建 work_task 领域行失败: 所属任务 ID 须为正数，实际 %d", taskID)
	}
	wt.SetID(taskID)
	fillSharedKeyTimestamps(wt.BaseEntity)
	return r.base.Create(ctx, wt)
}

// SaveForTask 为任务 taskID 全字段 UPSERT 作品任务领域行（GORM Save 语义：存在即完整替换）。
// 主键同样覆写为 taskID，不信任调用方赋值；供通用编辑端点写领域行，执行面写入走 CreateForTask/原生 SQL
func (r *WorkTaskRepository) SaveForTask(ctx context.Context, taskID int64, wt *entity.WorkTask) error {
	if taskID <= 0 {
		return fmt.Errorf("保存 work_task 领域行失败: 所属任务 ID 须为正数，实际 %d", taskID)
	}
	wt.SetID(taskID)
	fillSharedKeyTimestamps(wt.BaseEntity)
	return r.base.Save(ctx, wt)
}

// CreateBatchForTask 批量建作品任务领域行：各领域行须已持核心行共享主键（核心行批量落库后
// 由调用方回填 id），任一 id 非正即拒绝整批
func (r *WorkTaskRepository) CreateBatchForTask(ctx context.Context, wts []*entity.WorkTask) error {
	if len(wts) == 0 {
		return nil
	}
	for _, wt := range wts {
		if wt.GetID() <= 0 {
			return fmt.Errorf("批量创建 work_task 领域行失败: 领域行须已持核心行 id，实际 %d", wt.GetID())
		}
		fillSharedKeyTimestamps(wt.BaseEntity)
	}
	return r.base.CreateBatch(ctx, wts)
}

// GetById 按共享主键（=所属任务 id）查询领域行
func (r *WorkTaskRepository) GetById(ctx context.Context, id int64) (*entity.WorkTask, error) {
	return r.base.GetById(ctx, id)
}

// ListByIds 按共享主键集合批量查询领域行（树/分页双查的领域行装配步）
func (r *WorkTaskRepository) ListByIds(ctx context.Context, ids []int64) (map[int64]*entity.WorkTask, error) {
	result := make(map[int64]*entity.WorkTask, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	vals := make([]interface{}, len(ids))
	for i, id := range ids {
		vals[i] = id
	}
	rows, err := r.base.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: clause.Column{Name: "id"}, Values: vals}},
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.GetID()] = row
	}
	return result, nil
}

// UpdatePendingResourceID 更新任务的 pending_resource_id（作品任务领域行）
func (r *WorkTaskRepository) UpdatePendingResourceID(ctx context.Context, taskId int64, resourceID sql.NullInt64) error {
	result := r.dbFromCtx(ctx).WithContext(ctx).Model(&entity.WorkTask{}).Where("id = ?", taskId).Update("pending_resource_id", resourceID)
	return result.Error
}

// UpdateRedownloadSections 批量更新任务的板块重执行选择(store_roles + include_work_info)（作品任务领域行）。
// include_work_info 经 map 写入规避 GORM Updates 跳零值（置 false 须落库）
func (r *WorkTaskRepository) UpdateRedownloadSections(ctx context.Context, taskIds []int64, storeRoles sql.NullString, includeWorkInfo bool) error {
	if len(taskIds) == 0 {
		return nil
	}
	result := r.dbFromCtx(ctx).WithContext(ctx).Model(&entity.WorkTask{}).Where("id IN ?", taskIds).
		Updates(map[string]any{
			"store_roles":       storeRoles,
			"include_work_info": includeWorkInfo,
		})
	return result.Error
}

// BatchUpdatePendingResourceID 批量更新任务的 pending_resource_id（作品任务领域行，CASE WHEN 模式）
func (r *WorkTaskRepository) BatchUpdatePendingResourceID(ctx context.Context, updates map[int64]sql.NullInt64) error {
	if len(updates) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(updates))
	cases := ""
	args := make([]any, 0, len(updates)*2+len(updates))
	for id := range updates {
		ids = append(ids, id)
		cases += "WHEN id = ? THEN ? "
	}
	for _, id := range ids {
		args = append(args, id, updates[id])
	}
	for _, id := range ids {
		args = append(args, id)
	}

	statement := "UPDATE work_task SET pending_resource_id = CASE " + cases + "END WHERE id IN (" + strings.Repeat("?,", len(ids)-1) + "?)"
	result := r.base.GORM().WithContext(ctx).Exec(statement, args...)
	return result.Error
}

// fillSharedKeyTimestamps 共享主键非零时 BaseRepository.Create/Save 不补 CreateTime
// （其自动填充以零主键为条件），零值时间戳在此补齐；调用方已带核心行时间戳（同事务同值场景）则不覆盖
func fillSharedKeyTimestamps(base *model.BaseEntity) {
	if base.GetCreateTime() == 0 {
		base.SetCreateTime(util.GetCurrentTimestamp())
	}
}
