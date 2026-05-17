package taskManager

import (
	"context"
	"sync"

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
}

// WorkDirProvider 工作目录提供者接口
type WorkDirProvider interface {
	GetWorkDir() string
}

// Manager 任务管理器
type Manager struct {
	// 任务Map（所有运行中的任务）
	taskMap map[int64]*ManagedTask
	// 父任务Map
	parentMap map[int64]*ParentTask
	// 互斥锁
	mu sync.RWMutex

	// 信号量（控制并发数）
	semaphore   chan struct{}
	maxParallel int

	// 任务执行器工厂（用于创建任务执行器）
	pluginExecFactory func(pluginPublicId string) (TaskExecutor, error)

	// 进度推送器
	pusher *SSEProgressPusher

	// Repository（任务数据库操作）
	repo Repository

	// 作品信息保存器
	workInfoSaver WorkInfoSaver
	// 资源保存器
	resourceSaver ResourceSaver

	// 工作目录提供者（实时读取，不缓存）
	workDirProvider WorkDirProvider
}

// NewManager 创建任务管理器
func NewManager(maxParallel int, workDirProvider WorkDirProvider, repo Repository, pusher *SSEProgressPusher, pluginExecFactory func(pluginPublicId string) (TaskExecutor, error), workInfoSaver WorkInfoSaver, resourceSaver ResourceSaver) *Manager {
	return &Manager{
		taskMap:         make(map[int64]*ManagedTask),
		parentMap:       make(map[int64]*ParentTask),
		maxParallel:     maxParallel,
		semaphore:       make(chan struct{}, maxParallel),
		workDirProvider: workDirProvider,
		repo:            repo,
		pusher:          pusher,
		pluginExecFactory: pluginExecFactory,
		workInfoSaver:   workInfoSaver,
		resourceSaver:   resourceSaver,
	}
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
	parentTask := NewParentTask(taskId)
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

	// 3. 启动所有子任务（受信号量控制）
	for _, child := range parentTask.GetChildren() {
		m.startWithSemaphore(child)
	}

	return nil
}

// startWithSemaphore 使用信号量启动任务
func (m *Manager) startWithSemaphore(task *ManagedTask) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Log.Errorf("[TaskManager] startWithSemaphore panic: %v", r)
			}
		}()

		// 获取信号量
		m.semaphore <- struct{}{}
		defer func() { <-m.semaphore }()

		// 设置状态为等待
		task.setState(TaskStateWaiting)

		// 启动任务
		task.Start()

		// 等待任务完成信号
		<-task.Done()
	}()
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
		child.Stop()
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
func (m *Manager) GetPusher() *SSEProgressPusher {
	return m.pusher
}

// IsIdle 检查任务管理器是否处于空闲状态（没有运行中的任务）
func (m *Manager) IsIdle() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.taskMap) == 0
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

func (m *Manager) getTask(taskId int64) (*ManagedTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	managedTask, ok := m.taskMap[taskId]
	return managedTask, ok
}

func (m *Manager) newManagedTask(task *domain.Task) *ManagedTask {
	// 获取任务执行器
	if !task.PluginPublicID.Valid {
		logger.Log.Error("获取任务执行器失败: pluginPublicID is null")
		return nil
	}
	pluginExec, err := m.pluginExecFactory(task.PluginPublicID.String)
	if err != nil {
		logger.Log.Errorf("获取任务执行器失败: %v", err)
		return nil
	}

	parentId := int64(0)
	if task.Pid.Valid {
		parentId = task.Pid.Int64
	}
	mt := NewManagedTask(task.GetID(), parentId, task, pluginExec, m.workInfoSaver, m.resourceSaver, m.workDirProvider)

	// 设置状态变化回调
	mt.SetOnStateChange(func(taskId int64, oldState, newState TaskState) {
		// 更新数据库
		dbStatus := m.taskStateToDbStatus(newState)
		if _, err := m.repo.SetTaskTreeStatus(context.Background(), []int64{taskId}, dbStatus); err != nil {
			logger.Log.Errorf("[TaskManager] 更新任务 %d 状态到数据库失败: %v", taskId, err)
		}

		// 推送状态到前端
		m.pusher.PushStateChange(taskId, newState)

		// 刷新父任务状态
		if mt.parentId != 0 {
			if parent, ok := m.parentMap[mt.parentId]; ok {
				parent.RefreshState()
				m.pusher.PushStateChange(parent.taskId, parent.GetState())
			}
		}
	})

	return mt
}

// taskStateToDbStatus 将 TaskState 转换为数据库 TaskStatusEnum
func (m *Manager) taskStateToDbStatus(state TaskState) task.TaskStatusEnum {
	switch state {
	case TaskStateCreated:
		return task.TaskStatusCreated
	case TaskStateWaiting:
		return task.TaskStatusWaiting
	case TaskStateProcessing:
		return task.TaskStatusProcessing
	case TaskStatePausing:
		return task.TaskStatusPause
	case TaskStatePaused:
		return task.TaskStatusPause
	case TaskStateStopping:
		return task.TaskStatusFailed
	case TaskStateFinished:
		return task.TaskStatusFinished
	case TaskStateFailed:
		return task.TaskStatusFailed
	default:
		return task.TaskStatusCreated
	}
}
