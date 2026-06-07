# 任务事件推送时序问题修复进展

## 问题描述

`ConfirmReplaceBatch` 批量跳过任务时，循环中每次迭代通过 `setState` → `onStateChange` 回调 + `cleanupFinishedTask` 跨多个 Wails topic 发射消息。Wails IPC 对同 topic 保证 FIFO，但跨 topic 不保证顺序，导致前端收到的消息乱序。

## 已完成的工作

### 阶段一：统一状态控制（已提交 e53b2d5）

**问题**：`removeWaitingTask` 方法手动重写了 `onStateChange` 回调和 `cleanupFinishedTask` 中的全部逻辑（状态推送、父任务刷新、DB 持久化、移除清理），形成两条独立的状态控制路径。

**修复**：
1. 移除 `removeWaitingTask` 方法（~60 行）
2. `ConfirmReplace` / `ConfirmReplaceBatch` 的 skip 分支改为 `setState(原始DB状态)` + `cleanupFinishedTask` + `cancel`
3. `AllChildrenTerminal` 将 `Created` 视为终态（跳过回退的任务状态为 Created，需视为终态才能正确触发父任务清理）
4. 删除无调用方的 `RemoveChild` 方法

**涉及文件**：
- `backend/taskManager/manager.go`：调用方替换
- `backend/taskManager/model.go`：AllChildrenTerminal 终态扩展 + RemoveChild 删除

### 阶段二：合并 Topic + 事件信封（已实现未提交）

**方案**：将 7 个 Wails event topic 合并为 2 个，利用同 topic FIFO 保证消除跨 topic 乱序。

| 合并后 topic | 原有 topic | 事件类型标识 |
|---|---|---|
| `task-events` | `taskStatus-updateTask`、`taskStatus-updateSchedule`、`taskStatus-removeTask` | `updateTask`、`updateSchedule`、`removeTask` |
| `parent-events` | `parentTaskStatus-updateParentTask`、`parentTaskStatus-updateSchedule`、`parentTaskStatus-removeParentTask` | `updateParentTask`、`updateParentSchedule`、`removeParentTask` |
| `taskStatus-duplicateDetected` | 保持不变 | — |

每个事件包装为信封 `{ type, data }`，前端按 type 分发到对应 store action。

**涉及文件**：
- `backend/taskManager/progress_pusher.go`：新增 `ipcEvent` 信封结构，`WailsTaskProgressPusher` 的 7 个 Push 方法改用 `emitTaskEvent`/`emitParentEvent`，合并到 2 个 topic
- `frontend/src/MainIpcListener.ts`：7 个 `Events.On` 监听合并为 2 个统一监听 + switch 分发

**`TaskProgressPusher` 接口和所有调用方（`manager.go`）不变。**

## 当前问题：topic 合并后仍然乱序

合并 topic 后前端日志显示：

```
update task 20 undefined        ← task-events topic 的 updateTask 事件
update P task 17 2               ← parent-events topic 的 updateParentTask 事件
update task 21 undefined         ← 后续的 updateTask
update P task 17 2               ← 后续的 updateParentTask
...
update task 20 9                 ← ← 第二次收到 task 20 的 updateTask（status=9 即 Created）
setTimer task 20                 ← removeTask 到达，设置延迟删除
...
update P task 17 6               ← 父任务最终状态 Finished
setTimer P task 17               ← 父任务 removeParentTask 到达
remove task 20                   ← 延迟删除触发
...
remove P task 17                 ← 父任务延迟删除触发
remove task 19                   ← 最后一个子任务删除
```

### 关键观察

1. **同一个 task 收到两次 updateTask**：task 20 先收到 `undefined`（status 未设置），后又收到 `status=9`（Created）。这意味着同 topic 内事件出现了重复或时序异常。

2. **两个 topic 之间仍乱序**：`task-events` 和 `parent-events` 是两个独立 topic，它们之间仍无 FIFO 保证。`updateTask(task 20)` 和 `updateParentTask(parent 17)` 交替出现。

3. **同 topic 内的 FIFO 似乎也被破坏**：task 20 在同一 `task-events` topic 中先收到 `undefined` status，后收到 `status=9`。如果后端在同 topic 内只发射了一次 updateTask(task 20)，那前端不应收到两次。需要排查：
   - 后端是否对同一任务发射了两次 updateTask（一次来自 `batchCheckDuplicates` 的 `setState(WaitingForInput)`，一次来自 `ConfirmReplaceBatch` 的 `setState(Created)`）
   - Wails `Emit` 是否在内部对事件进行了异步分批处理

4. **后端日志时序分析**：
   - `14:00:42.867`：`batchCheckDuplicates` 中 7 个任务逐一 `setState(WaitingForInput)` → 各触发 `onStateChange` → `PushStateChange` + `PushParentStateChange` + `PushParentProgress`
   - `14:00:44.207`：`ConfirmReplaceBatch` 循环中 7 个任务逐一 `setState(Created)` → 各触发 `onStateChange` + `cleanupFinishedTask`
   - 两次批量操作间隔约 1.3 秒，各自在几毫秒内完成，但前端收到的消息混杂了两次操作的事件

### 推测根因

**两次批量操作（`batchCheckDuplicates` 和 `ConfirmReplaceBatch`）各自在紧密循环中发射大量事件，Wails 的 `Emit` 实现可能使用了异步事件队列，导致即使同一 topic 内也不保证严格的发射顺序到达到顺序的映射。**

另一种可能：`batchCheckDuplicates` 中的事件和 `ConfirmReplaceBatch` 中的事件确实通过同一 topic 到达，但由于两次操作的事件在时间上非常接近，前端的 `Events.On` 回调可能在宏任务队列中与定时器、微任务等交替执行，导致处理顺序与到达顺序不一致。

### 下一步方向

1. **排查 Wails Emit 的实现**：确认 `Emit` 是否为同步调用，以及前端 `Events.On` 回调的执行模型（宏任务/微任务/直接调用）
2. **验证同 topic 内是否真的保证 FIFO**：添加事件序号，在前端验证是否严格按序到达
3. **考虑更激进的方案**：将所有事件（包括 task 和 parent）合并为单个 topic `task-manager-events`，彻底消除跨 topic 乱序
4. **或者考虑前端缓冲方案**：前端收到事件后按序号排序再处理，容忍一定延迟
