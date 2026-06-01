package taskManager

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
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
	return model.HandleVoid(h.mgr.StartTaskTree(ctx, taskId))
}

// PauseTaskTree 暂停任务树
func (h *Handler) PauseTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.PauseTaskTree(ctx, taskId))
}

// ResumeTaskTree 恢复任务树
func (h *Handler) ResumeTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.ResumeTaskTree(ctx, taskId))
}

// StopTaskTree 停止任务树
func (h *Handler) StopTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.StopTaskTree(ctx, taskId))
}

// RetryTaskTree 重试任务树
func (h *Handler) RetryTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.RetryTaskTree(ctx, taskId))
}

// GetTaskTreeState 获取任务树状态
func (h *Handler) GetTaskTreeState(taskId int64) *model.ApiResponse[int] {
	state, err := h.mgr.GetTaskTreeState(taskId)
	if err != nil {
		return model.HandleError[int](err)
	}
	return model.Success(int(state))
}

// GetTaskState 获取任务状态
func (h *Handler) GetTaskState(taskId int64) *model.ApiResponse[int] {
	state, err := h.mgr.GetTaskState(taskId)
	if err != nil {
		return model.HandleError[int](err)
	}
	return model.Success(int(state))
}

// IsIdle 检查任务管理器是否处于空闲状态
func (h *Handler) IsIdle() *model.ApiResponse[bool] {
	result := h.mgr.IsIdle()
	return model.Success(result)
}

// ConfirmReplace 用户确认替换或跳过重复作品
func (h *Handler) ConfirmReplace(ctx context.Context, taskId int64, action string) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.ConfirmReplace(taskId, action))
}

// ConfirmReplaceBatch 批量确认替换或跳过重复作品
func (h *Handler) ConfirmReplaceBatch(ctx context.Context, taskIds []int64, action string) *model.ApiResponse[any] {
	h.mgr.ConfirmReplaceBatch(taskIds, action)
	return model.Success[any](nil)
}
