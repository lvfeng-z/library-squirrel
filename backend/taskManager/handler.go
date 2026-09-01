package taskManager

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/config"
)

// Handler 任务管理器 Handler
type Handler struct {
	mgr *Manager
}

// NewHandler 创建任务管理器 Handler
func NewHandler(mgr *Manager) *Handler {
	return &Handler{mgr: mgr}
}

// TaskControlConfigDTO 任务控制操作防重入配置（IPC 响应体）
type TaskControlConfigDTO struct {
	OperationCooldownMs int `json:"operationCooldownMs"` // 控制操作冷却毫秒（0=不启用，开发者调试放开）
}

// GetTaskControlConfig 获取任务控制操作防重入配置（前端操作栏按钮冷却依据，读 config.yaml）
func (h *Handler) GetTaskControlConfig() *model.ApiResponse[*TaskControlConfigDTO] {
	dto := &TaskControlConfigDTO{}
	if cfg := config.Get(); cfg != nil {
		dto.OperationCooldownMs = cfg.Task.OperationCooldownMs
	}
	return model.Success(dto)
}

// StartTaskTrees 批量启动任务
func (h *Handler) StartTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.StartTaskTrees(ctx, taskIds))
}

// PauseTaskTrees 批量暂停任务
func (h *Handler) PauseTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.PauseTaskTrees(ctx, taskIds))
}

// ResumeTaskTrees 批量恢复任务
func (h *Handler) ResumeTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.ResumeTaskTrees(ctx, taskIds))
}

// StopTaskTrees 批量停止任务
func (h *Handler) StopTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.StopTaskTrees(ctx, taskIds))
}

// RetryTaskTrees 批量重试任务
func (h *Handler) RetryTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.RetryTaskTrees(ctx, taskIds))
}

// GetTaskTreeState 获取任务状态:父任务返回聚合状态、叶子/独立返回自身状态
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

// GetActiveTaskCount 获取插件名下运行中任务数（Processing/Pausing/Stopping/WaitingForInput），
// 供插件停用/换版确认框明示代价与拦截提醒
func (h *Handler) GetActiveTaskCount(pluginPublicId string) *model.ApiResponse[int] {
	return model.Success(h.mgr.CountActiveByPlugin(pluginPublicId))
}

// ConfirmReplace 用户确认替换或跳过重复作品
func (h *Handler) ConfirmReplace(ctx context.Context, taskId int64, action string) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.ConfirmReplace(taskId, action))
}

// ConfirmReplaceBatch 批量确认替换或跳过重复作品（replace 答复遇涉及作品被分享拉取持有时
// 整体不投递并返回错误，任务留在等待确认表）
func (h *Handler) ConfirmReplaceBatch(ctx context.Context, taskIds []int64, action string) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.ConfirmReplaceBatch(taskIds, action))
}

// GetTaskSnapshot 获取当前所有活跃任务的完整状态快照
func (h *Handler) GetTaskSnapshot() *model.ApiResponse[*TaskSnapshotDTO] {
	snapshot := h.mgr.GetTaskSnapshot()
	return model.Success(snapshot)
}

// Redownload 板块重执行入口:storeRoles 为所选 store_type 集合,includeWorkInfo 决定是否执行作品元数据板块
func (h *Handler) Redownload(ctx context.Context, taskIds []int64, storeRoles []string, includeWorkInfo bool) *model.ApiResponse[any] {
	return model.HandleVoid(h.mgr.Redownload(ctx, taskIds, storeRoles, includeWorkInfo))
}
