package taskManager

// 任务运行控制面：per-task actor 模型（命令通道 + 串行状态机）、信号量槽位、信号控制
// （可排空阶段上报/软暂停广播/中断通知）、策略主体执行与终态/进度上报桥接、父任务聚合。
// 任务主体的执行逻辑（板块组合/下载/续传等）在执行面策略（ExecutionStrategy）。

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/resource"
)

// TaskState 任务状态
type TaskState int32

const (
	TaskStateCreated         TaskState = iota // 0: 已创建（未启动）
	TaskStateWaiting                          // 1: 等待中（排队中）
	TaskStateProcessing                       // 2: 处理中
	TaskStatePausing                          // 3: 暂停中
	TaskStatePaused                           // 4: 已暂停
	TaskStateStopping                         // 5: 停止中
	TaskStateFinished                         // 6: 已完成
	TaskStateFailed                           // 7: 失败
	TaskStatePartlyFinished                   // 8: 部分完成（父任务专用）
	TaskStateWaitingForInput                  // 9: 等待用户确认（瞬态，不持久化）
)

// runResult runStrategy 的返回值类型
type runResult int

const (
	runResultDone    runResult = iota // 终态完成（Finish/Fail 上报后的返回）
	runResultPaused                   // RunCtx 取消中断（用户暂停/停止），交控制面接管
	runResultSkipped                  // 跳过收口（Skip 上报：回执行前状态、不产生终态，处置已在 Skip 内完成）
)

// taskStateName 任务状态名称映射
func taskStateName(s TaskState) string {
	switch s {
	case TaskStateCreated:
		return "Created"
	case TaskStateWaiting:
		return "Waiting"
	case TaskStateProcessing:
		return "Processing"
	case TaskStatePausing:
		return "Pausing"
	case TaskStatePaused:
		return "Paused"
	case TaskStateStopping:
		return "Stopping"
	case TaskStateFinished:
		return "Finished"
	case TaskStateFailed:
		return "Failed"
	case TaskStatePartlyFinished:
		return "PartlyFinished"
	case TaskStateWaitingForInput:
		return "WaitingForInput"
	default:
		return "Unknown"
	}
}

// isStableState 判断任务状态是否为稳定状态（需要持久化到数据库）
func isStableState(state TaskState) bool {
	switch state {
	case TaskStatePaused, TaskStateFinished, TaskStateFailed, TaskStatePartlyFinished:
		return true
	default:
		return false
	}
}

// WorkLockChecker 作品锁查询（由 shareLock.ShareLockRegistry 实现）。替换确认投递前置守卫：
// 确认替换的执行会软删涉及作品的活行 store 文件，作品正被分享拉取持有时在途拉取会读到
// 源文件消失，须在投递前同步拒绝（哨兵错误直达前端触发强制解锁回路）
type WorkLockChecker interface {
	IsLocked(ctx context.Context, workID int64) bool
}

// TaskDeps ManagedTask 的共享依赖集合
// 将 NewManager 和 NewManagedTask 的大量参数收敛为一个结构体，新增依赖只需改此处
type TaskDeps struct {
	Pusher          TaskProgressPusher       // 状态/进度/事件推送
	WorkLockChecker WorkLockChecker          // 替换确认投递前置作品锁守卫（Manager 确认答复面使用）
	ReplaceStoreOps resource.ReplaceStoreOps // 替换链复活能力（终态回滚单点触发登记的受害者清单）
}

