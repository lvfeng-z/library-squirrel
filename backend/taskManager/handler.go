package taskManager

import (
	"context"
	"time"

	"github.com/library-squirrel/backend/base/logger"
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
	start := time.Now()
	logger.Log.Infof("[IPC] StartTaskTree 开始: taskId=%d", taskId)
	result := model.HandleVoid(h.mgr.StartTaskTree(ctx, taskId))
	logger.Log.Infof("[IPC] StartTaskTree 完成: taskId=%d, elapsed=%v", taskId, time.Since(start))
	return result
}

// PauseTaskTree 暂停任务树
func (h *Handler) PauseTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	start := time.Now()
	logger.Log.Infof("[IPC] PauseTaskTree 开始: taskId=%d", taskId)
	result := model.HandleVoid(h.mgr.PauseTaskTree(ctx, taskId))
	logger.Log.Infof("[IPC] PauseTaskTree 完成: taskId=%d, elapsed=%v", taskId, time.Since(start))
	return result
}

// ResumeTaskTree 恢复任务树
func (h *Handler) ResumeTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	start := time.Now()
	logger.Log.Infof("[IPC] ResumeTaskTree 开始: taskId=%d", taskId)
	result := model.HandleVoid(h.mgr.ResumeTaskTree(ctx, taskId))
	logger.Log.Infof("[IPC] ResumeTaskTree 完成: taskId=%d, elapsed=%v", taskId, time.Since(start))
	return result
}

// StopTaskTree 停止任务树
func (h *Handler) StopTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	start := time.Now()
	logger.Log.Infof("[IPC] StopTaskTree 开始: taskId=%d", taskId)
	result := model.HandleVoid(h.mgr.StopTaskTree(ctx, taskId))
	logger.Log.Infof("[IPC] StopTaskTree 完成: taskId=%d, elapsed=%v", taskId, time.Since(start))
	return result
}

// RetryTaskTree 重试任务树
func (h *Handler) RetryTaskTree(ctx context.Context, taskId int64) *model.ApiResponse[any] {
	start := time.Now()
	logger.Log.Infof("[IPC] RetryTaskTree 开始: taskId=%d", taskId)
	result := model.HandleVoid(h.mgr.RetryTaskTree(ctx, taskId))
	logger.Log.Infof("[IPC] RetryTaskTree 完成: taskId=%d, elapsed=%v", taskId, time.Since(start))
	return result
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
	start := time.Now()
	logger.Log.Infof("[IPC] ConfirmReplace 开始: taskId=%d, action=%s", taskId, action)
	result := model.HandleVoid(h.mgr.ConfirmReplace(taskId, action))
	logger.Log.Infof("[IPC] ConfirmReplace 完成: taskId=%d, elapsed=%v", taskId, time.Since(start))
	return result
}

// ConfirmReplaceBatch 批量确认替换或跳过重复作品
func (h *Handler) ConfirmReplaceBatch(ctx context.Context, taskIds []int64, action string) *model.ApiResponse[any] {
	start := time.Now()
	logger.Log.Infof("[IPC] ConfirmReplaceBatch 开始: count=%d, action=%s", len(taskIds), action)
	h.mgr.ConfirmReplaceBatch(taskIds, action)
	logger.Log.Infof("[IPC] ConfirmReplaceBatch 完成: elapsed=%v", time.Since(start))
	return model.Success[any](nil)
}
