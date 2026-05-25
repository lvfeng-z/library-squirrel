package taskManager

import (
	"context"
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
	// BatchSetStatus 批量设置任务状态
	BatchSetStatus(ctx context.Context, statuses map[int64]task.TaskStatusEnum) error
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
	pendingStatusUpdates map[int64]task.TaskStatusEnum
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
	// 资源文件备份
	resourceBackuper ResourceFileBackuper
	// 等待用户确认的任务（WaitingForInput 状态，已释放信号量）
	waitingForInputMap map[int64]*ManagedTask
	waitingForInputMu  sync.Mutex
}

// NewManager 创建任务管理器
func NewManager(maxParallel int, workDirProvider WorkDirProvider, fileNameFormatProvider FileNameFormatProvider, repo Repository, pusher TaskProgressPusher, pluginExecFactory func(pluginPublicId string) (TaskExecutor, error), workInfoSaver WorkInfoSaver, resourceSaver ResourceSaver, workChecker WorkChecker, resourceReader ResourceReader, resourceBackuper ResourceFileBackuper) *Manager {
	m := &Manager{
		taskMap:                make(map[int64]*ManagedTask),
		parentMap:              make(map[int64]*ParentTask),
		waitingQueue:           make([]*ManagedTask, 0),
		maxParallel:            maxParallel,
		semaphore:              make(chan struct{}, maxParallel),
		pendingStatusUpdates:   make(map[int64]task.TaskStatusEnum),
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
		resourceBackuper:       resourceBackuper,
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
	for _, t := range tasks {
		if t.Pid.Valid && t.Pid.Int64 == taskId {
			// 直接子任务
			mt := m.newManagedTask(t)
			parentTask.AddChild(mt)
			m.addTask(mt)
		} else if t.ID == taskId && (!t.IsCollection.Valid || t.IsCollection.Int64 == 0) {
			// 单个任务（非集合），自身作为子任务执行
			mt := m.newManagedTask(t)
			parentTask.AddChild(mt)
			m.addTask(mt)
		}
	}

	// 保存父任务
	m.mu.Lock()
	m.parentMap[taskId] = parentTask
	m.mu.Unlock()

	// 3. 尝试分发所有子任务（受信号量控制）
	for _, child := range parentTask.GetChildren() {
		m.tryDispatch(child)
	}

	return nil
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

	result := task.run()

	if result == runResultNeedConfirm {
		// 停放到等待队列，信号量由 defer 释放
		m.waitingForInputMu.Lock()
		m.waitingForInputMap[task.taskId] = task
		m.waitingForInputMu.Unlock()
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
	m.mu.RLock()
	parent, ok := m.parentMap[taskId]
	m.mu.RUnlock()

	if !ok {
		return ErrTaskTreeNotFound
	}

	for _, child := range parent.GetChildren() {
		if err := child.Pause(); err != nil {
			logger.Log.Errorf("暂停子任务 %d 失败: %v", child.taskId, err)
		}
	}

	return nil
}

// ResumeTaskTree 恢复任务树
func (m *Manager) ResumeTaskTree(ctx context.Context, taskId int64) error {
	m.mu.RLock()
	parent, ok := m.parentMap[taskId]
	m.mu.RUnlock()

	if !ok {
		return ErrTaskTreeNotFound
	}

	for _, child := range parent.GetChildren() {
		if err := child.Resume(); err != nil {
			logger.Log.Errorf("恢复子任务 %d 失败: %v", child.taskId, err)
		}
	}

	return nil
}

// StopTaskTree 停止任务树
func (m *Manager) StopTaskTree(ctx context.Context, taskId int64) error {
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
			child.setState(TaskStateFailed)
		} else {
			child.Stop()
		}
	}

	return nil
}

// RetryTaskTree 重试任务树
func (m *Manager) RetryTaskTree(ctx context.Context, taskId int64) error {
	// 重置任务状态为 Created
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
		m.removeWaitingTask(task)
		return nil
	}

	// action == "replace": 跳过重复检查，重新调度
	task.skipDuplicateCheck = true
	m.tryDispatch(task)
	return nil
}

