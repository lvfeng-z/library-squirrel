# LibrarySquirrel 任务执行流程文档

本文档详细描述任务从创建到完成的完整生命周期，包括状态模型、执行步骤、事务边界、补偿机制和崩溃恢复。

## 关键文件

| 组件 | 文件路径 |
|------|---------|
| 任务状态、ManagedTask、run()（runMode 分流）、downloadLoop、板块方法 | `backend/taskManager/model.go` |
| Manager（多根调度、信号量、flush、onStateChange） | `backend/taskManager/manager.go` |
| Handler（Start/Retry/Resume、板块 Redownload*） | `backend/taskManager/handler.go` |
| Task Service（创建、重试） | `backend/task/service.go` |
| Task Repository（DB 操作、关联任务查询） | `backend/task/repository.go` |
| 备份还原（BackupAllStores / BackupStores 板块隔离） | `backend/backup/store_backup_orchestrator.go` |
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

所有任务执行统一走**唯二入口**——`startTaskTrees`（开始/重试）与 `resumeTaskTrees`（恢复），二者内部都调用多根的 `loadAndStartTaskTrees`：

```
Handler.StartTaskTree(taskId, isLeaf) → startTaskTrees([taskId], runModeFull)
Handler.RetryTaskTree(taskId, isLeaf) → startTaskTrees([taskId], runModeFull)
Handler.ResumeTaskTree(taskId, isLeaf) → resumeTaskTrees([taskId])           // 固定 runModeFull, skipTerminal=true
板块 Redownload*(taskIds)               → startTaskTrees(taskIds, runModeXxx)
```

`loadAndStartTaskTrees(taskIds, skipTerminal, mode)` 接受任意多个 taskId（子任务或父任务），一次构建完整任务树：

```
loadAndStartTaskTrees(taskIds, skipTerminal, mode)
  → repo.ListTaskTree(taskIds)                    // 多根共享一次递归 CTE 查询
  → 重复执行保护：快照运行中的 taskMap/parentMap，跳过已运行单元
  → 确定处理单元并去重：
      独立任务(pid=0,hasChild=false) → 单元 = 自身 taskId
      叶子任务(pid>0)                → 单元 = 其 actualParentId（同父多叶子归一）
      父任务(hasChild=true)          → 单元 = 自身 taskId
      processedUnits 保证同单元只处理一次
  → buildOrReuseChild(task, skipTerminal, mode)    // 构建或复用，注入 runMode
      - skipTerminal=true 且 DB 已终态 → 返回 nil（Resume 跳过已完成）
      - DB 状态为 Paused 且 PendingResourceID 有效 → resumeFromDB=true
      - DB 状态为 Paused 但无 PendingResourceID → 从头执行
  → processParentUnit(...)                        // 父单元：建 ParentTask + 收集直接子任务
      - 所有子任务已终态（仅 skipTerminal 时）→ 计算父最终状态 + PushTaskRemove
  → batchCheckDuplicates(allToCheck)              // 多根共享一次批量查重
      - 查询 workChecker.ListBySiteAndSiteWorkIDs
      - 重复 → WaitingForInput + 前端推送 DuplicateDetected
      - 不重复 → skipDuplicateCheck=true
  → tryDispatch(child)                            // 受信号量调度（逐个）
      - 成功获取信号量 → go executeTask(child)
      - 信号量已满 → 加入 waitingQueue + Waiting 状态
```

**多根设计要点**：跨父批量执行时，所有根共享一次 `ListTaskTree` 与一次 `batchCheckDuplicates`；内存任务树严格按 DB 真实父子关系构建，无统一 N/M 跨父聚合（各父任务独立刷新状态）。`runMode` 在 `buildOrReuseChild` / `newManagedTask` 阶段注入到每个 `ManagedTask`，`run()` 据此分流（见阶段 3、板块单独执行章节）。

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

