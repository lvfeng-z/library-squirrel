package taskManager

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/task"
)

// Repository 任务仓储接口（TaskManager 需要的数据库操作）
type Repository interface {
	// ListTaskTree 获取任务树列表
	ListTaskTree(ctx context.Context, taskIds []int64, includeStatus ...task.TaskStatusEnum) ([]*domain.Task, error)
	// SetTaskTreeStatus 设置任务树状态
	SetTaskTreeStatus(ctx context.Context, taskIds []int64, status task.TaskStatusEnum, includeStatus ...task.TaskStatusEnum) (int64, error)
	// SetStatus 设置指定任务的状态（不级联）
	SetStatus(ctx context.Context, taskId int64, status task.TaskStatusEnum) error
	// BatchSetStatus 批量设置任务状态（同时更新 error_message）
	BatchSetStatus(ctx context.Context, statuses map[int64]task.StatusUpdate) error
	// UpdatePendingResourceID 更新任务的 pending_resource_id
	UpdatePendingResourceID(ctx context.Context, taskId int64, resourceID sql.NullInt64) error
}

// WorkDirProvider 工作目录提供者接口
type WorkDirProvider interface {
	GetWorkDir() string
}

// FileNameFormatProvider 文件名格式模板提供者接口
type FileNameFormatProvider interface {
	GetFileNameFormat() string
}

// Manager 任务管理器
type Manager struct {
	// 任务Map（所有运行中的任务）
	taskMap map[int64]*ManagedTask
	// 父任务Map
	parentMap map[int64]*ParentTask
	// 等待信号量的任务队列（FIFO）
	waitingQueue []*ManagedTask
	// 互斥锁
	mu sync.RWMutex
	// 优雅关闭标记
	shuttingDown atomic.Bool
	// 批量状态写入
	pendingStatusUpdates map[int64]task.StatusUpdate
	pendingMu            sync.Mutex
	flushCh              chan struct{}
	closeCh              chan struct{}
	flushDone            chan struct{}

	// 信号量（控制并发数）
	semaphore   chan struct{}
	maxParallel int

	// 任务执行器工厂（用于创建任务执行器）
	pluginExecFactory func(pluginPublicId string) (TaskExecutor, error)

	// 进度推送器
	pusher TaskProgressPusher

	// Repository（任务数据库操作）
	repo Repository

	// 作品信息保存器
	workInfoSaver WorkInfoSaver
	// 资源保存器
	resourceSaver ResourceSaver

	// 工作目录提供者（实时读取，不缓存）
	workDirProvider WorkDirProvider
	// 文件名格式模板提供者（实时读取，不缓存）
	fileNameFormatProvider FileNameFormatProvider

	// 作品查重
	workChecker WorkChecker
	// 资源查询（查找已有作品的资源文件）
	resourceReader ResourceReader
	// 资源备份编排器
	backupOrchestrator ResourceBackupOrchestrator
	// 等待用户确认的任务（WaitingForInput 状态，已释放信号量）
	waitingForInputMap map[int64]*ManagedTask
	waitingForInputMu  sync.Mutex
}

// NewManager 创建任务管理器
func NewManager(maxParallel int, workDirProvider WorkDirProvider, fileNameFormatProvider FileNameFormatProvider, repo Repository, pusher TaskProgressPusher, pluginExecFactory func(pluginPublicId string) (TaskExecutor, error), workInfoSaver WorkInfoSaver, resourceSaver ResourceSaver, workChecker WorkChecker, resourceReader ResourceReader, backupOrchestrator ResourceBackupOrchestrator) *Manager {
	m := &Manager{
		taskMap:                make(map[int64]*ManagedTask),
		parentMap:              make(map[int64]*ParentTask),
		waitingQueue:           make([]*ManagedTask, 0),
		maxParallel:            maxParallel,
		semaphore:              make(chan struct{}, maxParallel),
		pendingStatusUpdates:   make(map[int64]task.StatusUpdate),
		flushCh:                make(chan struct{}, 1),
		closeCh:                make(chan struct{}),
		flushDone:              make(chan struct{}),
		workDirProvider:        workDirProvider,
		fileNameFormatProvider: fileNameFormatProvider,
		repo:                   repo,
		pusher:                 pusher,
		pluginExecFactory:      pluginExecFactory,
		workInfoSaver:          workInfoSaver,
		resourceSaver:          resourceSaver,
		workChecker:            workChecker,
		resourceReader:         resourceReader,
		backupOrchestrator:     backupOrchestrator,
		waitingForInputMap:     make(map[int64]*ManagedTask),
	}
	go m.flushLoop()
	return m
}

