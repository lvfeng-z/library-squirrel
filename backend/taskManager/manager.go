package taskManager

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	// BatchUpdatePendingResourceID 批量更新任务的 pending_resource_id
	BatchUpdatePendingResourceID(ctx context.Context, updates map[int64]sql.NullInt64) error
	// ListBySiteAndSiteWorkID 按 (site_id, site_work_id) 反查关联任务记录（用于作品删除时停止运行中任务）
	ListBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) ([]*domain.Task, error)
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
	pendingStatusUpdates     map[int64]task.StatusUpdate
	pendingResourceIDUpdates map[int64]sql.NullInt64
	pendingProgressUpdates   map[int64]*taskScheduleDTO
	pendingMu                sync.Mutex
	flushCh                  chan struct{}
	closeCh                  chan struct{}
	flushDone                chan struct{}

	// 信号量（控制并发数）
	semaphore   chan struct{}
	maxParallel int

	// 任务执行器工厂（用于创建任务执行器）
	pluginExecFactory func(pluginPublicId string) (TaskExecutor, error)

	// 进度推送器
	pusher TaskProgressPusher
	// 快照推送器引用（仅在快照模式下非 nil，供 GetTaskSnapshot 使用）
	snapshotPusher *SnapshotPusher

	// Repository（任务数据库操作）
	repo Repository

	// 共享依赖（透传给 ManagedTask）
	deps *TaskDeps
	// 等待用户确认的任务（WaitingForInput 状态，已释放信号量）
	waitingForInputMap map[int64]*ManagedTask
	waitingForInputMu  sync.Mutex
}

// NewManager 创建任务管理器
func NewManager(maxParallel int, repo Repository, pusher TaskProgressPusher, pluginExecFactory func(pluginPublicId string) (TaskExecutor, error), deps *TaskDeps) *Manager {
	m := &Manager{
		taskMap:                  make(map[int64]*ManagedTask),
		parentMap:                make(map[int64]*ParentTask),
		waitingQueue:             make([]*ManagedTask, 0),
		maxParallel:              maxParallel,
		semaphore:                make(chan struct{}, maxParallel),
		pendingStatusUpdates:     make(map[int64]task.StatusUpdate),
		pendingResourceIDUpdates: make(map[int64]sql.NullInt64),
		pendingProgressUpdates:   make(map[int64]*taskScheduleDTO),
		flushCh:                  make(chan struct{}, 1),
		closeCh:                  make(chan struct{}),
		flushDone:                make(chan struct{}),
		repo:                     repo,
		pusher:                   pusher,
		pluginExecFactory:        pluginExecFactory,
		deps:                     deps,
		waitingForInputMap:       make(map[int64]*ManagedTask),
	}
	go m.flushLoop()
	return m
}

// StartTaskTree 启动任务树
func (m *Manager) StartTaskTree(ctx context.Context, taskId int64, isLeaf bool) error {
	return m.startTaskTrees(ctx, []int64{taskId}, runModeFull)
}

// startTaskTrees 开始执行多个任务树（开始/重试入口）
// mode 指定执行板块（Full/ResourceOnly/WorkInfo/Thumbnail）
func (m *Manager) startTaskTrees(ctx context.Context, taskIds []int64, mode runMode) error {
	return m.loadAndStartTaskTrees(ctx, taskIds, false, mode)
}

// resumeTaskTrees 恢复执行多个任务树（恢复入口，固定 Full 模式）
// 所有任务执行的唯二入口之一，skipTerminal 跳过已终态子任务
func (m *Manager) resumeTaskTrees(ctx context.Context, taskIds []int64) error {
	return m.loadAndStartTaskTrees(ctx, taskIds, true, runModeFull)
}