// removeWaitingTask 处理跳过任务的清理（从内存中移除，不写 DB）
func (m *Manager) removeWaitingTask(task *ManagedTask) {
	// 推送状态回到 DB 中的原始状态，让前端看到状态转换
	originalState := TaskState(task.task.Status)
	taskName := ""
	if task.task.TaskName.Valid {
		taskName = task.task.TaskName.String
	}
	m.pusher.PushStateChange(task.taskId, taskName, originalState)

	m.mu.Lock()
	delete(m.taskMap, task.taskId)

	if task.parentId != 0 {
		parent, ok := m.parentMap[task.parentId]
		if ok {
			parent.RemoveChild(task.taskId)
			if parent.AllChildrenTerminal() {
				delete(m.parentMap, task.parentId)
				m.mu.Unlock()
				m.pusher.PushParentTaskRemove([]int64{task.parentId})
			} else {
				_, newParentState := parent.RefreshState()
				m.mu.Unlock()
				m.pusher.PushParentStateChange(parent.taskId, parent.taskName, newParentState)
			}
		} else {
			m.mu.Unlock()
		}
	} else {
		m.mu.Unlock()
	}

	task.cancel()
	m.pusher.PushTaskRemove([]int64{task.taskId})
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
func (m *Manager) cleanupFinishedTask(task *ManagedTask) {
	// 从 taskMap 移除子任务
	m.mu.Lock()
	delete(m.taskMap, task.taskId)

	// 检查父任务是否所有子任务已终态
	if task.parentId != 0 {
		parent, ok := m.parentMap[task.parentId]
		if ok && parent.AllChildrenTerminal() {
			delete(m.parentMap, task.parentId)
			m.mu.Unlock()
			m.pusher.PushParentTaskRemove([]int64{task.parentId})
		} else {
			m.mu.Unlock()
		}
	} else {
		m.mu.Unlock()
	}

	// 通知前端移除子任务
	m.pusher.PushTaskRemove([]int64{task.taskId})
}

func (m *Manager) getTask(taskId int64) (*ManagedTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	managedTask, ok := m.taskMap[taskId]
	return managedTask, ok
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
	mt := NewManagedTask(t.GetID(), parentId, t, pluginExec, m.workInfoSaver, m.resourceSaver, m.workDirProvider, m.fileNameFormatProvider, m.workChecker, m.resourceReader, m.resourceBackuper, m.pusher)

	// 设置状态变化回调
	taskName := t.TaskName.String
	mt.SetOnStateChange(func(taskId int64, oldState, newState TaskState) {
		// 仅稳定状态写入数据库，瞬态只更新内存和前端
		if isStableState(newState) {
			m.addToPending(taskId, task.TaskStatusEnum(newState))
		}

		// 推送状态到前端
		m.pusher.PushStateChange(taskId, taskName, newState)

		// 刷新并持久化父任务状态
		if mt.parentId != 0 {
			if parent, ok := m.parentMap[mt.parentId]; ok {
				oldParentState, newParentState := parent.RefreshState()
				if oldParentState != newParentState && isStableState(newParentState) {
					m.addToPending(parent.taskId, task.TaskStatusEnum(newParentState))
				}
				m.pusher.PushParentStateChange(parent.taskId, parent.taskName, newParentState)
			}
		}
	})

	// 设置进度回调
	mt.SetOnProgress(func(taskId int64, total int64, finished int64) {
		m.pusher.PushProgress(taskId, total, finished)
	})
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
	m.pendingStatusUpdates = make(map[int64]task.TaskStatusEnum)
	m.pendingMu.Unlock()

	if err := m.repo.BatchSetStatus(context.Background(), pending); err != nil {
		logger.Log.Errorf("[TaskManager] 批量写入任务状态失败: %v", err)
	}
}

// addToPending 添加待刷盘的状态变更，非阻塞通知 flushLoop
func (m *Manager) addToPending(taskId int64, status task.TaskStatusEnum) {
	m.pendingMu.Lock()
	m.pendingStatusUpdates[taskId] = status
	m.pendingMu.Unlock()

	select {
	case m.flushCh <- struct{}{}:
	default:
	}
}