`run()` 是核心执行入口，按 `ManagedTask.runMode` 分流到不同板块（Full 完整下载 / ResourceOnly 板块 B / WorkInfo 板块 A / Thumbnail 板块 C，详见[板块单独执行](#板块单独执行)章节）：

```go
func (m *ManagedTask) run() runResult {
    // defer 栈：① 失败还原备份（最先注册最后执行）② panic recovery
    // ctx 检查 + setState(Processing)
    switch m.runMode {
    case runModeWorkInfo:     return m.runWorkInfoSection()      // 板块 A
    case runModeThumbnail:    return m.runThumbnailSection()     // 板块 C
    case runModeResourceOnly: return m.runResourceOnlySection()  // 板块 B
    default:                  return m.runFullSection()          // 完整下载
    }
}
```

**defer 栈**（LIFO，后注册先执行）：

| 顺序 | defer | 作用 |
|:---:|-------|------|
| 1（最先执行） | `recover()` | panic 恢复，调用 `setFailed()` |
| 2（最后执行） | 备份还原检查 | 如果最终状态为 `Failed` 且有备份项，调用 `RestoreAllStores`。使用 `context.Background()` 确保任务取消后仍能执行 |

**runFullSection 逐步执行流程**（完整下载 = 作品信息 + 资源 + 封面全流程）：

| 步骤 | 操作 | DB 操作 | 事务 |
|:---:|------|---------|:---:|
| 0 | 检查 workdir 是否已配置 | 无 | - |
| 0.0 | 重复检测（`workChecker.GetBySiteAndSiteWorkID`） | SELECT | - |
| 0.1 | 替换场景备份（`BackupAllStores`） | SELECT + DELETE/备份 per store | - |
| 1 | `pluginExec.CreateWorkInfo`（插件 RPC） | 无 | - |
| 2 | `workInfoSaver.SaveWorkInfo` | 批量 INSERT（Work、作者、标签等） | **事务 1** |
| 3 | `pluginExec.Start`（插件 RPC，返回数据流） | 无 | - |
| — | `startDownload(reader, startResp)`：解析路径 + 事务 2 + downloadLoop | 见下 | **事务 2** |

**startDownload（Full 与 ResourceOnly 共享）**：解析保存路径（`resolveLocalPath`）→ 事务 2（StoreStream + saveResource + UpdatePendingResourceID）→ `downloadLoop()`。板块 B 的隔离行为（保留旧缩略图）由 `runMode` 在 `saveResource` 与 `downloadLoop` 内控制，详见板块单独执行章节。

**事务 2 详解**（`startDownload` 内）：

```
transactor.ExecInTransaction(context.Background(), func(txCtx) {
    // StoreStream：创建文件 + PersistentStore DB 记录
    storeId, writer = storeStreamer.StoreStream(txCtx, relPath, fileName)

    // saveResource：替换场景更新 / 新建场景创建 Resource
    resourceId = saveResource(txCtx, workId, storeId, startResp)

    // 更新 pending_resource_id（崩溃恢复标记）
    task.PendingResourceID = {Int64: resourceId, Valid: true}
    pendingResourceUpdater.UpdatePendingResourceID(txCtx, taskId, ...)
})
```

- 传入 `context.Background()` 而非 `m.ctx`，确保任务取消不会中断事务提交
- StoreStream 的文件创建发生在事务外行为（磁盘 I/O），但 DB 记录参与事务
- **事务失败**时：DB 自动回滚，磁盘文件通过 `writer.Close()` + `storeFileCleaner.CleanupFile()` 显式清理

**saveResource 的两种场景**：

| 场景 | 条件 | 行为 |
|------|------|------|
| 替换 | `isReplace=true` | 查询已有 Resource，更新 `WorkStoreID` 和 `ResourceComplete=0` |
| 新建 | 首次执行 | 创建新 Resource（`WorkID`、`TaskID`、`WorkStoreID`、`ResourceComplete=0`） |

#### 3B 断点续传 — resumeFromPersistedState()

应用重启后，`loadAndStartTaskTrees`（经 `buildOrReuseChild`）发现 DB 中 `Paused` 且 `PendingResourceID` 有效的任务，设置 `resumeFromDB=true`。

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
Manager.ResumeTaskTree(taskId, isLeaf)
  → resumeTaskTrees([taskId]) → loadAndStartTaskTrees([taskId], skipTerminal=true, runModeFull)
  → 任务在内存中：prepareForResume() + tryDispatch()
  → 任务不在内存（跨重启）：从 DB 重新加载（buildOrReuseChild 按 PendingResourceID 决定续传/重跑）

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
Manager.RetryTaskTree(taskId, isLeaf)
  → startTaskTrees([taskId], runModeFull) → loadAndStartTaskTrees([taskId], skipTerminal=false, runModeFull)
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
| **事务 2** | StoreStream DB 记录 + saveResource + UpdatePendingResourceID | `taskManager/model.go` `startDownload()` |
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

`BackupAllStores(ctx, workId)` 是 `BackupStores(ctx, workId, StoreTypeWork, StoreTypeThumbnail)` 的包装（备份 Work + Thumbnail 全部 Store）。板块隔离场景按需只备份单一类型：

| 调用方 | 备份范围 | 说明 |
|--------|---------|------|
| Full 替换（`BackupAllStores`） | Work + Thumbnail | 完整重下，两者都备份 |
| 板块 B（`BackupStores(StoreTypeWork)`） | 仅 Work | 资源重下不动缩略图 |
| 板块 C（`BackupStores(StoreTypeThumbnail)`） | 仅 Thumbnail | 封面重下不动资源 |

**BackupStores 流程**：

```
BackupStores(ctx, workId, types...)        // types 决定只处理哪些 StoreType
  → 查询 workId 下所有 enabled 的 Resource
  → 对每个 Resource 的每个匹配 types 的 Store（Work / Thumbnail）：
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

还原有两处触发：`run()` 的 defer（任务最终 Failed 时还原全部备份项）、`doSaveThumbnail(force=true)` 的 defer（板块 C 新缩略图未保存成功时还原旧缩略图）。

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
  → resumeTaskTrees → loadAndStartTaskTrees 发现 DB 中 Paused 状态的任务
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

## 板块单独执行

对已下载的作品，支持单独重新执行某个板块（板块 A 作品信息 / 板块 B 资源文件 / 板块 C 封面）。板块执行与普通任务执行**等价**——同样走唯二入口 `startTaskTrees`、新建 `ManagedTask` 实例，只是携带不同的 `runMode`。

### runMode 模式

| runMode | 板块 | 入口方法 | 终态性 | 持久化 |
|:---:|------|---------|:---:|:---:|
| `runModeFull` | 完整下载（信息+资源+封面） | `runFullSection` | 终态 | 是 |
| `runModeResourceOnly` | B 资源文件 | `runResourceOnlySection` | 终态 | 是 |
| `runModeWorkInfo` | A 作品信息 | `runWorkInfoSection` | 非终态 | 否 |
| `runModeThumbnail` | C 封面 | `runThumbnailSection` | 非终态 | 否 |

板块 Handler 直接接收前端选定的 `taskIds`，内部调 `startTaskTrees`：

```go
func (h *Handler) RedownloadWorkInfo(ctx, taskIds []int64)  → startTaskTrees(ctx, taskIds, runModeWorkInfo)
func (h *Handler) RedownloadResource(ctx, taskIds []int64)  → startTaskTrees(ctx, taskIds, runModeResourceOnly)
func (h *Handler) RedownloadThumbnail(ctx, taskIds []int64) → startTaskTrees(ctx, taskIds, runModeThumbnail)
```

任务定位：板块执行所需数据（`PluginPublicID`/`PluginContributionID`/`PluginData`/`URL`/`site_id`/`site_work_id`）均在 `entity.Task` 上。前端选定任务的方式：任务列表行内入口已有 taskId 直接使用；作品详情入口经 `(site_id, site_work_id)` 调 `task.ListTasksBySiteAndSiteWorkID` 反查关联任务，由用户选定（该入口暂未实现）。

### A/C 非终态语义

板块 A、C 是**非终态操作**：执行不改变任务的终态、不持久化任务状态，仅短暂进入 Processing 后回到执行前状态。

- `isNonTerminalMode()`：`runMode == WorkInfo || Thumbnail`。
- `finishNonTerminalSection(errMsg)`：成功（空 errMsg）回到执行前状态（`setState(TaskState(m.task.Status))`）；失败记录错误 + 推送通知，同样回到执行前状态。**不调用 `setFailed`**，不污染源任务。
- `onStateChange` 持久化闸门：稳定态写入由 `isStableState(newState) && !mt.isNonTerminalMode()` 双重判断，A/C runMode 一律跳过 `addToPending`（但仍推送前端状态变化，让前端可见 Processing）。
- 前端表现：板块执行期间任务卡片短暂可见（Processing），结束（成功/失败）后由 `cleanupFinishedTask` 走标准移除流程（A/C 作根 parentId=0 → 从 `taskMap` 移除 + `PushTaskRemove`）。

### 板块隔离（各板块只备份/操作自身 Store）

| 板块 | 备份范围 | 行为 |
|:---:|---------|------|
| A 作品信息 | 无 | 不动文件 |
| B 资源文件 | 仅 `StoreTypeWork` | `BackupStores(workId, StoreTypeWork)`；`saveResource` 替换分支**不清 `ThumbnailStoreID`**（仅 Full 清）；`downloadLoop` 完成分支**跳过 `saveThumbnail`**（仅 Full 生成） |
| C 封面 | 仅 `StoreTypeThumbnail` | `doSaveThumbnail(force=true)` 先 `BackupStores(workId, StoreTypeThumbnail)` 备份旧缩略图，下载新缩略图失败时 defer 还原 |

`saveThumbnail` 已从耦合 `PendingResourceID` 的下载流程中解耦，改为 `workId` 基：`saveThumbnail()`（Full 完成分支，force=false）→ `saveThumbnailByWorkId(ctx, workId, force)` → `doSaveThumbnail(ctx, resource, force)`。板块 C 调用 `saveThumbnailByWorkId(ctx, workId, true)` 强制覆盖。

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

### 2026-06-17
- [修改] 任务启动/重试/恢复/崩溃恢复入口由单根 `loadAndStartTaskTree` 统一改为多根 `loadAndStartTaskTrees`（唯二入口 `startTaskTrees`/`resumeTaskTrees`，多根共享一次 ListTaskTree + 一次批量查重 + 重复执行保护）
- [新增] 「板块单独执行」章节：runMode 四值分流、A/C 非终态语义（不产生终态/不持久化/失败恢复执行前状态）、板块隔离备份、Redownload* Handler、`run()` 按 runMode 分流、`onStateChange` 跳过 A/C 持久化
- [修改] 事务 2 位置由 `run()` 步骤 4-6 更正为 `startDownload()`
- [新增] 补偿机制补充 `BackupStores(types...)` 板块隔离与 `BackupAllStores` 包装关系；`saveThumbnail` 解耦为 `saveThumbnailByWorkId`/`doSaveThumbnail(force)` 改造说明

### 2026-06-12
- [新增] 创建任务执行流程文档，覆盖完整生命周期、状态模型、事务边界、补偿机制、崩溃恢复
