package taskManager

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/shareLock"
	"github.com/library-squirrel/backend/task"
)

// Repository 任务仓储接口（TaskManager 需要的数据库操作——任务核心控制行）
type Repository interface {
	// ListTaskTreeCore 获取任务树核心行列表（只查 task 核心行；树加载路径用，领域数据由执行面策略自取）
	ListTaskTreeCore(ctx context.Context, taskIds []int64, includeStatus ...task.TaskStatusEnum) ([]*domain.Task, error)
	// SetTaskTreeStatus 设置任务树状态
	SetTaskTreeStatus(ctx context.Context, taskIds []int64, status task.TaskStatusEnum, includeStatus ...task.TaskStatusEnum) (int64, error)
	// BatchSetStatus 批量设置任务状态（同时更新 error_message）
	BatchSetStatus(ctx context.Context, statuses map[int64]task.StatusUpdate) error
	// ListBySiteAndSiteWorkID 按 (site_id, site_work_id) 反查关联任务记录（用于作品删除时停止运行中任务）
	ListBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) ([]*domain.Task, error)
	// CountRunningByTreeIds 统计入参任务树内运行态（Processing/Waiting）行数（删除编排「先停后删」
	// 的运行态判定与停止后等待终态的轮询依据）
	CountRunningByTreeIds(ctx context.Context, ids []int64) (int64, error)
}

// WorkTaskProjector 作品任务领域行窄投影（作品任务领域行仓储实现，装配注入）：活跃插件计数先取
// 内存活跃任务集，再经投影批量读领域行按插件身份内存过滤（调用频率=插件停用/换版拦截，低频）
type WorkTaskProjector interface {
	ListByIds(ctx context.Context, ids []int64) (map[int64]*domain.WorkTask, error)
}

