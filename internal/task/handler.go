package task

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
	dto2 "github.com/library-squirrel/wails/pkg/model/dto"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
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
func (h *Handler) Save(ctx context.Context, task *dto2.TaskDTO) *model.ApiResponse[int64] {
	domainTask := dto2.ToTaskEntity(task)

	if err := h.svc.Save(ctx, domainTask); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainTask.GetID())
}

// Update 更新任务
func (h *Handler) Update(ctx context.Context, task *dto2.TaskDTO) *model.ApiResponse[any] {
	domainTask := dto2.ToTaskEntity(task)

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
func (h *Handler) CreateTask(ctx context.Context, req *dto2.CreateTaskRequest) *model.ApiResponse[int64] {
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
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto2.TaskDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*dto2.TaskDTO](err.Error())
	}
	return model.Success(dto2.NewTaskDTO(result))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto2.TaskDTO], query TaskQueryDTO) *model.ApiResponse[*model.Page[dto2.TaskDTO]] {
	if page == nil {
		page = &model.Page[dto2.TaskDTO]{}
	}
	entityPage := &model.Page[entity2.Task]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.Page(ctx, entityPage, query)
	if err != nil {
		return model.Error[*model.Page[dto2.TaskDTO]](err.Error())
	}
	// 转换为 DTO
	data := make([]*dto2.TaskDTO, 0, len(result.Data))
	for _, task := range result.Data {
		data = append(data, dto2.NewTaskDTO(task))
	}
	return model.Success(&model.Page[dto2.TaskDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// QueryParentPage 分页查询父任务（返回带站点名称的 TaskProgressTreeDTO）
func (h *Handler) QueryParentPage(ctx context.Context, page *model.Page[dto2.TaskProgressTreeDTO], query TaskQueryDTO) *model.ApiResponse[*model.Page[dto2.TaskProgressTreeDTO]] {
	if page == nil {
		page = &model.Page[dto2.TaskProgressTreeDTO]{}
	}
	entityPage := &model.Page[entity2.Task]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.QueryParentPage(ctx, entityPage, query)
	if err != nil {
		return model.Error[*model.Page[dto2.TaskProgressTreeDTO]](err.Error())
	}
	enriched, err := h.svc.EnrichTaskProgressTreePage(ctx, result)
	if err != nil {
		return model.Error[*model.Page[dto2.TaskProgressTreeDTO]](err.Error())
	}
	return model.Success(enriched)
}

// QueryChildrenTaskPage 查询子任务分页（返回带站点名称的 TaskProgressTreeDTO）
func (h *Handler) QueryChildrenTaskPage(ctx context.Context, page *model.Page[dto2.TaskProgressTreeDTO], query TaskQueryDTO) *model.ApiResponse[*model.Page[dto2.TaskProgressTreeDTO]] {
	if page == nil {
		page = &model.Page[dto2.TaskProgressTreeDTO]{}
	}
	entityPage := &model.Page[entity2.Task]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.QueryChildrenTaskPage(ctx, entityPage, query)
	if err != nil {
		return model.Error[*model.Page[dto2.TaskProgressTreeDTO]](err.Error())
	}
	enriched, err := h.svc.EnrichTaskProgressTreePage(ctx, result)
	if err != nil {
		return model.Error[*model.Page[dto2.TaskProgressTreeDTO]](err.Error())
	}
	return model.Success(enriched)
}

// ListChildrenTask 查询子任务列表
func (h *Handler) ListChildrenTask(ctx context.Context, pid int64) *model.ApiResponse[[]*dto2.TaskDTO] {
	result, err := h.svc.ListChildrenTask(ctx, pid)
	if err != nil {
		return model.Error[[]*dto2.TaskDTO](err.Error())
	}
	// 转换为 DTO
	resultDTOs := make([]*dto2.TaskDTO, len(result))
	for i, task := range result {
		resultDTOs[i] = dto2.NewTaskDTO(task)
	}
	return model.Success(resultDTOs)
}

// QueryTreeDataPage 查询任务树数据分页
func (h *Handler) QueryTreeDataPage(ctx context.Context, page *model.Page[dto2.TaskDTO], query TaskQueryDTO) *model.ApiResponse[*dto2.TreeDataPageDTO] {
	result, err := h.svc.QueryTreeDataPage(ctx, page.PageNumber, page.PageSize, &query)
	if err != nil {
		return model.Error[*dto2.TreeDataPageDTO](err.Error())
	}
	return model.Success(result)
}

// ListTaskTree 获取任务树列表
func (h *Handler) ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...int) *model.ApiResponse[[]*dto2.TaskDTO] {
	// 转换 includeStatus
	var statusEnums []TaskStatusEnum
	for _, s := range includeStatus {
		statusEnums = append(statusEnums, TaskStatusEnum(s))
	}
	result, err := h.svc.ListTaskTree(ctx, taskIds, statusEnums...)
	if err != nil {
		return model.Error[[]*dto2.TaskDTO](err.Error())
	}
	// 转换为 DTO
	resultDTOs := make([]*dto2.TaskDTO, len(result))
	for i, task := range result {
		resultDTOs[i] = dto2.NewTaskDTO(task)
	}
	return model.Success(resultDTOs)
}

// ListStatus 查询状态列表
func (h *Handler) ListStatus(ctx context.Context, ids []int64) *model.ApiResponse[[]*dto2.TaskProgressDTO] {
	result, err := h.svc.ListStatus(ctx, ids)
	if err != nil {
		return model.Error[[]*dto2.TaskProgressDTO](err.Error())
	}
	return model.Success(result)
}

// ListSchedule 查询任务进度列表
func (h *Handler) ListSchedule(ctx context.Context, ids []int64) *model.ApiResponse[[]*dto2.TaskProgressDTO] {
	result, err := h.svc.ListSchedule(ctx, ids)
	if err != nil {
		return model.Error[[]*dto2.TaskProgressDTO](err.Error())
	}
	return model.Success(result)
}