// ManagedTask 任务运行控制结构体
type ManagedTask struct {
	taskId   int64
	parentId int64 // 父任务ID，0表示无父任务
	state    atomic.Int32
	ctx      context.Context
	cancel   context.CancelFunc

	// 任务完成信号
	done     chan struct{}
	doneOnce sync.Once

	// actor 通信与生命周期:一条常驻 goroutine 串行处理命令,任务级可变状态只在其内修改
	cmdCh          chan taskCmd    // 命令通道(外部→actor,postCmd 非阻塞投递)
	actorDone      chan struct{}   // actor goroutine 退出信号(终态关闭)
	ackWaitTimeout time.Duration   // 命令应答等待上界(Pause/Stop 等带应答命令的等待上界,测试可注入缩短)
	actorStarted   atomic.Bool     // 是否已首次 dispatch:dispatch 的 CAS(false→true) 守卫,保证一任务只投一次 cmdStart,重复调用幂等返回 false(取代 dispatchState 三态)
	manager        *Manager        // back-reference:actor 入队/清理需访问 Manager
	semaphore      chan struct{}   // Manager 信号量(取/释槽位)
	slotHeld       bool            // 当前是否持信号量槽位(仅 actor goroutine 访问)
	runCtx         context.Context // 当前 start/resume 命令的子 ctx(中断在途长任务)
	runCancel      context.CancelFunc

	// 可排空阶段标志（执行面经 MarkDrainPhase 上报，进入/离开下载循环时翻转）：命令监听据此
	// 分流暂停处置——可排空阶段走软暂停（排空在途数据块再停），其余阶段立即取消执行
	drainPhase atomic.Bool
	// 软暂停广播通道（进入软暂停时 close 单次广播；随每条运行命令重建）：执行面以 select 消费，
	// 收到后完成当前在途读取并落盘再收尾退出，使磁盘落点对齐真实中断点（供 Range 续传）
	softPauseCh chan struct{}
	// drain 超时兜底定时器:在途数据迟迟不落盘(插件卡死/网络黑洞)时强制 runCancel,退化为有损立即暂停
	drainTimer *time.Timer
	// 长任务执行期间 watcher 累积的命令(主循环稍后处理)
	pendingCmds []taskCmd

	// 执行面策略（按 task_type 查表注入，恒非 nil；主体执行/终态上报经此）
	strategy ExecutionStrategy
	// 策略任务确认通道：WaitReplaceConfirm 挂起等待期间经此接收 Manager.ConfirmReplace
	// 投递的整体答复（缓冲 1，防答复竞态下投递方阻塞）
	confirmCh chan replaceConfirmResult
	// 策略任务终态回滚登记（软删成功后执行器经 SetTerminalRollback 登记；setFailed 单点
	// 触发后清空，Finish 清空；仅 actor goroutine 访问）
	terminalRollback *TerminalRollback
	// 覆盖确认决策记忆（WaitReplaceConfirm 答复到达时记录；跨暂停/恢复保留，终态
	// Finish/setFailed 清空——重跑重新弹窗；仅 actor goroutine 访问）
	confirmMemo *ReplaceConfirmMemo
	// 等待确认涉及的冲突作品集合（策略任务 WaitReplaceConfirm 进入等待时记录；替换答复投递前的
	// 作品锁预检用，答复投递后清空）
	confirmConflictWorkIds []int64

	// 用户在覆盖确认选择跳过：本会话视为终态参与父聚合（AllChildrenTerminal），不持久化、崩溃重启当作未跳过；仅对本次 Start/Retry 执行有效，Resume 不读
	skipped bool

	// 当前错误信息（仅 TaskStateFailed 时有效，通过 onStateChange 回调传递到 Manager）
	errorMessage string

	// 共享依赖
	deps *TaskDeps

	// 任务核心行（加载时 DB 快照：跳过收口回执行前状态的回退基准与展示用；
	// 运行态经 setState 的内存 atomic 承载，不经此字段）
	task *entity.Task

	// atomic 进度快照字段，供 BuildSnapshot 并发安全读取
	progressTotal    atomic.Int64 // 资源总大小
	progressFinished atomic.Int64 // 已下载字节数

	// 回调函数
	onStateChange func(taskId int64, oldState, newState TaskState, errMsg string)
	onProgress    func(taskId int64, total int64, finished int64)
}

// NewManagedTask 创建托管任务并启动 actor goroutine(一生一灭,任务级可变状态只在其内修改)。
// 运行态按任务行 status 初始化（直接 Store，不经 setState——构造期不触发状态回调、不关 done
// 通道）：构造方均为冷加载（任务不在内存时自 DB 行构建），DB 行是上一会话落定的执行前稳态
// （仅稳定态落库、无刷盘滞后）——Paused 行带着该状态进入执行即命中恢复信号分叉（跨重启续传），
// Created 行首启、终态行重试均全新执行。
func NewManagedTask(taskId, parentId int64, task *entity.Task, deps *TaskDeps, manager *Manager, semaphore chan struct{}) *ManagedTask {
	ctx, cancel := context.WithCancel(context.Background())
	m := &ManagedTask{
		taskId:         taskId,
		parentId:       parentId,
		state:          atomic.Int32{},
		ctx:            ctx,
		cancel:         cancel,
		done:           make(chan struct{}),
		cmdCh:          make(chan taskCmd, 8),
		actorDone:      make(chan struct{}),
		ackWaitTimeout: ackWaitTimeoutDefault,
		confirmCh:      make(chan replaceConfirmResult, 1),
		softPauseCh:    make(chan struct{}),
		manager:        manager,
		semaphore:      semaphore,
		deps:           deps,
		task:           task,
	}
	// 运行态自任务行 status 起步（语义见构造注释）；actorStarted 保持零值 false:它是 dispatch 的
	// CAS(false→true) 首派守卫,首次 dispatch 据此投 cmdStart。
	// 创建期赋 true 会使守卫永远失败、cmdStart 不投递,新任务卡在 Created 永不执行。
	m.state.Store(int32(TaskState(task.Status)))
	go m.actorLoop()
	return m
}

