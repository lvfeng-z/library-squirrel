package task

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// Handler 任务 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建任务 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存任务
func (h *Handler) Save(ctx context.Context, task *TaskDTO) *model.ApiResponse[int64] {
	domainTask := &domain.Task{
		BaseEntity: &model.BaseEntity{},
	}
	if task.ID != 0 {
		domainTask.SetID(task.ID)
	}
	if task.Pid != nil {
		domainTask.Pid.Valid = true
		domainTask.Pid.Int64 = *task.Pid
	}
	if task.TaskName != nil {
		domainTask.TaskName.Valid = true
		domainTask.TaskName.String = *task.TaskName
	}
	if task.SiteID != nil {
		domainTask.SiteID.Valid = true
		domainTask.SiteID.Int64 = *task.SiteID
	}
	if task.SiteWorkID != nil {
		domainTask.SiteWorkID.Valid = true
		domainTask.SiteWorkID.String = *task.SiteWorkID
	}
	if task.URL != nil {
		domainTask.URL.Valid = true
		domainTask.URL.String = *task.URL
	}
	if task.Status != 0 {
		domainTask.Status = task.Status
	}
	if task.IsCollection != nil {
		domainTask.IsCollection.Valid = true
		domainTask.IsCollection.Int64 = *task.IsCollection
	}
	if task.PluginPublicID != nil {
		domainTask.PluginPublicID.Valid = true
		domainTask.PluginPublicID.String = *task.PluginPublicID
	}
	if task.PluginContributionID != nil {
		domainTask.PluginContributionID.Valid = true
		domainTask.PluginContributionID.String = *task.PluginContributionID
	}
	if task.PluginData != nil {
		domainTask.PluginData.Valid = true
		domainTask.PluginData.String = *task.PluginData
	}

	if err := h.svc.Save(ctx, domainTask); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainTask.GetID())
}

// Update 更新任务
func (h *Handler) Update(ctx context.Context, task *TaskDTO) *model.ApiResponse[any] {
	domainTask := &domain.Task{
		BaseEntity: &model.BaseEntity{},
	}
	if task.ID == 0 {
		return model.Error[any]("更新任务失败，id不能为空")
	}
	domainTask.SetID(task.ID)
	if task.Pid != nil {
		domainTask.Pid.Valid = true
		domainTask.Pid.Int64 = *task.Pid
	}
	if task.TaskName != nil {
		domainTask.TaskName.Valid = true
		domainTask.TaskName.String = *task.TaskName
	}
	if task.SiteID != nil {
		domainTask.SiteID.Valid = true
		domainTask.SiteID.Int64 = *task.SiteID
	}
	if task.SiteWorkID != nil {
		domainTask.SiteWorkID.Valid = true
		domainTask.SiteWorkID.String = *task.SiteWorkID
	}
	if task.URL != nil {
		domainTask.URL.Valid = true
		domainTask.URL.String = *task.URL
	}
	if task.Status != 0 {
		domainTask.Status = task.Status
	}
	if task.IsCollection != nil {
		domainTask.IsCollection.Valid = true
		domainTask.IsCollection.Int64 = *task.IsCollection
	}
	if task.PluginPublicID != nil {
		domainTask.PluginPublicID.Valid = true
		domainTask.PluginPublicID.String = *task.PluginPublicID
	}
	if task.PluginContributionID != nil {
		domainTask.PluginContributionID.Valid = true
		domainTask.PluginContributionID.String = *task.PluginContributionID
	}
	if task.PluginData != nil {
		domainTask.PluginData.Valid = true
		domainTask.PluginData.String = *task.PluginData
	}

	if err := h.svc.Update(ctx, domainTask); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// RefreshStatus 刷新任务状态
func (h *Handler) RefreshStatus(ctx context.Context, taskId int64) *model.ApiResponse[int64] {
	result, err := h.svc.RefreshTaskStatus(ctx, taskId)
	if err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(result)
}

// SetTreeStatus 设置任务树状态
func (h *Handler) SetTreeStatus(ctx context.Context, taskIds []int64, status int, includeStatus []int) *model.ApiResponse[int64] {
	// 转换status
	taskStatus := TaskStatusEnum(status)
	// 转换includeStatus
	var incStatus []TaskStatusEnum
	for _, s := range includeStatus {
		incStatus = append(incStatus, TaskStatusEnum(s))
	}

	result, err := h.svc.SetTreeStatus(ctx, taskIds, taskStatus, incStatus...)
	if err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(result)
}

// CreateTask 创建任务
func (h *Handler) CreateTask(ctx context.Context, req *CreateTaskRequest) *model.ApiResponse[int64] {
	result, err := h.svc.CreateTask(ctx, req)
	if err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(result.GetID())
}

// DeleteTask 删除任务（包含子任务）- 批量删除
func (h *Handler) DeleteTask(ctx context.Context, ids []int64) *model.ApiResponse[any] {
	if err := h.svc.DeleteTask(ctx, ids); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// CreateTaskByURL 根据传入的url创建任务
func (h *Handler) CreateTaskByURL(ctx context.Context, url string) *model.ApiResponse[*CreateTaskByURLResponse] {
	result, err := h.svc.CreateTaskByURL(ctx, url)
	if err != nil {
		return model.Error[*CreateTaskByURLResponse](err.Error())
	}
	return model.Success(result)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*TaskResultDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*TaskResultDTO](err.Error())
	}
	return model.Success(ToTaskResultDTO(result))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[TaskQueryDTO]) *model.ApiResponse[*model.Page[TaskResultDTO]] {
	if page == nil {
		page = &model.Page[TaskQueryDTO]{}
	}
	if page.Data == nil || len(page.Data) == 0 {
		page.Data = []*TaskQueryDTO{{}}
	}
	result, err := h.svc.PageByDTO(ctx, page.PageNumber, page.PageSize, *page.Data[0])
	if err != nil {
		return model.Error[*model.Page[TaskResultDTO]](err.Error())
	}
	return model.Success(ToTaskPageResultDTO(result))
}

