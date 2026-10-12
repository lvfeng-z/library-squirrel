package journal

import (
	"context"
	"encoding/json"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/taskManager"
)

// EventTypeTaskTerminal 任务域事件类型：任务终态迁移（子任务/父任务同型，载荷以 parent 区分）
const EventTypeTaskTerminal = "task_terminal"

// taskTerminalPayload 终态账行的 JSON 摘要载荷（主体引用已在账行 subject_id 列，载荷只补展示面）
type taskTerminalPayload struct {
	// TaskID 任务 ID（与账行 subject_id 同值；载荷内冗余一份便于消费方直接读摘要）
	TaskID int64 `json:"taskId"`
	// TaskName 任务名（展示用，任务行可能随后被清理）
	TaskName string `json:"taskName"`
	// State 终态枚举值（taskManager.TaskState，见 taskManager/model.go:23-34）
	State int `json:"state"`
	// StateName 终态可读名（仅三终态取值，见 taskTerminalStateName）
	StateName string `json:"stateName"`
	// Parent 是否父任务推送（父任务同为任务行，终态迁移同属本记账目录）
	Parent bool `json:"parent"`
}

// isTerminalTaskState 终态判定：终态集 = {Finished, Failed, PartlyFinished}，与 taskManager 域内
// isImmediateTerminal（taskManager/manager.go:1424-1426）同集。Paused 是稳定态（model.go:74-81）
// 但可续传、非终态，进度类与其余瞬态一律不落账（方案风险3：防写账风暴）
func isTerminalTaskState(state taskManager.TaskState) bool {
	switch state {
	case taskManager.TaskStateFinished, taskManager.TaskStateFailed, taskManager.TaskStatePartlyFinished:
		return true
	default:
		return false
	}
}

// taskTerminalStateName 终态可读名（只覆盖三终态：本函数仅在 isTerminalTaskState 通过后调用，
// 故不复制 taskManager.taskStateName 的全量表）
func taskTerminalStateName(state taskManager.TaskState) string {
	switch state {
	case taskManager.TaskStateFinished:
		return "Finished"
	case taskManager.TaskStateFailed:
		return "Failed"
	case taskManager.TaskStatePartlyFinished:
		return "PartlyFinished"
	default:
		return "Unknown"
	}
}

// TaskTerminalRecorder 记账适配器：任务推送的记账一路，内部做终态过滤——仅状态类推送且仅终态
// 落账（域=任务域、主体=任务 ID、载荷=JSON 摘要），进度类推送零落账。写账尽力而为：失败仅
// 日志告警，不上抛、不阻断推送链（方案定论：提交后写、尽力而为）
type TaskTerminalRecorder struct {
	svc *Service
}

// NewTaskTerminalRecorder 创建任务终态记账适配器
func NewTaskTerminalRecorder(svc *Service) *TaskTerminalRecorder {
	return &TaskTerminalRecorder{svc: svc}
}

// RecordTerminalState 终态迁移落账：非终态立即返回 false（零落账、零日志）；终态经 journal.Service
// 写入，返回是否落账成功。svc 为 nil（未装配记账服务）时静默跳过，推送链不受影响
func (r *TaskTerminalRecorder) RecordTerminalState(ctx context.Context, taskID int64, taskName string, state taskManager.TaskState, parent bool) bool {
	if !isTerminalTaskState(state) {
		return false
	}
	if r == nil || r.svc == nil {
		return false
	}
	payload, err := json.Marshal(taskTerminalPayload{
		TaskID:    taskID,
		TaskName:  taskName,
		State:     int(state),
		StateName: taskTerminalStateName(state),
		Parent:    parent,
	})
	if err != nil {
		logger.Log.Errorw("任务终态记账载荷序列化失败（不阻断推送）", "taskId", taskID, "state", int(state), "error", err)
		return false
	}
	if err := r.svc.Append(ctx, entity.JournalDomainTask, EventTypeTaskTerminal, taskID, string(payload)); err != nil {
		logger.Log.Errorw("任务终态记账落账失败（不阻断推送）", "taskId", taskID, "state", int(state), "error", err)
		return false
	}
	return true
}

// TaskTerminalPusher 组合推送器：taskManager.TaskProgressPusher 的两路 fan-out——一路原推送器
// （Wails/Snapshot/Noop 任一，纯透传：参数、调用顺序、返回不增不改），一路任务终态记账适配器。
// 装配点在应用主文件把我们包在原推送器外注入，taskManager 域代码零改动。
//
// 内嵌接口而非逐方法实现：接口的 PushProgressBatch 形参类型 taskManager.taskScheduleDTO 未导出
// （taskManager/progress_pusher.go:15），包外无法声明该方法；内嵌令其经接口提升转发给原推送器，
// 进度类（风险3 的零落账对象）由此天然不落账
type TaskTerminalPusher struct {
	taskManager.TaskProgressPusher
	recorder *TaskTerminalRecorder
}

// NewTaskTerminalPusher 创建任务终态组合推送器（wailsPusher = 原推送器，保持其语义不变）
func NewTaskTerminalPusher(wailsPusher taskManager.TaskProgressPusher, recorder *TaskTerminalRecorder) *TaskTerminalPusher {
	return &TaskTerminalPusher{TaskProgressPusher: wailsPusher, recorder: recorder}
}

// 编译期断言：组合器满足注入面接口（PushProgressBatch 等未导出形参方法经内嵌接口提升）
var _ taskManager.TaskProgressPusher = (*TaskTerminalPusher)(nil)

// PushStateChange 子任务状态推送：先原样透传原推送器（前端链路零变化），再对终态落账
// （非终态由适配器内部零落账）
func (p *TaskTerminalPusher) PushStateChange(taskId int64, taskName string, state taskManager.TaskState) {
	p.TaskProgressPusher.PushStateChange(taskId, taskName, state)
	p.recorder.RecordTerminalState(context.Background(), taskId, taskName, state, false)
}

// PushParentStateChange 父任务状态推送：同款两路（父任务同为任务行，终态迁移同属记账目录）
func (p *TaskTerminalPusher) PushParentStateChange(taskId int64, taskName string, state taskManager.TaskState) {
	p.TaskProgressPusher.PushParentStateChange(taskId, taskName, state)
	p.recorder.RecordTerminalState(context.Background(), taskId, taskName, state, true)
}