// ==== per-task actor:命令通道 + 主循环 ====

// cmdKind actor 命令种类
type cmdKind int

const (
	cmdStart  cmdKind = iota // 初始执行 / Retry / Redownload
	cmdResume                // 从 Paused/Pausing 恢复(含跨重启续传)
	cmdPause                 // 暂停(仅 Processing/Pausing/Paused 有效)
	cmdStop                  // 停止(终态 Failed)
)

// taskCmd actor 命令
type taskCmd struct {
	kind cmdKind
	ack  chan error // 可选应答(Pause/Stop 等待 actor 处理;nil=fire-and-forget;队列满丢弃时回写错误)
}

// postCmd 非阻塞投递命令到 actor。绝不阻塞投递方(投递路径不持 Manager.mu,防死锁)。
// 队列满丢弃时对带应答的命令回写错误——等待方随即收到失败返回,不空等到应答超时
func (m *ManagedTask) postCmd(cmd taskCmd) {
	select {
	case m.cmdCh <- cmd:
	default:
		logger.Log.Warnf("[TaskManager] 任务 %d cmdCh 满,丢弃命令 %d", m.taskId, cmd.kind)
		if cmd.ack != nil {
			select {
			case cmd.ack <- ErrCmdQueueFull:
			default:
			}
		}
	}
}

// actorLoop actor 主循环:串行处理命令,任务级可变状态只在此 goroutine 内修改。
// 对象创建时启动,终态(或主 ctx 取消)退出;退出前取消任务 ctx 并启动 cmdCh 善后消费。
// 作用域:串行覆盖限主程序调度层(命令按序、状态机一致、槽位独占);不覆盖插件 transport 层
// 的 serveSpecsPull goroutine(per-RPC,受 gRPC stream 控制)。reader 等跨 RPC 对象的访问
// 串行性由每次 Start/Resume 新建 reader 结构性保证,不依赖此 actor(见 plugin-dev-guide.md「ctx 与 reader 契约」)。
func (m *ManagedTask) actorLoop() {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("[TaskManager] actor panic taskId=%d: %v", m.taskId, r)
			m.setFailed(fmt.Sprintf("actor panic: %v", r))
		}
		// 取消任务 ctx——命令通道善后消费(drainCmds)的退出条件
		m.cancel()
		go m.drainCmds()
		close(m.actorDone)
	}()
	for {
		select {
		case <-m.ctx.Done():
			return
		case cmd, ok := <-m.cmdCh:
			if !ok {
				return
			}
			switch cmd.kind {
			case cmdStart, cmdResume:
				m.handleRunCmd(cmd)
			case cmdPause:
				m.handlePauseCmd(cmd)
			case cmdStop:
				m.handleStopCmd(cmd)
			}
			if isTerminalState(m.GetState()) {
				return
			}
		}
	}
}

// drainCmds actor 退出后的命令通道善后消费:退出时任务 ctx 已被取消,本循环消费缓冲内
// 残留命令后随之退出,不随任务对象常驻。投递方经非阻塞 postCmd 投递(满即丢弃并回写
// 应答错误),善后消费不承担解除投递方阻塞的职责
func (m *ManagedTask) drainCmds() {
	for {
		select {
		case _, ok := <-m.cmdCh:
			if !ok {
				return
			}
		case <-m.ctx.Done():
			return
		}
	}
}

// isTerminalState 是否终态(Finished/Failed/PartlyFinished)
func isTerminalState(s TaskState) bool {
	return s == TaskStateFinished || s == TaskStateFailed || s == TaskStatePartlyFinished
}

