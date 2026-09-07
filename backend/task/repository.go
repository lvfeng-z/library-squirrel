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
	"gorm.io/gorm/clause"
)

// TaskRepository 任务仓储实现：核心表 task 的读写 + 作品/分享领域行仓储的组合持有
type TaskRepository struct {
	*database.BaseRepository[domain.Task]
	workTaskRepo  *WorkTaskRepository
	shareTaskRepo *ShareTaskRepository
}

// NewRepository 创建任务仓储
func NewRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{
		BaseRepository: database.NewBaseRepository[domain.Task](db),
		workTaskRepo:   NewWorkTaskRepository(db),
		shareTaskRepo:  NewShareTaskRepository(db),
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

// workTaskLeftJoin 任务查询挂作品任务领域表的左连接：领域列（site_id/plugin_public_id 等）
// 经全限定列名过滤/排序；1:1 共享主键连接不放大行集，无领域行的任务（内置类型）不因连接被过滤
func workTaskLeftJoin() clause.Join {
	return clause.Join{
		Type:  clause.LeftJoin,
		Table: clause.Table{Name: "work_task"},
		ON: clause.Where{Exprs: []clause.Expression{
			clause.Eq{
				Column: clause.Column{Table: "work_task", Name: "id"},
				Value:  clause.Column{Table: clause.CurrentTable, Name: "id"},
			},
		}},
	}
}

// applyTaskQueryClauses 应用任务查询子句：领域表左连接 + 条件 + 排序
func applyTaskQueryClauses(query *gorm.DB, opt *database.QueryOption) *gorm.DB {
	query = query.Clauses(clause.From{Joins: []clause.Join{workTaskLeftJoin()}})
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
	return query
}

// TaskTreeRows 任务树查询结果：核心行全集 + 领域行按共享主键 id 关联成表
// （无对应领域行的任务——内置类型/父容器——不在 map 中）
type TaskTreeRows struct {
	Tasks      []*domain.Task
	WorkTasks  map[int64]*domain.WorkTask
	ShareTasks map[int64]*domain.ShareTask
}

// QueryParentPage 分页查询父任务
func (r *TaskRepository) QueryParentPage(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Task], error) {
	query := r.GORM().WithContext(ctx).Model(&domain.Task{})

	// 查询是父任务的或者只有单个任务的（根级任务 pid=NULL）
	query = applyTaskQueryClauses(query, &opt.QueryOption).
		Where("task.has_child = 1 OR task.pid IS NULL")

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

// UpdatePendingResourceID 更新任务的 pending_resource_id（作品任务领域行）
func (r *TaskRepository) UpdatePendingResourceID(ctx context.Context, taskId int64, resourceID sql.NullInt64) error {
	result := r.dbFromCtx(ctx).WithContext(ctx).Model(&domain.WorkTask{}).Where("id = ?", taskId).Update("pending_resource_id", resourceID)
	return result.Error
}

// UpdateRedownloadSections 批量更新任务的板块重执行选择(store_roles + include_work_info)（作品任务领域行）。
// include_work_info 经 map 写入规避 GORM Updates 跳零值（置 false 须落库）
func (r *TaskRepository) UpdateRedownloadSections(ctx context.Context, taskIds []int64, storeRoles sql.NullString, includeWorkInfo bool) error {
	if len(taskIds) == 0 {
		return nil
	}
	result := r.dbFromCtx(ctx).WithContext(ctx).Model(&domain.WorkTask{}).Where("id IN ?", taskIds).
		Updates(map[string]any{
			"store_roles":       storeRoles,
			"include_work_info": includeWorkInfo,
		})
	return result.Error
}

// BatchUpdatePendingResourceID 批量更新任务的 pending_resource_id（作品任务领域行，CASE WHEN 模式）
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

	statement := "UPDATE work_task SET pending_resource_id = CASE " + cases + "END WHERE id IN (" + strings.Repeat("?,", len(ids)-1) + "?)"
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

// ListTaskTree 获取任务树列表：核心行圈定（id/pid/has_child/status 条件不变）后按 id 集批量
// 查各类领域行，返回双查组装结构
func (r *TaskRepository) ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...TaskStatusEnum) (*TaskTreeRows, error) {
	rows := &TaskTreeRows{
		Tasks:      make([]*domain.Task, 0),
		WorkTasks:  make(map[int64]*domain.WorkTask),
		ShareTasks: make(map[int64]*domain.ShareTask),
	}
	if len(taskIds) == 0 {
		return rows, nil
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

	if err := r.GORM().WithContext(ctx).Raw(statement).Scan(&rows.Tasks).Error; err != nil {
		return nil, err
	}
	if len(rows.Tasks) == 0 {
		return rows, nil
	}

	// 按 id 集批量查领域行（ELIMINATE_N_PLUS_1_QUERY：一次各类单查）
	allIds := make([]int64, 0, len(rows.Tasks))
	for _, t := range rows.Tasks {
		allIds = append(allIds, t.GetID())
	}
	workTasks, err := r.workTaskRepo.ListByIds(ctx, allIds)
	if err != nil {
		return nil, err
	}
	shareTasks, err := r.shareTaskRepo.ListByIds(ctx, allIds)
	if err != nil {
		return nil, err
	}
	rows.WorkTasks = workTasks
	rows.ShareTasks = shareTasks
	return rows, nil
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

// CreateTask 创建任务核心行
func (r *TaskRepository) CreateTask(ctx context.Context, task *domain.Task) error {
	return r.Create(ctx, task)
}

// CreateWorkTaskForTask 为已落库核心行建作品任务领域行（主键覆写为 taskID；事务感知）
func (r *TaskRepository) CreateWorkTaskForTask(ctx context.Context, taskID int64, wt *domain.WorkTask) error {
	return r.workTaskRepo.CreateForTask(ctx, taskID, wt)
}

// CreateWorkTaskBatch 批量建作品任务领域行（各领域行须已持核心行共享主键）
func (r *TaskRepository) CreateWorkTaskBatch(ctx context.Context, wts []*domain.WorkTask) error {
	return r.workTaskRepo.CreateBatchForTask(ctx, wts)
}

// SaveWorkTaskForTask 全字段 UPSERT 作品任务领域行（通用编辑端点用）
func (r *TaskRepository) SaveWorkTaskForTask(ctx context.Context, taskID int64, wt *domain.WorkTask) error {
	return r.workTaskRepo.SaveForTask(ctx, taskID, wt)
}

// GetWorkTaskById 按共享主键（=所属任务 id）查询作品任务领域行
func (r *TaskRepository) GetWorkTaskById(ctx context.Context, taskID int64) (*domain.WorkTask, error) {
	return r.workTaskRepo.GetById(ctx, taskID)
}

// ListWorkTasksByIds 按共享主键集合批量查询作品任务领域行
func (r *TaskRepository) ListWorkTasksByIds(ctx context.Context, ids []int64) (map[int64]*domain.WorkTask, error) {
	return r.workTaskRepo.ListByIds(ctx, ids)
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

// ListBySiteAndSiteWorkID 根据站点和站点作品ID查询关联任务列表（按创建时间倒序）。
// 站点身份列在作品任务领域行，经共享主键左连接过滤
func (r *TaskRepository) ListBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) ([]*domain.Task, error) {
	var tasks []*domain.Task
	err := r.GORM().WithContext(ctx).Model(&domain.Task{}).
		Clauses(clause.From{Joins: []clause.Join{workTaskLeftJoin()}}).
		Where("work_task.site_id = ? AND work_task.site_work_id = ?", siteId, siteWorkId).
		Order("work_task.create_time DESC").
		Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// QueryChildrenTaskPage 查询子任务分页
func (r *TaskRepository) QueryChildrenTaskPage(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Task], error) {
	query := r.GORM().WithContext(ctx).Model(&domain.Task{})
	query = applyTaskQueryClauses(query, &opt.QueryOption)

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

// ClearResourceTaskId 批量清空资源行对任务及其子任务的 task_id 引用（置 NULL=非任务产）。
// 任务行删除链的前置步：外键强制下引用未清即删任务行被拒；子任务行同随删除链消亡，引用面一并覆盖。
// resource.task_id 引用 work_task（与 task.id 同值 1:1），此处按 task.id 圈定即可命中同值领域行
func (r *TaskRepository) ClearResourceTaskId(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.dbFromCtx(ctx).WithContext(ctx).
		Exec("UPDATE resource SET task_id = NULL WHERE task_id IN (SELECT id FROM task WHERE id IN ? OR pid IN ?)", ids, ids).Error
}

// DeleteTask 删除任务（包含子任务）- 批量删除。
// dbFromCtx 模式：删除链在事务内执行——清 resource.task_id 引用（见 Service.DeleteTask）→
// 删 work_task/share_task 领域行 → 删核心行（共享主键 id→task 外键要求领域行先于核心行消亡）
func (r *TaskRepository) DeleteTask(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	db := r.dbFromCtx(ctx).WithContext(ctx)

	// 子任务核心行 id 集（其领域行随删除链一并消亡）
	var childIds []int64
	if err := db.Model(&domain.Task{}).Where("pid IN ?", ids).Pluck("id", &childIds).Error; err != nil {
		return err
	}
	allIds := ids
	if len(childIds) > 0 {
		allIds = append(append([]int64{}, ids...), childIds...)
	}

	// 领域行先删：共享主键外键（id→task）下，核心行先删会被在册领域行拒绝
	if err := db.Where("id IN ?", allIds).Delete(&domain.WorkTask{}).Error; err != nil {
		return err
	}
	if err := db.Where("id IN ?", allIds).Delete(&domain.ShareTask{}).Error; err != nil {
		return err
	}

	// 先删除所有子任务核心行，再删除主任务核心行
	if err := db.Where("pid IN ?", ids).Delete(&domain.Task{}).Error; err != nil {
		return err
	}
	return db.Where("id IN ?", ids).Delete(&domain.Task{}).Error
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