// QueryParentPage 分页查询父任务
func (h *Handler) QueryParentPage(ctx context.Context, page *model.Page[TaskQueryDTO]) *model.ApiResponse[*model.Page[TaskResultDTO]] {
	if page == nil {
		page = &model.Page[TaskQueryDTO]{}
	}
	if page.Data == nil || len(page.Data) == 0 {
		page.Data = []*TaskQueryDTO{{}}
	}
	result, err := h.svc.QueryParentPageByDTO(ctx, page.PageNumber, page.PageSize, *page.Data[0])
	if err != nil {
		return model.Error[*model.Page[TaskResultDTO]](err.Error())
	}
	return model.Success(ToTaskPageResultDTO(result))
}

// QueryChildrenTaskPage 查询子任务分页
func (h *Handler) QueryChildrenTaskPage(ctx context.Context, pid int64, page *model.Page[TaskQueryDTO]) *model.ApiResponse[*model.Page[TaskResultDTO]] {
	if page == nil {
		page = &model.Page[TaskQueryDTO]{}
	}
	if page.Data == nil || len(page.Data) == 0 {
		page.Data = []*TaskQueryDTO{{}}
	}
	result, err := h.svc.QueryChildrenTaskPageByDTO(ctx, pid, page.PageNumber, page.PageSize, *page.Data[0])
	if err != nil {
		return model.Error[*model.Page[TaskResultDTO]](err.Error())
	}
	return model.Success(ToTaskPageResultDTO(result))
}

// ListChildrenTask 查询子任务列表
func (h *Handler) ListChildrenTask(ctx context.Context, pid int64) *model.ApiResponse[[]*TaskResultDTO] {
	result, err := h.svc.ListChildrenTask(ctx, pid)
	if err != nil {
		return model.Error[[]*TaskResultDTO](err.Error())
	}
	// 转换为 ResultDTO
	resultDTOs := make([]*TaskResultDTO, len(result))
	for i, task := range result {
		resultDTOs[i] = ToTaskResultDTO(task)
	}
	return model.Success(resultDTOs)
}

// QueryTreeDataPage 查询任务树数据分页
func (h *Handler) QueryTreeDataPage(ctx context.Context, page *model.Page[TaskQueryDTO]) *model.ApiResponse[*TreeDataPageDTO] {
	result, err := h.svc.QueryTreeDataPage(ctx, page)
	if err != nil {
		return model.Error[*TreeDataPageDTO](err.Error())
	}
	return model.Success(result)
}

// ListTaskTree 获取任务树列表
func (h *Handler) ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...int) *model.ApiResponse[[]*TaskResultDTO] {
	// 转换 includeStatus
	var statusEnums []TaskStatusEnum
	for _, s := range includeStatus {
		statusEnums = append(statusEnums, TaskStatusEnum(s))
	}
	result, err := h.svc.ListTaskTree(ctx, taskIds, statusEnums...)
	if err != nil {
		return model.Error[[]*TaskResultDTO](err.Error())
	}
	// 转换为 ResultDTO
	resultDTOs := make([]*TaskResultDTO, len(result))
	for i, task := range result {
		resultDTOs[i] = ToTaskResultDTO(task)
	}
	return model.Success(resultDTOs)
}