// handleRunCmd 处理 start/resume:取槽位 → 重置执行期信号并派生 runCtx → watcher 监听中断命令 → 执行策略主体 → 处理 result + pendingCmds
func (m *ManagedTask) handleRunCmd(cmd taskCmd) {
	// 取信号量槽位(首次进入);取到则出队,取不到入 waitingQueue 等唤醒
	if !m.slotHeld {
		select {
		case m.semaphore <- struct{}{}:
			m.slotHeld = true
			m.dequeueSelf()
		default:
			logger.Log.Infof("[TaskManager] 任务 %d 信号量槽位满,入等待队列(等空闲槽位)", m.taskId)
			m.enqueueSelf()
			return
		}
	}

	// 重置执行期信号:排空阶段标志清零、软暂停广播通道重建(close 为单次广播,按执行轮次
	// 重建恢复广播能力)、drain 兜底定时器停用——覆盖全部运行命令(start/resume)
	m.resetRunSignals()
	// 派生 runCtx(每条 run 命令新建,中断在途长任务用)
	m.runCtx, m.runCancel = context.WithCancel(m.ctx)
	stopWatcher := make(chan struct{})
	go m.cmdWatcher(stopWatcher)

	// 执行策略主体(阻塞在此,可被 watcher 的 runCancel 中断)
	result := m.runStrategy()

	close(stopWatcher)
	if m.drainTimer != nil {
		// drain 正常完成则停定时器;已触发(超时)则 Stop 返回 false,无副作用
		m.drainTimer.Stop()
		m.drainTimer = nil
	}
	if m.runCancel != nil {
		// 此时无在途(drain 已完成或超时已强制取消),取消 stream 作清理
		m.runCancel()
		m.runCancel = nil
	}

	// 跳过收口：Skip 上报内已完成槽位释放（含等待队列唤醒）/状态回退/清理与 actor 取消，
	// 此处不再处置（防清理双跑致前端双推），也不消费 watcher 暂存命令——actor 主 ctx 已取消，
	// 命令通道残留由 actorLoop 退出的 drain 消费
	if result == runResultSkipped {
		return
	}

	// 处理 result + 释放槽位
	switch result {
	case runResultDone: // 终态 Finished/Failed
		m.releaseSlot()
		m.manager.dispatchFromQueue()
		m.manager.cleanupFinishedTask(m)
	case runResultPaused: // 中断(暂停交后续恢复;停止的终态由 handleStopCmd 收口)
		m.releaseSlot()
		m.manager.dispatchFromQueue()
	}

	// 处理 watcher 累积的命令(按序)
	pending := m.pendingCmds
	m.pendingCmds = nil
	for _, c := range pending {
		switch c.kind {
		case cmdStart, cmdResume:
			m.handleRunCmd(c)
		case cmdPause:
			m.handlePauseCmd(c)
		case cmdStop:
			m.handleStopCmd(c)
		}
		if isTerminalState(m.GetState()) {
			return
		}
	}
}

// resetRunSignals 重置执行期信号：排空阶段标志清零、软暂停广播通道重建、drain 兜底定时器停用。
// 每条运行命令进入执行前调用——上一轮执行遗留的信号不得影响新一轮（软暂停广播已 close 的
// 通道会使执行面误判已暂停立即收尾）
func (m *ManagedTask) resetRunSignals() {
	m.drainPhase.Store(false)
	m.softPauseCh = make(chan struct{})
	if m.drainTimer != nil {
		m.drainTimer.Stop()
		m.drainTimer = nil
	}
}

// runStrategy 任务主体执行：委托执行面策略，映射其结果到控制面。
// 策略经 handle 自行上报终态（Finish/Fail）或跳过收口（Skip）；RunCtx 取消（用户暂停/停止）时
// 策略返回而不上报终态，此处按中断处理（暂停→runResultPaused 交还控制面，停止→handleStopCmd 置 Failed）。
// 下载阶段软暂停的排空收尾同属中断：执行面收到广播后完成在途数据落盘即返回（无终态、RunCtx 未取消）。
func (m *ManagedTask) runStrategy() runResult {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("[TaskManager] 任务 %d 执行 panic: %v", m.taskId, r)
			m.setFailed(fmt.Sprintf("任务执行 panic: %v", r))
		}
	}()
	if m.runCtx.Err() != nil {
		// 命令排队竞态（派发前已取消）：按中断处理，不进执行
		return runResultPaused
	}
	// 恢复信号在进入执行前置位（策略据其分叉续传与全新执行），故先建句柄再置 Processing
	h := newStrategyHandle(m)
	m.setState(TaskStateProcessing)
	m.strategy.Execute(h)
	if h.terminal {
		return runResultDone
	}
	if h.skipReported {
		// 跳过收口：处置（释放槽位/置收口标志/状态回退/清理/取消 actor）已在 Skip 上报内完成
		return runResultSkipped
	}
	if m.runCtx.Err() != nil {
		return runResultPaused
	}
	if m.softPauseBroadcast() {
		// 下载阶段软暂停的排空收尾：广播已发出且 RunCtx 未取消（排空早于超时兜底），执行面
		// 完成在途数据落盘后正常返回。按中断交还控制面，待处理命令中的暂停随后置 Paused。
		// 广播通道随每条运行命令重建，此处为真即本派发内发起的暂停
		return runResultPaused
	}
	// 契约违约防御：既未上报终态也未被取消——置失败终态，避免任务停在 Processing
	logger.Log.Errorf("[TaskManager] 任务 %d 执行器未上报终态", m.taskId)
	m.setFailed("执行器未上报终态")
	return runResultDone
}

