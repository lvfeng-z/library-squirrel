# LibrarySquirrel 任务执行流程文档

本文档详细描述任务从创建到完成的完整生命周期，包括状态模型、执行步骤、事务边界、补偿机制和崩溃恢复。

## 关键文件

| 组件 | 文件路径 |
|------|---------|
| 任务状态、ManagedTask、run()、downloadLoop | `backend/taskManager/model.go` |
| Manager（调度、信号量、flush） | `backend/taskManager/manager.go` |
| Task Service（创建、重试） | `backend/task/service.go` |
| Task Repository（DB 操作） | `backend/task/repository.go` |
| 备份还原 | `backend/backup/store_backup_orchestrator.go` |
| 事务基础设施 | `backend/database/transaction.go`、`backend/database/tx_context.go` |
| PersistentStore 操作 | `backend/persistentStore/service.go` |
| 插件交互桥接 | `backend/plugin/extension/task_executor.go` |
| 依赖注入 | `app.go`（`initAdvancedServices`） |

---

## 任务状态模型

### 状态定义

| 值 | 状态 | 类别 | 说明 |
|:---:|------|:---:|------|
| 0 | `Created` | 瞬态 | 已创建未启动；DB 初始状态 |
| 1 | `Waiting` | 瞬态 | 信号量已满，排队等待 |
| 2 | `Processing` | 瞬态 | 正在执行 |
| 3 | `Pausing` | 瞬态 | 暂停进行中 |
| 4 | `Paused` | **稳定** | 已暂停，goroutine 已退出 |
| 5 | `Stopping` | 瞬态 | 停止进行中 |
| 6 | `Finished` | **稳定** | 下载完成 |
| 7 | `Failed` | **稳定** | 失败或被用户停止 |
| 8 | `PartlyFinished` | **稳定** | 父任务专用：部分子任务完成、部分失败 |
| 9 | `WaitingForInput` | 瞬态 | 检测到重复，等待用户确认替换/跳过 |

**稳定 vs 瞬态**：只有稳定状态（`Paused`、`Finished`、`Failed`、`PartlyFinished`）通过 `doFlush` 批量持久化到 DB。瞬态状态仅存在于内存和前端推送中。`WaitingForInput` 永不持久化。

### 状态转换图

```
                         ┌──────────────────────────────────────────────────────────┐
                         │                      任务生命周期                          │
                         └──────────────────────────────────────────────────────────┘

  Created ──────► Processing ──────► Finished
    │                 │  ▲               ▲
    │                 │  │               │
    │                 │  │  (信号量释放    │
    │                 │  │   后重新调度)   │
    │                 │  │               │
    │                 ▼  │               │
    │              Waiting ──────────────┘ (排队等待，非执行路径)
    │                 │
    │                 │ (duplicate detected)
    │                 ▼
    │          WaitingForInput ──┬── (skip) ──► 清理退出
    │                            │
    │                            └── (replace) ──► Processing (2nd run)
    │                                                  │
    │                          ┌────────────────────────┤
    │                          │                        │
    │                          ▼                        ▼
    │                       Paused                  Failed
    │                          │                        ▲
    │                          │                        │
    │                   (resume) ──► Processing          │
    │                                           │       │
    │                                    (stop) ┤  (error)
    │                                           │       │
    │                                           ▼       │
    │                                       Stopping ───┘
    │
    │  (app restart, load from DB)
    └── Paused (DB) ──► resumeFromPersistedState ──► downloadLoop ──► Finished/Failed/Paused
```

### 父任务状态聚合

父任务不直接执行，其状态由子任务聚合得出（`ParentTask.RefreshState`）：

| 子任务状态 | 父任务聚合结果 |
|-----------|--------------|
| 存在 Processing/Pausing/Stopping/WaitingForInput | `Processing` |
| 存在 Waiting | `Waiting` |
| 存在 Paused（其余为终态） | `Paused` |
| 全部 Finished | `Finished` |
| 全部 Failed | `Failed` |
| 混合 Finished + Failed | `PartlyFinished` |

---

## 任务生命周期

### 阶段 1：任务创建

两条创建路径：

**路径 A：URL 触发**（`task/service.go` `CreateTaskByURL`）

