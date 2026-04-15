package taskManager

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
)

// Handler 任务管理器 Handler
type Handler struct {
	mgr *Manager
}

// NewHandler 创建任务管理器 Handler
func NewHandler(mgr *Manager) *Handler {
	return &Handler{mgr: mgr}
}

// StartTaskTree 启动任务树
func (h *Handler) StartTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	if err := h.mgr.StartTaskTree(ctx, taskId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// PauseTaskTree 暂停任务树
func (h *Handler) PauseTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	if err := h.mgr.PauseTaskTree(ctx, taskId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ResumeTaskTree 恢复任务树
func (h *Handler) ResumeTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	if err := h.mgr.ResumeTaskTree(ctx, taskId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// StopTaskTree 停止任务树
func (h *Handler) StopTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	if err := h.mgr.StopTaskTree(ctx, taskId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// RetryTaskTree 重试任务树
func (h *Handler) RetryTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	if err := h.mgr.RetryTaskTree(ctx, taskId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// GetTaskTreeState 获取任务树状态
func (h *Handler) GetTaskTreeState(taskId int64) *model.ApiResponse[int] {
	state, err := h.mgr.GetTaskTreeState(taskId)
	if err != nil {
		return model.Error[int](err.Error())
	}
	return model.Success(int(state))
}

// GetTaskState 获取任务状态
func (h *Handler) GetTaskState(taskId int64) *model.ApiResponse[int] {
	state, err := h.mgr.GetTaskState(taskId)
	if err != nil {
		return model.Error[int](err.Error())
	}
	return model.Success(int(state))
}

// IsIdle 检查任务管理器是否处于空闲状态
func (h *Handler) IsIdle() *model.ApiResponse[bool] {
	result := h.mgr.IsIdle()
	return model.Success(result)
}
