package task

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// taskRepository 任务仓储实现
// 嵌入 database.BaseRepository[domain.Task] 获得基础 CRUD 实现
type taskRepository struct {
	*database.BaseRepository[domain.Task]
}

// NewRepository 创建任务仓储
func NewRepository(db *gorm.DB) Repository {
	return &taskRepository{
		BaseRepository: database.NewBaseRepository[domain.Task](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *taskRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// Page 分页查询
func (r *taskRepository) Page(ctx context.Context, page, pageSize int, conditions []clause.Expression, order clause.Expression) (*model.Page[domain.Task], error) {
	data, total, err := r.BaseRepository.Page(ctx, page, pageSize, conditions, order)
	if err != nil {
		return nil, err
	}
	return model.NewPage(data, total, page, pageSize), nil
}

// QueryParentPage 分页查询父任务
func (r *taskRepository) QueryParentPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.Task], error) {
	query := r.GORM().WithContext(ctx).Model(&domain.Task{})

	// 查询是父任务的或者只有单个任务的
	query = query.Where("is_collection = 1 OR pid IS NULL OR pid = 0")

	if where != nil {
		query = query.Clauses(where)
	}

	if order != nil {
		query = query.Clauses(order)
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页
	offset := (page - 1) * pageSize
	var tasks []*domain.Task
	if err := query.Offset(offset).Limit(pageSize).Find(&tasks).Error; err != nil {
		return nil, err
	}

	return model.NewPage(tasks, total, page, pageSize), nil
}

// RefreshTaskStatus 刷新任务状态
func (r *taskRepository) RefreshTaskStatus(ctx context.Context, taskId int64) (int64, error) {
	statement := fmt.Sprintf(`
		WITH total AS (
			SELECT COUNT(1) AS num FROM task WHERE pid = %d
		),
		finished AS (
			SELECT COUNT(1) AS num FROM task WHERE pid = %d AND status = %d
		),
		failed AS (
			SELECT COUNT(1) AS num FROM task WHERE pid = %d AND status = %d
		)
		UPDATE task SET status = (
			CASE
				WHEN (SELECT num FROM finished) = (SELECT num FROM total) THEN %d
				WHEN (SELECT num FROM failed) = (SELECT num FROM total) THEN %d
				WHEN (SELECT num FROM total) > (SELECT num FROM finished) AND (SELECT num FROM finished) > 0 THEN %d
			END
		)
		WHERE id = %d`,
		taskId, taskId, TaskStatusFinished, taskId, TaskStatusFailed,
		TaskStatusFinished, TaskStatusFailed, TaskStatusPartlyFinished, taskId)

	result := r.GORM().WithContext(ctx).Exec(statement)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// SetTaskTreeStatus 设置任务树状态
func (r *taskRepository) SetTaskTreeStatus(ctx context.Context, taskIds []int64, status TaskStatusEnum, includeStatus ...TaskStatusEnum) (int64, error) {
	if len(taskIds) == 0 {
		return 0, nil
	}

	idsStr := int64ArrayToString(taskIds)

	var statement string
	if len(includeStatus) > 0 {
		includeStatusStr := intArrayToString(intStatusToArray(includeStatus[0]))
		statement = fmt.Sprintf(`
			WITH children AS (
				SELECT id, is_collection FROM task WHERE id IN (%s) AND is_collection = 0
			),
			parent AS (
				SELECT id, is_collection FROM task WHERE id IN (%s) AND is_collection = 1
			)
			UPDATE task SET status = %d WHERE id IN (
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
				SELECT id, is_collection FROM task WHERE id IN (%s) AND is_collection = 0
			),
			parent AS (
				SELECT id, is_collection FROM task WHERE id IN (%s) AND is_collection = 1
			)
			UPDATE task SET status = %d WHERE id IN (
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

// ListTaskTree 获取任务树列表
func (r *taskRepository) ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...TaskStatusEnum) ([]*domain.Task, error) {
	if len(taskIds) == 0 {
		return make([]*domain.Task, 0), nil
	}

	idsStr := int64ArrayToString(taskIds)

	var statement string
	if len(includeStatus) > 0 {
		statusStr := intArrayToString(intStatusToArray(includeStatus[0]))
		statement = fmt.Sprintf(`
			WITH children AS (
				SELECT * FROM task WHERE id IN (%s) AND is_collection = 0 AND status IN (%s)
			),
			parent AS (
				SELECT * FROM task WHERE id IN (%s) AND is_collection = 1
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
				SELECT * FROM task WHERE id IN (%s) AND is_collection = 0
			),
			parent AS (
				SELECT * FROM task WHERE id IN (%s) AND is_collection = 1
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
func (r *taskRepository) ListStatus(ctx context.Context, ids []int64) ([]*TaskScheduleDTO, error) {
	if len(ids) == 0 {
		return make([]*TaskScheduleDTO, 0), nil
	}

	idsStr := int64ArrayToString(ids)
	statement := fmt.Sprintf(`
		SELECT id, pid, status,
			CASE WHEN status = %d THEN 100 END AS schedule
		FROM task
		WHERE id IN (%s)`,
		TaskStatusFinished, idsStr)

	var results []*TaskScheduleDTO
	err := r.GORM().WithContext(ctx).Raw(statement).Scan(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// CreateTask 创建任务
func (r *taskRepository) CreateTask(ctx context.Context, task *domain.Task) error {
	return r.Save(ctx, task)
}

// ListChildrenTask 查询子任务列表
func (r *taskRepository) ListChildrenTask(ctx context.Context, pid int64) ([]*domain.Task, error) {
	var tasks []*domain.Task
	err := r.GORM().WithContext(ctx).Where("pid = ?", pid).Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// QueryChildrenTaskPage 查询子任务分页
func (r *taskRepository) QueryChildrenTaskPage(ctx context.Context, pid int64, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.Task], error) {
	query := r.GORM().WithContext(ctx).Model(&domain.Task{}).Where("pid = ?", pid)

	if where != nil {
		query = query.Clauses(where)
	}

	if order != nil {
		query = query.Clauses(order)
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页
	offset := (page - 1) * pageSize
	var tasks []*domain.Task
	if err := query.Offset(offset).Limit(pageSize).Find(&tasks).Error; err != nil {
		return nil, err
	}

	return model.NewPage(tasks, total, page, pageSize), nil
}

// ListSchedule 查询任务进度列表
func (r *taskRepository) ListSchedule(ctx context.Context, ids []int64) ([]*TaskScheduleDTO, error) {
	return r.ListStatus(ctx, ids)
}

// DeleteTask 删除任务（包含子任务）
func (r *taskRepository) DeleteTask(ctx context.Context, id int64) error {
	// 先删除所有子任务
	if err := r.GORM().WithContext(ctx).Where("pid = ?", id).Delete(&domain.Task{}).Error; err != nil {
		return err
	}
	// 再删除主任务
	return r.Delete(ctx, id)
}

// listChildrenByParentsTask 按父任务ID列表查询子任务
func (r *taskRepository) listChildrenByParentsTask(ctx context.Context, pids []int64) ([]*domain.Task, error) {
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
