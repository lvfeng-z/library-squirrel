package taskManager

// 执行面策略（可插拔）：控制面（actor 循环/信号量/状态机/进度/持久化/恢复调度）留在
// taskManager，「任务主体怎么执行」外提为接口——内置任务类型（task.task_type 非空，如
// share-host/share-receive）经按类型注册的策略执行；插件任务（task_type 空）维持既有
// 执行路径（板块组合 + 多轨下载/续传）。策略按 task_type 在 Manager 构造时注入
// （app.go 装配），taskManager 不感知具体业务类型。

import (
	"context"

	"github.com/library-squirrel/backend/base/model/entity"
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

// Finish 上报成功终态：置 Finished（幂等，重复调用与 Fail 之后的调用均为 no-op）
func (h *strategyHandle) Finish() {
	if h.terminal {
		return
	}
	h.terminal = true
	h.m.setState(TaskStateFinished)
}

// Fail 上报失败终态：置 Failed 并记录错误信息（幂等，同 Finish）
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
