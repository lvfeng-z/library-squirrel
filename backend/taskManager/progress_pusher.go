package taskManager

import (
	"github.com/library-squirrel/backend/base/logger"
)

// TaskProgressPusher 任务进度推送器接口
type TaskProgressPusher interface {
	PushStateChange(taskId int64, taskName string, state TaskState)
	PushParentStateChange(taskId int64, taskName string, state TaskState)
	PushProgress(taskId int64, total int64, finished int64)
	PushError(taskId int64, err string)
	PushTaskRemove(taskIds []int64)
	PushParentTaskRemove(taskIds []int64)
	PushDuplicateDetected(taskId int64, taskName string, existingWorkId int64, existingWorkName string)
}

// WailsEventEmitter Wails 事件发射器接口
type WailsEventEmitter interface {
	Emit(eventName string, data ...any) bool
}

// WailsTaskProgressPusher 基于 Wails Events 的任务进度推送器
type WailsTaskProgressPusher struct {
	emitter WailsEventEmitter
}

// NewWailsTaskProgressPusher 创建 Wails 任务进度推送器
func NewWailsTaskProgressPusher(emitter WailsEventEmitter) *WailsTaskProgressPusher {
	logger.Log.Info("[TaskPusher] 创建 WailsTaskProgressPusher")
	return &WailsTaskProgressPusher{emitter: emitter}
}

// PushStateChange 推送任务状态变化到前端
func (p *WailsTaskProgressPusher) PushStateChange(taskId int64, taskName string, state TaskState) {
	dto := &taskStateDTO{
		ID:       taskId,
		TaskName: taskName,
		Status:   int(state),
	}
	data := []*taskStateDTO{dto}
	p.emitter.Emit("taskStatus-updateTask", data)
}

// PushProgress 推送下载进度到前端
func (p *WailsTaskProgressPusher) PushProgress(taskId int64, total int64, finished int64) {
	dto := &taskScheduleDTO{
		ID:       taskId,
		Total:    total,
		Finished: finished,
	}
	data := []*taskScheduleDTO{dto}
	p.emitter.Emit("taskStatus-updateSchedule", data)
}

// PushError 推送错误到前端
func (p *WailsTaskProgressPusher) PushError(taskId int64, err string) {
}

// PushTaskRemove 通知前端移除任务
func (p *WailsTaskProgressPusher) PushTaskRemove(taskIds []int64) {
	p.emitter.Emit("taskStatus-removeTask", taskIds)
}

// PushParentTaskRemove 通知前端移除父任务
func (p *WailsTaskProgressPusher) PushParentTaskRemove(taskIds []int64) {
	p.emitter.Emit("parentTaskStatus-removeParentTask", taskIds)
}

// PushParentStateChange 推送父任务状态变化到前端
func (p *WailsTaskProgressPusher) PushParentStateChange(taskId int64, taskName string, state TaskState) {
	dto := &taskStateDTO{
		ID:       taskId,
		TaskName: taskName,
		Status:   int(state),
	}
	data := []*taskStateDTO{dto}
	p.emitter.Emit("parentTaskStatus-updateParentTask", data)
}

// taskStateDTO 任务状态推送 DTO
type taskStateDTO struct {
	ID       int64  `json:"id"`
	TaskName string `json:"taskName"`
	Status   int    `json:"status"`
}

// taskScheduleDTO 任务进度推送 DTO，对应前端 TaskScheduleDTO 的 flat 格式
type taskScheduleDTO struct {
	ID       int64 `json:"id"`
	Total    int64 `json:"total"`
	Finished int64 `json:"finished"`
}

// duplicateDetectedDTO 作品重复检测推送 DTO
type duplicateDetectedDTO struct {
	TaskId           int64  `json:"taskId"`
	TaskName         string `json:"taskName"`
	ExistingWorkId   int64  `json:"existingWorkId"`
	ExistingWorkName string `json:"existingWorkName"`
}

// PushDuplicateDetected 推送作品重复检测事件到前端
func (p *WailsTaskProgressPusher) PushDuplicateDetected(taskId int64, taskName string, existingWorkId int64, existingWorkName string) {
	dto := &duplicateDetectedDTO{
		TaskId:           taskId,
		TaskName:         taskName,
		ExistingWorkId:   existingWorkId,
		ExistingWorkName: existingWorkName,
	}
	p.emitter.Emit("taskStatus-duplicateDetected", dto)
}

// NoopProgressPusher 空推送器，用于测试或 emitter 未就绪时
type NoopProgressPusher struct{}

// NewNoopProgressPusher 创建空推送器
func NewNoopProgressPusher() *NoopProgressPusher {
	logger.Log.Warn("[TaskPusher] 创建 NoopProgressPusher（emitter 未就绪，事件将丢失）")
	return &NoopProgressPusher{}
}

func (p *NoopProgressPusher) PushStateChange(int64, string, TaskState)       {}
func (p *NoopProgressPusher) PushParentStateChange(int64, string, TaskState) {}
func (p *NoopProgressPusher) PushProgress(int64, int64, int64)       {}
func (p *NoopProgressPusher) PushError(int64, string)                {}
func (p *NoopProgressPusher) PushTaskRemove([]int64)                 {}
func (p *NoopProgressPusher) PushParentTaskRemove([]int64)           {}
func (p *NoopProgressPusher) PushDuplicateDetected(int64, string, int64, string) {}
