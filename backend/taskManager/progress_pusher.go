package taskManager

import (
	"sync"
	"time"

	"github.com/library-squirrel/backend/base/logger"
)

// TaskProgressPusher 任务进度推送器接口
type TaskProgressPusher interface {
	PushStateChange(taskId int64, taskName string, state TaskState)
	PushParentStateChange(taskId int64, taskName string, state TaskState)
	PushProgress(taskId int64, total int64, finished int64)
	PushProgressBatch(batch []*taskScheduleDTO)
	PushParentProgress(taskId int64, total int64, finished int64)
	PushError(taskId int64, err string)
	PushTaskRemove(taskIds []int64)
	PushParentTaskRemove(taskIds []int64)
	PushDuplicateDetected(taskId int64, taskName string, existingWorkId int64, existingWorkName string)
}

// WailsEventEmitter Wails 事件发射器接口
type WailsEventEmitter interface {
	Emit(eventName string, data ...any) bool
}

// ipcEvent IPC 事件信封，将多种事件类型合并到同一 topic 以保证 FIFO 顺序
type ipcEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
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

// emitTaskEvent 向子任务统一 topic 推送事件
func (p *WailsTaskProgressPusher) emitTaskEvent(eventType string, data any) {
	p.emitter.Emit("task-events", &ipcEvent{Type: eventType, Data: data})
}

// emitParentEvent 向父任务统一 topic 推送事件
func (p *WailsTaskProgressPusher) emitParentEvent(eventType string, data any) {
	p.emitter.Emit("parent-events", &ipcEvent{Type: eventType, Data: data})
}

// PushStateChange 推送任务状态变化到前端
func (p *WailsTaskProgressPusher) PushStateChange(taskId int64, taskName string, state TaskState) {
	dto := &taskStateDTO{
		ID:       taskId,
		TaskName: taskName,
		Status:   int(state),
	}
	data := []*taskStateDTO{dto}
	p.emitTaskEvent("updateTask", data)
}

// PushProgress 推送下载进度到前端
func (p *WailsTaskProgressPusher) PushProgress(taskId int64, total int64, finished int64) {
	dto := &taskScheduleDTO{
		ID:       taskId,
		Total:    total,
		Finished: finished,
	}
	data := []*taskScheduleDTO{dto}
	p.emitTaskEvent("updateSchedule", data)
}

// PushProgressBatch 批量推送下载进度到前端（合并多次进度更新为一次 Emit）
func (p *WailsTaskProgressPusher) PushProgressBatch(batch []*taskScheduleDTO) {
	if len(batch) == 0 {
		return
	}
	p.emitTaskEvent("updateSchedule", batch)
}

// PushError 推送错误到前端
func (p *WailsTaskProgressPusher) PushError(taskId int64, err string) {
}

// PushTaskRemove 通知前端移除任务
func (p *WailsTaskProgressPusher) PushTaskRemove(taskIds []int64) {
	p.emitTaskEvent("removeTask", taskIds)
}

// PushParentTaskRemove 通知前端移除父任务
func (p *WailsTaskProgressPusher) PushParentTaskRemove(taskIds []int64) {
	p.emitParentEvent("removeParentTask", taskIds)
}

// PushParentStateChange 推送父任务状态变化到前端
func (p *WailsTaskProgressPusher) PushParentStateChange(taskId int64, taskName string, state TaskState) {
	dto := &taskStateDTO{
		ID:       taskId,
		TaskName: taskName,
		Status:   int(state),
	}
	data := []*taskStateDTO{dto}
	p.emitParentEvent("updateParentTask", data)
}

