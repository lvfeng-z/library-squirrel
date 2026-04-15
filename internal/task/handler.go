package task

import (
	"context"

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

// CreateTask 创建任务
func (h *Handler) CreateTask(ctx context.Context, req *CreateTaskRequest) *model.ApiResponse[int64] {
	result, err := h.svc.CreateTask(ctx, req)
	if err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(result.GetID())
}

// DeleteTask 删除任务（包含子任务）
func (h *Handler) DeleteTask(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.DeleteTask(ctx, id); err != nil {
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
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*domain.Task] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*domain.Task](err.Error())
	}
	return model.Success(result)
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page, pageSize int, queryDTO *TaskQueryDTO) *model.ApiResponse[*model.Page[domain.Task]] {
	if queryDTO == nil {
		queryDTO = &TaskQueryDTO{}
	}
	result, err := h.svc.Page(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[domain.Task]](err.Error())
	}
	return model.Success(result)
}

// QueryParentPage 分页查询父任务
func (h *Handler) QueryParentPage(ctx context.Context, page, pageSize int, queryDTO *TaskQueryDTO) *model.ApiResponse[*model.Page[domain.Task]] {
	if queryDTO == nil {
		queryDTO = &TaskQueryDTO{}
	}
	result, err := h.svc.QueryParentPageByDTO(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[domain.Task]](err.Error())
	}
	return model.Success(result)
}

// QueryChildrenTaskPage 查询子任务分页
func (h *Handler) QueryChildrenTaskPage(ctx context.Context, pid int64, page, pageSize int, queryDTO *TaskQueryDTO) *model.ApiResponse[*model.Page[domain.Task]] {
	if queryDTO == nil {
		queryDTO = &TaskQueryDTO{}
	}
	result, err := h.svc.QueryChildrenTaskPageByDTO(ctx, pid, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[domain.Task]](err.Error())
	}
	return model.Success(result)
}

// ListChildrenTask 查询子任务列表
func (h *Handler) ListChildrenTask(ctx context.Context, pid int64) *model.ApiResponse[[]*domain.Task] {
	result, err := h.svc.ListChildrenTask(ctx, pid)
	if err != nil {
		return model.Error[[]*domain.Task](err.Error())
	}
	return model.Success(result)
}

// QueryTreeDataPage 查询任务树数据分页
func (h *Handler) QueryTreeDataPage(ctx context.Context, treeId int64) *model.ApiResponse[*TreeDataPageDTO] {
	result, err := h.svc.QueryTreeDataPage(ctx, treeId)
	if err != nil {
		return model.Error[*TreeDataPageDTO](err.Error())
	}
	return model.Success(result)
}

// ListTaskTree 获取任务树列表
func (h *Handler) ListTaskTree(ctx context.Context, taskIds []int64) *model.ApiResponse[[]*domain.Task] {
	result, err := h.svc.ListTaskTree(ctx, taskIds)
	if err != nil {
		return model.Error[[]*domain.Task](err.Error())
	}
	return model.Success(result)
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