```
CreateTaskByURL(ctx, url)
  → urlListener.ListListener(url)          // 查找匹配 URL 模式的插件
  → taskHandler.Create(url)                 // 调用插件 RPC
  → 判断返回类型：
      IsStream = true  → handleCreateTaskStream(...)   // 流式创建
      IsStream = false → handleCreateTaskArray(...)    // 批量创建
```

**路径 B：直接创建**（`task/service.go` `CreateTask`）

单条 INSERT，无事务包装。初始状态：`Created`。

**批量创建事务**（`handleCreateTaskArray`）：

- 单个子任务：直接 `repo.CreateTask`（单条 INSERT）
- 多个子任务：**事务内**创建父任务（`HasChild=true`）+ 全部子任务（`Pid=parentId`），保证父子原子性

**流式创建**（`handleCreateTaskStream`）：

- 单条 INSERT 创建父任务
- goroutine 异步读取 channel，批量 `SaveBatch` 创建子任务（每批 100 条）

### 阶段 2：任务启动

前端调用 `TaskManagerHandler.StartTaskTree` → `Manager.loadAndStartTaskTree`。

```
loadAndStartTaskTree(taskId)
  → repo.ListTaskTree(taskId)                    // 递归 CTE 查询任务树
  → 判断结构：
      独立任务（pid=0, hasChild=false）→ 直接创建 ManagedTask
      父子任务 → 创建 ParentTask + 为每个子任务创建 ManagedTask
  → buildOrReuseChild(task)                       // 构建或复用任务
      - DB 状态为 Paused 且 PendingResourceID 有效 → resumeFromDB=true
      - DB 状态为 Paused 但无 PendingResourceID → 从头执行
  → batchCheckDuplicates(children)                // 批量重复检测
      - 查询 workChecker.ListBySiteAndSiteWorkIDs
      - 重复 → WaitingForInput + 前端推送 DuplicateDetected
      - 不重复 → skipDuplicateCheck=true
  → tryDispatch(task)                             // 信号量调度
      - 成功获取信号量 → go executeTask(task)
      - 信号量已满 → 加入 waitingQueue + Waiting 状态
```

### 阶段 3：任务执行

`Manager.executeTask` 在独立 goroutine 中运行：

```go
func (m *Manager) executeTask(task *ManagedTask) {
    defer: recover panic, 释放信号量, dispatchFromQueue

    if task.resumeFromDB → task.resumeFromPersistedState()
    else                → task.run()
}
```

#### 3A 全新执行 — run()

**defer 栈**（LIFO，后注册先执行）：

| 顺序 | defer | 作用 |
|:---:|-------|------|
| 1（最先执行） | `recover()` | panic 恢复，调用 `setFailed()` |
| 2（最后执行） | 备份还原检查 | 如果最终状态为 `Failed` 且有备份项，调用 `RestoreAllStores`。使用 `context.Background()` 确保任务取消后仍能执行 |

**逐步执行流程**：

| 步骤 | 操作 | DB 操作 | 事务 |
|:---:|------|---------|:---:|
| 0 | 检查 workdir 是否已配置 | 无 | - |
| 0.0 | 重复检测（`workChecker.GetBySiteAndSiteWorkID`） | SELECT | - |
| 0.1 | 替换场景备份（`BackupAllStores`） | SELECT + DELETE/备份 per store | - |
| 1 | `pluginExec.CreateWorkInfo`（插件 RPC） | 无 | - |
| 2 | `workInfoSaver.SaveWorkInfo` | 批量 INSERT（Work、作者、标签等） | **事务 1** |
| 3 | `pluginExec.Start`（插件 RPC，返回数据流） | 无 | - |
| — | 解析文件保存路径（`resolveLocalPath`） | 无 | - |
| 4 | `storeStreamer.StoreStream`：创建磁盘文件 + PersistentStore DB 记录 | INSERT persistent_store | **事务 2** |
| 5 | `saveResource`：保存或更新 Resource | INSERT/UPDATE resource | **事务 2** |
| 6 | `UpdatePendingResourceID`：设置任务待处理资源 ID | UPDATE task.pending_resource_id | **事务 2** |
| 7 | 保存下载状态引用 | 无 | - |
| 8 | 进入 `downloadLoop()` | 无 | - |

