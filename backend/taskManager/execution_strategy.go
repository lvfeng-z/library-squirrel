package taskManager

// 执行面策略（可插拔）：控制面（actor 循环/信号量/状态机/进度/持久化/恢复调度）留在
// taskManager，「任务主体怎么执行」外提为接口——内置任务类型（task.task_type 非空，如
// share-receive）经按类型注册的策略执行；插件任务（task_type 空）维持既有
// 执行路径（板块组合 + 多轨下载/续传）。策略按 task_type 在 Manager 构造时注入
// （app.go 装配），taskManager 不感知具体业务类型。

import (
	"context"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/resource"
)

// ExecutionStrategy 内置任务类型的执行面策略（taskManager 定义，业务模块实现）。
// Execute 在任务 actor goroutine 内同步调用（阻塞直至终态或中断），期间：
//   - 主体自然完成 → 经 handle 上报 Finish（任务置 Finished 终态）
//   - 主体失败     → 经 handle 上报 Fail(errMsg)（任务置 Failed 终态）
//   - RunCtx 取消（用户暂停/停止）→ 尽快返回且不上报终态，状态由控制面接管
//     （暂停→Paused 可恢复重跑，停止→Failed("任务被用户停止")）
//
// 实现约定：Execute 返回前必须已调用 Finish/Fail，或 RunCtx 已取消；暂停/停止后的
// 重新执行（Resume/Retry/重开）仍走同一 Execute 入口——主体自身负责从任务 payload
// 与自有持久化状态恢复（内置类型无插件续传协议，恢复语义由各类型自行定义）。
type ExecutionStrategy interface {
	// Execute 执行任务主体至终态或 RunCtx 取消
	Execute(handle StrategyHandle)
}

// ReplaceDecision 覆盖确认的整体决策（任务粒度，对应现有 ConfirmReplace 单值答复）
type ReplaceDecision int

const (
	ReplaceDecisionSkip   ReplaceDecision = iota // 跳过：不替换，保留已有作品
	ReplaceDecisionReplace                        // 替换：软删旧 store 后挂新
)

// ConflictInfo 单个冲突作品的任务域通用信息（WaitReplaceConfirm 输入，供前端弹窗展示）。
// 载荷只含任务域通用概念（已有作品 ID、冲突角色），不含任何具体业务类型（manifest/分享）概念
type ConflictInfo struct {
	WorkID        int64    // 已有作品 ID（查重命中对象）
	WorkName      string   // 已有作品名（展示）
	ConflictRoles []string // 冲突角色（行级交集，将被覆盖的板块）
}

// TerminalRollback 终态回滚登记载荷（失败/停止时由控制面 setFailed 单点统一触发）。
// 载荷只含任务域通用概念：被软删的 victim store 清单（多作品，软删成功后登记）
type TerminalRollback struct {
	Victims []resource.StoreRef // 被软删行清单（失败回滚时按清单复活）
}

// replaceConfirmResult 确认通道投递的答复（策略任务执行内挂起等待的唤醒载荷）
type replaceConfirmResult struct {
	decision ReplaceDecision
}

// confirmDecision 由前端确认动作推导整体决策（"skip" → 跳过，其余 → 替换；
// 对应现有 ConfirmReplace 单值答复的 action 语义）
func confirmDecision(action string) ReplaceDecision {
	if action == "skip" {
		return ReplaceDecisionSkip
	}
	return ReplaceDecisionReplace
}

// StrategyHandle 执行面策略访问任务上下文与控制面上报的句柄（仅任务 actor
// goroutine 内有效，实现方不得跨 goroutine 持有调用）。
type StrategyHandle interface {
	// Task 任务实体（含 task_type 与该类型的 payload 载荷）
	Task() *entity.Task
	// RunCtx 本次执行的 ctx（暂停/停止时由控制面取消）
	RunCtx() context.Context
	// Finish 上报成功终态（任务 → Finished）
	Finish()
	// Fail 上报失败终态（任务 → Failed，errMsg 落 error_message）
	Fail(errMsg string)
	// ReportProgress 上报进度（total/finished 语义与插件任务一致，前端按比值展示）
	ReportProgress(total, finished int64)
	// WaitReplaceConfirm 覆盖确认等待（执行中挂起）：置任务 WaitingForInput、对每个冲突作品
	// 复用现有 PushDuplicateDetected 逐条推送（同 taskId 多事件、现有载荷与事件名不动），
	// 阻塞直至用户整体答复或 RunCtx 取消。返回整体决策（replace/skip，任务粒度）；
	// canceled=true 表示 RunCtx 取消（防御性分支——模态弹窗期间暂停/停止实际不可达，
	// 真正中断窗口在确认之后），Execute 约定不上报终态交控制面接管。
	WaitReplaceConfirm(conflicts []ConflictInfo) (decision ReplaceDecision, canceled bool)
	// SetTerminalRollback 登记终态回滚（失败/停止时由控制面 setFailed 单点统一触发；
	// Execute 的取消返回路径无法区分暂停与停止，不得自行回滚）。同一任务多次软删
	// （如暂停恢复后延续替换再次软删）的受害者清单合并登记（按 store ID 去重），
	// 保证终态回滚覆盖全部软删行。
	SetTerminalRollback(rollback TerminalRollback)
}

