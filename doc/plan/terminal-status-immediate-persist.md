# 终态即时落盘（任务图 H）

> 关联：多轨资源谱系 · 节点 H「终态即时落盘」；来源 `task-manager-followup-improvements.md` ③（该计划整体已废弃，唯③仍有效，本文取代之）。
> 范围：`backend/taskManager/manager.go`；附带清理 `backend/task` 的 dead code `SetStatus`（可选）。

## 背景与问题

`addToPending`（manager.go）把任务状态变更写入 `pendingStatusUpdates`，由 `flushLoop` 每 200ms 批量刷库（`doFlush`）。**所有稳定态都走这条批量通道**，包括终态（`Finished`/`Failed`/`PartlyFinished`）。

问题：

1. **崩溃窗口**：终态产生后最长 200ms 内可能未落盘。进程在此窗口崩溃（kill/断电/panic），重启从 DB 读到的状态滞后于实际——本应 Finished 的任务停在 Processing/Paused。
2. **写不一致**：`addToPending` 对终态**已经**同步调用 `repo.UpdateRedownloadSections` 清空执行模式（即时写），但**状态本身**却走批量——同一终态事件，一列即时、一列批量，语义割裂。
3. **优雅关闭不可靠**：`flushLoop` 在 `closeCh` 触发时做最后一次 `doFlush`。若终态事件尚未被 flush（在 200ms 窗口内）就收到关闭，依赖最后一次 flush 兜底；但更早的崩溃窗口无兜底。

目标：**终态状态即时落盘**；非终态（`Paused`、进度、pending_resource_id）维持批量（避免进度高频写放大）。

## 方案

### 分流：终态即时 / 非终态批量

`addToPending` 入口按状态分流：

- **终态**（`isClearableTerminal`：Finished/Failed/PartlyFinished，已不含 Paused）：**不入** `pendingStatusUpdates`，同步调用 `repo.BatchSetStatus` 传**单条 map** 即时写该任务状态（含 error_message）。
- **非终态**（`Paused`）：维持原批量逻辑（写入 `pendingStatusUpdates` + 通知 `flushCh`）。
- 进度（`pendingProgressUpdates`）、`pending_resource_id`（`pendingResourceIDUpdates`）路径完全不变。

**为什么用 `BatchSetStatus` 单条 map 而非 `SetStatus`**：终态 `Failed` 需带 `error_message`（1121 行 `addToPending(taskId, …, errMsg)`），父任务终态 `errMsg=""` 需清空 error_message。`BatchSetStatus` 的 CASE WHEN 同时写 status + error_message，语义与原批量路径一致；`SetStatus` 不带 error_message，且无调用方（见附带清理）。

### 并发正确性：status 写路径串行化在 `pendingMu` 内

朴素实现（即时写锁内、`doFlush` 锁外）有**覆盖竞态**：

```
doFlush: Lock → swap 取出 {X:Processing} → Unlock          // 快照已飞走
即时写:  Lock → delete(map空) → BatchSetStatus(X:Finished) → DB=Finished → Unlock
doFlush: BatchSetStatus({X:Processing})  → DB=Processing   // 过时值覆盖 Finished ❌
```

根因：`doFlush`「取快照（锁内）」与「写库（锁外）」分离，即时写插在中间，doFlush 用旧快照覆盖。

**解法**：把 `doFlush` 的 status 写库（`BatchSetStatus`）也移入 `pendingMu` 临界区，与即时写互斥。锁内原子完成「swap status map + 写库」，两者无交错：

- doFlush 先持锁：写 Processing → 释放；即时写再持锁写 Finished → 释放。最终 Finished ✓
- 即时写先持锁：删 map[X]、写 Finished → 释放；doFlush 再持锁取到空 map，不写 X。最终 Finished ✓

`pending_resource_id` 与进度推送维持锁外写/推（与 status 不同列、不同通道，无覆盖竞态，且避免不必要地延长锁持有）。

### 锁持有代价（已论证可接受）

`doFlush` 持 `pendingMu` 期间多做一次 `BatchSetStatus`（单条/少量 DB 写，~5-10ms）。期间被阻塞：

- `SetOnProgress`（写 `pendingProgressUpdates`，高频）：progress map 为覆盖写，仅延迟推送、不丢数据，200ms 合并窗口本就容忍该延迟。
- `onResourceIDUpdate`、非终态 `addToPending`、即时写终态：均低频。

锁内仅做 status 的 swap + write（其余 swap 紧随其后立即 Unlock），最小化持锁段。

## 改动点

### 1. `backend/taskManager/manager.go` — `addToPending` 分流

当前（1243-1265）无条件把状态塞进 map，再对终态即时清执行模式。改为：

```go
// addToPending 添加状态变更:终态即时落盘,非终态进批量通道由 flushLoop 刷库
func (m *Manager) addToPending(taskId int64, status task.TaskStatusEnum, errMsg string) {
    if isClearableTerminal(TaskState(status)) {
        // 终态即时落盘:同步写库,不进批量通道,消除 200ms 崩溃窗口
        m.pendingMu.Lock()
        // 清掉该任务可能残留的过时非终态快照(如 Paused 未刷又被终态覆盖),避免 doFlush 用旧值回写
        delete(m.pendingStatusUpdates, taskId)
        if err := m.repo.BatchSetStatus(context.Background(), map[int64]task.StatusUpdate{
            taskId: {Status: status, ErrorMessage: sql.NullString{String: errMsg, Valid: errMsg != ""}},
        }); err != nil {
            logger.Log.Errorf("[TaskManager] 即时写入任务 %d 终态 %d 失败: %v", taskId, status, err)
        }
        m.pendingMu.Unlock()

        // 终态清空执行模式持久化(StoreRoles/IncludeWorkInfo 仅为在途任务服务,完成后保留无意义且会泄漏到下次执行)
        if err := m.repo.UpdateRedownloadSections(context.Background(), []int64{taskId}, sql.NullString{}, false); err != nil {
            logger.Log.Warnf("[TaskManager] 清空任务 %d 终态执行模式失败: %v", taskId, err)
        }
        return
    }

    // 非终态(Paused):进批量通道
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
```