// drainTimeout 优雅暂停的 drain 超时阈值:暂停后等待在途数据落盘的最长时间,超时则强制取消 runCtx,退化为有损立即暂停。
// 单个数据块往返(主程序发起读取 → 插件返回数据 → 主程序落盘)通常 <1s,2s 足够覆盖常态往返,且暂停延迟用户无感。
const drainTimeout = 2 * time.Second

// interruptNotifyTimeout 暂停/停止的执行面中断通知（插件 Pause/Stop RPC 转发）调用上界，
// 与插件代理层 unary 超时同量级——插件侧无响应时控制命令不被无限拖住
const interruptNotifyTimeout = 30 * time.Second

// ackWaitTimeoutDefault 命令应答等待上界的生产默认值:暂停/停止命令的处理时长主体是
// 执行面中断通知(上界 30s),此值覆盖其上加余量——插件侧无响应时控制命令按此上界返回
const ackWaitTimeoutDefault = 35 * time.Second

// cmdWatcher 长任务执行期间监听 cmdCh:可排空阶段（下载循环内）的暂停走优雅暂停（广播软暂停 +
// 启动 drainTimer,不立即取消,在途数据照常落盘）;其余阶段的暂停与停止立即 runCancel
// (无在途数据块需排空/放弃语义);所有命令暂存 pendingCmds 由主循环稍后处理
func (m *ManagedTask) cmdWatcher(stop <-chan struct{}) {
	for {
		select {
		case c := <-m.cmdCh:
			if c.kind == cmdPause {
				if m.drainPhase.Load() {
					// 可排空阶段:优雅暂停——广播执行面完成当前在途往返(读取→落盘)后收尾退出
					m.broadcastSoftPause()
					logger.Log.Infof("[TaskManager] 任务 %d 暂停:下载阶段走优雅暂停(softPause drain)", m.taskId)
					// drain 超时兜底:在途迟迟不落盘则强制取消,退化为有损立即暂停
					m.drainTimer = time.AfterFunc(drainTimeout, m.forceCancelDrainStuck)
				} else {
					// 其余阶段:无在途数据块需排空,立即取消 runCtx 中断插件 RPC(快速暂停)
					logger.Log.Infof("[TaskManager] 任务 %d 暂停:立即 runCancel", m.taskId)
					if m.runCancel != nil {
						m.runCancel()
					}
				}
				m.pendingCmds = append(m.pendingCmds, c)
				return
			}
			if c.kind == cmdStop {
				// 停止是放弃,立即取消(不走 drain)
				logger.Log.Infof("[TaskManager] 任务 %d 停止:立即 runCancel", m.taskId)
				if m.runCancel != nil {
					m.runCancel()
				}
				m.pendingCmds = append(m.pendingCmds, c)
				return
			}
			m.pendingCmds = append(m.pendingCmds, c)
		case <-stop:
			return
		}
	}
}

// broadcastSoftPause 进入软暂停：close 广播通道（单次；通道随每条运行命令重建）
func (m *ManagedTask) broadcastSoftPause() {
	select {
	case <-m.softPauseCh:
		// 已广播（防御：watcher 生命周期内至多处理一次暂停，正常不可达）
	default:
		close(m.softPauseCh)
	}
}

// softPauseBroadcast 软暂停广播是否已发出（通道 close 后恒真）
func (m *ManagedTask) softPauseBroadcast() bool {
	select {
	case <-m.softPauseCh:
		return true
	default:
		return false
	}
}

