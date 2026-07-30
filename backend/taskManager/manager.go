package taskManager

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
	// BatchSetStatus 批量设置任务状态（同时更新 error_message）
	BatchSetStatus(ctx context.Context, statuses map[int64]task.StatusUpdate) error
	// BatchUpdatePendingResourceID 批量更新任务的 pending_resource_id
	BatchUpdatePendingResourceID(ctx context.Context, updates map[int64]sql.NullInt64) error
	// UpdateRedownloadSections 批量更新任务的板块重执行选择(store_roles + include_work_info)
	UpdateRedownloadSections(ctx context.Context, taskIds []int64, storeRoles sql.NullString, includeWorkInfo bool) error
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

// StartTaskTrees 批量启动任务(全量执行)
func (m *Manager) StartTaskTrees(ctx context.Context, taskIds []int64) error {
	mode := runModeFull
	return m.startTaskTrees(ctx, taskIds, &mode)
}

// startTaskTrees 开始执行多个任务树(开始/重试入口)
// recordMode 非 nil(开始)时把执行模式记录到任务树全部成员;nil(重试)时保留各任务已记录的模式
func (m *Manager) startTaskTrees(ctx context.Context, taskIds []int64, recordMode *runMode) error {
	return m.loadAndStartTaskTrees(ctx, taskIds, false, recordMode)
}

// Redownload 板块重执行:把所选板块记录到任务树全部成员(父+子)后启动
// 记录到全部成员保证每个子任务都持有该模式(供执行派生与单独续传读取),而非仅父任务持有
func (m *Manager) Redownload(ctx context.Context, taskIds []int64, storeRoles []string, includeWorkInfo bool) error {
	mode := runMode{workInfo: includeWorkInfo, storeRoles: storeRoles}
	return m.loadAndStartTaskTrees(ctx, taskIds, false, &mode)
}

// resumeTaskTrees 恢复执行多个任务树（恢复入口）
// 所有任务执行的唯二入口之一，skipTerminal 跳过已终态子任务;按各任务已记录的执行模式恢复(不重新记录)
func (m *Manager) resumeTaskTrees(ctx context.Context, taskIds []int64) error {
	return m.loadAndStartTaskTrees(ctx, taskIds, true, nil)
}