**事务 2 详解**（步骤 4-6）：

```
transactor.ExecInTransaction(context.Background(), func(txCtx) {
    // 4. StoreStream：创建文件 + PersistentStore DB 记录
    storeId, writer = storeStreamer.StoreStream(txCtx, relPath, fileName)

    // 5. saveResource：替换场景更新 / 新建场景创建 Resource
    resourceId = saveResource(txCtx, workId, storeId, startResp)

    // 6. 更新 pending_resource_id（崩溃恢复标记）
    task.PendingResourceID = {Int64: resourceId, Valid: true}
    pendingResourceUpdater.UpdatePendingResourceID(txCtx, taskId, ...)
})
```

- 传入 `context.Background()` 而非 `m.ctx`，确保任务取消不会中断事务提交
- 步骤 4 的文件创建发生在事务外行为（磁盘 I/O），但 DB 记录参与事务
- **事务失败**时：DB 自动回滚，磁盘文件通过 `writer.Close()` + `storeFileCleaner.CleanupFile()` 显式清理

**saveResource 的两种场景**：

| 场景 | 条件 | 行为 |
|------|------|------|
| 替换 | `isReplace=true` | 查询已有 Resource，更新 `WorkStoreID` 和 `ResourceComplete=0` |
| 新建 | 首次执行 | 创建新 Resource（`WorkID`、`TaskID`、`WorkStoreID`、`ResourceComplete=0`） |

#### 3B 断点续传 — resumeFromPersistedState()

应用重启后，`loadAndStartTaskTree` 发现 DB 中 `Paused` 且 `PendingResourceID` 有效的任务，设置 `resumeFromDB=true`。

```
resumeFromPersistedState()
  → 加载 Resource（resourceReader.GetById）
  → 加载 PersistentStore（storeReader.GetById）
  → 检查本地文件存在（os.Stat）
  → 任一步失败 → 降级为完整 run()
  → pluginExec.Resume（传入已下载字节数）
  → 检查插件响应：
      Continuable=true  → ResumeStream（追加模式打开文件）
      Continuable=false → StoreStream（从头创建新文件）
  → downloadLoop()
```

#### downloadLoop 详解

```
downloadLoop:
  defer: close(reader)
  for {
      select {
      case <-pauseCh:    // 暂停信号
          drainReader(buf)          // 排空缓冲区数据
          writer.Sync() + Close()   // 关闭文件句柄（DB 记录保留未完成状态）
          close(drainDone)
          setState(Paused)
          return runResultPaused

      case <-ctx.Done():  // 取消/停止
          writer.Abort()            // 删除文件 + DB 记录
          setFailed("任务被取消")
          return runResultDone

      default:
          n = reader.Read(buf)
          writer.Write(buf)
          if EOF:
              writer.Complete()      // 同步 + 关闭 + DB 状态=完成
              校验下载完整性
              saveThumbnail()        // 获取并保存缩略图
              clearPendingResourceID()  // 清除崩溃恢复标记
              setState(Finished)
              return runResultDone
          if error:
              if Pausing/Paused → drain + close → Paused
              else → Abort + setFailed → Failed
  }
```

**暂停 drain 机制**：暂停时调用 `pluginExec.Pause()` 关闭上游，`downloadLoop` 读取剩余缓冲区数据并写入文件，确保不丢数据。Writer 执行 `Sync()` + `Close()`（不调用 `Complete`），DB 记录保持 `Incomplete` 状态，`PendingResourceID` 保持有效。

### 阶段 4：暂停与恢复

**暂停流程**：

```
Manager.PauseTaskTree(taskId)
  → 遍历子任务：
      Waiting → 移出队列，直接 Paused
      其他    → child.Pause()

ManagedTask.Pause():
  → 前置条件：当前状态为 Processing
  → setState(Pausing)
  → Setup 阶段（currentReader == nil）：取消 context，直接 Paused
  → Download 阶段：
      1. 创建 drainDone channel
      2. 发送 pauseCh 信号 → downloadLoop 开始 drain
      3. pluginExec.Pause() → 关闭插件上游
      4. 等待 <-drainDone → downloadLoop 完成排空 + 关闭 writer
      5. goroutine 退出，信号量释放
```