// loadAndStartTaskTrees 从数据库加载多个任务树并启动
// skipTerminal 为 true 时跳过已终态（Finished/Failed/PartlyFinished）的子任务，仅 Resume 使用
// mode 指定执行板块；多根：一次 ListTaskTree 查询 + 一次批量查重，按 DB 真实父子关系构建内存树
func (m *Manager) loadAndStartTaskTrees(ctx context.Context, taskIds []int64, skipTerminal bool, mode runMode) error {
	if len(taskIds) == 0 {
		return nil
	}
	logger.Log.Infof("loadAndStartTaskTrees: taskIds=%v, skipTerminal=%v", taskIds, skipTerminal)

	// 1. 共享一次 ListTaskTree 查询
	tasks, err := m.repo.ListTaskTree(ctx, taskIds)
	if err != nil {
		logger.Log.Errorf("loadAndStartTaskTrees: ListTaskTree 失败: %v", err)
		return err
	}
	logger.Log.Infof("loadAndStartTaskTrees: 查询到 %d 条任务记录", len(tasks))
	if len(tasks) == 0 {
		return ErrTaskTreeNotFound
	}

	taskById := make(map[int64]*domain.Task, len(tasks))
	for _, t := range tasks {
		taskById[t.ID] = t
	}

	// 2. 重复执行保护：快照运行中的 taskId / parentId，避免同一任务重复启动
	m.mu.RLock()
	runningTasks := make(map[int64]struct{}, len(m.taskMap))
	for id := range m.taskMap {
		runningTasks[id] = struct{}{}
	}
	runningParents := make(map[int64]struct{}, len(m.parentMap))
	for id := range m.parentMap {
		runningParents[id] = struct{}{}
	}
	m.mu.RUnlock()

	// 3. 确定处理单元并去重：独立任务为自身 taskId，叶子/父任务为其 actualParentId
	// 同父多叶子归一为同一单元，跳过已运行的单元
	processedUnits := make(map[int64]struct{})
	var standaloneChildren []*ManagedTask
	var parentUnits []int64

	for _, taskId := range taskIds {
		rootTask := taskById[taskId]
		if rootTask == nil {
			continue
		}

		isStandalone := (!rootTask.Pid.Valid || rootTask.Pid.Int64 == 0) &&
			(!rootTask.HasChild.Valid || !rootTask.HasChild.Bool)

		var unitId int64
		if isStandalone {
			unitId = taskId
		} else {
			isLeaf := rootTask.Pid.Valid && rootTask.Pid.Int64 > 0 &&
				(!rootTask.HasChild.Valid || !rootTask.HasChild.Bool)
			if isLeaf {
				unitId = rootTask.Pid.Int64
			} else {
				unitId = taskId // 父任务（has_child=1）
			}
		}

		// 重复执行保护
		if isStandalone {
			if _, running := runningTasks[unitId]; running {
				logger.Log.Infof("loadAndStartTaskTrees: 任务 %d 已在运行，跳过", unitId)
				continue
			}
		} else {
			if _, running := runningParents[unitId]; running {
				logger.Log.Infof("loadAndStartTaskTrees: 父任务 %d 已在运行，跳过", unitId)
				continue
			}
		}
		// 同单元去重
		if _, processed := processedUnits[unitId]; processed {
			continue
		}
		processedUnits[unitId] = struct{}{}

		if isStandalone {
			child := m.buildOrReuseChild(rootTask, skipTerminal, mode)
			if child == nil {
				// 已终态，直接持久化当前状态
				finalState := TaskState(rootTask.Status)
				if isStableState(finalState) {
					m.addToPending(unitId, task.TaskStatusEnum(finalState), "")
				}
				continue
			}
			standaloneChildren = append(standaloneChildren, child)
		} else {
			parentUnits = append(parentUnits, unitId)
		}
	}

	// 4. 处理各父任务单元，收集需调度的子任务
	allToCheck := make([]*ManagedTask, 0, len(standaloneChildren)+len(parentUnits)*4)
	allToCheck = append(allToCheck, standaloneChildren...)
	for _, parentId := range parentUnits {
		allToCheck = append(allToCheck, m.processParentUnit(tasks, taskById, parentId, skipTerminal, mode)...)
	}

	// 5. 共享一次批量预检重复 + 分发（受信号量控制）
	if len(allToCheck) > 0 {
		toDispatch := m.batchCheckDuplicates(ctx, allToCheck)
		for _, child := range toDispatch {
			m.tryDispatch(child)
		}
	}

	return nil
}

// processParentUnit 处理一个父任务单元：构建 ParentTask 并收集其直接子任务
// 返回需参与批量查重与调度的子任务；所有子任务已终态时计算父任务最终状态并推送移除，返回 nil
func (m *Manager) processParentUnit(tasks []*domain.Task, taskById map[int64]*domain.Task, actualParentId int64, skipTerminal bool, mode runMode) []*ManagedTask {
	parentTaskName := ""
	if parentEntity := taskById[actualParentId]; parentEntity != nil && parentEntity.TaskName.Valid {
		parentTaskName = parentEntity.TaskName.String
	}

	parentTask := NewParentTask(actualParentId, parentTaskName)
	for _, t := range tasks {
		if t.Pid.Valid && t.Pid.Int64 == actualParentId {
			if child := m.buildOrReuseChild(t, skipTerminal, mode); child != nil {
				parentTask.AddChild(child)
			}
		}
	}

	// 所有子任务已终态（仅 skipTerminal=true 时可能出现）
	if len(parentTask.GetChildren()) == 0 {
		finalState := m.computeParentFinalState(tasks, actualParentId)
		if isStableState(finalState) {
			m.addToPending(actualParentId, task.TaskStatusEnum(finalState), "")
		}
		m.deps.Pusher.PushParentStateChange(actualParentId, parentTaskName, finalState)
		m.deps.Pusher.PushParentTaskRemove([]int64{actualParentId})
		return nil
	}

	// 注册父任务
	m.mu.Lock()
	m.parentMap[actualParentId] = parentTask
	m.mu.Unlock()

	return parentTask.GetChildren()
}