// strategyHandle StrategyHandle 的 taskManager 实现：桥接到 ManagedTask 控制面。
type strategyHandle struct {
	m        *ManagedTask
	terminal bool // 是否已上报终态（Finish/Fail 二选一，只首次生效）
}

// newStrategyHandle 构建执行句柄
func newStrategyHandle(m *ManagedTask) *strategyHandle {
	return &strategyHandle{m: m}
}

func (h *strategyHandle) Task() *entity.Task { return h.m.task }

func (h *strategyHandle) RunCtx() context.Context { return h.m.runCtx }

// Finish 上报成功终态：置 Finished（幂等，重复调用与 Fail 之后的调用均为 no-op）。
// 替换完成即软删行进入终态，清空回滚登记——重试从空态重新登记，不复活历史软删行
func (h *strategyHandle) Finish() {
	if h.terminal {
		return
	}
	h.terminal = true
	h.m.terminalRollback = nil
	h.m.setState(TaskStateFinished)
}

// Fail 上报失败终态：置 Failed 并记录错误信息（幂等，同 Finish）。
// 失败经控制面 setFailed 单点触发登记的回滚钩子（与停止共用，见 SetTerminalRollback）
func (h *strategyHandle) Fail(errMsg string) {
	if h.terminal {
		return
	}
	h.terminal = true
	h.m.setFailed(errMsg)
}

// ReportProgress 上报进度（复用插件任务的进度回调链：atomic 快照 + 批量推送）
func (h *strategyHandle) ReportProgress(total, finished int64) {
	if h.m.onProgress != nil {
		h.m.onProgress(h.m.taskId, total, finished)
	}
}

// WaitReplaceConfirm 覆盖确认等待：置任务 WaitingForInput、逐条推送冲突事件、注册进等待确认表，
// 释放信号量槽位（确认挂起期间不占并发额度），阻塞直至用户整体答复（确认通道）或 RunCtx 取消。
// 答复后重新排队取槽继续执行；取消时返回 canceled=true、槽位保持释放（交回控制面，
// 由 handleRunCmd 释放路径按 slotHeld 守卫防重复释放）。
func (h *strategyHandle) WaitReplaceConfirm(conflicts []ConflictInfo) (ReplaceDecision, bool) {
	m := h.m
	// 排空历史残留答复：上次挂起被取消时可能遗留缓冲中的答复，避免被误当本次答复
	select {
	case <-m.confirmCh:
	default:
	}
	m.setState(TaskStateWaitingForInput)
	if m.deps.Pusher != nil {
		for _, c := range conflicts {
			m.deps.Pusher.PushDuplicateDetected(m.taskId, m.task.TaskName.String, c.WorkID, c.WorkName, c.ConflictRoles)
		}
	}
	// 注册进等待确认表：供 Manager.ConfirmReplace 按任务类型分流投递答复、前端状态展示
	m.manager.enqueueWaitingForInput(m)
	// 释放信号量槽位：挂起等待期间不挤占并发额度（对齐插件任务命中冲突不入队不占槽的既有形态）
	if m.slotHeld {
		m.slotHeld = false
		<-m.semaphore
		m.manager.dispatchFromQueue()
	}
	// 阻塞等待整体答复或 RunCtx 取消（防御性：模态弹窗期间暂停/停止实际不可达，真正的
	// 中断窗口在确认之后，由 setFailed 单点经登记钩子收口）
	select {
	case res := <-m.confirmCh:
		// 答复后重新排队取槽（阻塞取槽可被取消打断；取消时槽位保持释放）
		select {
		case m.semaphore <- struct{}{}:
			m.slotHeld = true
		case <-m.runCtx.Done():
			m.manager.removeWaitingForInput(m.taskId)
			return res.decision, true
		}
		m.setState(TaskStateProcessing)
		return res.decision, false
	case <-m.runCtx.Done():
		m.manager.removeWaitingForInput(m.taskId)
		return ReplaceDecisionSkip, true
	}
}

// SetTerminalRollback 登记终态回滚载荷（合并累积，按 store ID 去重）。
// 软删成功后登记；多次软删（如暂停恢复后延续替换再次软删）的受害者并集登记，
// 保证终态回滚覆盖全部软删行。触发与清空归控制面 setFailed 单点 / Finish
func (h *strategyHandle) SetTerminalRollback(rollback TerminalRollback) {
	if len(rollback.Victims) == 0 {
		return
	}
	m := h.m
	if m.terminalRollback == nil {
		m.terminalRollback = &TerminalRollback{}
	}
	seen := make(map[int64]struct{}, len(m.terminalRollback.Victims))
	for _, v := range m.terminalRollback.Victims {
		seen[v.StoreID] = struct{}{}
	}
	for _, v := range rollback.Victims {
		if _, dup := seen[v.StoreID]; dup {
			continue
		}
		seen[v.StoreID] = struct{}{}
		m.terminalRollback.Victims = append(m.terminalRollback.Victims, v)
	}
}