// StagingCleaner 任务下载暂存目录清理（task 模块暂存基建提供，装配注入）：work 删除链治理
// 调用——作品资源即删，残留暂存会在任务恢复时误续传已删作品的下载产物
type StagingCleaner interface {
	CleanStagingByTaskIds(ctx context.Context, taskIds []int64) error
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
	pendingStatusUpdates   map[int64]task.StatusUpdate
	pendingProgressUpdates map[int64]*taskScheduleDTO
	pendingMu              sync.Mutex
	flushCh                chan struct{}
	closeCh                chan struct{}
	flushDone              chan struct{}

	// 信号量（控制并发数）
	semaphore   chan struct{}
	maxParallel int

	// 任务类型执行面策略表（task_type → 策略；构造时注入，运行期只读）
	strategies map[string]ExecutionStrategy

	// 作品任务领域行窄投影（活跃插件计数用）
	workTaskProjector WorkTaskProjector
	// 任务下载暂存目录清理（work 删除链治理调用）
	stagingCleaner StagingCleaner

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

// NewManager 创建任务管理器。
// builtinStrategies 任务类型执行面策略表（task_type → 策略；可为 nil=无注册类型）；
// workTaskProjector/stagingCleaner 为作品任务领域行窄访问与暂存治理（可为 nil=活跃插件计数
// 恒零/清理跳过，测试用）
func NewManager(maxParallel int, repo Repository, pusher TaskProgressPusher, deps *TaskDeps, builtinStrategies map[string]ExecutionStrategy, workTaskProjector WorkTaskProjector, stagingCleaner StagingCleaner) *Manager {
	if builtinStrategies == nil {
		builtinStrategies = make(map[string]ExecutionStrategy)
	}
	m := &Manager{
		taskMap:                make(map[int64]*ManagedTask),
		parentMap:              make(map[int64]*ParentTask),
		waitingQueue:           make([]*ManagedTask, 0),
		maxParallel:            maxParallel,
		semaphore:              make(chan struct{}, maxParallel),
		pendingStatusUpdates:   make(map[int64]task.StatusUpdate),
		pendingProgressUpdates: make(map[int64]*taskScheduleDTO),
		flushCh:                make(chan struct{}, 1),
		closeCh:                make(chan struct{}),
		flushDone:              make(chan struct{}),
		repo:                   repo,
		pusher:                 pusher,
		strategies:             builtinStrategies,
		workTaskProjector:      workTaskProjector,
		stagingCleaner:         stagingCleaner,
		deps:                   deps,
		waitingForInputMap:     make(map[int64]*ManagedTask),
	}
	go m.flushLoop()
	return m
}

// StartTaskTrees 批量启动任务(全量执行)
func (m *Manager) StartTaskTrees(ctx context.Context, taskIds []int64) error {
	return m.startTaskTrees(ctx, taskIds)
}

// startTaskTrees 开始执行多个任务树(开始/重试入口;重试=按各任务已记录的执行模式再来一次)
func (m *Manager) startTaskTrees(ctx context.Context, taskIds []int64) error {
	return m.loadAndStartTaskTrees(ctx, taskIds, false)
}

// resumeTaskTrees 恢复执行多个任务树（恢复入口）
// 所有任务执行的唯二入口之一，skipTerminal 跳过已终态子任务;按各任务已记录的执行模式恢复
func (m *Manager) resumeTaskTrees(ctx context.Context, taskIds []int64) error {
	return m.loadAndStartTaskTrees(ctx, taskIds, true)
}

// loadAndStartTaskTrees 从数据库加载多个任务树并启动（只查任务核心行——领域数据由执行面策略
// 按 taskId 自取；板块模式唯一源=作品任务领域行持久化字段，重下载入口先行写行）
// skipTerminal 为 true 时跳过已终态（Finished/Failed/PartlyFinished）的子任务，仅 Resume 使用
// 多根：一次 ListTaskTreeCore 查询，按 DB 真实父子关系构建内存树
func (m *Manager) loadAndStartTaskTrees(ctx context.Context, taskIds []int64, skipTerminal bool) error {
	if len(taskIds) == 0 {
		return nil
	}
	logger.Log.Infof("loadAndStartTaskTrees: taskIds=%v, skipTerminal=%v", taskIds, skipTerminal)

	// 1. 共享一次任务树核心行查询
	tasks, err := m.repo.ListTaskTreeCore(ctx, taskIds)
	if err != nil {
		logger.Log.Errorf("loadAndStartTaskTrees: ListTaskTreeCore 失败: %v", err)
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

	// 2. 重复执行保护改由创建层 claimTask/claimParent 原子保证(取代快照,消除 TOCTOU);
	//    循环内仅做实时 isUnitLoaded 检查以跳过已加载单元。

	// 3. 确定处理单元并去重：独立任务为自身 taskId，叶子/父任务为其 actualParentId
	// 同父多叶子归一为同一单元，跳过已运行的单元
	processedUnits := make(map[int64]struct{})
	// parentUnitId → 需 dispatch 的叶子集合(unitLeaf 触发;nil/空=整树 Start,dispatch 全部子任务)
	leafIdsPerParent := make(map[int64]map[int64]struct{})
	var standaloneChildren []*ManagedTask
	var parentUnits []int64

	for _, taskId := range taskIds {
		rootTask := taskById[taskId]
		if rootTask == nil {
			continue
		}

		kind := classifyTaskUnit(rootTask)
		var unitId int64
		switch kind {
		case unitStandalone:
			unitId = taskId
		case unitLeaf:
			unitId = rootTask.Pid.Int64
		case unitParent:
			unitId = taskId
		}

		// unitLeaf 先累积到所属父单元的叶子集合(须在单元去重与 loaded 跳过之前,保证同父多叶子都记录、
		// 且父已 loaded 时仍累积,供 processParentUnit 重新纳入)
		if kind == unitLeaf {
			if leafIdsPerParent[unitId] == nil {
				leafIdsPerParent[unitId] = make(map[int64]struct{})
			}
			leafIdsPerParent[unitId][taskId] = struct{}{}
		}
		// 重复执行保护:独立/父单元已 loaded 则跳过整树重复加载;unitLeaf 不跳过——
		// 其父已 loaded 时由 processParentUnit 的 !created 分支重新纳入终态叶子(运行中父单元重纳)。
		// 创建层 claimTask/claimParent 保证并发安全,此处 isUnitLoaded 仅作快路径优化。
		if kind != unitLeaf && m.isUnitLoaded(unitId, kind == unitStandalone) {
			logger.Log.Infof("loadAndStartTaskTrees: 单元 %d 已在运行，跳过", unitId)
			continue
		}
		// 同单元去重
		if _, processed := processedUnits[unitId]; processed {
			continue
		}
		processedUnits[unitId] = struct{}{}

		if kind == unitStandalone {
			child, _ := m.buildOrReuseChild(rootTask, skipTerminal)

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

	// 3. 处理各父任务单元，收集需调度的子任务
	allToCheck := make([]*ManagedTask, 0, len(standaloneChildren)+len(parentUnits)*4)
	allToCheck = append(allToCheck, standaloneChildren...)
	for _, parentId := range parentUnits {
		allToCheck = append(allToCheck, m.processParentUnit(tasks, taskById, parentId, skipTerminal, leafIdsPerParent[parentId])...)
	}

	// 4. 分发（受信号量控制；查重在执行面策略内按任务逐个进行——命中冲突经执行内挂起等待用户答复）
	for _, child := range allToCheck {
		m.dispatch(child)
	}

	return nil
}

// processParentUnit 处理一个父任务单元:创建层 claim 父任务后构建其直接子任务（整树加载到 children 供聚合）。
// leafSet 非空时仅返回集合内的子任务(单独 Start 选中叶子:整树加载但只 dispatch 这些叶子,其余兄弟 Created 不 dispatch);
// leafSet 为 nil 时返回全部子任务(整树 Start)。并发开始同一父任务时输者(claim 失败)直接返回 nil;
// 所有子任务已终态时计算父任务最终状态、回退 claim、推送移除,返回 nil。
func (m *Manager) processParentUnit(tasks []*domain.Task, taskById map[int64]*domain.Task, actualParentId int64, skipTerminal bool, leafSet map[int64]struct{}) []*ManagedTask {
	parentTaskName := ""
	if parentEntity := taskById[actualParentId]; parentEntity != nil && parentEntity.TaskName.Valid {
		parentTaskName = parentEntity.TaskName.String
	}

	// 创建层 claim:赢家构建子任务;输者(父单元已在运行)复用 parentTask,把请求的终态叶子重新纳入
	parentTask, created := m.claimParent(actualParentId, parentTaskName)
	if !created {
		// 父单元已在运行:整树 Start(leafSet 空)不重复加载;单独请求叶子(leafSet 非空)重纳终态叶子
		if len(leafSet) == 0 {
			return nil
		}
		return m.reinjectLeaves(parentTask, taskById, leafSet)
	}

	for _, t := range tasks {
		if t.Pid.Valid && t.Pid.Int64 == actualParentId {
			if child, _ := m.buildOrReuseChild(t, skipTerminal); child != nil {
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
		// claim 了但立即终态:回退 claim,避免 parentMap 残留导致重启误判"已在运行"
		m.mu.Lock()
		delete(m.parentMap, actualParentId)
		m.mu.Unlock()
		m.deps.Pusher.PushParentStateChange(actualParentId, parentTaskName, finalState)
		m.deps.Pusher.PushParentTaskRemove([]int64{actualParentId})
		return nil
	}

	// leafSet 非空:整树已加载到 children(供聚合/完成判定),仅 dispatch 集合内的叶子
	if len(leafSet) > 0 {
		toDispatch := make([]*ManagedTask, 0, len(leafSet))
		for _, c := range parentTask.GetChildren() {
			if _, ok := leafSet[c.taskId]; ok {
				toDispatch = append(toDispatch, c)
			}
		}
		return toDispatch
	}
	return parentTask.GetChildren()
}

// reinjectLeaves 把请求的终态叶子重新纳入已运行的父单元(claimParent 输者路径)。
// 仅纳入终态子任务(Finished/Failed,已从 taskMap 清理、actor 已退出):buildOrReuseChild 重建新对象,
// AddChild 覆盖 children 中的旧终态对象,返回者统一交 dispatch 重跑。
// 非终态子任务(Paused/Processing/Waiting 等,仍在 taskMap)一律跳过:
//  1. Paused 的恢复由 ResumeTaskTrees 经 resolveTargets 内存路径直接投 cmdResume,不经本路径;
//  2. 对运行中任务重投开始会重入其查重/板块组合执行,破坏运行中任务;
//  3. 对 Processing/Waiting "开始"属幂等/语义模糊,跳过最安全。
func (m *Manager) reinjectLeaves(parent *ParentTask, taskById map[int64]*domain.Task, leafSet map[int64]struct{}) []*ManagedTask {
	out := make([]*ManagedTask, 0, len(leafSet))
	for leafId := range leafSet {
		t := taskById[leafId]
		if t == nil {
			continue
		}
		if !isTerminalState(TaskState(t.Status)) {
			continue // 非终态不纳入,理由见方法注释
		}
		// 终态已从 taskMap 清理→claimTask 重建新对象;skipTerminal=false 表示用户显式请求、不跳过
		child, _ := m.buildOrReuseChild(t, false)
		if child == nil {
			continue
		}
		parent.AddChild(child) // 覆盖 children 中的旧终态对象(其 actor 已退出、无在途命令)
		out = append(out, child)
	}
	return out
}

// buildOrReuseChild 构建子任务 ManagedTask(创建层 claim 保证对象唯一)。
// skipTerminal 为 true 时跳过已终态(Finished/Failed/PartlyFinished)的子任务。
// 返回的第二个 bool 表示是否为本次创建(输者复用赢家对象时为 false)。
// 恢复任务的续传判定不经此处——由恢复信号（执行进入策略前的实时内存状态==Paused）与作品
// 领域行的 pending 在执行面会合
func (m *Manager) buildOrReuseChild(t *domain.Task, skipTerminal bool) (*ManagedTask, bool) {
	dbState := TaskState(t.Status)

	// 跳过已终态的子任务（仅 Resume 场景需要）
	if skipTerminal && (dbState == TaskStateFinished || dbState == TaskStateFailed || dbState == TaskStatePartlyFinished) {
		return nil, false
	}

	mt, created := m.claimTask(t)
	if mt == nil {
		return nil, false
	}
	return mt, created
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

// dispatch 任务进入执行的入口:首次 dispatch(actorStarted CAS)投 cmdStart 启动执行;
// 已 dispatch 的任务若处于 Paused/Pausing 投 cmdResume(恢复),其他幂等返回 false。
// actor goroutine 由 NewManagedTask 启动,此处只投首条/恢复命令;槽位获取移入 actor 内部。
func (m *Manager) dispatch(task *ManagedTask) bool {
	if !task.actorStarted.CompareAndSwap(false, true) {
		if s := task.GetState(); s == TaskStatePaused || s == TaskStatePausing {
			task.postCmd(taskCmd{kind: cmdResume})
			return true
		}
		return false
	}
	task.postCmd(taskCmd{kind: cmdStart})
	return true
}

// dispatchFromQueue 信号量槽位释放后唤醒等待队列:向队首投 cmdResume,actor 重新竞争槽位(取到则 dequeueSelf 出队,取不到则留在队内)。
func (m *Manager) dispatchFromQueue() {
	m.mu.Lock()
	if len(m.waitingQueue) > 0 {
		task := m.waitingQueue[0]
		m.mu.Unlock()
		logger.Log.Debugf("[TaskManager] dispatchFromQueue 唤醒等待队列任务 %d (队内剩 %d)", task.taskId, len(m.waitingQueue)-1)
		task.postCmd(taskCmd{kind: cmdResume})
		return
	}
	m.mu.Unlock()
}

// enqueueWaitingForInput 把任务放入等待确认 map(策略 WaitReplaceConfirm 进入执行内挂起等待时由其调用)
func (m *Manager) enqueueWaitingForInput(task *ManagedTask) {
	m.waitingForInputMu.Lock()
	m.waitingForInputMap[task.taskId] = task
	m.waitingForInputMu.Unlock()
}

// removeWaitingForInput 从等待确认表移除任务（答复由 ConfirmReplace 移除；确认挂起被
// RunCtx 取消时由等待原语自移除，防后续确认命令误投）
func (m *Manager) removeWaitingForInput(taskId int64) {
	m.waitingForInputMu.Lock()
	delete(m.waitingForInputMap, taskId)
	m.waitingForInputMu.Unlock()
}

// removeFromQueue 从等待队列中移除指定任务(Pause/Stop 第一阶段清队列用)
func (m *Manager) removeFromQueue(taskId int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := make([]*ManagedTask, 0, len(m.waitingQueue))
	for _, t := range m.waitingQueue {
		if t.taskId == taskId {
			continue
		}
		kept = append(kept, t)
	}
	m.waitingQueue = kept
}

// PauseTaskTrees 批量暂停任务:对每个 taskId 的目标(父→整树、叶子→自身、独立→自身)投 cmdPause。
// 整树加载但未 dispatch 的 Created 兄弟不响应(守卫);!ok(不在内存)静默跳过。
func (m *Manager) PauseTaskTrees(ctx context.Context, taskIds []int64) error {
	if len(taskIds) == 0 {
		return nil
	}
	logger.Log.Infof("[TaskManager] 批量暂停任务: taskIds=%v", taskIds)
	// seen 去重:同一 child 可能被多个 taskId 的 resolveTargets 解析出(如父+其子),避免重复投命令
	seen := make(map[int64]struct{})
	for _, taskId := range taskIds {
		targets, _, ok := m.resolveTargets(taskId)
		if !ok {
			continue
		}
		// 取 targets 快照(resolveTargets 已释放 m.mu);投递 cmdPause 不持任何 Manager 锁,防死锁。
		// actor 命令队列保证:即使任务已被投 cmdResume,pause 排在其后最终生效(Paused)。
		for _, child := range targets {
			if _, dup := seen[child.taskId]; dup {
				continue
			}
			seen[child.taskId] = struct{}{}
			// 守卫:未首次 dispatch 的 Created 兄弟(整树加载驻留 children 但未启动)不响应控制命令
			if !child.actorStarted.Load() && child.GetState() == TaskStateCreated {
				continue
			}
			if isTerminalState(child.GetState()) {
				continue
			}
			// Waiting:先出队,避免 dispatchFromQueue 再投 cmdResume(命令队列仍保证 pause 覆盖,出队减少干扰)
			if child.GetState() == TaskStateWaiting {
				m.removeFromQueue(child.taskId)
			}
			// WaitingForInput:从确认 map 移除,避免 ConfirmReplace 再投答复
			if child.GetState() == TaskStateWaitingForInput {
				m.waitingForInputMu.Lock()
				delete(m.waitingForInputMap, child.taskId)
				m.waitingForInputMu.Unlock()
			}
			child.postCmd(taskCmd{kind: cmdPause})
		}
	}
	return nil
}

// ResumeTaskTrees 批量恢复任务:内存命中(Paused/Pausing)投 cmdResume;不在内存的收集后走 resumeTaskTrees(DB 加载)。
// 整树加载但未 dispatch 的 Created 兄弟不响应(守卫);DB 路径错误静默(尽力恢复)。
func (m *Manager) ResumeTaskTrees(ctx context.Context, taskIds []int64) error {
	if len(taskIds) == 0 {
		return nil
	}
	logger.Log.Infof("[TaskManager] 批量恢复任务: taskIds=%v", taskIds)
	seen := make(map[int64]struct{})
	var dbIds []int64
	for _, taskId := range taskIds {
		targets, _, ok := m.resolveTargets(taskId)
		if !ok {
			// 任务不在内存中（如应用重启后），收集走 DB 加载
			dbIds = append(dbIds, taskId)
			continue
		}
		for _, child := range targets {
			if _, dup := seen[child.taskId]; dup {
				continue
			}
			seen[child.taskId] = struct{}{}
			// 守卫:未首次 dispatch 的 Created 兄弟不响应控制命令
			if !child.actorStarted.Load() && child.GetState() == TaskStateCreated {
				continue
			}
			state := child.GetState()
			if state != TaskStatePaused && state != TaskStatePausing {
				continue
			}
			// 投 cmdResume:actor 命令队列记忆,不丢失唤醒。无需 pendingResume 标志。
			child.postCmd(taskCmd{kind: cmdResume})
		}
	}
	if len(dbIds) > 0 {
		logger.Log.Infof("[TaskManager] 恢复任务: %v 不在内存中，从数据库加载", dbIds)
		_ = m.resumeTaskTrees(ctx, dbIds)
	}
	return nil
}

// StopTaskTrees 批量停止任务:对每个 taskId 的目标并行投 cmdStop(带 ack),wg.Wait 后对去重 parent 调 cleanup。
// 整树加载但未 dispatch 的 Created 兄弟不响应(守卫);!ok 静默跳过;独立任务(parent==nil)由 actor 终态自清理。
func (m *Manager) StopTaskTrees(ctx context.Context, taskIds []int64) error {
	if len(taskIds) == 0 {
		return nil
	}
	logger.Log.Infof("[TaskManager] 批量停止任务: taskIds=%v", taskIds)
	seen := make(map[int64]struct{})
	seenParents := make(map[int64]*ParentTask)
	// 并行投 cmdStop(各 actor 独立处理,setFailed 终态);wg.Wait 等全部完成再 cleanup
	var wg sync.WaitGroup
	for _, taskId := range taskIds {
		targets, parent, ok := m.resolveTargets(taskId)
		if !ok {
			continue
		}
		if parent != nil {
			// 同父多 taskId 只 cleanup 一次
			seenParents[parent.taskId] = parent
		}
		for _, child := range targets {
			if _, dup := seen[child.taskId]; dup {
				continue
			}
			seen[child.taskId] = struct{}{}
			// 守卫:未首次 dispatch 的 Created 兄弟不响应控制命令
			if !child.actorStarted.Load() && child.GetState() == TaskStateCreated {
				continue
			}
			state := child.GetState()
			// 跳过已终态子任务：不对已完成的任务重复停止，避免 finished 计数倒退与重复清理
			if state == TaskStateFinished || state == TaskStateFailed || state == TaskStatePartlyFinished {
				continue
			}
			if state == TaskStateWaiting {
				m.removeFromQueue(child.taskId)
			}
			if state == TaskStateWaitingForInput {
				m.waitingForInputMu.Lock()
				delete(m.waitingForInputMap, child.taskId)
				m.waitingForInputMu.Unlock()
			}
			wg.Add(1)
			go func(c *ManagedTask) {
				defer wg.Done()
				ack := make(chan error, 1)
				c.postCmd(taskCmd{kind: cmdStop, ack: ack})
				// 有界等待:命令被丢弃、actor 已退出或应答超时即返回,批量停止不因单任务异常无限等待
				if err := c.waitAck(ack); err != nil {
					logger.Log.Warnf("[TaskManager] 批量停止:任务 %d 停止应答异常: %v", c.taskId, err)
				}
			}(child)
		}
	}
	wg.Wait()

	// 父任务树:主动清理 parentMap + 残留子任务。不依赖子任务 actor 退出时 cleanupFinishedTask 的
	// 时序——已 Finished/Paused/Waiting 的子任务无运行 goroutine 不会触发 cleanupFinishedTask,
	// 会导致 parentMap 残留、重启时误判"已在运行"
	for _, parent := range seenParents {
		m.cleanupStoppedTree(parent.taskId, parent)
	}
	// 独立任务(parent==nil):各 actor 终态退出时由 cleanupFinishedTask 自清理(taskMap 移除+前端通知),
	// 无 parentMap 条目,无需主动清理

	return nil
}

// StopRunningBySiteWork 停止指定作品关联的运行中任务实例（不删 task 记录）
// 反查 (site_id, site_work_id) 关联任务，批量转发到 StopTaskTrees；非运行中的（不在 taskMap/parentMap）静默忽略
func (m *Manager) StopRunningBySiteWork(ctx context.Context, siteId int64, siteWorkId string) error {
	tasks, err := m.repo.ListBySiteAndSiteWorkID(ctx, siteId, siteWorkId)
	if err != nil {
		return fmt.Errorf("查询作品关联任务失败: %w", err)
	}
	taskIds := make([]int64, 0, len(tasks))
	for _, t := range tasks {
		taskIds = append(taskIds, t.GetID())
	}
	if len(taskIds) > 0 {
		_ = m.StopTaskTrees(ctx, taskIds)
	}

	// 清理所有关联任务（含仅存于 DB 的 Paused 任务）的下载暂存目录：
	// work 的 resource/store 即将被删除，残留暂存会在任务恢复时误续传已删作品的下载产物。
	// StopTaskTrees 只停内存中的运行实例，DB 中的任务暂存须在此显式清理
	if len(taskIds) > 0 && m.stagingCleaner != nil {
		if err := m.stagingCleaner.CleanStagingByTaskIds(ctx, taskIds); err != nil {
			logger.Log.Warnf("清理作品关联任务下载暂存目录失败: %v", err)
		}
	}

	return nil
}

// stopWaitPollInterval 停止后等待终态的轮询间隔（终态即时落盘，正常路径首轮或次轮即命中）
const stopWaitPollInterval = 100 * time.Millisecond

// StopAndWaitTerminal 停止任务树并等待全部行离开运行态（task.RunningStopper 实现，任务删除链
// 「先停后删」编排调用）。停止走 StopTaskTrees 立即取消路径（Waiting=队列摘除、Processing=
// 取消执行 ctx，终态即时落盘）；等待按 DB 行运行态计数轮询，timeout 内未清零返回
// ErrStopWaitTimeout（调用方拒绝本次删除，不强行删除——删行但执行继续属不可预期态）。
// 崩溃残留的运行态行（无内存实例可停）恒不清零，同样由超时兜底拒绝
func (m *Manager) StopAndWaitTerminal(ctx context.Context, taskIds []int64, timeout time.Duration) error {
	if len(taskIds) == 0 {
		return nil
	}
	// 等待预算覆盖整体编排（含停止本身的命令应答等待），单次删除的总时延有界
	deadline := time.Now().Add(timeout)
	if err := m.StopTaskTrees(ctx, taskIds); err != nil {
		return err
	}
	for {
		running, err := m.repo.CountRunningByTreeIds(ctx, taskIds)
		if err != nil {
			return fmt.Errorf("查询任务运行态失败: %w", err)
		}
		if running == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrStopWaitTimeout
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(stopWaitPollInterval):
		}
	}
}

// RetryTaskTrees 批量重试任务(保留各任务已记录的执行模式:重试=按原模式再来一次)
func (m *Manager) RetryTaskTrees(ctx context.Context, taskIds []int64) error {
	logger.Log.Infof("[TaskManager] 批量重试任务: taskIds=%v", taskIds)
	// 不重置 DB 状态，Finished/Failed/Created 子任务均会重新执行
	return m.startTaskTrees(ctx, taskIds)
}

// GetTaskTreeState 获取任务状态:父任务返回聚合状态、叶子/独立任务返回自身状态
func (m *Manager) GetTaskTreeState(taskId int64) (TaskState, error) {
	targets, parent, ok := m.resolveTargets(taskId)
	if !ok {
		return TaskStateCreated, ErrTaskTreeNotFound
	}
	if parent != nil {
		// 父任务树:返回聚合的父任务状态
		return parent.GetState(), nil
	}
	// 叶子/独立任务:返回自身状态
	return targets[0].GetState(), nil
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

// CountActiveByPlugin 统计插件名下运行中的任务数。运行中=Processing/Pausing/Stopping/
// WaitingForInput（在途执行、执行中收敛或执行中待用户确认——插件停用会打断其插件交互）；
// Created/Waiting/Paused/终态不计（未启动与已暂停的任务不阻塞插件停用，暂停任务随停用
// 的存续由用户自行处置）。插件身份在作品任务领域行——先取内存活跃任务集，再经窄投影批量读
// 领域行按插件身份内存过滤（调用频率=插件停用/换版拦截，低频）
func (m *Manager) CountActiveByPlugin(pluginPublicId string) int {
	m.mu.RLock()
	ids := make([]int64, 0, len(m.taskMap))
	for id, mt := range m.taskMap {
		switch mt.GetState() {
		case TaskStateProcessing, TaskStatePausing, TaskStateStopping, TaskStateWaitingForInput:
			ids = append(ids, id)
		}
	}
	m.mu.RUnlock()

	if m.workTaskProjector == nil || len(ids) == 0 {
		return 0
	}
	rows, err := m.workTaskProjector.ListByIds(context.Background(), ids)
	if err != nil {
		logger.Log.Warnf("[TaskManager] 活跃插件计数投影作品任务领域行失败: %v", err)
		return 0
	}
	count := 0
	for _, id := range ids {
		wt := rows[id]
		if wt != nil && wt.PluginPublicID.Valid && wt.PluginPublicID.String == pluginPublicId {
			count++
		}
	}
	return count
}

// IsKnownTaskType 任务类型是否已注册：执行面策略表成员查询。
// 实现 task.TaskTypeRegistry（app.go 装配注入 task.Service，供创建路径成员校验）
func (m *Manager) IsKnownTaskType(taskType string) bool {
	return m.strategies[taskType] != nil
}

// ConfirmReplace 用户确认替换或跳过重复作品。
// 任务阻塞在执行内的确认等待（actor 阻塞在 Execute 内，cmdCh 无人消费），直接向确认通道
// 投递答复，由 WaitReplaceConfirm 唤醒继续执行。
// replace 答复在投递前同步预检涉及作品的分享拉取锁（checkConfirmWorkLocks）：命中不摘确认
// 条目、不投递，返回 shareLock.ErrWorkLocked 直达前端（前端弹强制解锁确认，用户知情解锁后
// 重发本答复即放行）；skip 答复不动作品资源，不查锁
func (m *Manager) ConfirmReplace(taskId int64, action string) error {
	m.waitingForInputMu.Lock()
	task, ok := m.waitingForInputMap[taskId]
	if !ok {
		m.waitingForInputMu.Unlock()
		return ErrTaskTreeNotFound
	}
	if action != "skip" {
		if err := m.checkConfirmWorkLocks(task); err != nil {
			m.waitingForInputMu.Unlock()
			return err
		}
	}
	delete(m.waitingForInputMap, taskId)
	m.waitingForInputMu.Unlock()
	task.confirmConflictWorkIds = nil

	logger.Log.Infof("[TaskManager] 确认替换任务: taskId=%d, action=%s", taskId, action)
	task.confirmCh <- replaceConfirmResult{decision: confirmDecision(action)}
	return nil
}

// ConfirmReplaceBatch 批量确认替换或跳过重复作品
// 未在等待确认Map中的任务ID会被静默跳过（尽力而为）。replace 答复投递前同步预检全部涉及作品
// 的分享拉取锁：任一命中即整体不投递（任务全部留在等待确认表，用户解锁后重发本答复），
// 返回 shareLock.ErrWorkLocked；skip 答复不查锁。加锁提取任务后逐个投确认通道，各 actor 独立处理
func (m *Manager) ConfirmReplaceBatch(taskIds []int64, action string) error {
	m.waitingForInputMu.Lock()
	tasks := make([]*ManagedTask, 0, len(taskIds))
	for _, id := range taskIds {
		if task, ok := m.waitingForInputMap[id]; ok {
			tasks = append(tasks, task)
		}
	}
	if action != "skip" {
		for _, task := range tasks {
			if err := m.checkConfirmWorkLocks(task); err != nil {
				m.waitingForInputMu.Unlock()
				return err
			}
		}
	}
	for _, task := range tasks {
		delete(m.waitingForInputMap, task.taskId)
	}
	m.waitingForInputMu.Unlock()

	logger.Log.Infof("[TaskManager] 批量确认替换: count=%d, action=%s", len(tasks), action)
	for _, task := range tasks {
		task.confirmConflictWorkIds = nil
		task.confirmCh <- replaceConfirmResult{decision: confirmDecision(action)}
	}
	return nil
}

// checkConfirmWorkLocks 替换答复投递前置作品锁预检：涉及作品=任务等待确认时记录的冲突作品集合
// （WaitReplaceConfirm 进入等待时记录），任一被分享拉取持有即返回 shareLock.ErrWorkLocked
// （确认替换的执行会软删其活行 store 文件，在途拉取会读到源文件消失）
func (m *Manager) checkConfirmWorkLocks(task *ManagedTask) error {
	for _, workId := range task.confirmConflictWorkIds {
		if m.deps.WorkLockChecker.IsLocked(context.Background(), workId) {
			logger.Log.Infof("[TaskManager] 替换确认被作品锁拒绝: taskId=%d 涉及作品 %d 正被分享拉取持有", task.taskId, workId)
			return shareLock.ErrWorkLocked
		}
	}
	return nil
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

	// 等待用户确认的任务：全部阻塞在执行内的确认等待（actor 阻塞在 Execute 内），取消其 ctx
	// 使 WaitReplaceConfirm 返回取消、Execute 退出、actor 收尾——DB 行停留执行前值、不落库，
	// 重启后按执行前状态给操作入口。在释放等待确认表锁后取消（等待原语的取消分支会取同一把锁自移除）
	m.waitingForInputMu.Lock()
	waiting := make([]*ManagedTask, 0, len(m.waitingForInputMap))
	for _, t := range m.waitingForInputMap {
		waiting = append(waiting, t)
	}
	m.waitingForInputMap = make(map[int64]*ManagedTask)
	m.waitingForInputMu.Unlock()
	for _, t := range waiting {
		t.cancel()
	}

	// 并行暂停所有 Processing 任务:暂停发起与应答等待按任务并行,单任务的中断通知等待
	// (执行面 Pause RPC 转发,至多中断通知上界)不叠加进整体关闭耗时
	var pauseWg sync.WaitGroup
	for _, t := range tasks {
		if t.GetState() == TaskStateProcessing {
			pauseWg.Add(1)
			go func(t *ManagedTask) {
				defer pauseWg.Done()
				if err := t.Pause(); err != nil {
					logger.Log.Warnf("[TaskManager] 优雅关闭：暂停任务 %d 失败: %v", t.taskId, err)
				}
			}(t)
		}
	}
	pauseWg.Wait()

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

// claimTask 原子 insert-or-get 子任务(创建层守卫):同一 taskId 在 taskMap 中只存在一个 ManagedTask。
// 并发开始同一任务时,赢家创建+注册,输者复用赢家的对象(其本轮 newManagedTask 产物被丢弃)。
// newManagedTask 在锁外执行,用 double-check 保证唯一插入。
func (m *Manager) claimTask(t *domain.Task) (mt *ManagedTask, created bool) {
	m.mu.Lock()
	if existing, ok := m.taskMap[t.GetID()]; ok {
		m.mu.Unlock()
		return existing, false
	}
	m.mu.Unlock()

	nm := m.newManagedTask(t)
	if nm == nil {
		return nil, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.taskMap[t.GetID()]; ok {
		return existing, false // 输者复用赢家
	}
	m.taskMap[nm.taskId] = nm
	return nm, true
}

// claimParent 原子 insert-or-get 父任务(创建层守卫):NewParentTask 仅结构初始化,可在锁内完成
func (m *Manager) claimParent(id int64, name string) (*ParentTask, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.parentMap[id]; ok {
		return existing, false
	}
	p := NewParentTask(id, name)
	m.parentMap[id] = p
	return p, true
}

// isUnitLoaded 实时检查单元是否已加载(独立任务查 taskMap,父任务查 parentMap)。
// 仅作 loadAndStartTaskTrees 的快路径优化;并发安全最终由 claimTask/claimParent 保证。
func (m *Manager) isUnitLoaded(unitId int64, isStandalone bool) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if isStandalone {
		_, ok := m.taskMap[unitId]
		return ok
	}
	_, ok := m.parentMap[unitId]
	return ok
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
			// 未首次 dispatch 的兄弟 actor 空转阻塞 cmdCh,须 cancel 退出避免 goroutine 泄漏
			if !child.actorStarted.Load() {
				child.cancel()
			}
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

// taskUnitKind 任务单元类型:Start 加载层与控制操作(Pause/Stop/Resume)共用的分类语义
type taskUnitKind int

const (
	unitStandalone taskUnitKind = iota // 独立任务:Pid==0 且无子(单子折叠产物)
	unitLeaf                           // 叶子任务:Pid>0 且无子
	unitParent                         // 父任务:有子(集合任务)
)

// classifyTaskUnit 据任务的 Pid/HasChild 判定单元类型(单一分类来源)。
// Start 据此决定如何构建(taskMap/parentMap),控制操作经 resolveTargets 据内存状态解析目标,
// 二者共享同一套"独立/叶子/父"语义,避免判定分叉(曾致独立任务无法暂停/停止的回归)。
// 不变量:改此分类须保独立任务(pid=0、HasChild=false)与叶子(pid>0)均被正确归类,
// 否则执行/控制操作会漏掉独立 leaf——参见 memory leaf-task-regression-hotspot / standalone-task-pause-regression。
func classifyTaskUnit(t *domain.Task) taskUnitKind {
	if t.HasChild.Valid && t.HasChild.Bool {
		return unitParent
	}
	if t.Pid.Valid && t.Pid.Int64 > 0 {
		return unitLeaf
	}
	return unitStandalone
}

// resolveTargets 解析控制操作(Pause/Stop/Resume)的目标子任务集合。
// 返回目标切片、所属父任务(独立任务与叶子为 nil)、是否命中。父任务返回其全部子任务(整树操作);
// 叶子返回自身(操作叶子只作用于该叶子,不扩散兄弟);独立任务返回自身。单次 RLock 读取,返回的对象指针
// 在锁外迭代安全(对象自身线程安全:GetState/postCmd 各有同步)。目标范围据内存状态派生,无需 isLeaf。
func (m *Manager) resolveTargets(taskId int64) (targets []*ManagedTask, parent *ParentTask, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 父任务本身:返回其全部子任务(整树操作)
	if p, hit := m.parentMap[taskId]; hit {
		return p.GetChildren(), p, true
	}
	// 叶子/独立任务
	mt, hit := m.taskMap[taskId]
	if !hit {
		return nil, nil, false
	}
	// 叶子(parentId>0)与独立任务(parentId==0)均返回自身;parent=nil 使 Stop 不触发整树 cleanup
	return []*ManagedTask{mt}, nil, true
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

// newManagedTask 构建托管任务并绑定执行面策略：按 task_type 查注册的策略表，
// 空类型/未注册类型拒启（创建路径已显式写类型，空值属数据异常）
func (m *Manager) newManagedTask(t *domain.Task) *ManagedTask {
	taskType := t.TaskType.String
	if !t.TaskType.Valid || taskType == "" {
		logger.Log.Errorf("获取任务执行面失败: 任务 %d 的 task_type 为空（创建路径未写显式类型）", t.GetID())
		return nil
	}
	strategy, ok := m.strategies[taskType]
	if !ok {
		logger.Log.Errorf("获取任务执行面失败: task_type %q 未注册策略", taskType)
		return nil
	}

	parentId := int64(0)
	if t.Pid.Valid {
		parentId = t.Pid.Int64
	}
	mt := NewManagedTask(t.GetID(), parentId, t, m.deps, m, m.semaphore)
	mt.strategy = strategy

	// 设置状态变化回调
	taskName := t.TaskName.String
	mt.SetOnStateChange(func(taskId int64, oldState, newState TaskState, errMsg string) {
		// 仅稳定状态写入数据库，瞬态只更新内存和前端（跳过收口回执行前状态，非稳定态天然不落盘）
		if isStableState(newState) {
			m.addToPending(taskId, task.TaskStatusEnum(newState), errMsg)
		}

		// 推送状态到前端
		m.deps.Pusher.PushStateChange(taskId, taskName, newState)

		// 刷新并持久化父任务状态
		if mt.parentId != 0 {
			// 持 m.mu(RLock) 跨 refreshMu,统一锁顺序为 m.mu → refreshMu(与 cleanupFinishedTask/cleanupStoppedTree/BuildSnapshot 一致)。
			// 旧实现先释放 m.mu 再 refreshMu.Lock→m.mu.RLock,与 cleanupFinishedTask(m.mu.Lock→refreshMu.Lock) 形成锁顺序死锁:
			// 并发完成时 cleanupFinishedTask 阻塞 → executeTask 不退出 → 信号量槽泄漏 → 并行度逐渐下降到 1。
			// 持 RLock 期间 cleanupFinishedTask 无法删 parentMap(需 m.mu Lock),故 ok 判定稳定,无需二次检查。
			m.mu.RLock()
			parent, ok := m.parentMap[mt.parentId]
			if ok {
				parent.refreshMu.Lock()
				oldParentState, newParentState, finishedCount, total := parent.RefreshState()
				logger.Log.Infof("[TaskManager] 父任务状态刷新: parentId=%d, old=%s, new=%s, finished=%d/%d", parent.taskId, taskStateName(oldParentState), taskStateName(newParentState), finishedCount, total)
				if oldParentState != newParentState && isStableState(newParentState) {
					// 父任务无错误信息，传空字符串（清除 error_message）
					m.addToPending(parent.taskId, task.TaskStatusEnum(newParentState), "")
				}
				m.deps.Pusher.PushParentStateChange(parent.taskId, parent.taskName, newParentState)
				m.deps.Pusher.PushParentProgress(parent.taskId, int64(total), int64(finishedCount))
				parent.refreshMu.Unlock()
			}
			m.mu.RUnlock()
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

// doFlush 将积攒的任务状态变更批量写入数据库，以及积攒进度变化推送到前端
func (m *Manager) doFlush() {
	m.pendingMu.Lock()
	if len(m.pendingStatusUpdates) == 0 && len(m.pendingProgressUpdates) == 0 {
		m.pendingMu.Unlock()
		return
	}
	pendingStatus := m.pendingStatusUpdates
	m.pendingStatusUpdates = make(map[int64]task.StatusUpdate)
	// 状态写库在 pendingMu 内完成,与 addToPending 终态即时写互斥:
	// 终态即时写已把该任务从批量通道移除并即时落盘,此处取出的快照不含终态,回写不会覆盖终态
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
	pendingProgress := m.pendingProgressUpdates
	m.pendingProgressUpdates = make(map[int64]*taskScheduleDTO)
	m.pendingMu.Unlock()

	// 批量推送下载进度到前端
	if len(pendingProgress) > 0 {
		batch := make([]*taskScheduleDTO, 0, len(pendingProgress))
		for _, dto := range pendingProgress {
			batch = append(batch, dto)
		}
		m.deps.Pusher.PushProgressBatch(batch)
	}
}

// addToPending 添加状态变更:终态即时落盘,非终态进批量通道由 flushLoop 合并刷库
func (m *Manager) addToPending(taskId int64, status task.TaskStatusEnum, errMsg string) {
	if isImmediateTerminal(TaskState(status)) {
		// 终态即时落盘:同步写库,不进批量通道,进程崩溃也不丢失终态
		m.pendingMu.Lock()
		// 该任务可能在终态前已把 Paused 等非终态写进批量通道;终态即时写后,
		// 残留的非终态快照会被随后的 doFlush 回写覆盖终态,故先从批量通道移除
		delete(m.pendingStatusUpdates, taskId)
		if err := m.repo.BatchSetStatus(context.Background(), map[int64]task.StatusUpdate{
			taskId: {Status: status, ErrorMessage: sql.NullString{String: errMsg, Valid: errMsg != ""}},
		}); err != nil {
			logger.Log.Errorf("[TaskManager] 即时写入任务 %d 终态 %d 失败: %v", taskId, status, err)
		}
		m.pendingMu.Unlock()
		return
	}

	// 非终态(Paused):进批量通道,由 flushLoop 合并刷库
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

// isImmediateTerminal 是否为应即时落盘的终态(Finished/Failed/PartlyFinished);Paused 保留批量通道以支持续传。
// 执行模式(StoreRoles/IncludeWorkInfo)在终态不清空——保留供重试按原模式再来一次;
// 后续全量开始/重下经板块选择写行能力覆盖，不泄漏
func isImmediateTerminal(s TaskState) bool {
	return s == TaskStateFinished || s == TaskStateFailed || s == TaskStatePartlyFinished
}