// PushParentProgress 推送父任务进度（已完成子任务数/总子任务数）到前端
func (p *WailsTaskProgressPusher) PushParentProgress(taskId int64, total int64, finished int64) {
	dto := &taskScheduleDTO{
		ID:       taskId,
		Total:    total,
		Finished: finished,
	}
	data := []*taskScheduleDTO{dto}
	p.emitParentEvent("updateParentSchedule", data)
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
func (p *NoopProgressPusher) PushProgress(int64, int64, int64)              {}
func (p *NoopProgressPusher) PushProgressBatch([]*taskScheduleDTO)          {}
func (p *NoopProgressPusher) PushParentProgress(int64, int64, int64)        {}
func (p *NoopProgressPusher) PushError(int64, string)                       {}
func (p *NoopProgressPusher) PushTaskRemove([]int64)                        {}
func (p *NoopProgressPusher) PushParentTaskRemove([]int64)                  {}
func (p *NoopProgressPusher) PushDuplicateDetected(int64, string, int64, string) {}

// taskSnapshotDTO 快照推送 DTO，包含 Manager 实时快照 + 被移除任务的缓冲区
type taskSnapshotDTO struct {
	Tasks               []*taskSnapshotItem `json:"tasks"`
	ParentTasks         []*taskSnapshotItem `json:"parentTasks"`
	RemovedTasks        []*taskSnapshotItem `json:"removedTasks"`
	RemovedParentTasks  []*taskSnapshotItem `json:"removedParentTasks"`
}

// taskSnapshotItem 快照中的单个任务条目
type taskSnapshotItem struct {
	ID       int64  `json:"id"`
	TaskName string `json:"taskName"`
	Status   int    `json:"status"`
	Total    int64  `json:"total"`
	Finished int64  `json:"finished"`
}

// SnapshotDataProvider 快照数据提供者接口，由 Manager 实现
type SnapshotDataProvider interface {
	BuildSnapshot() *taskSnapshotDTO
}

// SnapshotPusher 快照推送器，收集变更后防抖推送完整状态快照
// 快照来源：Manager 的 taskMap/parentMap 实时状态
// 移除缓冲区：记录被 PushTaskRemove 标记的任务的最新状态（由 PushStateChange 积累）
// flush 时将实时快照和移除缓冲区一并推送，前端先全量替换再用缓冲区补充 + 设定时器
type SnapshotPusher struct {
	provider   SnapshotDataProvider
	emitter    WailsEventEmitter
	debounceMs int
	mu         sync.Mutex
	timer      *time.Timer
	dirty      bool
	// 移除缓冲区：被移除任务的最新状态（带终态信息）
	removedTaskItems   []*taskSnapshotItem
	removedParentItems []*taskSnapshotItem
	// 状态记录：用于在 PushTaskRemove 时查找最后状态并转入缓冲区
	taskStates   map[int64]*taskSnapshotItem
	parentStates map[int64]*taskSnapshotItem
}

// NewSnapshotPusher 创建快照推送器
func NewSnapshotPusher(emitter WailsEventEmitter, provider SnapshotDataProvider, debounceMs int) *SnapshotPusher {
	logger.Log.Infof("[TaskPusher] 创建 SnapshotPusher（防抖 %dms）", debounceMs)
	return &SnapshotPusher{
		provider:     provider,
		emitter:      emitter,
		debounceMs:   debounceMs,
		taskStates:   make(map[int64]*taskSnapshotItem),
		parentStates: make(map[int64]*taskSnapshotItem),
	}
}

// markDirty 标记脏数据并重置防抖定时器
func (s *SnapshotPusher) markDirty() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(time.Duration(s.debounceMs)*time.Millisecond, s.flush)
}

// flush 执行快照推送：Manager 实时快照 + 移除缓冲区
func (s *SnapshotPusher) flush() {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	s.dirty = false
	// 取出移除缓冲区
	removedTasks := s.removedTaskItems
	removedParents := s.removedParentItems
	s.removedTaskItems = nil
	s.removedParentItems = nil
	// 清空状态记录
	s.taskStates = make(map[int64]*taskSnapshotItem)
	s.parentStates = make(map[int64]*taskSnapshotItem)
	s.mu.Unlock()

	// 获取 Manager 实时快照（基于 taskMap/parentMap）
	snapshot := s.provider.BuildSnapshot()
	// 附加移除缓冲区
	snapshot.RemovedTasks = removedTasks
	snapshot.RemovedParentTasks = removedParents

	s.emitter.Emit("task-snapshot", snapshot)
}

// EmitSnapshot 立即推送一次快照（供前端主动请求时使用）
func (s *SnapshotPusher) EmitSnapshot() {
	s.mu.Lock()
	s.dirty = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	s.flush()
}

