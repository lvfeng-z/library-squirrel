package task

import (
	"context"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
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
func (h *Handler) Save(ctx context.Context, task *sdkdto.TaskDTO) *model.ApiResponse[int64] {
	domainTask := dto2.ToTaskEntity(task)

	if err := h.svc.Save(ctx, domainTask); err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(domainTask.GetID())
}

// Update 更新任务
func (h *Handler) Update(ctx context.Context, task *sdkdto.TaskDTO) *model.ApiResponse[any] {
	domainTask := dto2.ToTaskEntity(task)

	if err := h.svc.Update(ctx, domainTask); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// RefreshStatus 刷新任务状态
func (h *Handler) RefreshStatus(ctx context.Context, taskId int64) *model.ApiResponse[int64] {
	return model.HandleResult(h.svc.RefreshTaskStatus(ctx, taskId))
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
		return model.HandleError[int64](err)
	}
	return model.Success(result)
}

// CreateTask 创建任务
func (h *Handler) CreateTask(ctx context.Context, req *sdkdto.CreateTaskRequest) *model.ApiResponse[int64] {
	result, err := h.svc.CreateTask(ctx, req)
	if err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(result.GetID())
}

// DeleteTask 删除任务（包含子任务）- 批量删除
func (h *Handler) DeleteTask(ctx context.Context, ids []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.DeleteTask(ctx, ids))
}

// CreateTaskByURL 根据传入的url创建任务
func (h *Handler) CreateTaskByURL(ctx context.Context, url string) *model.ApiResponse[*CreateTaskByURLResponse] {
	return model.HandleResult(h.svc.CreateTaskByURL(ctx, url))
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*sdkdto.TaskDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*sdkdto.TaskDTO](err)
	}
	return model.Success(dto2.NewTaskDTO(result))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[sdkdto.TaskDTO], query TaskQueryDTO) *model.ApiResponse[*model.Page[sdkdto.TaskDTO]] {
	if page == nil {
		page = &model.Page[sdkdto.TaskDTO]{}
	}
	entityPage := &model.Page[entity2.Task]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.Page(ctx, entityPage, query)
	if err != nil {
		return model.HandleError[*model.Page[sdkdto.TaskDTO]](err)
	}
	// 转换为 DTO
	data := make([]*sdkdto.TaskDTO, 0, len(result.Data))
	for _, task := range result.Data {
		data = append(data, dto2.NewTaskDTO(task))
	}
	return model.Success(&model.Page[sdkdto.TaskDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// QueryParentPage 分页查询父任务（返回带站点名称的 TaskProgressTreeDTO）
func (h *Handler) QueryParentPage(ctx context.Context, page *model.Page[sdkdto.TaskProgressTreeDTO], query TaskQueryDTO) *model.ApiResponse[*model.Page[sdkdto.TaskProgressTreeDTO]] {
	start := time.Now()
	logger.Log.Infof("[IPC] QueryParentPage 开始: pid=%d", query.Pid)
	defer func() {
		logger.Log.Infof("[IPC] QueryParentPage 完成: elapsed=%v", time.Since(start))
	}()
	if page == nil {
		page = &model.Page[sdkdto.TaskProgressTreeDTO]{}
	}
	entityPage := &model.Page[entity2.Task]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.QueryParentPage(ctx, entityPage, query)
	if err != nil {
		return model.HandleError[*model.Page[sdkdto.TaskProgressTreeDTO]](err)
	}
	enriched, err := h.svc.EnrichTaskProgressTreePage(ctx, result)
	if err != nil {
		return model.HandleError[*model.Page[sdkdto.TaskProgressTreeDTO]](err)
	}
	return model.Success(enriched)
}

// QueryChildrenTaskPage 查询子任务分页（返回带站点名称的 TaskProgressTreeDTO）
func (h *Handler) QueryChildrenTaskPage(ctx context.Context, page *model.Page[sdkdto.TaskProgressTreeDTO], query TaskQueryDTO) *model.ApiResponse[*model.Page[sdkdto.TaskProgressTreeDTO]] {
	start := time.Now()
	logger.Log.Infof("[IPC] QueryChildrenTaskPage 开始: pid=%d", query.Pid)
	defer func() {
		logger.Log.Infof("[IPC] QueryChildrenTaskPage 完成: elapsed=%v", time.Since(start))
	}()
	if page == nil {
		page = &model.Page[sdkdto.TaskProgressTreeDTO]{}
	}
	entityPage := &model.Page[entity2.Task]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.QueryChildrenTaskPage(ctx, entityPage, query)
	if err != nil {
		return model.HandleError[*model.Page[sdkdto.TaskProgressTreeDTO]](err)
	}
	enriched, err := h.svc.EnrichTaskProgressTreePage(ctx, result)
	if err != nil {
		return model.HandleError[*model.Page[sdkdto.TaskProgressTreeDTO]](err)
	}
	return model.Success(enriched)
}

// ListChildrenTask 查询子任务列表
func (h *Handler) ListChildrenTask(ctx context.Context, pid int64) *model.ApiResponse[[]*sdkdto.TaskDTO] {
	result, err := h.svc.ListChildrenTask(ctx, pid)
	if err != nil {
		return model.HandleError[[]*sdkdto.TaskDTO](err)
	}
	// 转换为 DTO
	resultDTOs := make([]*sdkdto.TaskDTO, len(result))
	for i, task := range result {
		resultDTOs[i] = dto2.NewTaskDTO(task)
	}
	return model.Success(resultDTOs)
}

// QueryTreeDataPage 查询任务树数据分页
func (h *Handler) QueryTreeDataPage(ctx context.Context, page *model.Page[sdkdto.TaskDTO], query TaskQueryDTO) *model.ApiResponse[*sdkdto.TreeDataPageDTO] {
	return model.HandleResult(h.svc.QueryTreeDataPage(ctx, page.PageNumber, page.PageSize, &query))
}

// ListTaskTree 获取任务树列表
func (h *Handler) ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...int) *model.ApiResponse[[]*sdkdto.TaskDTO] {
	// 转换 includeStatus
	var statusEnums []TaskStatusEnum
	for _, s := range includeStatus {
		statusEnums = append(statusEnums, TaskStatusEnum(s))
	}
	result, err := h.svc.ListTaskTree(ctx, taskIds, statusEnums...)
	if err != nil {
		return model.HandleError[[]*sdkdto.TaskDTO](err)
	}
	// 转换为 DTO
	resultDTOs := make([]*sdkdto.TaskDTO, len(result))
	for i, task := range result {
		resultDTOs[i] = dto2.NewTaskDTO(task)
	}
	return model.Success(resultDTOs)
}

// ListStatus 查询状态列表
func (h *Handler) ListStatus(ctx context.Context, ids []int64) *model.ApiResponse[[]*sdkdto.TaskProgressDTO] {
	return model.HandleResult(h.svc.ListStatus(ctx, ids))
}

// ListSchedule 查询任务进度列表
func (h *Handler) ListSchedule(ctx context.Context, ids []int64) *model.ApiResponse[[]*sdkdto.TaskProgressDTO] {
	return model.HandleResult(h.svc.ListSchedule(ctx, ids))
}
