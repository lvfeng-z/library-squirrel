# 修复：任务树恢复后再次暂停遗漏子任务

## 问题描述

任务树（父任务 + 多个子任务）的操作流程：
1. 首次暂停 → 所有子任务正常暂停 ✅
2. 恢复 → 子任务重新调度
3. 再次暂停 → **只有一个子任务被暂停**，其余继续运行 ❌

## 根因分析

### 状态窗口

`ResumeTaskTree` → `PauseTaskTree` 流程中，`prepareForResume()` 重置了任务内部状态但保留了 `Paused` 状态。当 `tryDispatch` 派发 goroutine 后、goroutine 实际执行 `setState(Processing)` 之前，存在一个状态窗口——此时任务仍是 `Paused`，但已有 goroutine 待运行。

`PauseTaskTree` 对 `Paused` 状态的子任务调用 `Pause()`，但 `Pause()` 要求 `Processing` 状态，返回错误被静默跳过。goroutine 随后启动继续运行，无人能暂停它。

### setState 调用现状

当前所有 `setState` 调用方（按文件分组）：

**model.go（任务自身）— 执行阶段状态变更：**
| 行号 | 调用 | 语义 |
|------|------|------|
| 303 | `setState(Processing)` | `run()` 开始执行 |
| 331 | `setState(WaitingForInput)` | 等待用户确认重复 |
| 555 | `setState(Finished)` | 下载完成 |
| 594 | `setState(Paused)` | `drainAndPause` 完成 |
| 608 | `setState(Processing)` | `resumeFromPersistedState` 开始 |
| 724 | `setState(Pausing)` | `Pause()` 进入暂停中 |
| 730 | `setState(Paused)` | `Pause()` setup 阶段暂停 |
| 776 | `setState(Stopping)` | `Stop()` 进入停止中 |
| 830 | `setState(Failed)` | `setFailed()` 失败 |

**manager.go（Manager）— 生命周期管理状态变更：**
| 行号 | 调用 | 语义 |
|------|------|------|
| 358 | `setState(WaitingForInput)` | 批量预检命中重复 |
| 383 | `setState(Waiting)` | 入等待队列 |
| 473 | `setState(Paused)` | `PauseTaskTree` 处理排队任务 |
| 632, 663 | `setState(task.Status)` | 跳过重复任务，恢复 DB 状态 |
| 706 | `setState(Created)` | `GracefulShutdown` 处理等待确认任务 |

### 状态所有权设计原则

**谁拥有当前阶段，谁负责状态变更。** 划分为两层：

- **Manager（生命周期管理）**：负责任务的调度/入队/移队
  - `Created → Waiting`（入队：`tryDispatch`）
  - `Waiting → Paused`（出队暂停：`PauseTaskTree`）

- **Task goroutine（执行阶段）**：负责任务的实际执行
  - `Created/Waiting/Paused → Processing`（开始执行：`run()`/`resumeFromPersistedState()`）
  - `Processing → Finished/Failed/Paused/Pausing`（执行结果）

**关键约束：Manager 不设置 Processing，goroutine 不设置 Waiting。**

## 修复方案

### 1. `manager.go` - `PauseTaskTree`：增加 Paused 状态处理

当前只处理 `Waiting`。`Paused` 状态有两种含义：
- **真正暂停**（goroutine 已退出，无人运行）→ 无需操作
- **已派发但 goroutine 未启动**（`prepareForResume` + `tryDispatch` 后的状态窗口）→ 需要 `cancel()` 使 goroutine 启动时退出

两者通过 `cancel()` 统一处理：对已退出的 goroutine 无副作用，对未启动的 goroutine 令其退出。

```go
for _, child := range parent.GetChildren() {
    state := child.GetState()
    switch state {
    case TaskStateWaiting:
        m.removeFromQueue(child.taskId)
        child.cancel()
    case TaskStatePaused:
        child.cancel()
    case TaskStateProcessing, TaskStatePausing:
        if err := child.Pause(); err != nil {
            logger.Log.Errorf("[TaskManager] 暂停子任务 %d 失败: %v", child.taskId, err)
        }
    }
}
```

> **为什么 `cancel()` 是正确的：** `prepareForResume` 创建了新 context。`cancel()` 取消这个新 context。当 goroutine 启动时，它检查 `ctx.Done()`，发现已取消，直接退出。对于真正 Paused 的任务（无 goroutine 待运行），cancel 只是多取消了一个不会被使用的 context，`prepareForResume` 会在下次 resume 时创建新 context，无副作用。