**恢复流程**：

```
Manager.ResumeTaskTree(taskId)
  → 任务在内存中：prepareForResume() + tryDispatch()
  → 任务不在内存（跨重启）：loadAndStartTaskTree(skipTerminal=true)

ManagedTask.prepareForResume():
  → 取消旧 context，创建新 context
  → 重置 pauseCh、currentReader、storeWriter、totalWritten
  → resumeFromDB = task.PendingResourceID.Valid
```

### 阶段 5：停止

```
Manager.StopTaskTree(taskId)
  → 遍历子任务：
      Paused  → 直接 setFailed
      Waiting → 移出队列 + 取消 context + setFailed
      Running → child.Stop()

ManagedTask.Stop():
  → setState(Stopping)
  → 取消 context（中断 downloadLoop）
  → storeWriter.Abort()（清理文件 + DB 记录）
  → pluginExec.Stop()
  → setFailed("任务被用户停止")
```

### 阶段 6：重试

```
Manager.RetryTaskTree(taskId)
  → loadAndStartTaskTree(taskId, skipTerminal=false)
  → 从 DB 重新加载任务树，创建新 ManagedTask 实例
  → 不重置 DB 状态，任务以当前 DB 值开始执行
```

### 阶段 7：完成与清理

```
Manager.cleanupFinishedTask(mt)  // executeTask defer 调用
  → 从 taskMap 移除任务
  → 如果有父任务：
      检查所有兄弟任务是否终态
      → 是：RefreshState() 聚合父状态 + 持久化
             移除父任务 + 所有子任务
      → 推送 PushTaskRemove 给前端
```

### 阶段 8：重复确认

```
Manager.ConfirmReplace(taskId, action):
  "skip":    恢复 DB 状态，清理任务映射，取消 context
  "replace": skipDuplicateCheck=true → tryDispatch → run() 从步骤 0.1 开始
```

批量版本 `ConfirmReplaceBatch` 并行处理多个任务。

---

## 事务边界

| 名称 | 范围 | 位置 |
|------|------|------|
| **事务 1** | SaveWorkInfo：Work + 作者 + 标签 + WorkSet 等周边数据 | `work/service.go` `SaveWorkInfo` |
| **事务 2** | StoreStream DB 记录 + saveResource + UpdatePendingResourceID | `taskManager/model.go` `run()` 步骤 4-6 |
| **事务 3** | 任务创建：父任务 + 全部子任务 | `task/service.go` `handleCreateTaskArray` |

### 事务基础设施

```
dbTransactorAdapter (app.go)
  → database.WithTransactionContext(ctx, db, func(tx))
    → context.WithValue(ctx, TxKey, tx)    // 注入事务 DB 到 context

BaseRepository.getDb(ctx)
  → database.DBFromContext(ctx, defaultDB)  // 优先从 context 获取事务 DB
```

所有 Repository 通过 `DBFromContext` 自动参与事务，无需感知事务存在。

---

## 补偿机制

### 备份还原（StoreBackupOrchestrator）

**触发时机**：替换场景（步骤 0.1），用户确认替换后在第二次 `run()` 中执行。

**BackupAllStores 流程**：

```
BackupAllStores(ctx, workId)
  → 查询 workId 下所有 enabled 的 Resource
  → 对每个 Resource 的每个 Store（Work + Thumbnail）：
      storeDeleter.Delete(ctx, storeId, backup=true)
        → 物理文件移动到 workdir/backup/YYYY/MM/DD/
        → 创建 Backup DB 记录（保存原始路径元数据）
        → 删除 PersistentStore DB 记录
  → 返回 []*StoreBackupItem（记录 ResourceID、BackupID、StoreType）
```

**RestoreAllStores 流程**：

```
RestoreAllStores(ctx, items)                     // 使用 context.Background()
  → 跳过 BackupID <= 0 的条目（已直接删除，无法恢复）
  → 对每个可恢复条目：
      1. 获取 Backup 记录 → 读取 OriginalFilePath、OriginalFileName
      2. storeImporter.StoreFromExternal → 移动备份文件回原路径 + 创建新 PersistentStore 记录
      3. 更新 Resource 的 WorkStoreID/ThumbnailStoreID → 指向新 store ID
      4. 删除 Backup DB 记录
```