// buildOrReuseChild 构建子任务 ManagedTask
// 若任务在数据库中为 Paused 状态（应用重启后内存已丢失），根据 pending_resource_id 决定续传或重新执行
// skipTerminal 为 true 时跳过已终态（Finished/Failed/PartlyFinished）的子任务
func (m *Manager) buildOrReuseChild(t *domain.Task, skipTerminal bool, mode runMode) *ManagedTask {
	dbState := TaskState(t.Status)

	// 跳过已终态的子任务（仅 Resume 场景需要）
	if skipTerminal && (dbState == TaskStateFinished || dbState == TaskStateFailed || dbState == TaskStatePartlyFinished) {
		return nil
	}

	isPausedInDB := dbState == TaskStatePaused

	if isPausedInDB {
		// 数据库中为 Paused 但内存已丢失（如应用重启）
		if t.PendingResourceID.Valid {
			// pending_resource_id 有效，创建跨重启续传的 ManagedTask
			logger.Log.Infof("StartTaskTree: 子任务 %d 跨重启续传，pendingResourceID=%d", t.GetID(), t.PendingResourceID.Int64)
			mt := m.newManagedTask(t, mode)
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
	mt := m.newManagedTask(t, mode)
	if mt != nil {
		m.addTask(mt)
	}
	return mt
}

// computeParentFinalState 从 DB 任务记录计算父任务的最终状态
// 当所有子任务都已终态时调用，无需创建 ManagedTask 即可确定父任务状态
func (m *Manager) computeParentFinalState(tasks []*domain.Task, parentId int64) TaskState {
	var finished, failed int
	total := 0
	for _, t := range tasks {
		if t.Pid.Valid && t.Pid.Int64 == parentId {
			total++
			switch TaskState(t.Status) {
			case TaskStateFinished:
				finished++
			case TaskStateFailed:
				failed++
			default:
				panic("unhandled default case")
			}
		}
	}
	if total == 0 {
		return TaskStateFinished
	}
	switch {
	case finished == total:
		return TaskStateFinished
	case failed == total:
		return TaskStateFailed
	case finished > 0:
		return TaskStatePartlyFinished
	default:
		return TaskStateFailed
	}
}

// batchCheckDuplicates 在任务派发前批量检查重复
// 将重复任务直接放入等待确认队列（不消耗信号量），非重复任务标记 skipDuplicateCheck 并返回以供派发
func (m *Manager) batchCheckDuplicates(ctx context.Context, children []*ManagedTask) []*ManagedTask {
	var toCheck []*ManagedTask
	for _, child := range children {
		// 跳过跨重启续传任务（直接走 resumeFromPersistedState）
		if child.resumeFromDB {
			continue
		}
		// 无 B 的板块组合不查重（A/C 不覆盖资源文件，无需"作品已存在"提醒）
		if !child.runMode.hasResource() {
			child.skipDuplicateCheck = true
			continue
		}
		// 不具备查重条件，标记跳过 run() 中的重复检测
		if child.task == nil || !child.task.SiteID.Valid || !child.task.SiteWorkID.Valid || child.task.SiteWorkID.String == "" {
			child.skipDuplicateCheck = true
			continue
		}
		toCheck = append(toCheck, child)
	}

	if len(toCheck) == 0 || m.deps.WorkChecker == nil {
		// 无需查重或未配置查重器，所有可预检任务标记跳过
		for _, child := range children {
			if !child.resumeFromDB {
				child.skipDuplicateCheck = true
			}
		}
		return children
	}

	// 收集查询参数，一次批量查询
	siteIds := make([]int64, len(toCheck))
	siteWorkIds := make([]string, len(toCheck))
	for i, child := range toCheck {
		siteIds[i] = child.task.SiteID.Int64
		siteWorkIds[i] = child.task.SiteWorkID.String
	}

	// 包裹超时 context，避免查重查询异常卡死时独占唯一 DB 连接拖垮全局
	// 超时后走 err 分支降级为 run() 逐个查重
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	existingWorks, err := m.deps.WorkChecker.ListBySiteAndSiteWorkIDs(checkCtx, siteIds, siteWorkIds)
	if err != nil {
		logger.Log.Errorf("[TaskManager] batchCheckDuplicates 批量查重失败: %v，降级为 run() 逐个检查", err)
		// 查询失败时不设 skipDuplicateCheck，由 run() 兜底
		return children
	}

	// 构建查重结果映射
	existingMap := make(map[string]*domain.Work, len(existingWorks))
	for _, w := range existingWorks {
		key := fmt.Sprintf("%d:%s", w.SiteID.Int64, w.SiteWorkID.String)
		existingMap[key] = w
	}

	var toDispatch []*ManagedTask
	for _, child := range children {
		// 跨重启续传的任务直接派发（需要信号量）
		if child.resumeFromDB {
			toDispatch = append(toDispatch, child)
			continue
		}

		// 无 B 的板块组合：已标记 skipDuplicateCheck，直接派发（不参与查重命中判断）
		if !child.runMode.hasResource() {
			toDispatch = append(toDispatch, child)
			continue
		}

		// 不具备查重条件的任务
		if child.task == nil || !child.task.SiteID.Valid || !child.task.SiteWorkID.Valid || child.task.SiteWorkID.String == "" {
			toDispatch = append(toDispatch, child)
			continue
		}

		key := fmt.Sprintf("%d:%s", child.task.SiteID.Int64, child.task.SiteWorkID.String)
		if existing, found := existingMap[key]; found {
			// 重复命中：放入等待确认队列，不参与信号量派发
			existingWorkName := ""
			if existing.SiteWorkName.Valid {
				existingWorkName = existing.SiteWorkName.String
			}
			child.existingWorkId = existing.GetID()
			child.setState(TaskStateWaitingForInput)
			m.deps.Pusher.PushDuplicateDetected(child.taskId, child.task.TaskName.String, existing.GetID(), existingWorkName)

			m.waitingForInputMu.Lock()
			m.waitingForInputMap[child.taskId] = child
			m.waitingForInputMu.Unlock()
		} else {
			// 非重复：标记跳过 run() 中的重复检测
			child.skipDuplicateCheck = true
			toDispatch = append(toDispatch, child)
		}
	}

	return toDispatch
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

	// 检查任务是否在 goroutine 启动前已被暂停/停止
	// PauseTaskTree/StopTaskTree 对 Paused/Waiting 状态的任务调用 cancel()
	// 如果 goroutine 启动时 context 已取消，直接退出（状态已由调用方设置）
	select {
	case <-task.ctx.Done():
		return
	default:
	}

	var result runResult
	if task.resumeFromDB {
		logger.Log.Infof("[TaskManager] executeTask: taskId=%d, resumeFromDB=%v", task.taskId, task.resumeFromDB)
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
		// 暂停（setup 或 download 阶段）：goroutine 退出，任务保留在 taskMap，信号量由 defer 释放
		// ResumeTaskTree 通过 prepareForResume + tryDispatch 重新调度
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
func (m *Manager) PauseTaskTree(ctx context.Context, taskId int64, isLeaf bool) error {
	logger.Log.Infof("[TaskManager] 暂停任务树: taskId=%d", taskId)
	parentKey, ok := m.resolveParentKey(taskId, isLeaf)
	if !ok {
		return ErrTaskTreeNotFound
	}
	m.mu.RLock()
	parent, ok := m.parentMap[parentKey]
	m.mu.RUnlock()

	if !ok {
		return ErrTaskTreeNotFound
	}

	children := parent.GetChildren()

	// 第一阶段：处理非阻塞任务（Waiting/Paused），先清空等待队列
	// 必须在第二阶段（阻塞的 Pause()）之前完成，否则 goroutine 退出时
	// dispatchFromQueue 会从队列取出尚未遍历到的 Waiting 任务并启动
	for _, child := range children {
		state := child.GetState()
		switch state {
		case TaskStateWaiting:
			// 排队中的任务：移出队列、设为暂停、取消 context
			m.removeFromQueue(child.taskId)
			child.setState(TaskStatePaused)
			child.cancel()
		case TaskStatePaused:
			// 已暂停但可能已派发 goroutine（prepareForResume 后的状态窗口）
			// cancel 使 goroutine 启动时通过 ctx.Done 检查直接退出
			// 对真正 Paused 的任务（无 goroutine），cancel 无副作用
			child.cancel()
		}
	}

	// 第二阶段：处理阻塞任务（Processing/Pausing），此时等待队列已清空
	// dispatchFromQueue 不会再取出新任务
	for _, child := range children {
		state := child.GetState()
		if state == TaskStateProcessing || state == TaskStatePausing {
			if err := child.Pause(); err != nil {
				logger.Log.Errorf("[TaskManager] 暂停子任务 %d 失败: %v", child.taskId, err)
			}
		}
	}

	return nil
}

// ResumeTaskTree 恢复任务树
func (m *Manager) ResumeTaskTree(ctx context.Context, taskId int64, isLeaf bool) error {
	logger.Log.Infof("[TaskManager] 恢复任务树: taskId=%d", taskId)
	parentKey, ok := m.resolveParentKey(taskId, isLeaf)
	if !ok {
		// taskMap 中无此叶子任务，尝试从数据库加载
		logger.Log.Infof("[TaskManager] 恢复任务树: taskId=%d 不在 taskMap 中，从数据库加载", taskId)
		return m.resumeTaskTrees(ctx, []int64{taskId})
	}
	m.mu.RLock()
	parent, ok := m.parentMap[parentKey]
	m.mu.RUnlock()

	if !ok {
		// 任务树不在内存中（如应用重启后），从数据库加载并跳过已终态子任务
		logger.Log.Infof("[TaskManager] 恢复任务树: taskId=%d 不在 parentMap 中，从数据库加载", taskId)
		return m.resumeTaskTrees(ctx, []int64{taskId})
	}

	for _, child := range parent.GetChildren() {
		if child.GetState() != TaskStatePaused {
			continue
		}
		child.prepareForResume()
		m.tryDispatch(child)
	}

	return nil
}

// StopTaskTree 停止任务树
func (m *Manager) StopTaskTree(ctx context.Context, taskId int64, isLeaf bool) error {
	logger.Log.Infof("[TaskManager] 停止任务树: taskId=%d", taskId)
	parentKey, ok := m.resolveParentKey(taskId, isLeaf)
	if !ok {
		return ErrTaskTreeNotFound
	}
	m.mu.RLock()
	parent, ok := m.parentMap[parentKey]
	m.mu.RUnlock()

	if !ok {
		return ErrTaskTreeNotFound
	}

	for _, child := range parent.GetChildren() {
		state := child.GetState()
		// 跳过已终态子任务：不对已完成的任务重复停止，避免 finished 计数倒退与重复清理
		if state == TaskStateFinished || state == TaskStateFailed || state == TaskStatePartlyFinished {
			continue
		}
		if state == TaskStatePaused {
			// 暂停任务可能已派发 goroutine 但未启动，cancel 确保其退出
			child.cancel()
			child.setFailed("任务被用户停止")
		} else if state == TaskStateWaiting {
			m.removeFromQueue(child.taskId)
			child.cancel()
			child.setFailed("任务被用户停止")
		} else {
			child.Stop()
		}
	}

	// 主动清理任务树：停止后父任务进入终态，从 parentMap/taskMap 移除并通知前端。
	// 不依赖子任务 goroutine 退出时 cleanupFinishedTask 的时序——已 Finished/Paused/Waiting 的
	// 子任务无运行 goroutine 不会触发 cleanupFinishedTask，会导致 parentMap 残留、重启时误判"已在运行"
	m.cleanupStoppedTree(parentKey, parent)

	return nil
}

// StopRunningBySiteWork 停止指定作品关联的运行中任务实例（不删 task 记录）
// 反查 (site_id, site_work_id) 关联任务，转发到 StopTaskTree 复用现有停止逻辑；
// 非运行中的关联任务（不在 taskMap/parentMap）由 StopTaskTree 返回 ErrTaskTreeNotFound，忽略
func (m *Manager) StopRunningBySiteWork(ctx context.Context, siteId int64, siteWorkId string) error {
	tasks, err := m.repo.ListBySiteAndSiteWorkID(ctx, siteId, siteWorkId)
	if err != nil {
		return fmt.Errorf("查询作品关联任务失败: %w", err)
	}
	taskIds := make([]int64, 0, len(tasks))
	for _, t := range tasks {
		taskId := t.GetID()
		taskIds = append(taskIds, taskId)
		// has_child=true 为父任务（isLeaf=false），否则叶子/独立任务（isLeaf=true）
		isLeaf := true
		if t.HasChild.Valid && t.HasChild.Bool {
			isLeaf = false
		}
		if err := m.StopTaskTree(ctx, taskId, isLeaf); err != nil {
			if !errors.Is(err, ErrTaskTreeNotFound) {
				logger.Log.Warnf("停止任务 %d 失败: %v", taskId, err)
			}
		}
	}

	// 清空所有关联任务（含仅存于 DB 的 Paused 任务）的 pending_resource_id：
	// work 的 resource/store 即将被删除，残留的 pending_resource_id 会指向失效的 resource/store，
	// 导致后续恢复时误续传。StopTaskTree 只停内存中的运行实例，DB 中的任务须在此显式清理
	if len(taskIds) > 0 {
		updates := make(map[int64]sql.NullInt64, len(taskIds))
		for _, id := range taskIds {
			updates[id] = sql.NullInt64{Valid: false}
		}
		if err := m.repo.BatchUpdatePendingResourceID(ctx, updates); err != nil {
			logger.Log.Warnf("清理作品关联任务 pending_resource_id 失败: %v", err)
		}
	}

	return nil
}

// RetryTaskTree 重试任务树
func (m *Manager) RetryTaskTree(ctx context.Context, taskId int64, isLeaf bool) error {
	logger.Log.Infof("[TaskManager] 重试任务树: taskId=%d", taskId)
	// 不重置 DB 状态，Finished/Failed/Created 子任务均会重新执行
	return m.startTaskTrees(ctx, []int64{taskId}, runModeFull)
}

// GetTaskTreeState 获取任务树状态
func (m *Manager) GetTaskTreeState(taskId int64, isLeaf bool) (TaskState, error) {
	parentKey, ok := m.resolveParentKey(taskId, isLeaf)
	if !ok {
		return TaskStateCreated, ErrTaskTreeNotFound
	}

	// 优先查 parentMap（父任务场景），回退查 taskMap（独立单任务场景）
	m.mu.RLock()
	parent, inParent := m.parentMap[parentKey]
	mt, inTask := m.taskMap[parentKey]
	m.mu.RUnlock()
	if inParent {
		return parent.GetState(), nil
	}

	if !inTask {
		return TaskStateCreated, ErrTaskTreeNotFound
	}
	return mt.GetState(), nil
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
	return m.deps.Pusher
}

// SetPusher 设置进度推送器（用于 emitter 延迟就绪时替换 Noop）
func (m *Manager) SetPusher(pusher TaskProgressPusher) {
	m.deps.Pusher = pusher
	// 若为快照推送器，保存引用以供 GetTaskSnapshot 使用
	if sp, ok := pusher.(*SnapshotPusher); ok {
		m.snapshotPusher = sp
	} else {
		m.snapshotPusher = nil
	}
}

// GetTaskSnapshot 获取当前所有活跃任务的完整状态快照
func (m *Manager) GetTaskSnapshot() *TaskSnapshotDTO {
	if m.snapshotPusher != nil {
		m.snapshotPusher.EmitSnapshot()
	}
	return m.BuildSnapshot()
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
		task.setState(TaskState(task.task.Status))
		m.cleanupFinishedTask(task)
		task.cancel()
		return nil
	}

	// action == "replace": 跳过重复检查，重新调度
	logger.Log.Infof("[TaskManager] 替换重复任务: taskId=%d", taskId)
	task.skipDuplicateCheck = true
	m.tryDispatch(task)
	return nil
}

// ConfirmReplaceBatch 批量确认替换或跳过重复作品
// 未在等待确认Map中的任务ID会被静默跳过（尽力而为）
// 加锁提取任务后立即返回，调度工作异步执行
func (m *Manager) ConfirmReplaceBatch(taskIds []int64, action string) {
	m.waitingForInputMu.Lock()
	tasks := make([]*ManagedTask, 0, len(taskIds))
	for _, id := range taskIds {
		if task, ok := m.waitingForInputMap[id]; ok {
			delete(m.waitingForInputMap, id)
			tasks = append(tasks, task)
		}
	}
	m.waitingForInputMu.Unlock()

	go func() {
		if action == "skip" {
			logger.Log.Infof("[TaskManager] 批量跳过重复任务: count=%d", len(tasks))
			for _, task := range tasks {
				task.setState(TaskState(task.task.Status))
				m.cleanupFinishedTask(task)
				task.cancel()
			}
		} else {
			logger.Log.Infof("[TaskManager] 批量替换重复任务: count=%d", len(tasks))
			for _, task := range tasks {
				task.skipDuplicateCheck = true
				m.tryDispatch(task)
			}
		}
	}()
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

	// 保证最终刷盘
	defer func() {
		close(m.closeCh)
		<-m.flushDone
	}()

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
			return nil
		}
		select {
		case <-ctx.Done():
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
	// 收集需要从前端 Store 移除的任务 ID（包含当前任务）
	removeIds := []int64{mt.taskId}

	// 从 taskMap 移除子任务
	m.mu.Lock()
	delete(m.taskMap, mt.taskId)

	// 检查父任务是否所有子任务已终态
	if mt.parentId != 0 {
		parent, ok := m.parentMap[mt.parentId]
		if ok && parent.AllChildrenTerminal() {
			// 确保父任务的最终状态被持久化到数据库
			parent.refreshMu.Lock()
			_, newParentState, _, _ := parent.RefreshState()
			if isStableState(newParentState) {
				m.addToPending(parent.taskId, task.TaskStatusEnum(newParentState), "")
			}
			parent.refreshMu.Unlock()

			// 收集仍在 taskMap 中的子任务（暂停中的任务），一并清理
			for _, child := range parent.GetChildren() {
				if _, exists := m.taskMap[child.taskId]; exists {
					removeIds = append(removeIds, child.taskId)
					delete(m.taskMap, child.taskId)
				}
			}

			logger.Log.Infof("[TaskManager] cleanupFinishedTask: 删除 parentMap[%d]（所有子任务终态）", mt.parentId)
			delete(m.parentMap, mt.parentId)
			m.mu.Unlock()
			m.deps.Pusher.PushParentTaskRemove([]int64{mt.parentId})
		} else {
			m.mu.Unlock()
		}
	} else {
		m.mu.Unlock()
	}

	// 通知前端批量移除子任务
	m.deps.Pusher.PushTaskRemove(removeIds)
}

// cleanupStoppedTree 停止任务树后主动清理内存：移除 parentMap 及仍在 taskMap 的子任务，持久化父任务终态。
// 停止后已 Finished/Paused/Waiting 的子任务无运行 goroutine 不会触发 cleanupFinishedTask，须由停止流程主动清理，
// 否则 parentMap 残留会导致重启时 loadAndStartTaskTrees 误判"已在运行"而跳过。
// 对后续退出的 Processing 子任务 goroutine 幂等：其 cleanupFinishedTask 发现 parent 已不在 parentMap 时仅清理自身。
func (m *Manager) cleanupStoppedTree(parentId int64, parent *ParentTask) {
	m.mu.Lock()

	// 持久化父任务最终状态（停止后通常为 Failed 或 PartlyFinished）
	parent.refreshMu.Lock()
	_, newParentState, _, _ := parent.RefreshState()
	if isStableState(newParentState) {
		m.addToPending(parent.taskId, task.TaskStatusEnum(newParentState), "")
	}
	parent.refreshMu.Unlock()

	// 收集并移除仍在 taskMap 的子任务（Paused/Waiting 等无 goroutine 清理的残留）
	removeIds := make([]int64, 0, len(parent.GetChildren()))
	for _, child := range parent.GetChildren() {
		if _, exists := m.taskMap[child.taskId]; exists {
			removeIds = append(removeIds, child.taskId)
			delete(m.taskMap, child.taskId)
		}
	}

	delete(m.parentMap, parentId)
	logger.Log.Infof("[TaskManager] cleanupStoppedTree: 删除 parentMap[%d]（任务树已停止）", parentId)
	m.mu.Unlock()

	if len(removeIds) > 0 {
		m.deps.Pusher.PushTaskRemove(removeIds)
	}
	m.deps.Pusher.PushParentTaskRemove([]int64{parentId})
}

func (m *Manager) getTask(taskId int64) (*ManagedTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	managedTask, ok := m.taskMap[taskId]
	return managedTask, ok
}

// resolveParentKey 根据 isLeaf 标志解析 parentMap 的实际查找键
// 父任务直接使用 taskId；叶子任务和独立单任务通过 taskMap 反查 parentId
func (m *Manager) resolveParentKey(taskId int64, isLeaf bool) (int64, bool) {
	if !isLeaf {
		// 非叶子：检查是否在 parentMap 中（真正的父任务）
		m.mu.RLock()
		_, inParent := m.parentMap[taskId]
		m.mu.RUnlock()
		if inParent {
			return taskId, true
		}
		// 不在 parentMap：可能是独立单任务，当作叶子处理（查 taskMap）
	}
	// 叶子 / 独立任务：从 taskMap 获取 parentId
	m.mu.RLock()
	mt, ok := m.taskMap[taskId]
	m.mu.RUnlock()
	if !ok {
		return 0, false
	}
	if mt.parentId == 0 {
		return taskId, true
	}
	return mt.parentId, true
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

// BuildSnapshot 构建当前所有活跃任务的完整状态快照（基于 taskMap/parentMap 实时状态）
// 实现 SnapshotDataProvider 接口
func (m *Manager) BuildSnapshot() *TaskSnapshotDTO {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := &TaskSnapshotDTO{
		Tasks:       make([]*TaskSnapshotItem, 0, len(m.taskMap)+len(m.waitingForInputMap)),
		ParentTasks: make([]*TaskSnapshotItem, 0, len(m.parentMap)),
	}

	// 收集子任务快照（从 atomic 字段读取进度，并发安全）
	for _, mt := range m.taskMap {
		snapshot.Tasks = append(snapshot.Tasks, &TaskSnapshotItem{
			ID:       mt.taskId,
			TaskName: mt.task.TaskName.String,
			Status:   int(mt.GetState()),
			Total:    mt.progressTotal.Load(),
			Finished: mt.progressFinished.Load(),
		})
	}

	// 收集父任务快照
	for _, pt := range m.parentMap {
		pt.refreshMu.Lock()
		_, state, finished, total := pt.RefreshState()
		pt.refreshMu.Unlock()
		snapshot.ParentTasks = append(snapshot.ParentTasks, &TaskSnapshotItem{
			ID:       pt.taskId,
			TaskName: pt.taskName,
			Status:   int(state),
			Total:    int64(total),
			Finished: int64(finished),
		})
	}

	// 收集等待确认的任务（无进度数据）
	m.waitingForInputMu.Lock()
	for _, mt := range m.waitingForInputMap {
		snapshot.Tasks = append(snapshot.Tasks, &TaskSnapshotItem{
			ID:       mt.taskId,
			TaskName: mt.task.TaskName.String,
			Status:   int(mt.GetState()),
		})
	}
	m.waitingForInputMu.Unlock()

	return snapshot
}

func (m *Manager) newManagedTask(t *domain.Task, mode runMode) *ManagedTask {
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
	mt := NewManagedTask(t.GetID(), parentId, t, pluginExec, m.deps)
	mt.runMode = mode

	// 设置状态变化回调
	taskName := t.TaskName.String
	mt.SetOnStateChange(func(taskId int64, oldState, newState TaskState, errMsg string) {
		// 仅稳定状态写入数据库，瞬态只更新内存和前端
		// A/C 非终态板块不产生任务终态，跳过持久化
		if isStableState(newState) && !mt.isNonTerminalMode() {
			m.addToPending(taskId, task.TaskStatusEnum(newState), errMsg)
		}

		// 推送状态到前端
		m.deps.Pusher.PushStateChange(taskId, taskName, newState)

		// 刷新并持久化父任务状态
		if mt.parentId != 0 {
			// 加读锁读取 parentMap，防止与 cleanupFinishedTask 的 delete 并发读写
			m.mu.RLock()
			parent, ok := m.parentMap[mt.parentId]
			m.mu.RUnlock()
			if ok {
				// 加锁保证 RefreshState（读子任务 + Swap）和推送之间的原子性，
				// 防止并发 goroutine 的过时状态覆盖正确状态
				parent.refreshMu.Lock()
				// 二次检查：在等待 refreshMu 期间，父任务可能已被其他 goroutine 的 cleanupFinishedTask 清理
				m.mu.RLock()
				_, stillExists := m.parentMap[mt.parentId]
				m.mu.RUnlock()
				if stillExists {
					oldParentState, newParentState, finishedCount, total := parent.RefreshState()
					logger.Log.Infof("[TaskManager] 父任务状态刷新: parentId=%d, old=%s, new=%s, finished=%d/%d", parent.taskId, taskStateName(oldParentState), taskStateName(newParentState), finishedCount, total)
					if oldParentState != newParentState && isStableState(newParentState) {
						// 父任务无错误信息，传空字符串（清除 error_message）
						m.addToPending(parent.taskId, task.TaskStatusEnum(newParentState), "")
					}
					m.deps.Pusher.PushParentStateChange(parent.taskId, parent.taskName, newParentState)
					m.deps.Pusher.PushParentProgress(parent.taskId, int64(total), int64(finishedCount))
				}
				parent.refreshMu.Unlock()
			}
		}
	})

	// 设置进度回调（写入待合并 map，由 flushLoop 批量推送；同时更新 atomic 字段供快照使用）
	mt.SetOnProgress(func(taskId int64, total int64, finished int64) {
		// 同步更新 atomic 进度字段（快照模式使用）
		mt.progressTotal.Store(total)
		mt.progressFinished.Store(finished)

		m.pendingMu.Lock()
		m.pendingProgressUpdates[taskId] = &taskScheduleDTO{ID: taskId, Total: total, Finished: finished}
		m.pendingMu.Unlock()
		select {
		case m.flushCh <- struct{}{}:
		default:
		}
	})

	// 设置 pending_resource_id 持久化回调
	mt.onResourceIDUpdate = func(taskId int64, resourceID sql.NullInt64) {
		m.pendingMu.Lock()
		m.pendingResourceIDUpdates[taskId] = resourceID
		m.pendingMu.Unlock()
		select {
		case m.flushCh <- struct{}{}:
		default:
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

// doFlush 将积攒的状态变更和 pendingResourceID 批量写入数据库，以及积攒进度变化推送到前端
func (m *Manager) doFlush() {
	m.pendingMu.Lock()
	if len(m.pendingStatusUpdates) == 0 && len(m.pendingResourceIDUpdates) == 0 && len(m.pendingProgressUpdates) == 0 {
		m.pendingMu.Unlock()
		return
	}
	pendingStatus := m.pendingStatusUpdates
	m.pendingStatusUpdates = make(map[int64]task.StatusUpdate)
	pendingResourceIDs := m.pendingResourceIDUpdates
	m.pendingResourceIDUpdates = make(map[int64]sql.NullInt64)
	pendingProgress := m.pendingProgressUpdates
	m.pendingProgressUpdates = make(map[int64]*taskScheduleDTO)
	m.pendingMu.Unlock()

	// 批量写入任务状态
	if len(pendingStatus) > 0 {
		for id, u := range pendingStatus {
			errMsg := ""
			if u.ErrorMessage.Valid {
				errMsg = u.ErrorMessage.String
			}
			logger.Log.Infof("[TaskManager] doFlush: taskId=%d, status=%d, errMsg=%s", id, u.Status, errMsg)
		}
		if err := m.repo.BatchSetStatus(context.Background(), pendingStatus); err != nil {
			logger.Log.Errorf("[TaskManager] 批量写入任务状态失败: %v", err)
		}
	}

	// 批量写入 pending_resource_id
	if len(pendingResourceIDs) > 0 {
		if err := m.repo.BatchUpdatePendingResourceID(context.Background(), pendingResourceIDs); err != nil {
			logger.Log.Errorf("[TaskManager] 批量写入 pending_resource_id 失败: %v", err)
		}
	}

	// 批量推送下载进度到前端
	if len(pendingProgress) > 0 {
		batch := make([]*taskScheduleDTO, 0, len(pendingProgress))
		for _, dto := range pendingProgress {
			batch = append(batch, dto)
		}
		m.deps.Pusher.PushProgressBatch(batch)
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