### 2. `manager.go` - `executeTask`：goroutine 启动防护

goroutine 启动时检查 context 是否已被取消（被 `PauseTaskTree` 的 `cancel()` 取消）：

```go
func (m *Manager) executeTask(task *ManagedTask) {
    defer func() {
        if r := recover(); r != nil {
            logger.Log.Errorf("[TaskManager] executeTask panic: %v", r)
        }
        <-m.semaphore
        m.dispatchFromQueue()
    }()

    // 新增：检查任务是否在 goroutine 启动前已被暂停/停止
    // PauseTaskTree/StopTaskTree 对 Paused/Waiting 状态的任务调用 cancel()
    // 如果 goroutine 启动时 context 已取消，直接退出（状态已由调用方设置）
    select {
    case <-task.ctx.Done():
        return
    default:
    }

    var result runResult
    // ... 后续不变
}
```

### 3. `model.go` - `run()`/`resumeFromPersistedState()`：防止覆盖已暂停状态

处理极端竞态：`executeTask` 的 `ctx.Done` 检查通过后、`run()` 执行 `setState(Processing)` 前，`Pause()` 刚好执行了 `cancel() + setPaused`。

```go
func (m *ManagedTask) run() runResult {
    // 新增：防止 goroutine 覆盖已被 Pause() 设置的暂停状态
    if m.ctx.Err() != nil {
        if s := TaskState(m.state.Load()); s == TaskStatePausing || s == TaskStatePaused {
            return runResultPaused
        }
    }
    m.setState(TaskStateProcessing)
    // ... 后续不变
}

func (m *ManagedTask) resumeFromPersistedState() runResult {
    defer func() { ... }()

    // 新增：同 run() 的防护
    if m.ctx.Err() != nil {
        if s := TaskState(m.state.Load()); s == TaskStatePausing || s == TaskStatePaused {
            return runResultPaused
        }
    }
    m.setState(TaskStateProcessing)
    // ... 后续不变
}
```

### 4. `manager.go` - `StopTaskTree`：同步增加 cancel

`StopTaskTree` 对 Paused 状态的任务调用 `setFailed`，但缺少 `cancel()`——若任务已派发但 goroutine 未启动，goroutine 会覆盖 Failed 状态继续运行。增加 `cancel()`：

```go
case TaskStatePaused:
    child.cancel()              // 新增：确保已派发的 goroutine 退出
    child.setFailed("任务被用户停止")
```

## 修复后的状态流转

```
ResumeTaskTree:
  prepareForResume()   → 状态保持 Paused（不变）
  tryDispatch()        → 不修改状态
                        go m.executeTask(task)

executeTask (goroutine):
  ctx.Done 检查        → 已取消则退出
  run()/resume()       → setState(Processing)  ← 执行阶段（Task goroutine）

PauseTaskTree (修复后):
  Paused               → cancel()              ← 状态已是 Paused，无需再设 ✅
  Waiting              → removeFromQueue + cancel() ✅
  Processing/Pausing   → Pause() 正常工作 ✅
  Stopping/Finished/Failed → 已终态/过渡态，跳过 ✅
```

### 各场景验证

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| goroutine 未调度时暂停 | Paused → Pause() 失败 → 跳过 ❌ | Paused → cancel() → goroutine 启动即退出 ✅ |
| goroutine 已启动后暂停 | 正常 ✅ | 不变 ✅ |
| 在队列中时暂停 | 只 removeFromQueue，未 cancel ❌ | removeFromQueue + cancel() ✅ |
| dispatchFromQueue 后 goroutine 未调度 | Waiting 已出队，跳过 ❌ | Waiting → cancel() ✅ |
| Pause cancel 后 goroutine 才启动 | goroutine 覆盖 Paused 继续运行 ❌ | executeTask 检查 ctx.Done 退出 ✅ |
| 极端竞态：Pause 与 run() 交错 | run() 覆盖 Paused ❌ | run() 检查 ctx+state 防护 ✅ |

## 涉及文件

- `backend/taskManager/model.go`：`run()`、`resumeFromPersistedState()`
- `backend/taskManager/manager.go`：`PauseTaskTree`、`StopTaskTree`、`executeTask`