**降级处理**：如果备份过程中某个 Store 备份失败，`BackupID` 为 0。还原时跳过此类条目，该 Store 文件永久丢失，Resource 对应字段保持 null。

### 事务失败文件清理

事务 2 失败时：

```go
if writer != nil {
    writer.Close()                              // 关闭文件句柄
}
storeFileCleaner.CleanupFile(relativePath)      // 删除磁盘文件
```

不使用 `writer.Abort()`（因为 Abort 依赖 DB 查询获取文件路径，但事务回滚后 DB 记录已不存在）。

---

## 崩溃恢复

### PendingResourceID 的作用

`pending_resource_id` 是任务实体上的字段，在事务 2 中设置（步骤 6），在下载完成后清除（`clearPendingResourceID`）。

| 阶段 | pending_resource_id | 含义 |
|------|:---:|------|
| 任务创建 | null | 无待处理资源 |
| 事务 2 提交后 | Resource ID | 有资源正在下载中 |
| 下载完成后 | null | 资源已完成 |

### 应用重启恢复路径

```
App 启动
  → loadAndStartTaskTree 发现 DB 中 Paused 状态的任务
  → buildOrReuseChild 检查 PendingResourceID：
      有效 → resumeFromDB=true → resumeFromPersistedState()
      无效 → 从头 run()
```

`resumeFromPersistedState` 从 DB 加载 Resource 和 PersistentStore，检查本地文件是否存在，调用插件 `Resume` 接口尝试续传。

---

## 异步持久化（flush 机制）

`Manager.flushLoop` 作为后台 goroutine 运行，批量写入 DB 减少磁盘 I/O。

```
flushLoop:
  for {
      select:
      case <-closeCh:  doFlush() + close(flushDone) + return
      case <-flushCh:  // fall through
      }
      time.Sleep(200ms)     // 批量窗口，合并短时间内的多次更新
      doFlush()
  }

doFlush():
  1. 交换 pending maps（加锁）
  2. repo.BatchSetStatus(statusMap)                    // SQL CASE WHEN 批量更新状态 + 错误信息
  3. repo.BatchUpdatePendingResourceID(resourceMap)    // SQL CASE WHEN 批量更新 pending_resource_id
  4. pusher.PushProgressBatch(progressBatch)           // 批量推送进度到前端
```

**优雅停机**：`GracefulShutdown` 暂停所有 Processing 任务 → 等待全部达到稳定状态 → 触发最终 `doFlush` → 确保所有 DB 写入完成。

---

## 插件交互

### TaskExecutor 接口

| 方法 | 调用时机 | 说明 |
|------|---------|------|
| `CreateWorkInfo(ctx, task)` | 步骤 1 | 插件解析 URL 返回作品元数据 |
| `Start(ctx, task)` | 步骤 3 | 插件开始下载，返回 `io.ReadCloser` 数据流 |
| `Pause(ctx, param)` | 暂停时 | 通知插件关闭上游 |
| `Stop(ctx, param)` | 停止时 | 通知插件终止 |
| `Resume(ctx, param)` | 断点续传 | 传入已下载字节数，插件决定是否续传 |
| `GetThumbnail(ctx, task)` | 下载完成后 | 获取缩略图 |

### RPC 调用链

```
ManagedTask.pluginExec.Xxx()
  → TaskExecutorImpl (backend/plugin/extension/task_executor.go)
    → registry.GetTaskHandler(pluginPublicId, contributionId)
    → sdkTaskHandler.Xxx(sdkTask)      // 跨进程 RPC
```

### StoreWriter 生命周期

```
              StoreStream 创建
                    │
                    ▼
              ┌── Write() ──┐
              │  (循环写入)   │
              └──────────────┘
                    │
        ┌───────────┼───────────┐
        ▼           ▼           ▼
    Complete()   Close()     Abort()
   (下载完成)   (暂停)      (失败/停止)
    Sync+关闭    仅关闭      关闭+删文件
    DB=Complete  DB=Incomplete + 删DB记录
```

---

## 更新记录

### 2026-06-12
- [新增] 创建任务执行流程文档，覆盖完整生命周期、状态模型、事务边界、补偿机制、崩溃恢复