// StartTaskTree 启动任务树
func (m *Manager) StartTaskTree(ctx context.Context, taskId int64) error {
	// 1. 获取任务树
	logger.Log.Infof("StartTaskTree: taskId=%d", taskId)
	tasks, err := m.repo.ListTaskTree(ctx, []int64{taskId})
	if err != nil {
		logger.Log.Errorf("StartTaskTree: ListTaskTree 失败: %v", err)
		return err
	}

	logger.Log.Infof("StartTaskTree: 查询到 %d 条任务记录", len(tasks))
	if len(tasks) == 0 {
		return ErrTaskTreeNotFound
	}

	// 2. 构建父子关系
	parentTaskName := ""
	for _, t := range tasks {
		if t.ID == taskId && t.TaskName.Valid {
			parentTaskName = t.TaskName.String
			break
		}
	}
	parentTask := NewParentTask(taskId, parentTaskName)
	var pausedChildren []*ManagedTask
	for _, t := range tasks {
		var child *ManagedTask
		if t.Pid.Valid && t.Pid.Int64 == taskId {
			// 直接子任务
			child = m.buildOrReuseChild(t, &pausedChildren)
		} else if t.ID == taskId && (!t.HasChild.Valid || !t.HasChild.Bool) {
			// 单个任务（非集合），自身作为子任务执行
			child = m.buildOrReuseChild(t, &pausedChildren)
		}
		if child != nil {
			parentTask.AddChild(child)
		}
	}

	// 保存父任务
	m.mu.Lock()
	m.parentMap[taskId] = parentTask
	m.mu.Unlock()

	// 3. 分发所有子任务（受信号量控制）
	for _, child := range parentTask.GetChildren() {
		m.tryDispatch(child)
	}

	// 4. 恢复之前暂停的子任务
	for _, paused := range pausedChildren {
		if err := paused.Resume(); err != nil {
			logger.Log.Errorf("恢复暂停子任务 %d 失败: %v", paused.taskId, err)
		}
	}

	return nil
}

// buildOrReuseChild 构建子任务 ManagedTask
// 若该任务已存在于内存中且处于暂停状态则复用并恢复
// 若仅存在于数据库中（Paused 稳态但内存已丢失），根据 pending_resource_id 决定续传或重新执行
func (m *Manager) buildOrReuseChild(t *domain.Task, pausedChildren *[]*ManagedTask) *ManagedTask {
	isPausedInDB := t.Status == int(TaskStatePaused)

	// 场景一：内存中存在暂停状态的 ManagedTask，可直接复用并恢复
	if isPausedInDB {
		if existing, ok := m.getTask(t.GetID()); ok && existing.GetState() == TaskStatePaused {
			logger.Log.Infof("StartTaskTree: 复用内存中已暂停的子任务 %d", t.GetID())
			*pausedChildren = append(*pausedChildren, existing)
			return existing
		}

		// 场景二：数据库中为 Paused 但内存已丢失（如应用重启）
		if t.PendingResourceID.Valid {
			// pending_resource_id 有效，创建跨重启续传的 ManagedTask
			logger.Log.Infof("StartTaskTree: 子任务 %d 跨重启续传，pendingResourceID=%d", t.GetID(), t.PendingResourceID.Int64)
			mt := m.newManagedTask(t)
			if mt != nil {
				mt.resumeFromDB = true
				m.addTask(mt)
			}
			return mt
		}

		// 场景三：Paused 但无 pending_resource_id（setup 阶段暂停或旧数据），从头执行
		logger.Log.Warnf("StartTaskTree: 子任务 %d 在数据库中为 Paused 状态但无 pending_resource_id，将从头部重新执行", t.GetID())
	}

	// 创建新实例
	mt := m.newManagedTask(t)
	if mt != nil {
		m.addTask(mt)
	}
	return mt
}

// tryDispatch 尝试获取信号量并分发任务，无法获取时入等待队列
func (m *Manager) tryDispatch(task *ManagedTask) {
	select {
	case m.semaphore <- struct{}{}:
		go m.executeTask(task)
	default:
		m.mu.Lock()
		m.waitingQueue = append(m.waitingQueue, task)
		m.mu.Unlock()
		task.setState(TaskStateWaiting)
	}
}

