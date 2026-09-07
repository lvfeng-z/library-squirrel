package taskManager

import (
	"context"
	"database/sql"
	"strings"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/config"
)

// SectionRecorder 板块选择写行能力（download 提供，装配注入）：开始/重下载入口两步编排的
// 第一步——把板块选择写入各任务的作品任务领域行（父任务请求展开到全部子成员，各子任务持有
// 板块选择供执行派生与单独续传读取），第二步 StartTaskTrees 启动后执行面按行派生板块模式
type SectionRecorder interface {
	RecordSections(ctx context.Context, taskIds []int64, storeRoles sql.NullString, includeWorkInfo bool) error
}

// Handler 任务管理器 Handler
type Handler struct {
	mgr             *Manager
	sectionRecorder SectionRecorder
}

// NewHandler 创建任务管理器 Handler
func NewHandler(mgr *Manager, sectionRecorder SectionRecorder) *Handler {
	return &Handler{mgr: mgr, sectionRecorder: sectionRecorder}
}

// TaskControlConfigDTO 任务控制操作防重入配置（IPC 响应体）
type TaskControlConfigDTO struct {
	OperationCooldownMs   int  `json:"operationCooldownMs"`   // 控制操作冷却毫秒（0=不启用，开发者调试放开）
	OperationWaitResponse bool `json:"operationWaitResponse"` // 操作在途守卫是否等待 IPC 响应（false=提交即放行，开发者高频启停测试）
}

// GetTaskControlConfig 获取任务控制操作防重入配置（前端操作栏按钮冷却依据，读 config.yaml）
func (h *Handler) GetTaskControlConfig() *model.ApiResponse[*TaskControlConfigDTO] {
	dto := &TaskControlConfigDTO{}
	if cfg := config.Get(); cfg != nil {
		dto.OperationCooldownMs = cfg.Task.OperationCooldownMs
		dto.OperationWaitResponse = cfg.Task.OperationWaitResponse
	}
	return model.Success(dto)
}

// StartTaskTrees 批量启动任务（板块全量执行）：两步编排——先把首跑板块选择写入各任务的作品
// 任务领域行（store_roles=NULL 表示全量、include_work_info=true；创建默认不含作品信息，不写行
// 则执行面派生出不含作品信息的板块组合），再启动任务树。写行范围=各请求任务及其直接子成员
// （任务树两级：父→叶子）：整树启动覆盖全部子任务，单独请求叶子只写该叶子自身、不波及其
// 运行中兄弟。写行先于调度决策进行——请求树上已被调度层跳过的已运行单元，其行同样被覆盖
// 为全量。重试/恢复不经此处，按各任务已记录的板块模式执行
func (h *Handler) StartTaskTrees(ctx context.Context, taskIds []int64) *model.ApiResponse[any] {
	if err := h.sectionRecorder.RecordSections(ctx, taskIds,
		sql.NullString{Valid: false}, true); err != nil {
		return model.HandleError[any](err)
	}
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

// Redownload 板块重执行入口:storeRoles 为所选 store_type 集合,includeWorkInfo 决定是否执行作品元数据板块。
// 两步编排（发起方在 handler）：板块选择写行（空资源集=仅作品信息、非空=所选子集）→ 整树启动
func (h *Handler) Redownload(ctx context.Context, taskIds []int64, storeRoles []string, includeWorkInfo bool) *model.ApiResponse[any] {
	if err := h.sectionRecorder.RecordSections(ctx, taskIds,
		sql.NullString{String: strings.Join(storeRoles, ","), Valid: true}, includeWorkInfo); err != nil {
		return model.HandleError[any](err)
	}
	return model.HandleVoid(h.mgr.StartTaskTrees(ctx, taskIds))
}