// forceCancelDrainStuck drain 超时兜底：软暂停已广播且在途数据迟迟不落盘时强制取消 runCtx，
// 退化为有损立即暂停（软暂停未广播=非暂停窗口的误触，不取消）
func (m *ManagedTask) forceCancelDrainStuck() {
	if m.softPauseBroadcast() && m.runCancel != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d drain 超时,强制取消(退化为有损立即暂停)", m.taskId)
		m.runCancel()
	}
}

// releaseSlot 释放信号量槽位（仅 actor goroutine 调用）。策略任务确认挂起期间
// WaitReplaceConfirm 已自行释放槽位（slotHeld=false），此处按持有的槽位守卫防重复释放
func (m *ManagedTask) releaseSlot() {
	if !m.slotHeld {
		return
	}
	m.slotHeld = false
	<-m.semaphore
}

// enqueueSelf 取不到信号量槽位时入 Manager.waitingQueue(按 taskId 去重)并置 Waiting
func (m *ManagedTask) enqueueSelf() {
	m.manager.mu.Lock()
	for _, t := range m.manager.waitingQueue {
		if t.taskId == m.taskId {
			m.manager.mu.Unlock()
			return // 已在队
		}
	}
	m.manager.waitingQueue = append(m.manager.waitingQueue, m)
	m.manager.mu.Unlock()
	m.setState(TaskStateWaiting)
}

// dequeueSelf 取到信号量槽位后从 Manager.waitingQueue 移除自己(若在内)
func (m *ManagedTask) dequeueSelf() {
	m.manager.mu.Lock()
	kept := make([]*ManagedTask, 0, len(m.manager.waitingQueue))
	for _, t := range m.manager.waitingQueue {
		if t.taskId != m.taskId {
			kept = append(kept, t)
		}
	}
	m.manager.waitingQueue = kept
	m.manager.mu.Unlock()
}

// handlePauseCmd 处理 pause 命令:非终态任务 → Paused(命令队列保证 PauseTaskTree 的 pause 覆盖陈旧 resume)。
// Processing 状态额外中断在途执行(runCancel,watcher 在长任务期间已触发时此处幂等);
// 随后经执行面中断通知转发插件暂停 RPC(有上游时关上游 HTTP 保留 validBytes 供 Resume
// Range 续传;无上游时插件幂等处理)。策略未实现该能力时 RunCtx 取消即中断信号,无额外通知
func (m *ManagedTask) handlePauseCmd(cmd taskCmd) {
	s := m.GetState()
	if isTerminalState(s) {
		if cmd.ack != nil {
			cmd.ack <- ErrTaskNotProcessing
		}
		return
	}
	if s == TaskStateProcessing {
		m.setState(TaskStatePausing)
		if m.runCancel != nil {
			m.runCancel()
		}
	}
	m.notifyInterrupt(false)
	m.setState(TaskStatePaused)
	if cmd.ack != nil {
		cmd.ack <- nil
	}
}

// handleStopCmd 处理 stop 命令:终态 Failed
func (m *ManagedTask) handleStopCmd(cmd taskCmd) {
	if isTerminalState(m.GetState()) {
		if cmd.ack != nil {
			cmd.ack <- nil
		}
		return
	}
	m.setState(TaskStateStopping)
	if m.runCancel != nil {
		m.runCancel()
	}
	m.notifyInterrupt(true)
	m.setFailed("任务被用户停止")
	if cmd.ack != nil {
		cmd.ack <- nil
	}
}

// notifyInterrupt 暂停/停止的执行面通知：策略实现 InterruptNotifier（可选能力，按类型断言调用）
// 时转发插件 Pause/Stop RPC。RPC 失败仅由实现方告警——中断的主体信号是运行 ctx 取消，
// 插件侧通知为尽力而为
func (m *ManagedTask) notifyInterrupt(stop bool) {
	notifier, ok := m.strategy.(InterruptNotifier)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(m.ctx, interruptNotifyTimeout)
	defer cancel()
	notifier.NotifyInterrupt(ctx, m.taskId, stop)
}

// Pause 暂停任务(投递 cmdPause + 有界等 actor 处理;仅 Processing 时返回 nil,否则
// ErrTaskNotProcessing。命令被队列满丢弃、actor 已退出或应答超时返回对应错误,不无限等待)
func (m *ManagedTask) Pause() error {
	ack := make(chan error, 1)
	m.postCmd(taskCmd{kind: cmdPause, ack: ack})
	return m.waitAck(ack)
}