// executeTask 在独立协程中执行任务（已获取信号量）
func (m *Manager) executeTask(task *ManagedTask) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("[TaskManager] executeTask panic: %v", r)
		}
		<-m.semaphore
		m.dispatchFromQueue()
	}()

	var result runResult
	if task.resumeFromDB {
		result = task.resumeFromPersistedState()
	} else {
		result = task.run()
	}

	if result == runResultNeedConfirm {
		// 停放到等待队列，信号量由 defer 释放
		m.waitingForInputMu.Lock()
		m.waitingForInputMap[task.taskId] = task
		m.waitingForInputMu.Unlock()
		return
	}

	if result == runResultPaused {
		// Setup 阶段暂停：goroutine 退出，任务保留在 taskMap，信号量由 defer 释放
		// Resume 时通过 tryRestart 回调重新调度（go m.executeTask(task)）
		return
	}

	m.cleanupFinishedTask(task)
}

// dispatchFromQueue 从等待队列中分发任务到可用信号量槽位
func (m *Manager) dispatchFromQueue() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for len(m.waitingQueue) > 0 {
		select {
		case m.semaphore <- struct{}{}:
			task := m.waitingQueue[0]
			m.waitingQueue[0] = nil
			m.waitingQueue = m.waitingQueue[1:]
			go m.executeTask(task)
		default:
			return
		}
	}
}

// removeFromQueue 从等待队列中移除指定任务
func (m *Manager) removeFromQueue(taskId int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.waitingQueue {
		if t.taskId == taskId {
			m.waitingQueue[i] = m.waitingQueue[len(m.waitingQueue)-1]
			m.waitingQueue[len(m.waitingQueue)-1] = nil
			m.waitingQueue = m.waitingQueue[:len(m.waitingQueue)-1]
			return
		}
	}
}

// PauseTaskTree 暂停任务树
func (m *Manager) PauseTaskTree(ctx context.Context, taskId int64) error {
	logger.Log.Infof("[TaskManager] 暂停任务树: taskId=%d", taskId)
	m.mu.RLock()
	parent, ok := m.parentMap[taskId]
	m.mu.RUnlock()

	if !ok {
		return ErrTaskTreeNotFound
	}

	for _, child := range parent.GetChildren() {
		if err := child.Pause(); err != nil {
			logger.Log.Errorf("[TaskManager] 暂停子任务 %d 失败: %v", child.taskId, err)
		}
	}

	return nil
}

// ResumeTaskTree 恢复任务树
func (m *Manager) ResumeTaskTree(ctx context.Context, taskId int64) error {
	logger.Log.Infof("[TaskManager] 恢复任务树: taskId=%d", taskId)
	m.mu.RLock()
	parent, ok := m.parentMap[taskId]
	m.mu.RUnlock()

	if !ok {
		// 任务树不在内存中（如应用重启后），通过 StartTaskTree 加载
		// buildOrReuseChild 会根据 pending_resource_id 决定续传或重新执行
		logger.Log.Infof("[TaskManager] 恢复任务树: taskId=%d 不在 parentMap 中，调用 StartTaskTree 加载", taskId)
		return m.StartTaskTree(ctx, taskId)
	}

	for _, child := range parent.GetChildren() {
		if err := child.Resume(); err != nil {
			logger.Log.Errorf("[TaskManager] 恢复子任务 %d 失败: %v", child.taskId, err)
		}
	}

	return nil
}

// StopTaskTree 停止任务树
func (m *Manager) StopTaskTree(ctx context.Context, taskId int64) error {
	logger.Log.Infof("[TaskManager] 停止任务树: taskId=%d", taskId)
	m.mu.RLock()
	parent, ok := m.parentMap[taskId]
	m.mu.RUnlock()

	if !ok {
		return ErrTaskTreeNotFound
	}

	for _, child := range parent.GetChildren() {
		if child.GetState() == TaskStateWaiting {
			m.removeFromQueue(child.taskId)
			child.cancel()
			child.setFailed("任务被用户停止")
		} else {
			child.Stop()
		}
	}

	return nil
}

// RetryTaskTree 重试任务树
func (m *Manager) RetryTaskTree(ctx context.Context, taskId int64) error {
	logger.Log.Infof("[TaskManager] 重试任务树: taskId=%d", taskId)
	// 重置任务状态为 Created（同时清除 error_message）
	_, err := m.repo.SetTaskTreeStatus(ctx, []int64{taskId}, task.TaskStatusCreated, task.TaskStatusFailed)
	if err != nil {
		return err
	}

	// 重新启动任务树
	return m.StartTaskTree(ctx, taskId)
}