// loadAndStartTaskTrees 从数据库加载多个任务树并启动
// skipTerminal 为 true 时跳过已终态（Finished/Failed/PartlyFinished）的子任务，仅 Resume 使用
// recordMode 非 nil(开始/重下)时把执行模式记录到任务树全部成员,使各任务 runModeFromTask 派生一致并支持单独续传
// 多根：一次 ListTaskTree 查询 + 一次批量查重，按 DB 真实父子关系构建内存树
func (m *Manager) loadAndStartTaskTrees(ctx context.Context, taskIds []int64, skipTerminal bool, recordMode *runMode) error {
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

	// recordMode 非 nil(开始/重下):为各任务设置执行模式内存字段,供 runModeFromTask 派生执行。
	// 开始(StartTaskTree)记录空 selection(派生时回退 universe)+workInfo=true;重下(Redownload)记录用户所选子集。
	// 持久化推迟到确定调度范围后按"实际纳入调度的任务"写入(决策1·B):整树 Start 写全部子任务,
	// 运行中父单元重纳(reinject)只写请求的终态叶子,不波及未被纳入的运行中兄弟。
	var modeStoreRoles sql.NullString
	if recordMode != nil {
		// Start(runModeFull,storeRoles=nil)→重置 StoreRoles 为 NULL,执行期由 universe 派生(支持默认插件下全量)
		// Redownload(storeRoles 非 nil,可空)→记录所选;空=仅作品信息(不拉资源)
		if recordMode.storeRoles == nil {
			modeStoreRoles = sql.NullString{Valid: false}
		} else {
			modeStoreRoles = sql.NullString{String: strings.Join(recordMode.storeRoles, ","), Valid: true}
		}
		// 内存字段对全部 tasks 设置:被构建的任务据此派生 runMode,未被构建的任务(如 reinject 跳过的兄弟)不被读取、无害
		for _, t := range tasks {
			t.StoreRoles = modeStoreRoles
			t.IncludeWorkInfo = recordMode.workInfo
		}
	}

	taskById := make(map[int64]*domain.Task, len(tasks))
	for _, t := range tasks {
		taskById[t.ID] = t
	}

	// 2. 重复执行保护改由创建层 claimTask/claimParent 原子保证(取代快照,消除 TOCTOU);
	//    循环内仅做实时 isUnitLoaded 检查以跳过已加载单元,避免重复查重工作。

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

	// 4. 处理各父任务单元，收集需调度的子任务
	allToCheck := make([]*ManagedTask, 0, len(standaloneChildren)+len(parentUnits)*4)
	allToCheck = append(allToCheck, standaloneChildren...)
	for _, parentId := range parentUnits {
		allToCheck = append(allToCheck, m.processParentUnit(tasks, taskById, parentId, skipTerminal, leafIdsPerParent[parentId])...)
	}

	// 5. recordMode 持久化:按实际纳入调度的任务范围写入(决策1·B)。整树 Start 时 allToCheck 含全部子任务;
	// 运行中父单元重纳(reinject)时 allToCheck 仅含请求的终态叶子,不波及未被纳入的运行中兄弟。
	// allToCheck 含等待确认(WaitingForInput)的任务,其模式亦需持久化。
	if recordMode != nil && len(allToCheck) > 0 {
		ids := make([]int64, 0, len(allToCheck))
		for _, child := range allToCheck {
			ids = append(ids, child.taskId)
		}
		if err := m.repo.UpdateRedownloadSections(ctx, ids, modeStoreRoles, recordMode.workInfo); err != nil {
			logger.Log.Errorf("loadAndStartTaskTrees: 记录执行模式失败: %v", err)
			// 不阻断:runMode 已按内存记录值派生
		}
	}

	// 6. 共享一次批量预检重复 + 分发（受信号量控制）
	if len(allToCheck) > 0 {
		toDispatch := m.batchCheckDuplicates(ctx, allToCheck)
		for _, child := range toDispatch {
			m.dispatch(child)
		}
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
// AddChild 覆盖 children 中的旧终态对象,返回者统一交 batchCheckDuplicates + dispatch 重跑。
// 非终态子任务(Paused/Processing/Waiting 等,仍在 taskMap)一律跳过:
//  1. Paused 的恢复由 ResumeTaskTrees 经 resolveTargets 内存路径直接投 cmdResume,不经本路径;
//  2. 把它们送入 batchCheckDuplicates 会因自身作品命中→误置 WaitingForInput,破坏运行中任务;
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
// 若任务在数据库中为 Paused 状态(应用重启后内存已丢失),根据 pending_resource_id 决定续传或重新执行。
// skipTerminal 为 true 时跳过已终态(Finished/Failed/PartlyFinished)的子任务。
// 返回的第二个 bool 表示是否为本次创建(输者复用赢家对象时为 false)。
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
	// 仅赢家设置跨重启续传标记;输者复用赢家的对象,其 resumeFromDB 已由赢家设定
	if created {
		if dbState == TaskStatePaused && t.PendingResourceID.Valid {
			logger.Log.Infof("StartTaskTree: 子任务 %d 跨重启续传，pendingResourceID=%d", t.GetID(), t.PendingResourceID.Int64)
			mt.resumeFromDB = true
		} else if dbState == TaskStatePaused {
			// Paused 但无 pending_resource_id（setup 阶段暂停或旧数据），从头执行
			logger.Log.Warnf("StartTaskTree: 子任务 %d 在数据库中为 Paused 状态但无 pending_resource_id，将从头部重新执行", t.GetID())
		}
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

// batchCheckDuplicates 在任务派发前批量检查重复
// 将重复任务直接放入等待确认队列（不消耗信号量），非重复任务标记 skipDuplicateCheck 并返回以供派发
func (m *Manager) batchCheckDuplicates(ctx context.Context, children []*ManagedTask) []*ManagedTask {
	var toCheck []*ManagedTask
	for _, child := range children {
		// 跳过跨重启续传任务（直接走 resumeFromPersistedState）
		if child.resumeFromDB {
			continue
		}
		// 不含主资源的板块组合不查重（不覆盖资源文件，无需"作品已存在"提醒）
		if !child.runMode.hasStore(domain.StoreTypeImage) {
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

		// 不含主资源的板块组合：已标记 skipDuplicateCheck，直接派发（不参与查重命中判断）
		if !child.runMode.hasStore(domain.StoreTypeImage) {
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
		task.postCmd(taskCmd{kind: cmdResume})
		return
	}
	m.mu.Unlock()
}

// enqueueWaitingForInput 把任务放入等待确认 map(查重命中 runResultNeedConfirm 时由 actor 调用)
func (m *Manager) enqueueWaitingForInput(task *ManagedTask) {
	m.waitingForInputMu.Lock()
	m.waitingForInputMap[task.taskId] = task
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
			// WaitingForInput:从确认 map 移除,避免 ConfirmReplace 再投 cmdConfirmReplace
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
				<-ack
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

	// 清空所有关联任务（含仅存于 DB 的 Paused 任务）的 pending_resource_id：
	// work 的 resource/store 即将被删除，残留的 pending_resource_id 会指向失效的 resource/store，
	// 导致后续恢复时误续传。StopTaskTrees 只停内存中的运行实例，DB 中的任务须在此显式清理
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

// RetryTaskTrees 批量重试任务(保留各任务已记录的执行模式:重试=按原模式再来一次)
func (m *Manager) RetryTaskTrees(ctx context.Context, taskIds []int64) error {
	logger.Log.Infof("[TaskManager] 批量重试任务: taskIds=%v", taskIds)
	// 不重置 DB 状态，Finished/Failed/Created 子任务均会重新执行
	return m.startTaskTrees(ctx, taskIds, nil)
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

	// 投 cmdConfirmReplace:actor 处理(skip → 回执行前状态+清理+退出;replace → 设 skipDuplicateCheck + run)
	logger.Log.Infof("[TaskManager] 确认替换任务: taskId=%d, action=%s", taskId, action)
	task.postCmd(taskCmd{kind: cmdConfirmReplace, skipDup: action != "skip", skip: action == "skip"})
	return nil
}

// ConfirmReplaceBatch 批量确认替换或跳过重复作品
// 未在等待确认Map中的任务ID会被静默跳过（尽力而为）
// 加锁提取任务后投递命令,各 actor 独立处理
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

	logger.Log.Infof("[TaskManager] 批量确认替换: count=%d, action=%s", len(tasks), action)
	for _, task := range tasks {
		task.postCmd(taskCmd{kind: cmdConfirmReplace, skipDup: action != "skip", skip: action == "skip"})
	}
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
		// 关闭时未确认的任务视为本次跳过(skipped),回退 Created 使父聚合判定一致
		t.skipped = true
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

// claimTask 原子 insert-or-get 子任务(创建层守卫):同一 taskId 在 taskMap 中只存在一个 ManagedTask。
// 并发开始同一任务时,赢家创建+注册,输者复用赢家的对象(其本轮 newManagedTask 产物被丢弃)。
// newManagedTask 在锁外执行(避免持 m.mu 跑 pluginExecFactory),用 double-check 保证唯一插入。
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

func (m *Manager) getTask(taskId int64) (*ManagedTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	managedTask, ok := m.taskMap[taskId]
	return managedTask, ok
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
	mt := NewManagedTask(t.GetID(), parentId, t, pluginExec, m.deps, m, m.semaphore)
	mt.runMode = runModeFromTask(t)

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
	pendingResourceIDs := m.pendingResourceIDUpdates
	m.pendingResourceIDUpdates = make(map[int64]sql.NullInt64)
	pendingProgress := m.pendingProgressUpdates
	m.pendingProgressUpdates = make(map[int64]*taskScheduleDTO)
	m.pendingMu.Unlock()

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

// addToPending 添加状态变更:终态即时落盘,非终态进批量通道由 flushLoop 合并刷库
func (m *Manager) addToPending(taskId int64, status task.TaskStatusEnum, errMsg string) {
	if isClearableTerminal(TaskState(status)) {
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

		// 终态清空执行模式持久化。StoreRoles/IncludeWorkInfo 仅为在途任务(暂停→跨重启续传)服务,
		// 任务完成后保留无意义,且会泄漏到下次执行(如 Redownload 子集残留导致后续全量开始时该任务仍跑子集)
		if err := m.repo.UpdateRedownloadSections(context.Background(), []int64{taskId}, sql.NullString{}, false); err != nil {
			logger.Log.Warnf("[TaskManager] 清空任务 %d 终态执行模式失败: %v", taskId, err)
		}
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

// isClearableTerminal 是否为应清空执行模式的终态(Finished/Failed/PartlyFinished);Paused 保留以支持续传
func isClearableTerminal(s TaskState) bool {
	return s == TaskStateFinished || s == TaskStateFailed || s == TaskStatePartlyFinished
}