// Stop 停止任务(投递 cmdStop + 有界等 actor 处理;无返回值,应答等待异常仅告警)
func (m *ManagedTask) Stop() {
	ack := make(chan error, 1)
	m.postCmd(taskCmd{kind: cmdStop, ack: ack})
	if err := m.waitAck(ack); err != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 停止应答异常: %v", m.taskId, err)
	}
}

// waitAck 有界等待命令应答:应答到达返回其错误;actor 已退出(actorDone 关闭——任务终态
// 或任务 ctx 取消,命令通道改由善后消费接管、不再产出应答)返回 ErrTaskActorExited;超过
// 应答等待上界返回 ErrTaskAckTimeout。actor 处理命令写应答先于其终态退出关闭 actorDone,
// 故 actorDone 就绪时再查一次应答——成功处理的命令不被误报为 actor 已退出
func (m *ManagedTask) waitAck(ack <-chan error) error {
	timer := time.NewTimer(m.ackWaitTimeout)
	defer timer.Stop()
	select {
	case err := <-ack:
		return err
	case <-m.actorDone:
		select {
		case err := <-ack:
			return err
		default:
			return ErrTaskActorExited
		}
	case <-timer.C:
		return ErrTaskAckTimeout
	}
}

// setState 设置任务状态
func (m *ManagedTask) setState(state TaskState) {
	old := TaskState(m.state.Swap(int32(state)))
	if old == state {
		return
	}
	// 非 Failed 状态清除错误信息（重试后不应保留旧错误）
	if state != TaskStateFailed {
		m.errorMessage = ""
	}
	taskName := ""
	if m.task != nil && m.task.TaskName.Valid {
		taskName = m.task.TaskName.String
	}
	logger.Log.Infof("[TaskManager] 任务状态变更 [%s](%d): %s → %s", taskName, m.taskId, taskStateName(old), taskStateName(state))
	if m.onStateChange != nil {
		m.onStateChange(m.taskId, old, state, m.errorMessage)
	}
	if state == TaskStateFinished || state == TaskStateFailed {
		m.doneOnce.Do(func() { close(m.done) })
	}
}

// setFailed 设置任务为失败状态，并记录错误信息。失败即触发登记的替换链回滚：按执行器经
// SetTerminalRollback 登记的受害者显式清单复活被软删行（多作品清单由执行器自持；未登记即无
// 软删行可复活——本执行未发生替换，直接让位）；触发后清空登记，重试从空态重新登记。
// 暂停不触发，软删悬空为合法中间态（等恢复延续替换）
func (m *ManagedTask) setFailed(errMsg string) {
	m.errorMessage = errMsg
	m.setState(TaskStateFailed)
	m.confirmMemo = nil // 失败终态清空确认决策记忆（重跑重新弹窗）
	m.triggerTerminalRollback()
}

// triggerTerminalRollback 触发执行器登记的回滚清单：先丢弃替换执行期新建的 store 行
// （行+文件+关联，释放旧代 file_path），再按受害者显式清单复活被软删行（失败/停止经
// setFailed 单点触发）。受害者清单为空即本任务未发生软删替换——新建行清单随之作废
// （非替换失败保留已下载成果，不清理）。一次性：触发后清空登记，重试从空态重新登记
func (m *ManagedTask) triggerTerminalRollback() {
	if m.terminalRollback == nil || m.deps == nil || m.deps.ReplaceStoreOps == nil {
		return
	}
	rb := m.terminalRollback
	m.terminalRollback = nil
	if len(rb.Victims) == 0 {
		return
	}
	logger.Log.Infof("[TaskManager] 任务 %d 终态回滚: 丢弃 %d 个新建 store 行, 复活 %d 个被替换 store 行",
		m.taskId, len(rb.CreatedStoreIDs), len(rb.Victims))
	if err := m.deps.ReplaceStoreOps.RestoreReplacedStores(context.Background(), resource.RestoreScope{
		Victims:         rb.Victims,
		DiscardStoreIDs: rb.CreatedStoreIDs,
	}); err != nil {
		logger.Log.Warnf("[TaskManager] 任务 %d 终态回滚失败: %v", m.taskId, err)
	}
}

// Done 返回任务完成信号 channel，任务终态（Finished/Failed）时关闭
func (m *ManagedTask) Done() <-chan struct{} {
	return m.done
}

// GetState 获取当前状态
func (m *ManagedTask) GetState() TaskState {
	return TaskState(m.state.Load())
}