// GetTaskTreeState 获取任务树状态
func (m *Manager) GetTaskTreeState(taskId int64) (TaskState, error) {
	m.mu.RLock()
	parent, ok := m.parentMap[taskId]
	m.mu.RUnlock()

	if !ok {
		return TaskStateCreated, ErrTaskTreeNotFound
	}

	return parent.GetState(), nil
}

// GetTaskState 获取任务状态
func (m *Manager) GetTaskState(taskId int64) (TaskState, error) {
	m.mu.RLock()
	managedTask, ok := m.taskMap[taskId]
	m.mu.RUnlock()

	if !ok {
		return TaskStateCreated, ErrTaskTreeNotFound
	}

	return managedTask.GetState(), nil
}

// GetPusher 获取进度推送器
func (m *Manager) GetPusher() TaskProgressPusher {
	return m.pusher
}

// SetPusher 设置进度推送器（用于 emitter 延迟就绪时替换 Noop）
func (m *Manager) SetPusher(pusher TaskProgressPusher) {
	m.pusher = pusher
}

// IsIdle 检查任务管理器是否处于空闲状态（没有运行中的任务）
func (m *Manager) IsIdle() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.taskMap) == 0
}

// ConfirmReplace 用户确认替换或跳过
func (m *Manager) ConfirmReplace(taskId int64, action string) error {
	m.waitingForInputMu.Lock()
	task, ok := m.waitingForInputMap[taskId]
	if !ok {
		m.waitingForInputMu.Unlock()
		return ErrTaskTreeNotFound
	}
	delete(m.waitingForInputMap, taskId)
	m.waitingForInputMu.Unlock()

	if action == "skip" {
		logger.Log.Infof("[TaskManager] 跳过重复任务: taskId=%d", taskId)
		m.removeWaitingTask(task)
		return nil
	}

	// action == "replace": 跳过重复检查，重新调度
	logger.Log.Infof("[TaskManager] 替换重复任务: taskId=%d", taskId)
	task.skipDuplicateCheck = true
	m.tryDispatch(task)
	return nil
}

// removeWaitingTask 处理跳过任务的清理（从内存中移除，不写 DB）
func (m *Manager) removeWaitingTask(mt *ManagedTask) {
	// 推送状态回到 DB 中的原始状态，让前端看到状态转换
	originalState := TaskState(mt.task.Status)
	taskName := ""
	if mt.task.TaskName.Valid {
		taskName = mt.task.TaskName.String
	}
	m.pusher.PushStateChange(mt.taskId, taskName, originalState)

	m.mu.Lock()
	delete(m.taskMap, mt.taskId)

	if mt.parentId != 0 {
		parent, ok := m.parentMap[mt.parentId]
		if ok {
			// 重置子任务原子状态为原始 DB 状态，使 RefreshState 正确计算父任务状态
			mt.state.Store(int32(originalState))

			// 在移除子任务前，先根据所有子任务（含被跳过的）计算并持久化父任务状态
			_, newParentState := parent.RefreshState()
			if isStableState(newParentState) {
				m.addToPending(parent.taskId, task.TaskStatusEnum(newParentState), "")
			}
			m.pusher.PushParentStateChange(parent.taskId, parent.taskName, newParentState)

			// 移除子任务并清理父任务
			parent.RemoveChild(mt.taskId)
			if parent.AllChildrenTerminal() {
				logger.Log.Infof("[TaskManager] removeWaitingTask: 删除 parentMap[%d]（所有子任务终态）", mt.parentId)
				delete(m.parentMap, mt.parentId)
				m.mu.Unlock()
				m.pusher.PushParentTaskRemove([]int64{mt.parentId})
			} else {
				m.mu.Unlock()
			}
		} else {
			m.mu.Unlock()
		}
	} else {
		m.mu.Unlock()
	}

	mt.cancel()
	m.pusher.PushTaskRemove([]int64{mt.taskId})
}

// IsShuttingDown 检查是否正在优雅关闭
func (m *Manager) IsShuttingDown() bool {
	return m.shuttingDown.Load()
}