// updateTaskState 更新子任务状态记录（不存在则创建）
func (s *SnapshotPusher) updateTaskState(taskId int64, updateFunc func(item *taskSnapshotItem)) {
	s.mu.Lock()
	item, ok := s.taskStates[taskId]
	if !ok {
		item = &taskSnapshotItem{ID: taskId}
		s.taskStates[taskId] = item
	}
	updateFunc(item)
	s.mu.Unlock()
}

// updateParentState 更新父任务状态记录（不存在则创建）
func (s *SnapshotPusher) updateParentState(taskId int64, updateFunc func(item *taskSnapshotItem)) {
	s.mu.Lock()
	item, ok := s.parentStates[taskId]
	if !ok {
		item = &taskSnapshotItem{ID: taskId}
		s.parentStates[taskId] = item
	}
	updateFunc(item)
	s.mu.Unlock()
}

// updateExistingTaskState 仅更新已存在的子任务状态（进度更新不应创建新条目）
func (s *SnapshotPusher) updateExistingTaskState(taskId int64, updateFunc func(item *taskSnapshotItem)) {
	s.mu.Lock()
	if item, ok := s.taskStates[taskId]; ok {
		updateFunc(item)
	}
	s.mu.Unlock()
}

// updateExistingParentState 仅更新已存在的父任务状态
func (s *SnapshotPusher) updateExistingParentState(taskId int64, updateFunc func(item *taskSnapshotItem)) {
	s.mu.Lock()
	if item, ok := s.parentStates[taskId]; ok {
		updateFunc(item)
	}
	s.mu.Unlock()
}

func (s *SnapshotPusher) PushStateChange(taskId int64, taskName string, state TaskState) {
	s.updateTaskState(taskId, func(item *taskSnapshotItem) {
		item.TaskName = taskName
		item.Status = int(state)
	})
	s.markDirty()
}

func (s *SnapshotPusher) PushParentStateChange(taskId int64, taskName string, state TaskState) {
	s.updateParentState(taskId, func(item *taskSnapshotItem) {
		item.TaskName = taskName
		item.Status = int(state)
	})
	s.markDirty()
}

func (s *SnapshotPusher) PushProgress(taskId int64, total int64, finished int64) {
	s.updateExistingTaskState(taskId, func(item *taskSnapshotItem) {
		item.Total = total
		item.Finished = finished
	})
	s.markDirty()
}

func (s *SnapshotPusher) PushProgressBatch(batch []*taskScheduleDTO) {
	for _, dto := range batch {
		s.updateExistingTaskState(dto.ID, func(item *taskSnapshotItem) {
			item.Total = dto.Total
			item.Finished = dto.Finished
		})
	}
	s.markDirty()
}

func (s *SnapshotPusher) PushParentProgress(taskId int64, total int64, finished int64) {
	s.updateExistingParentState(taskId, func(item *taskSnapshotItem) {
		item.Total = total
		item.Finished = finished
	})
	s.markDirty()
}

func (s *SnapshotPusher) PushError(int64, string) {}

func (s *SnapshotPusher) PushTaskRemove(taskIds []int64) {
	s.mu.Lock()
	for _, id := range taskIds {
		if item, ok := s.taskStates[id]; ok {
			s.removedTaskItems = append(s.removedTaskItems, item)
			delete(s.taskStates, id)
		}
	}
	s.mu.Unlock()
	s.markDirty()
}

func (s *SnapshotPusher) PushParentTaskRemove(taskIds []int64) {
	s.mu.Lock()
	for _, id := range taskIds {
		if item, ok := s.parentStates[id]; ok {
			s.removedParentItems = append(s.removedParentItems, item)
			delete(s.parentStates, id)
		}
	}
	s.mu.Unlock()
	s.markDirty()
}

// PushDuplicateDetected 重复检测仍走独立 topic（与状态推送解耦）
func (s *SnapshotPusher) PushDuplicateDetected(taskId int64, taskName string, existingWorkId int64, existingWorkName string) {
	dto := &duplicateDetectedDTO{
		TaskId:           taskId,
		TaskName:         taskName,
		ExistingWorkId:   existingWorkId,
		ExistingWorkName: existingWorkName,
	}
	s.emitter.Emit("taskStatus-duplicateDetected", dto)
}