要点：
- 终态分支 `return` 前不通知 `flushCh`（无新批量值需刷；进度有独立触发）。
- `delete(pendingStatusUpdates, taskId)` 是兜底：清理终态前可能积压的 Paused 快照。
- 即时写的 `BatchSetStatus` 与下方 `doFlush` 的 `BatchSetStatus` 都在 `pendingMu` 内 → 互斥。

### 2. `backend/taskManager/manager.go` — `doFlush` 把 status 写移入锁内

当前（1198-1241）：锁内仅 swap，锁外写三类。改为：status 的 swap + 写库在锁内完成，resourceID/progress 维持锁外：

```go
func (m *Manager) doFlush() {
    m.pendingMu.Lock()
    if len(m.pendingStatusUpdates) == 0 && len(m.pendingResourceIDUpdates) == 0 && len(m.pendingProgressUpdates) == 0 {
        m.pendingMu.Unlock()
        return
    }
    pendingStatus := m.pendingStatusUpdates
    m.pendingStatusUpdates = make(map[int64]task.StatusUpdate)
    // status 写库在锁内完成:与 addToPending 终态即时写互斥,杜绝 doFlush 用过时快照覆盖刚写入的终态
    if len(pendingStatus) > 0 {
        if err := m.repo.BatchSetStatus(context.Background(), pendingStatus); err != nil {
            logger.Log.Errorf("[TaskManager] 批量写入任务状态失败: %v", err)
        }
    }
    pendingResourceIDs := m.pendingResourceIDUpdates
    m.pendingResourceIDUpdates = make(map[int64]sql.NullInt64)
    pendingProgress := m.pendingProgressUpdates
    m.pendingProgressUpdates = make(map[int64]*taskScheduleDTO)
    m.pendingMu.Unlock()

    // pending_resource_id 与进度推送维持锁外
    if len(pendingResourceIDs) > 0 {
        if err := m.repo.BatchUpdatePendingResourceID(context.Background(), pendingResourceIDs); err != nil {
            logger.Log.Errorf("[TaskManager] 批量写入 pending_resource_id 失败: %v", err)
        }
    }
    if len(pendingProgress) > 0 {
        batch := make([]*taskScheduleDTO, 0, len(pendingProgress))
        for _, dto := range pendingProgress {
            batch = append(batch, dto)
        }
        m.deps.Pusher.PushProgressBatch(batch)
    }
}
```

（原 doFlush 内的逐条 `logger.Log.Infof("doFlush: taskId=…")` 调试日志可保留或精简，非必须。）

### 3. 附带清理（可选，DEAD_CODE_CLEANUP）

`SetStatus` 全仓无调用方（仅接口声明 + 实现）。即时写复用 `BatchSetStatus` 后确认仍无引用，移除三处：

- `backend/taskManager/manager.go:25` — `Repository` 接口方法
- `backend/task/service.go:100` — service 接口方法
- `backend/task/repository.go:176-179` — 实现

> 若希望本次改动聚焦 manager.go，可跳过此清理，单独提一次 chore。

## 验证

1. **终态落盘即时性**：任务下载完成（Finished）/ 失败（Failed）后，在 `BatchSetStatus` 返回处立即 kill 进程（或断点停在返回后、flush 前），重启读 DB，断言状态为 Finished/Failed（而非 Processing/Paused）。
2. **覆盖竞态回归**：构造「Processing 刚入批量通道（未 flush）→ 立即 Finished」的紧邻时序（小板块快速任务），断言 DB 最终为 Finished，不被过时 Processing 覆盖。`go test -race ./backend/taskManager/...` 确认即时写与 flushLoop 无 data race。
3. **进度批量无回归**：`pendingProgressUpdates` 路径不变，断言进度仍走批量推送（无逐 chunk 写放大）；`onResourceIDUpdate` 批量不变。
4. **优雅关闭**：终态事件后立即触发 `closeCh`（不等 200ms），断言 DB 终态正确（即时写已落盘，不依赖最后一次 flush）。
5. **父任务终态**：多子任务全部完成触发父任务 PartlyFinished（924/962/1141 行路径），断言父任务终态即时落盘、error_message 被清空。

## 风险与权衡

- **批量退化（228 行循环）**：`loadAndStartTaskTrees` 对「已终态的独立任务」在循环内逐个 `addToPending`，原批量合并为 1 次，现变 N 次即时写。该场景仅出现在「一次性开始多个已完成任务」的冷启动，N 通常小且单条 UPDATE 快，可接受；且这些任务状态本就源于 DB，即时写是补全/清执行模式，符合落盘目标。不特判。
- **锁内写 DB 阻塞进度回调 ~10ms**：见上「锁持有代价」，progress 覆盖写不丢数据，200ms 合并窗口容忍。
- **即时写失败**：`BatchSetStatus` 返回错误仅记日志（与原 doFlush 批量写失败处理一致），不回退到批量通道——终态写失败属异常，下次重启会从内存/资源重建，不在此兜底。

## 关联

- 任务图：`.claude/workflow/active/multitrack-resource-lineage/TREE.md` 节点 H
- 来源（已废弃，唯③有效）：`doc/plan/task-manager-followup-improvements.md` ③
- 相关不变量：磁盘 stat 仍是唯一权威（D 约束），本次仅改状态列落盘时机，不动资源/续传链路