// SetOnStateChange 设置状态变化回调
func (m *ManagedTask) SetOnStateChange(fn func(taskId int64, oldState, newState TaskState, errMsg string)) {
	m.onStateChange = fn
}

// SetOnProgress 设置进度回调
func (m *ManagedTask) SetOnProgress(fn func(taskId int64, total int64, finished int64)) {
	m.onProgress = fn
}

// ParentTask 父任务运行结构体
type ParentTask struct {
	taskId    int64
	taskName  string
	state     atomic.Int32
	refreshMu sync.Mutex // 保护 RefreshState 的读取和写入原子性，防止并发 goroutine 推送过时状态
	children  map[int64]*ManagedTask
	mu        sync.RWMutex
}

// NewParentTask 创建父任务
func NewParentTask(taskId int64, taskName string) *ParentTask {
	return &ParentTask{
		taskId:   taskId,
		taskName: taskName,
		state:    atomic.Int32{},
		children: make(map[int64]*ManagedTask),
	}
}

// AddChild 添加子任务
func (p *ParentTask) AddChild(child *ManagedTask) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.children[child.taskId] = child
}

// GetChildren 获取所有子任务
func (p *ParentTask) GetChildren() []*ManagedTask {
	p.mu.RLock()
	defer p.mu.RUnlock()
	children := make([]*ManagedTask, 0, len(p.children))
	for _, child := range p.children {
		children = append(children, child)
	}
	return children
}

// GetChild 获取指定子任务
func (p *ParentTask) GetChild(taskId int64) (*ManagedTask, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	child, ok := p.children[taskId]
	return child, ok
}

// RefreshState 根据子任务状态刷新父任务状态
// 返回旧状态、新状态、已完成子任务数和子任务总数，供调用方判断是否需要持久化和推送进度
func (p *ParentTask) RefreshState() (oldState, newState TaskState, finishedCount, total int) {
	children := p.GetChildren()
	if len(children) == 0 {
		return p.GetState(), p.GetState(), 0, 0
	}

	var fc, failedCount int
	var anyProcessing, anyWaiting, anyPaused bool
	for _, child := range children {
		switch child.GetState() {
		case TaskStateFinished:
			fc++
		case TaskStateFailed:
			failedCount++
		case TaskStateProcessing, TaskStatePausing, TaskStateStopping, TaskStateWaitingForInput:
			anyProcessing = true
		case TaskStatePaused:
			anyPaused = true
		case TaskStateWaiting:
			anyWaiting = true
		}
	}

	t := len(children)

	switch {
	case anyProcessing:
		newState = TaskStateProcessing
	case anyWaiting:
		newState = TaskStateWaiting
	case anyPaused:
		newState = TaskStatePaused
	case fc == t:
		newState = TaskStateFinished
	case failedCount == t:
		newState = TaskStateFailed
	case fc > 0 && fc < t:
		newState = TaskStatePartlyFinished
	default:
		newState = TaskStateCreated
	}

	oldState = TaskState(p.state.Swap(int32(newState)))
	return oldState, newState, fc, t
}

// GetState 获取父任务状态
func (p *ParentTask) GetState() TaskState {
	return TaskState(p.state.Load())
}

// AllChildrenTerminal 检查所有子任务是否都已进入终态（含显式跳过）
// 终态 = Finished/Failed/PartlyFinished（isTerminalState）或被用户跳过（skipped 标志）。
// 不再把 Created 当终态：未启动的 Created 兄弟需保留父任务运行态，跳过改由 skipped 显式表达。
func (p *ParentTask) AllChildrenTerminal() bool {
	for _, child := range p.GetChildren() {
		s := child.GetState()
		if isTerminalState(s) || child.skipped {
			continue
		}
		return false
	}
	return true
}

// 任务管理错误定义
var (
	ErrTaskNotProcessing = &TaskManagerError{message: "task is not in processing state"}
	ErrTaskNotPaused     = &TaskManagerError{message: "task is not in paused state"}
	ErrTaskTreeNotFound  = &TaskManagerError{message: "task tree not found"}
	ErrCmdQueueFull      = &TaskManagerError{message: "task command queue full, command dropped"}
	ErrTaskActorExited   = &TaskManagerError{message: "task actor exited"}
	ErrTaskAckTimeout    = &TaskManagerError{message: "task command ack wait timeout"}
)

// TaskManagerError 任务管理器错误
type TaskManagerError struct {
	message string
}

func (e *TaskManagerError) Error() string {
	return e.message
}