// ListStatus 查询状态列表
func (h *Handler) ListStatus(ctx context.Context, ids []int64) *model.ApiResponse[[]*TaskScheduleDTO] {
	result, err := h.svc.ListStatus(ctx, ids)
	if err != nil {
		return model.Error[[]*TaskScheduleDTO](err.Error())
	}
	return model.Success(result)
}

// ListSchedule 查询任务进度列表
func (h *Handler) ListSchedule(ctx context.Context, ids []int64) *model.ApiResponse[[]*TaskScheduleDTO] {
	result, err := h.svc.ListSchedule(ctx, ids)
	if err != nil {
		return model.Error[[]*TaskScheduleDTO](err.Error())
	}
	return model.Success(result)
}

// TaskResultDTO 任务返回结果DTO（用于屏蔽sql.Null*类型）
type TaskResultDTO struct {
	ID                   int64   `json:"id"`
	IsCollection         *int64  `json:"isCollection"`
	Pid                  *int64  `json:"pid"`
	TaskName             *string `json:"taskName"`
	SiteID               *int64  `json:"siteId"`
	SiteWorkID           *string `json:"siteWorkId"`
	URL                  *string `json:"url"`
	Status               int     `json:"status"`
	PendingResourceID    *int64  `json:"pendingResourceId"`
	Continuable          *int64  `json:"continuable"`
	PluginPublicID       *string `json:"pluginPublicId"`
	PluginContributionID *string `json:"pluginContributionId"`
	PluginData           *string `json:"pluginData"`
	ErrorMessage         *string `json:"errorMessage"`
	CreateTime           int64   `json:"createTime"`
	UpdateTime           int64   `json:"updateTime"`
}

// TaskDTO 任务数据传输对象
type TaskDTO struct {
	ID                   int64   `json:"id"`
	IsCollection         *int64  `json:"isCollection,omitempty"`
	Pid                  *int64  `json:"pid,omitempty"`
	TaskName             *string `json:"taskName,omitempty"`
	SiteID               *int64  `json:"siteId,omitempty"`
	SiteWorkID           *string `json:"siteWorkId,omitempty"`
	URL                  *string `json:"url,omitempty"`
	Status               int     `json:"status,omitempty"`
	PendingResourceID    *int64  `json:"pendingResourceId,omitempty"`
	Continuable          *int64  `json:"continuable,omitempty"`
	PluginPublicID       *string `json:"pluginPublicId,omitempty"`
	PluginContributionID *string `json:"pluginContributionId,omitempty"`
	PluginData           *string `json:"pluginData,omitempty"`
}

// ToTaskResultDTO 将 domain.Task 转换为 TaskResultDTO
func ToTaskResultDTO(task *domain.Task) *TaskResultDTO {
	if task == nil {
		return nil
	}
	return &TaskResultDTO{
		ID:                   task.GetID(),
		IsCollection:         nullInt64ToPointer(task.IsCollection),
		Pid:                  nullInt64ToPointer(task.Pid),
		TaskName:             nullStringToPointer(task.TaskName),
		SiteID:               nullInt64ToPointer(task.SiteID),
		SiteWorkID:           nullStringToPointer(task.SiteWorkID),
		URL:                  nullStringToPointer(task.URL),
		Status:               task.Status,
		PendingResourceID:    nullInt64ToPointer(task.PendingResourceID),
		Continuable:          nullInt64ToPointer(task.Continuable),
		PluginPublicID:       nullStringToPointer(task.PluginPublicID),
		PluginContributionID: nullStringToPointer(task.PluginContributionID),
		PluginData:           nullStringToPointer(task.PluginData),
		ErrorMessage:         nullStringToPointer(task.ErrorMessage),
		CreateTime:           task.GetCreateTime(),
		UpdateTime:           task.GetUpdateTime(),
	}
}

// ToTaskPageResultDTO 将 *model.Page[domain.Task] 转换为 *model.Page[TaskResultDTO]
func ToTaskPageResultDTO(page *model.Page[domain.Task]) *model.Page[TaskResultDTO] {
	if page == nil {
		return nil
	}
	data := make([]*TaskResultDTO, 0, len(page.Data))
	for _, task := range page.Data {
		data = append(data, ToTaskResultDTO(task))
	}
	return &model.Page[TaskResultDTO]{
		PageNumber:   page.PageNumber,
		PageSize:     page.PageSize,
		PageCount:    page.PageCount,
		DataCount:    page.DataCount,
		CurrentCount: page.CurrentCount,
		Query:        page.Query,
		Data:         data,
	}
}

// nullStringToPointer 将 sql.NullString 转换为 *string
func nullStringToPointer(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// nullInt64ToPointer 将 sql.NullInt64 转换为 *int64
func nullInt64ToPointer(ns sql.NullInt64) *int64 {
	if ns.Valid {
		return &ns.Int64
	}
	return nil
}