// GracefulShutdown 暂停所有瞬态任务并等待进入稳态
func (m *Manager) GracefulShutdown(ctx context.Context) error {
	if !m.shuttingDown.CompareAndSwap(false, true) {
		return nil
	}
	logger.Log.Info("[TaskManager] 开始优雅关闭")

	// 清空等待队列
	m.mu.Lock()
	for _, t := range m.waitingQueue {
		t.cancel()
	}
	m.waitingQueue = nil
	tasks := make([]*ManagedTask, 0, len(m.taskMap))
	for _, t := range m.taskMap {
		tasks = append(tasks, t)
	}
	m.mu.Unlock()

	// 清理等待用户确认的任务（无运行协程，直接移除）
	m.waitingForInputMu.Lock()
	waitingIds := make([]int64, 0, len(m.waitingForInputMap))
	for id, t := range m.waitingForInputMap {
		waitingIds = append(waitingIds, id)
		t.setState(TaskStateCreated)
	}
	m.waitingForInputMap = make(map[int64]*ManagedTask)
	m.waitingForInputMu.Unlock()

	// 暂停所有 Processing 任务
	for _, t := range tasks {
		if t.GetState() == TaskStateProcessing {
			if err := t.Pause(); err != nil {
				logger.Log.Warnf("[TaskManager] 优雅关闭：暂停任务 %d 失败: %v", t.taskId, err)
			}
		}
	}

	// 等待所有任务进入稳态
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		allStable := true
		for _, t := range tasks {
			if !isStableState(t.GetState()) {
				allStable = false
				break
			}
		}
		if allStable {
			// 等待瞬态回调完成
			time.Sleep(50 * time.Millisecond)

			// 触发最终刷盘
			close(m.closeCh)
			<-m.flushDone
			return nil
		}
		select {
		case <-ctx.Done():
			// 超时，仍尝试最终刷盘
			close(m.closeCh)
			<-m.flushDone
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// 辅助方法

func (m *Manager) addTask(task *ManagedTask) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.taskMap[task.taskId] = task
}

func (m *Manager) removeTask(taskId int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.taskMap, taskId)
}

// cleanupFinishedTask 清理已终态的子任务，并检查父任务是否可清理
func (m *Manager) cleanupFinishedTask(mt *ManagedTask) {
	// 从 taskMap 移除子任务
	m.mu.Lock()
	delete(m.taskMap, mt.taskId)

	// 检查父任务是否所有子任务已终态
	if mt.parentId != 0 {
		parent, ok := m.parentMap[mt.parentId]
		if ok && parent.AllChildrenTerminal() {
			// 确保父任务的最终状态被持久化到数据库
			_, newParentState := parent.RefreshState()
			logger.Log.Infof("[TaskManager] cleanupFinishedTask 兜底刷新父任务状态: parentId=%d, newState=%s", parent.taskId, taskStateName(newParentState))
			if isStableState(newParentState) {
				m.addToPending(parent.taskId, task.TaskStatusEnum(newParentState), "")
			}
			logger.Log.Infof("[TaskManager] cleanupFinishedTask: 删除 parentMap[%d]（所有子任务终态）", mt.parentId)
			delete(m.parentMap, mt.parentId)
			m.mu.Unlock()
			m.pusher.PushParentTaskRemove([]int64{mt.parentId})
		} else {
			m.mu.Unlock()
		}
	} else {
		m.mu.Unlock()
	}

	// 通知前端移除子任务
	m.pusher.PushTaskRemove([]int64{mt.taskId})
}

func (m *Manager) getTask(taskId int64) (*ManagedTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	managedTask, ok := m.taskMap[taskId]
	return managedTask, ok
}

// GetTaskStates 获取所有内存中任务的当前状态快照
// 实现 task.MemoryStateProvider 接口
func (m *Manager) GetTaskStates() map[int64]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make(map[int64]int)

	// 子任务状态
	for id, mt := range m.taskMap {
		states[id] = int(mt.GetState())
	}

	// 父任务状态
	for id, pt := range m.parentMap {
		states[id] = int(pt.GetState())
	}

	// 等待确认的任务
	m.waitingForInputMu.Lock()
	for id, mt := range m.waitingForInputMap {
		states[id] = int(mt.GetState())
	}
	m.waitingForInputMu.Unlock()

	return states
}

