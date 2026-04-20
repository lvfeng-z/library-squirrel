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
// 不嵌入 database.BaseRepository 以避免 Page 返回类型的泛型限制问题
type taskRepository struct {
	db *gorm.DB
}

// NewRepository 创建任务仓储
func NewRepository(db *gorm.DB) Repository {
	return &taskRepository{
		db: db,
	}
}

// GORM 返回底层 GORM DB 实例
func (r *taskRepository) GORM() *gorm.DB {
	return r.db
}

// Save 保存
func (r *taskRepository) Save(ctx context.Context, task *domain.Task) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// SaveBatch 批量保存
func (r *taskRepository) SaveBatch(ctx context.Context, tasks []*domain.Task) error {
	if len(tasks) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(tasks).Error
}

// Update 更新
func (r *taskRepository) Update(ctx context.Context, task *domain.Task) error {
	return r.db.WithContext(ctx).Save(task).Error
}

// GetById 根据ID获取
func (r *taskRepository) GetById(ctx context.Context, id int64) (*domain.Task, error) {
	var task domain.Task
	err := r.db.WithContext(ctx).First(&task, id).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// Get 根据条件获取单个
func (r *taskRepository) Get(ctx context.Context, opt *database.QueryOption) (*domain.Task, error) {
	var task domain.Task
	db := r.db.WithContext(ctx).Model(new(domain.Task))
	db = applyQueryOption(db, opt)
	err := db.First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// List 查询列表
func (r *taskRepository) List(ctx context.Context, opt *database.QueryOption) ([]*domain.Task, error) {
	var tasks []*domain.Task
	db := r.db.WithContext(ctx).Model(new(domain.Task))
	db = applyQueryOption(db, opt)
	err := db.Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// Count 统计数量
func (r *taskRepository) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	var count int64
	db := r.db.WithContext(ctx).Model(new(domain.Task))
	db = applyQueryOption(db, opt)
	err := db.Count(&count).Error
	return count, err
}

// Delete 删除
func (r *taskRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(new(domain.Task), id).Error
}

// Page 分页查询
func (r *taskRepository) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Task, TaskQueryDTO], error) {
	page := opt.Page
	pageSize := opt.PageSize

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	// 构建查询选项（设置 Limit 和 Offset）
	queryOpt := opt.QueryOption
	queryOpt.Limit = pageSize
	queryOpt.Offset = offset

	// 查询列表
	list, err := r.List(ctx, &queryOpt)
	if err != nil {
		return nil, err
	}

	// 统计总数（不需要 Limit 和 Offset）
	countOpt := opt.QueryOption
	countOpt.Limit = 0
	countOpt.Offset = 0
	total, err := r.Count(ctx, &countOpt)
	if err != nil {
		return nil, err
	}

	return model.NewPage[domain.Task, TaskQueryDTO](list, total, page, pageSize), nil
}

// QueryParentPage 分页查询父任务
func (r *taskRepository) QueryParentPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.Task, TaskQueryDTO], error) {
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

	return model.NewPage[domain.Task, TaskQueryDTO](tasks, total, page, pageSize), nil
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
func (r *taskRepository) QueryChildrenTaskPage(ctx context.Context, pid int64, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.Task, TaskQueryDTO], error) {
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

	return model.NewPage[domain.Task, TaskQueryDTO](tasks, total, page, pageSize), nil
}

// ListSchedule 查询任务进度列表
func (r *taskRepository) ListSchedule(ctx context.Context, ids []int64) ([]*TaskScheduleDTO, error) {
	return r.ListStatus(ctx, ids)
}

// DeleteTask 删除任务（包含子任务）- 批量删除
func (r *taskRepository) DeleteTask(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	// 先删除所有子任务
	if err := r.GORM().WithContext(ctx).Where("pid IN ?", ids).Delete(&domain.Task{}).Error; err != nil {
		return err
	}
	// 再删除主任务
	return r.GORM().WithContext(ctx).Where("id IN ?", ids).Delete(&domain.Task{}).Error
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

// applyQueryOption 将 QueryOption 应用到 db 实例
func applyQueryOption(db *gorm.DB, opt *database.QueryOption) *gorm.DB {
	// 1. Select（覆盖型）
	if opt.Select != nil {
		db = db.Select(opt.Select)
	}

	// 2. Joins（叠加型）
	for _, join := range opt.Joins {
		db = db.Clauses(join)
	}

	// 3. Conditions（叠加型）
	for _, cond := range opt.Conditions {
		db = db.Where(cond)
	}

	// 4. OrderBy（叠加型）
	if len(opt.OrderBy) > 0 {
		db = db.Order(opt.OrderBy)
	}

	// 5. GroupBy（覆盖型）
	if opt.GroupBy != nil {
		db = db.Clauses(opt.GroupBy)
	}

	// 6. Having（覆盖型）
	if opt.Having != nil {
		db = db.Having(opt.Having)
	}

	// 7. Limit & Offset（覆盖型）
	if opt.Limit > 0 {
		db = db.Limit(opt.Limit)
	}
	if opt.Offset > 0 {
		db = db.Offset(opt.Offset)
	}

	return db
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
