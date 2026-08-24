package task

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/model"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
)

// TaskRepository 任务仓储实现
type TaskRepository struct {
	*database.BaseRepository[domain.Task]
}

// NewRepository 创建任务仓储
func NewRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{
		BaseRepository: database.NewBaseRepository[domain.Task](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *TaskRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// dbFromCtx 从 context 获取事务 DB，无事务时返回默认 DB
func (r *TaskRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// QueryParentPage 分页查询父任务
func (r *TaskRepository) QueryParentPage(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Task], error) {
	query := r.GORM().WithContext(ctx).Model(&domain.Task{})

	// 查询是父任务的或者只有单个任务的（根级任务 pid=NULL）
	query = query.Where("has_child = 1 OR pid IS NULL")

	for _, cond := range opt.Conditions {
		if cond != nil {
			query = query.Clauses(cond)
		}
	}

	for _, order := range opt.OrderBy {
		if order != nil {
			query = query.Clauses(order)
		}
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页
	offset := (opt.Page - 1) * opt.PageSize
	var tasks []*domain.Task
	if err := query.Offset(offset).Limit(opt.PageSize).Find(&tasks).Error; err != nil {
		return nil, err
	}

	return model.NewPage[domain.Task](tasks, total, opt.Page, opt.PageSize), nil
}

// RefreshTaskStatus 刷新任务状态
func (r *TaskRepository) RefreshTaskStatus(ctx context.Context, taskId int64) (int64, error) {
	statement := fmt.Sprintf(`
			WITH total AS (
				SELECT COUNT(1) AS num FROM task WHERE pid = %d
			),
			finished AS (
				SELECT COUNT(1) AS num FROM task WHERE pid = %d AND status = %d
			),
			failed AS (
				SELECT COUNT(1) AS num FROM task WHERE pid = %d AND status = %d
			),
			processing AS (
				SELECT COUNT(1) AS num FROM task WHERE pid = %d AND status IN (%d, %d)
			),
			paused AS (
				SELECT COUNT(1) AS num FROM task WHERE pid = %d AND status = %d
			)
			UPDATE task SET status = (
				CASE
					WHEN (SELECT num FROM processing) > 0 THEN %d
					WHEN (SELECT num FROM paused) > 0 THEN %d
					WHEN (SELECT num FROM finished) = (SELECT num FROM total) THEN %d
					WHEN (SELECT num FROM failed) = (SELECT num FROM total) THEN %d
					WHEN (SELECT num FROM total) > (SELECT num FROM finished) AND (SELECT num FROM finished) > 0 THEN %d
				END
			)
			WHERE id = %d`,
		taskId,
		taskId, TaskStatusFinished,
		taskId, TaskStatusFailed,
		taskId, TaskStatusProcessing, TaskStatusWaiting,
		taskId, TaskStatusPaused,
		TaskStatusProcessing,
		TaskStatusPaused,
		TaskStatusFinished,
		TaskStatusFailed,
		TaskStatusPartlyFinished,
		taskId)

	result := r.GORM().WithContext(ctx).Exec(statement)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// SetTaskTreeStatus 设置任务树状态（同时清除 error_message）
func (r *TaskRepository) SetTaskTreeStatus(ctx context.Context, taskIds []int64, status TaskStatusEnum, includeStatus ...TaskStatusEnum) (int64, error) {
	if len(taskIds) == 0 {
		return 0, nil
	}

	idsStr := int64ArrayToString(taskIds)

	var statement string
	if len(includeStatus) > 0 {
		includeStatusStr := intArrayToString(intStatusToArray(includeStatus[0]))
		statement = fmt.Sprintf(`
				WITH children AS (
					SELECT id, has_child FROM task WHERE id IN (%s) AND has_child = 0
				),
				parent AS (
					SELECT id, has_child FROM task WHERE id IN (%s) AND has_child = 1
				)
				UPDATE task SET status = %d, error_message = NULL WHERE id IN (
					SELECT id FROM children WHERE status IN (%s)
					UNION
					SELECT id FROM parent WHERE status IN (%s)
					UNION
					SELECT id FROM task WHERE id IN (SELECT pid FROM children) AND status IN (%s)
					UNION
					SELECT id FROM task WHERE pid IN (SELECT id FROM parent) AND status IN (%s)
				)`,
			idsStr, idsStr, status, includeStatusStr, includeStatusStr, includeStatusStr, includeStatusStr)
	} else {
		statement = fmt.Sprintf(`
				WITH children AS (
					SELECT id, has_child FROM task WHERE id IN (%s) AND has_child = 0
				),
				parent AS (
					SELECT id, has_child FROM task WHERE id IN (%s) AND has_child = 1
				)
				UPDATE task SET status = %d, error_message = NULL WHERE id IN (
					SELECT id FROM children
					UNION
					SELECT id FROM parent
					UNION
					SELECT id FROM task WHERE id IN (SELECT pid FROM children)
					UNION
					SELECT id FROM task WHERE pid IN (SELECT id FROM parent)
				)`,
			idsStr, idsStr, status)
	}

	result := r.GORM().WithContext(ctx).Exec(statement)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// UpdatePendingResourceID 更新任务的 pending_resource_id
func (r *TaskRepository) UpdatePendingResourceID(ctx context.Context, taskId int64, resourceID sql.NullInt64) error {
	result := r.dbFromCtx(ctx).WithContext(ctx).Model(&domain.Task{}).Where("id = ?", taskId).Update("pending_resource_id", resourceID)
	return result.Error
}

// UpdateRedownloadSections 批量更新任务的板块重执行选择(store_roles + include_work_info)
func (r *TaskRepository) UpdateRedownloadSections(ctx context.Context, taskIds []int64, storeRoles sql.NullString, includeWorkInfo bool) error {
	if len(taskIds) == 0 {
		return nil
	}
	result := r.dbFromCtx(ctx).WithContext(ctx).Model(&domain.Task{}).Where("id IN ?", taskIds).
		Updates(map[string]any{
			"store_roles":       storeRoles,
			"include_work_info": includeWorkInfo,
		})
	return result.Error
}

// BatchUpdatePendingResourceID 批量更新任务的 pending_resource_id（CASE WHEN 模式）
func (r *TaskRepository) BatchUpdatePendingResourceID(ctx context.Context, updates map[int64]sql.NullInt64) error {
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

	statement := "UPDATE task SET pending_resource_id = CASE " + cases + "END WHERE id IN (" + strings.Repeat("?,", len(ids)-1) + "?)"
	result := r.GORM().WithContext(ctx).Exec(statement, args...)
	return result.Error
}

// BatchSetStatus 批量设置任务状态（同时更新 error_message）
func (r *TaskRepository) BatchSetStatus(ctx context.Context, statuses map[int64]StatusUpdate) error {
	if len(statuses) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(statuses))
	statusCases := ""
	errMsgCases := ""
	args := make([]any, 0, len(statuses)*4+len(statuses))
	for id := range statuses {
		ids = append(ids, id)
		statusCases += "WHEN id = ? THEN ? "
		errMsgCases += "WHEN id = ? THEN ? "
	}
	// status CASE 参数
	for _, id := range ids {
		args = append(args, id, statuses[id].Status)
	}
	// error_message CASE 参数
	for _, id := range ids {
		args = append(args, id, statuses[id].ErrorMessage)
	}
	// IN 子句参数
	for _, id := range ids {
		args = append(args, id)
	}

	statement := "UPDATE task SET status = CASE " + statusCases + "END, error_message = CASE " + errMsgCases + "END WHERE id IN (" + strings.Repeat("?,", len(ids)-1) + "?)"
	result := r.GORM().WithContext(ctx).Exec(statement, args...)

	return result.Error
}

// ListTaskTree 获取任务树列表
func (r *TaskRepository) ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...TaskStatusEnum) ([]*domain.Task, error) {
	if len(taskIds) == 0 {
		return make([]*domain.Task, 0), nil
	}

	idsStr := int64ArrayToString(taskIds)

	var statement string
	if len(includeStatus) > 0 {
		statusStr := intArrayToString(intStatusToArray(includeStatus[0]))
		statement = fmt.Sprintf(`
				WITH children AS (
					SELECT * FROM task WHERE id IN (%s) AND has_child = 0 AND status IN (%s)
				),
				parent AS (
					SELECT * FROM task WHERE id IN (%s) AND has_child = 1
				)
				SELECT * FROM children
				UNION
				SELECT * FROM parent
				UNION
				SELECT t.* FROM task t WHERE t.id IN (SELECT pid FROM children)
				UNION
				SELECT t.* FROM task t WHERE t.pid IN (SELECT id FROM parent) AND t.status IN (%s)`,
			idsStr, statusStr, idsStr, statusStr)
	} else {
		statement = fmt.Sprintf(`
				WITH children AS (
					SELECT * FROM task WHERE id IN (%s) AND has_child = 0
				),
				parent AS (
					SELECT * FROM task WHERE id IN (%s) AND has_child = 1
				)
				SELECT * FROM children
				UNION
				SELECT * FROM parent
				UNION
				SELECT t.* FROM task t WHERE t.id IN (SELECT pid FROM children)
				UNION
				SELECT t.* FROM task t WHERE t.pid IN (SELECT id FROM parent)`,
			idsStr, idsStr)
	}

	var tasks []*domain.Task
	err := r.GORM().WithContext(ctx).Raw(statement).Scan(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// ListStatus 查询状态列表
func (r *TaskRepository) ListStatus(ctx context.Context, ids []int64) ([]*domain.Task, error) {
	if len(ids) == 0 {
		return make([]*domain.Task, 0), nil
	}

	var tasks []*domain.Task
	err := r.GORM().WithContext(ctx).Where("id IN ?", ids).Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// CreateTask 创建任务
func (r *TaskRepository) CreateTask(ctx context.Context, task *domain.Task) error {
	return r.Create(ctx, task)
}

// ListChildrenTask 查询子任务列表
func (r *TaskRepository) ListChildrenTask(ctx context.Context, pid int64) ([]*domain.Task, error) {
	var tasks []*domain.Task
	err := r.GORM().WithContext(ctx).Where("pid = ?", pid).Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// ListBySiteAndSiteWorkID 根据站点和站点作品ID查询关联任务列表（按创建时间倒序）
func (r *TaskRepository) ListBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) ([]*domain.Task, error) {
	var tasks []*domain.Task
	err := r.GORM().WithContext(ctx).
		Where("site_id = ? AND site_work_id = ?", siteId, siteWorkId).
		Order("create_time DESC").
		Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// QueryChildrenTaskPage 查询子任务分页
func (r *TaskRepository) QueryChildrenTaskPage(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Task], error) {
	query := r.GORM().WithContext(ctx).Model(&domain.Task{})

	for _, cond := range opt.Conditions {
		if cond != nil {
			query = query.Clauses(cond)
		}
	}

	for _, order := range opt.OrderBy {
		if order != nil {
			query = query.Clauses(order)
		}
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页
	offset := (opt.Page - 1) * opt.PageSize
	var tasks []*domain.Task
	if err := query.Offset(offset).Limit(opt.PageSize).Find(&tasks).Error; err != nil {
		return nil, err
	}

	return model.NewPage[domain.Task](tasks, total, opt.Page, opt.PageSize), nil
}

// ListSchedule 查询任务进度列表
func (r *TaskRepository) ListSchedule(ctx context.Context, ids []int64) ([]*domain.Task, error) {
	return r.ListStatus(ctx, ids)
}

// CountBySiteId 统计站点的任务引用行数（站点删除守卫用，由 site 经窄接口注入；task 无软删）
func (r *TaskRepository) CountBySiteId(ctx context.Context, siteId int64) (int64, error) {
	var count int64
	err := r.dbFromCtx(ctx).WithContext(ctx).
		Model(new(domain.Task)).
		Where("site_id = ?", siteId).
		Count(&count).Error
	return count, err
}

// ClearResourceTaskId 批量清空资源行对任务及其子任务的 task_id 引用（置 NULL=非任务产）。
// 任务行删除链的前置步：外键强制下引用未清即删任务行被拒；子任务行同随删除链消亡，引用面一并覆盖
func (r *TaskRepository) ClearResourceTaskId(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.dbFromCtx(ctx).WithContext(ctx).
		Exec("UPDATE resource SET task_id = NULL WHERE task_id IN (SELECT id FROM task WHERE id IN ? OR pid IN ?)", ids, ids).Error
}

// DeleteTask 删除任务（包含子任务）- 批量删除
// dbFromCtx 模式：删除链在事务内执行（先清 resource.task_id 引用再删行，见 Service.DeleteTask）
func (r *TaskRepository) DeleteTask(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	// 先删除所有子任务
	if err := r.dbFromCtx(ctx).WithContext(ctx).Where("pid IN ?", ids).Delete(&domain.Task{}).Error; err != nil {
		return err
	}
	// 再删除主任务
	return r.dbFromCtx(ctx).WithContext(ctx).Where("id IN ?", ids).Delete(&domain.Task{}).Error
}

// listChildrenByParentsTask 按父任务ID列表查询子任务
func (r *TaskRepository) listChildrenByParentsTask(ctx context.Context, pids []int64) ([]*domain.Task, error) {
	if len(pids) == 0 {
		return make([]*domain.Task, 0), nil
	}

	idsStr := int64ArrayToString(pids)
	statement := fmt.Sprintf(`
			SELECT * FROM task
			WHERE pid IN (%s)`,
		idsStr)

	var tasks []*domain.Task
	err := r.GORM().WithContext(ctx).Raw(statement).Scan(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// 辅助函数：将int64数组转换为逗号分隔的字符串
func int64ArrayToString(ids []int64) string {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(strs, ",")
}

// 辅助函数：将int数组转换为逗号分隔的字符串
func intArrayToString(ids []int) string {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(strs, ",")
}

// 辅助函数：将TaskStatusEnum数组转换为int数组
func intStatusToArray(status TaskStatusEnum) []int {
	return []int{int(status)}
}