func (m *Manager) newManagedTask(t *domain.Task) *ManagedTask {
	// 获取任务执行器
	if !t.PluginPublicID.Valid {
		logger.Log.Error("获取任务执行器失败: pluginPublicID is null")
		return nil
	}
	pluginExec, err := m.pluginExecFactory(t.PluginPublicID.String)
	if err != nil {
		logger.Log.Errorf("获取任务执行器失败: %v", err)
		return nil
	}

	parentId := int64(0)
	if t.Pid.Valid {
		parentId = t.Pid.Int64
	}
	mt := NewManagedTask(t.GetID(), parentId, t, pluginExec, m.workInfoSaver, m.resourceSaver, m.workDirProvider, m.fileNameFormatProvider, m.workChecker, m.resourceReader, m.backupOrchestrator, m.pusher)

	// 设置状态变化回调
	taskName := t.TaskName.String
	mt.SetOnStateChange(func(taskId int64, oldState, newState TaskState, errMsg string) {
		// 仅稳定状态写入数据库，瞬态只更新内存和前端
		if isStableState(newState) {
			m.addToPending(taskId, task.TaskStatusEnum(newState), errMsg)
		}

		// 推送状态到前端
		m.pusher.PushStateChange(taskId, taskName, newState)

		// 刷新并持久化父任务状态
		if mt.parentId != 0 {
			if parent, ok := m.parentMap[mt.parentId]; ok {
				oldParentState, newParentState := parent.RefreshState()
				logger.Log.Infof("[TaskManager] 父任务状态刷新: parentId=%d, old=%s, new=%s", parent.taskId, taskStateName(oldParentState), taskStateName(newParentState))
				if oldParentState != newParentState && isStableState(newParentState) {
					// 父任务无错误信息，传空字符串（清除 error_message）
					m.addToPending(parent.taskId, task.TaskStatusEnum(newParentState), "")
				}
				m.pusher.PushParentStateChange(parent.taskId, parent.taskName, newParentState)
			}
		}
	})

	// 设置进度回调
	mt.SetOnProgress(func(taskId int64, total int64, finished int64) {
		m.pusher.PushProgress(taskId, total, finished)
	})

	// 设置 setup 阶段暂停后恢复的重新调度回调
	mt.tryRestart = func(task *ManagedTask) {
		go m.executeTask(task)
	}

	// 设置 pending_resource_id 持久化回调
	mt.onResourceIDUpdate = func(taskId int64, resourceID sql.NullInt64) {
		if err := m.repo.UpdatePendingResourceID(context.Background(), taskId, resourceID); err != nil {
			logger.Log.Errorf("[TaskManager] 持久化 pending_resource_id 失败: taskId=%d, err=%v", taskId, err)
		}
	}

	return mt
}

// flushLoop 后台批量刷盘协程，空闲时阻塞在 channel 上零开销
func (m *Manager) flushLoop() {
	for {
		select {
		case <-m.closeCh:
			m.doFlush()
			close(m.flushDone)
			return
		case <-m.flushCh:
		}

		// 批量窗口：200ms 内的变更合并为一次写入
		time.Sleep(200 * time.Millisecond)
		m.doFlush()
	}
}

// doFlush 将积攒的状态变更批量写入数据库
func (m *Manager) doFlush() {
	m.pendingMu.Lock()
	if len(m.pendingStatusUpdates) == 0 {
		m.pendingMu.Unlock()
		return
	}
	pending := m.pendingStatusUpdates
	m.pendingStatusUpdates = make(map[int64]task.StatusUpdate)
	m.pendingMu.Unlock()

// 诊断日志：记录待刷盘的任务状态
	for id, u := range pending {
		errMsg := ""
		if u.ErrorMessage.Valid {
			errMsg = u.ErrorMessage.String
		}
		logger.Log.Infof("[TaskManager] doFlush: taskId=%d, status=%d, errMsg=%s", id, u.Status, errMsg)
	}
	if err := m.repo.BatchSetStatus(context.Background(), pending); err != nil {
		logger.Log.Errorf("[TaskManager] 批量写入任务状态失败: %v", err)
	}
}

// addToPending 添加待刷盘的状态变更，非阻塞通知 flushLoop
func (m *Manager) addToPending(taskId int64, status task.TaskStatusEnum, errMsg string) {
	m.pendingMu.Lock()
	m.pendingStatusUpdates[taskId] = task.StatusUpdate{
		Status:       status,
		ErrorMessage: sql.NullString{String: errMsg, Valid: errMsg != ""},
	}
	m.pendingMu.Unlock()

	select {
	case m.flushCh <- struct{}{}:
	default:
	}
}
