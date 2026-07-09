# LibrarySquirrel 任务执行流程文档

本文档详细描述任务从创建到完成的完整生命周期，包括状态模型、执行步骤、事务边界、补偿机制和崩溃恢复。

## 关键文件

| 组件 | 文件路径 |
|------|---------|
| 任务状态、ManagedTask、run()（统一 runSectionCombo）、downloadLoop、板块组合执行、parseSections | `backend/taskManager/model.go` |
| Manager（多根调度、信号量、flush、onStateChange、batchCheckDuplicates） | `backend/taskManager/manager.go` |
| Handler（Start/Retry/Resume、Redownload 板块重执行） | `backend/taskManager/handler.go` |
| Task Service（创建、重试） | `backend/task/service.go` |
| Task Repository（DB 操作、关联任务查询） | `backend/task/repository.go` |
| 备份还原（BackupStores 按类型板块隔离） | `backend/backup/store_backup_orchestrator.go` |
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
板块 Redownload(taskIds, sections)    → startTaskTrees(taskIds, parseSections(sections))  // sections 为板块代码 A=1/B=2/C=3
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
      - 无 B（!hasResource）的板块组合直接标记 skipDuplicateCheck 并派发（A/C 不覆盖资源，无需"作品已存在"提醒）
      - 查询 workChecker.ListBySiteAndSiteWorkIDs（仅含 B 的组合参与）
      - 重复 → WaitingForInput + 前端推送 DuplicateDetected
      - 不重复 → skipDuplicateCheck=true
  → tryDispatch(child)                            // 受信号量调度（逐个）
      - dispatch(child):actorStarted CAS + postCmd(cmdStart / cmdResume)
      - 槽位在 handleRunCmd 内取(失败 → enqueueSelf Waiting)
```

**多根设计要点**：跨父批量执行时，所有根共享一次 `ListTaskTree` 与一次 `batchCheckDuplicates`；内存任务树严格按 DB 真实父子关系构建，无统一 N/M 跨父聚合（各父任务独立刷新状态）。`runMode`（板块选择结构体 `{workInfo, storeRoles}`）在 `buildOrReuseChild` / `newManagedTask` 阶段注入到每个 `ManagedTask`，`run()` 统一调 `runSectionCombo`（全集等价完整下载含查重，真子集按所选板块，见[板块重执行](#板块重执行多选组合)章节）。

### 阶段 3：任务执行

per-task actor 模型:每个 `ManagedTask` 创建时启动一条常驻 `actorLoop` goroutine(一生一灭),任务级可变状态只在其内修改。命令经 `cmdCh` 串行投递(`postCmd`,非阻塞),`actorLoop` 按 kind 分发;终态时退出并 drain `cmdCh`。

```go
actorLoop:
  for cmd := range cmdCh:
    cmdStart|cmdResume|cmdConfirmReplace → handleRunCmd(cmd)
    cmdPause                             → handlePauseCmd(cmd)
    cmdStop                              → handleStopCmd(cmd)
    终态 → return(defer 关闭 actorDone + drain cmdCh)
```

`handleRunCmd` 是长任务执行核心:取信号量槽位 → `cmdResume` 时 `prepareForResume` → 派生 `runCtx` + 启动 `cmdWatcher` → `runOnce()` 阻塞执行 → 关 watcher + 处理 `pendingCmds` + 释放槽位。

```go
handleRunCmd(cmd):
  取槽位(失败 → enqueueSelf Waiting)
  cmdResume → prepareForResume()(关旧 reader、streams=nil、resumeFromDB=PendingResourceID.Valid)
  softPause=false; inDownload=false; 清 drainTimer // 每条 run 命令重置优雅暂停状态
  runCtx, runCancel = context.WithCancel(m.ctx)   // 每条 run 命令新建,中断在途长任务
  go cmdWatcher(stop)                              // 监听 cmdCh:pause 走优雅暂停,stop 立即 runCancel
  result = runOnce()                               // resumeFromDB → resumeFromPersistedState;否则 run()
  close(stop); 停 drainTimer; runCancel()         // drain 完成或超时已强制取消,此时无在途
  按 result 释放槽位 + dispatchFromQueue;处理 pendingCmds(命令队列时序保证 pause 覆盖陈旧 resume)
```

`cmdWatcher` 是中断核心,按命令种类 + 阶段分流:**`cmdPause` 在 downloadLoop 阶段(`inDownload=true`)走优雅暂停**——置 `softPause=true` + 启动 `drainTimer`(2s 兜底),不立即 `runCancel`,在途数据照常落盘;**`cmdPause` 在 setup 阶段(`inDownload=false`)立即 `runCancel`**(无在途 chunk,快速中断插件 RPC);**`cmdStop` 不论阶段都立即 `runCancel`**(放弃语义)。命令暂存 `pendingCmds` 由主循环稍后处理。`runOnce` 据 `resumeFromDB` 选 `resumeFromPersistedState`(跨重启续传)或 `run`(全新/板块执行)。

#### 3A 全新执行 — run()

`run()` 调 `runSectionCombo`:全集等价完整下载(含查重),真子集按所选板块执行(详见[板块重执行](#板块重执行多选组合)章节)。

**defer 栈**(LIFO,后注册先执行):

| 顺序 | defer | 作用 |
|:---:|-------|------|
| 1(最先执行) | `recover()` | panic 恢复,调用 `setFailed()` |
| 2(最后执行) | 备份还原检查 | 最终状态为 `Failed` 且有备份项时调 `RestoreAllStores`。用 `context.Background()` 确保任务取消后仍能执行 |

**含 B(资源板块)的逐步流程**(`runSectionCombo`):

| 步骤 | 操作 | DB 操作 | 事务 |
|:---:|------|---------|:---:|
| 0 | 检查 workdir 已配置 | 无 | - |
| 0.0 | 重复检测(`workChecker.GetBySiteAndSiteWorkID`) | SELECT | - |
| 0.1 | 替换场景备份(`BackupStores`,板块隔离) | SELECT + 备份 per store | - |
| 1 | `pluginExec.CreateWorkInfo`(插件 RPC) | 无 | - |
| 2 | `workInfoSaver.SaveWorkInfo` | 批量 INSERT | **事务 1** |
| 3 | `pluginExec.Start(ctx, task, storeRoles)`(返回 `[]*StoreSpec` 多流) | 无 | - |
| — | `startDownload(specs, workResp)`:解析路径 + 事务 2 + downloadLoop | 见下 | **事务 2** |

**startDownload(多流)**:解析保存路径 → 事务 2(为每个 spec `StoreStream` + `mountResourceStores` 写 resource_store 行 + `saveResource` + `UpdatePendingResourceID`)→ `downloadLoop()`。

**事务 2 详解**(`startDownload` 内):

```
transactor.ExecInTransaction(context.Background(), func(txCtx) {
    for each spec:
        storeId, writer = storeStreamer.StoreStream(txCtx, relPath, fileName)   // 每轨建 store
        streams = append(streams, newStreamController(spec, storeId, writer, relPath))
        mounts = append(mounts, {role, generation, storeId})
    resourceId = saveResource(txCtx, workId, mounts)   // 替换更新/新建 Resource + mountResourceStores 写 resource_store
    task.PendingResourceID = {Int64: resourceId, Valid: true}
    pendingResourceUpdater.UpdatePendingResourceID(txCtx, taskId, ...)
})
```

- 传入 `context.Background()`,任务取消不中断事务提交
- store 关联只写 `resource_store` 表(role/generation/storeId/orderIdx),不再用 Resource 的固定列(WorkStoreID/ThumbnailStoreID 已废弃,改为多轨 resource_store)
- **事务失败**:DB 回滚 + 显式 `writer.Close()` + `CleanupFile` 清理磁盘

**saveResource 两种场景**:替换(查已有 Resource 更新 `ResourceComplete=0`)、新建(创建 Resource)。均经 `mountResourceStores` 写 resource_store 行(先删同 role 旧行再插)。

#### 3B 断点续传 — resumeFromPersistedState()

进程内 Pause→Resume 与跨重启均走此路(`pending_resource_id` 已持久化)。`loadAndStartTaskTrees`(经 `buildOrReuseChild`)发现 DB 中 `Paused` 且 `PendingResourceID` 有效的任务,设 `resumeFromDB=true`,`runOnce` 据此进入 `resumeFromPersistedState`。

```
resumeFromPersistedState()
  → runCtx 检查(取消则 runResultPaused)
  → 加载 Resource(pending_resource_id)→ 失败降级 run()
  → 读 resource_store 各轨关联
  → 逐轨计算续传偏移:
      store.Status==Complete 且文件存在 → completedRoles(跳过)
      downloaded 未完成 → streamOffsets[role] = os.Stat(文件大小)
      derived 未完成    → incompleteDerivedRoles(整轨重产)
  → pluginExec.Resume(ctx, {StreamOffsets})         // 续传未完成 downloaded 轨
  → 未完成 derived 轨 → pluginExec.Start(ctx, task, incompleteDerivedRoles) 整轨重产
  → 事务:continuable downloaded 轨 → ResumeStream(Truncate offset);其余 → StoreStream 重建
  → downloadLoop()
```

续传权威是**磁盘 stat**(`streamOffsets`),不是 reader 内部计数;插件 Resume 据此发 Range 请求。`ResumeStream` 用 `Truncate(offset) + Seek(offset)` 消除 stat 与文件打开间的 TOCTOU。

#### downloadLoop 详解(多流并发)

每条 stream 一个 goroutine 跑 `copyLoop`,`wg.Wait` 后按结果判定(暂停优先 → 取消 → 失败 → 完成):

```
downloadLoop:
  inDownload=true(defer false)            // 标记下载阶段:cmdWatcher 据此让 cmdPause 走 softPause
  defer: closeStreamReaders(关闭各 spec reader)
  for each stream s: go s.copyLoop(m)     // 并发
  wg.Wait()
  anyStreamPaused → setState(Paused) + runResultPaused
  hasCanceled     → abortedByPause ? runResultPaused : runResultDone
  hasFailed       → setFailed(msg) + runResultDone
  全部完成        → clearPendingResourceID + setState(Finished) + runResultDone

copyLoop(s):   // 单流 read→write→累计
  for:
    select runCtx.Done → handlePause(buf)     // 超时兜底/外部取消 runCancel 中断在途 reader
    n, err = reader.Read(buf[32K])             // pull 模型:驱动插件 serveSpecsPull
    if n>0: storeWriter.Write; written += n; reportProgress
    if softPause && runCtx.Err()==nil → handlePause(nil)  // 优雅暂停:在途已落盘,不发新 Pull
    if err==EOF → handleEOF(校验 written vs size、Complete、streamCompleted)
    if err && runCtx.Err()!=nil → handlePause
    ...失败/暂停分支
```

**中断传播(按阶段分流)**:
- **downloadLoop 优雅暂停(常态)**:`cmdPause`(`inDownload=true`)→ `softPause=true` + `drainTimer`,**不取消 runCtx**。copyLoop 完成当前在途往返(`reader.Read` 返回 + `storeWriter.Write` 落盘)后检测 `softPause`,调 `handlePause(nil)` 退出——传 nil 跳过 drain(pull 模型下 drain 会再 Read 即发起新 PullRequest 拉新数据,违背"暂停只阻止新数据发起")。此刻磁盘 stat = 真实中断点,Resume 无重复下载。
- **setup 立即中断**:`cmdPause`(`inDownload=false`)→ 立即 `runCancel`;setup 的插件 RPC(CreateWorkInfo/Start/Resume 用 `m.runCtx`)被 gRPC stream 取消,`abortedByPause()` 命中 → `runResultPaused`。setup 阶段无在途 chunk,立即切断无损且快速(避免 setup RPC 网络建连期间不响应暂停)。
- **超时兜底/停止(异常)**:`drainTimer`(2s)到期或 `cmdStop` → `runCancel` → `runCtx.Done`;pull stream ctx 继承任务 ctx(项一),取消经 gRPC 传播中断插件 `serveSpecsPull` 的 reader.Read(Close reader)。copyLoop 收到 Read 错误 + `runCtx.Err()` → `handlePause`(drain + Sync + Close,保留文件),退化为有损立即暂停。

**handleEOF 完整性校验**:`s.size > 0 && written < s.size` → Failed(下载不完整);`written == 0` → Failed(空产物)。校验依赖 `spec.Size` 为完整资源大小(206 续传需插件解析 Content-Range 还原,非剩余字节数)。

**暂停机制**:setup 阶段 `runCancel` 立即中断;downloadLoop 常态经 `softPause` 让 copyLoop 在在途落盘后 `handlePause(nil)`(`Sync` + `Close`,不调 Complete,不发新 Pull),异常(超时/停止)经 `runCancel` 让 copyLoop 走 `runCtx.Done` 的 `handlePause`(drain + Sync + Close)。各路径 DB 记录均保持 `Incomplete`,`PendingResourceID` 保持有效。

### 阶段 4：暂停与恢复

Pause/Resume 经 actor 命令通道投递,时序由 `cmdCh` 串行化 + `cmdWatcher` 保证。

**暂停流程**:

```
Manager.PauseTaskTree(taskId)
  → 取 children 快照(RLock 下拷贝,释放锁再投递,投递路径不持 Manager.mu)
  → 按状态 postCmd:
      Waiting → 移出队列 + Paused
      Processing/Pausing → postCmd(cmdPause, ack)   // 带 ack 等待 actor 处理
      其他 → 跳过

handlePauseCmd(cmd):
  → 非终态 → setState(Paused)
  → Processing:runCancel(幂等,handleRunCmd 长任务返回后已 runCancel)+ pluginExec.Pause()(drain 已完成,通知插件关 reader 是清理性质)
  → ack ← nil(对外阻塞契约)
```

> Processing 任务的暂停由 `cmdWatcher` 收到 cmdPause 时按阶段处理:setup(`inDownload=false`)立即 `runCancel` 中断 RPC;downloadLoop 置 `softPause`(在途落盘后退出)或 `drainTimer` 触发 `runCancel`(超时兜底)。`handlePauseCmd` 在长任务返回后处理(命令暂存于 pendingCmds),此时 runCancel 幂等、pluginExec.Pause 为清理性质。

**恢复流程**:

```
Manager.ResumeTaskTree(taskId, isLeaf)
  → resumeTaskTrees([taskId]) → loadAndStartTaskTrees([taskId], skipTerminal=true, runModeFull)
  → 内存中任务:postCmd(cmdResume)(fire-and-forget)
  → 不在内存(跨重启):buildOrReuseChild 重建 → dispatch → postCmd(cmdResume 或 cmdStart)

handleRunCmd(cmdResume):
  → prepareForResume()(关旧 reader、streams=nil、resumeFromDB=PendingResourceID.Valid)
  → ...派生 runCtx + cmdWatcher + runOnce(resumeFromDB → resumeFromPersistedState)
```

`prepareForResume` 只重置执行期可变字段(actor 主 ctx `m.ctx` 一生一灭不重建,`runCtx` 由 `handleRunCmd` 派生)。

**Pause 的数据持久化边界**:暂停按阶段分流——**setup 阶段(`inDownload=false`)无在途 chunk,立即 `runCancel` 中断插件 RPC(快速、无损)**;**downloadLoop 阶段(`inDownload=true`)走优雅暂停**,置 `softPause` 让 copyLoop 完成当前在途往返(已发起的 PullRequest 的数据照常 recv + 落盘)后再退出,不再发起新 PullRequest,磁盘 stat 天然对齐真实中断点,Resume 无重复下载。downloadLoop 阶段边界界定:
- **已持久化**:copyLoop 已 `storeWriter.Write` 的字节(= `os.Stat` 文件大小),Resume 从此继续。
- **常态排空(在途落盘)**:优雅暂停时已发起的 PullRequest 的数据,由 copyLoop 完成 recv + Write 落盘,stat 含这部分。
- **异常超时丢失(仅兜底)**:`drainTimer`(2s)到期强制 `runCancel` 时,在途未完成的 chunk 丢失(单个 ≤32K),退化为有损立即暂停,完整性由 Resume 从 stat 重下兜底。

主程序以磁盘 stat 为权威,不依赖插件在途状态。setup 快速中断 + downloadLoop 常态无损(在途排空)+ 异常兜底(2s 封顶有损),兼顾各阶段即时性与对齐。详见 `doc/plugin-dev-guide.md`「Pause 的数据持久化边界」。

### 阶段 5：停止

```
Manager.StopTaskTree(taskId)
  → 取 children 快照
  → 按状态 postCmd:
      Waiting → 移出队列 + setFailed
      Processing/Pausing → postCmd(cmdStop, ack)
      Paused → 直接 setFailed
      其他 → 跳过

handleStopCmd(cmd):
  → setState(Stopping) → runCancel(中断在途 copyLoop)
  → streams abort(storeWriter.Abort 清理文件 + DB 记录)
  → pluginExec.Stop() → setFailed("任务被用户停止")
  → ack ← nil
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
Manager.cleanupFinishedTask(mt)  // handleRunCmd 处理 runResultDone 时调用
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

**触发时机**：替换场景（步骤 0.1 等价位置），用户确认替换后在第二次 `run()` 中执行。

统一用 `BackupStores(ctx, workId, types...)` 按类型备份（板块隔离：各板块只备份/操作自身 Store）：

| 调用方 | 备份范围 | 说明 |
|--------|---------|------|
| Full / 含 B 的组合 替换 | `BackupStores(StoreTypeWork)` 仅 Work | 资源重下不动缩略图 |
| 含 C（缩略图重新生成时） | `BackupStores(StoreTypeThumbnail)` 仅 Thumbnail | 由 `doSaveThumbnail(force=true)` 内部触发，生成新缩略图失败时 defer 还原 |

> 历史 `BackupAllStores`（Work+Thumbnail 一次性全量）已删除：拆为 B 只备份 Work、缩略图延后到 `doSaveThumbnail(force=true)` 自管，各板块彻底隔离。

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
      3. 更新 resource_store 关联(同 role 指向新 store ID)
      4. 删除 Backup DB 记录
```

还原有两处触发：`run()` 的 defer（含 B 的组合终态 Failed 时还原 Work 备份项）、`doSaveThumbnail(force=true)` 的 defer（缩略图重新生成失败时还原旧缩略图）。两者各管自身 Store 的备份项。

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

## 状态持久化（终态即时 + 非终态批量）

状态落盘由 `addToPending` 按状态分两条路径：

- **终态即时写**（Finished/Failed/PartlyFinished）：`addToPending` 同步调用 `repo.BatchSetStatus`（单条 map，含 error_message）即时落盘，不进批量通道——进程崩溃也不丢失终态。终态同时同步清空执行模式（`UpdateRedownloadSections`）。
- **非终态批量写**（Paused、进度、pending_resource_id）：攒在内存 pending maps，由 `flushLoop` 后台批量刷库减少磁盘 I/O。

并发正确性：终态即时写与 `doFlush` 的批量 status 写都在 `pendingMu` 临界区内执行，互斥——避免 doFlush 用取出的过时快照（如 Processing/Paused）回写覆盖刚即时写入的终态。即时写还从批量通道删除该任务残留的非终态快照。

```
flushLoop:                              // 后台 goroutine，非终态批量通道
  for {
      select:
      case <-closeCh:  doFlush() + close(flushDone) + return
      case <-flushCh:  // fall through
      }
      time.Sleep(200ms)     // 批量窗口，合并短时间内的多次更新
      doFlush()
  }

doFlush():
  1. pendingMu 锁内:交换 status/resource/progress maps + repo.BatchSetStatus(statusMap)   // status 写与终态即时写互斥
  2. 释放 pendingMu
  3. repo.BatchUpdatePendingResourceID(resourceMap)    // SQL CASE WHEN 批量更新 pending_resource_id
  4. pusher.PushProgressBatch(progressBatch)           // 批量推送进度到前端
```

**优雅停机**：`GracefulShutdown` 暂停所有 Processing 任务 → 等待全部达到稳定状态 → 触发最终 `doFlush` → 确保所有 DB 写入完成。

---

## 板块重执行（多选组合）

对已下载的作品，支持**多选**重新执行若干板块（板块 A 作品信息 / 板块 B 资源文件 / 板块 C 封面）：前端勾选要执行的板块（不勾选视为全集），后端严格按所选板块组合执行。重执行与普通任务执行**等价**——同样走唯二入口 `startTaskTrees`、新建 `ManagedTask` 实例，只是 `runMode` 携带所选板块集合。

### runMode：板块选择结构体

```go
type runMode struct {
    workInfo   bool     // 板块 A(作品信息)
    storeRoles []string // 板块 B(资源):所选 store_type 子集(main/thumbnail/videoTrack/...),空=全量
}
var runModeFull = runMode{workInfo: true, storeRoles: allStoreRoles}
```

runMode 由 `workInfo`(板块 A)与 `storeRoles`(板块 B 资源,多角色)组成。缩略图作为 `storeRoles` 中的 `thumbnail` 角色由 derived StoreSpec 产出,不再单列板块 C。

`run()` 统一调 `runSectionCombo`：全集等价完整下载（含查重），真子集按所选板块 A→B→C 顺序执行、末尾统一终态化。

前端 `Redownload(taskIds, sections)` 收板块代码数组（A=1/B=2/C=3，不勾选时下发全集 `[1,2,3]`），后端 `parseSections` 映射为 `runMode`：

```go
func (h *Handler) Redownload(ctx, taskIds []int64, sections []int) → startTaskTrees(ctx, taskIds, parseSections(sections))
```

任务定位：重执行所需数据（`PluginPublicID`/`PluginExtensionID`/`PluginData`/`URL`/`site_id`/`site_work_id`）均在 `entity.Task` 上。任务列表行内入口已有 taskId 直接使用；作品详情入口经 `(site_id, site_work_id)` 调 `task.ListTasksBySiteAndSiteWorkID` 反查关联任务，由用户选定（该入口暂未实现）。

### runSectionCombo 执行流程

严格按 `runMode` 所选板块，A→B→C 顺序执行：

1. **workdir 检查**（B/C 需要，置于查重前避免无效确认）。
2. **含 B → 查重**（`runSectionCombo` 内 fallback；主路径在 `batchCheckDuplicates`）：作品已存在 → `PushDuplicateDetected` + `WaitingForInput` + `runResultNeedConfirm`，确认后第二次 run 走备份+执行。**无 B 不查重**（A/C 不覆盖资源，无需"作品已存在"提醒）。
3. **workId 定位 + B 备份**：含 B 则 `BackupStores(StoreTypeWork)` 备份旧资源（`existingWorkId` 由查重设置）；无 B 无 A（C-only）则 `resolveWorkIdByTask` 定位。
4. **板块 A**（`hasWorkInfo`）：`CreateWorkInfo` + `SaveWorkInfo`（提供 workId，并捕获 `workResp` 供 B 的文件名模板）。
5. **板块 B**（`hasResource`，终态）：`Start` →（若 A 执行过，合并 `workResp` 作品信息到 `startResp` 供文件名模板）→ `startDownload` → `downloadLoop`（完成 `setState(Finished)`；`hasThumbnail` 时以 `force=true` 顺带生成缩略图）。
6. **板块 C**（`hasThumbnail` 且无 B）：`saveThumbnailByWorkId(force=true)`。
7. **无 B → 非终态成功**：`finishNonTerminalSection("")` 回到执行前状态。

失败由 `comboFail(errMsg)` 统一处理：含 B → `setFailed`（终态，`run()` defer 还原 Work 备份）；无 B → `finishNonTerminalSection`（非终态，恢复执行前状态）。

### 终态性规则：由是否含 B 决定

| 组合 | 终态性 | 持久化 | 查重 |
|------|:---:|:---:|:---:|
| 含 B（B / A+B / B+C / 全集） | 终态（Finished/Failed） | 是 | 是（ConfirmReplace） |
| 无 B（A / C / A+C） | 非终态（保持执行前状态） | 否 | 否 |

- `isNonTerminalMode()` = `!hasResource()`。
- `finishNonTerminalSection(errMsg)`：成功（空 errMsg）回到执行前状态（`setState(TaskState(m.task.Status))`）；失败记录错误 + 推送通知，同样回到执行前状态，**不调用 `setFailed`**、不污染源任务。
- `onStateChange` 持久化闸门：`isStableState(newState) && !mt.isNonTerminalMode()`，无 B 的组合一律跳过 `addToPending`（仍推送前端状态变化）。
- 前端表现：重执行期间任务卡片短暂可见（Processing），结束（成功/失败）后由 `cleanupFinishedTask` 走标准移除流程（作根 parentId=0 → 从 `taskMap` 移除 + `PushTaskRemove`）。

> 注：A 为纯本地操作（`CreateWorkInfo`+`SaveWorkInfo`），执行极快（毫秒级），Processing 窗口可能短于一帧渲染，前端不一定可见闪烁——属正常现象，作品信息已实际更新。

### 板块隔离（各板块只备份/操作自身 Store）

| 板块 | 备份范围 | 行为 |
|:---:|---------|------|
| A 作品信息 | 无 | 不动文件 |
| B 资源文件 | 仅 main role | `BackupStores(workId, main)`；`saveResource` 替换分支只重建所选 role 的 resource_store 行,不动其他 role |
| C 封面 | 仅 `StoreTypeThumbnail` | `doSaveThumbnail(force=true)` 内部 `BackupStores(workId, StoreTypeThumbnail)` 备份旧缩略图，生成失败 defer 还原 |

含 B 的组合在 `downloadLoop` 完成时若同时选 C（`hasThumbnail`），以 `force=true` 生成新缩略图（备份旧→生成新→失败还原），并入 B 的完成路径（B 终态后无法再单独跑 C）。缩略图作为独立 role 的 resource_store 行，与 main role 互不影响，修复了原固定列模型下缩略图生成失败导致封面丢失的问题。

`saveThumbnail` 已从耦合 `PendingResourceID` 的下载流程解耦，改为 `workId` 基：`saveThumbnail()`（`downloadLoop` 完成分支，`force=true`）→ `saveThumbnailByWorkId(ctx, workId, force)` → `doSaveThumbnail(ctx, resource, force)`。板块 C 直接调 `saveThumbnailByWorkId(ctx, workId, true)`。

---

## 插件交互

### TaskExecutor 接口

| 方法 | 调用时机 | 说明 |
|------|---------|------|
| `CreateWorkInfo(ctx, task)` | 步骤 1 | 插件解析 URL 返回作品元数据 |
| `Start(ctx, task, storeRoles)` | 步骤 3 | 插件开始下载,返回 `[]*StoreSpec` 多流(含 downloaded 与 derived) |
| `Pause(ctx, param)` | 暂停时 | 通知插件关闭上游 reader(HTTP body / 文件句柄) |
| `Stop(ctx, param)` | 停止时 | 通知插件终止 |
| `Resume(ctx, param)` | 断点续传 | 传入 `StreamOffsets`(role→已落盘字节数),插件据此发 Range 续传 |

> 缩略图作为 `Generation=derived` 的 StoreSpec 在 Start 里产出,无单独 `GetThumbnail` 接口。reader 需响应 ctx:`Close()` 可中断阻塞中的 `Read`(SDK serveSpecsPull 靠此在任务取消时退出)。

### RPC 调用链

```
ManagedTask.pluginExec.Xxx()
  → TaskExecutorImpl (backend/plugin/extension/task_executor.go)
    → registry.GetTaskHandler(pluginPublicId, extensionId)
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

### 2026-07-08
- [修改] 阶段 3-5 重写为 per-task actor 模型(项三):`Manager.executeTask`/`go executeTask` 删除,改为每任务常驻 `actorLoop` + `cmdCh` 串行命令;`handleRunCmd`(取槽位→prepareForResume→runCtx→cmdWatcher→runOnce);`cmdWatcher` 收 pause/stop 立即 `runCancel` 中断在途长任务
- [修改] downloadLoop 改为多流并发(每 stream 一个 copyLoop goroutine),中断统一经 `runCtx.Done`(stream ctx 继承任务 ctx,项一),废弃旧 `pauseCh`/`drainDone`
- [修改] 资源关联由 Resource 固定列(WorkStoreID/ThumbnailStoreID)改为多轨 `resource_store` 表;`startDownload(specs, workResp)` 为每轨建 StoreStream + `mountResourceStores`
- [修改] 缩略图由 `GetThumbnail` 接口改为 `Generation=derived` 的 StoreSpec 在 Start 产出;`TaskExecutor` 删除 GetThumbnail
- [修改] runMode `{workInfo,resource,thumbnail}` → `{workInfo, storeRoles []string}`(板块 B 多角色)
- [修改] Resume 续传:`resumeFromPersistedState` 用磁盘 stat 算 `streamOffsets`;`ResumeStream` 改 `Truncate(offset)+Seek(offset)`;未完成 derived 轨由主程序另调 Start 整轨重产
- [修改] 插件交互表更新:Start 返回 `[]*StoreSpec` 多流;reader 契约(Close 中断 Read、响应 ctx)

### 2026-06-18
- [修改] 板块重执行由单板块单选改为**多选组合执行**：`runMode` 单枚举 → 板块选择结构体 `{workInfo,resource,thumbnail}`；`run()` 按 `isFull` 分流 `runFullSection`/`runSectionCombo`（A→B→C）；三 `Redownload*` Handler 收敛为 `Redownload(taskIds, sections)`（顺序码 A=1/B=2/C=3，`parseSections` 映射）
- [修改] 查重改为按 B 触发：`batchCheckDuplicates` 与 `runSectionCombo` 均对 `!hasResource`（无 B）跳过查重；含 B 走 ConfirmReplace
- [修改] 终态性规则改为由是否含 B 决定（`isNonTerminalMode = !hasResource`）；新增 `comboFail` 统一终态化
- [修改] 备份改为板块隔离：删除 `BackupAllStores`；B 始终 `BackupStores(StoreTypeWork)`，缩略图备份统一归 `doSaveThumbnail(force=true)`（`saveResource` 不清 `ThumbnailStoreID`、`downloadLoop` 以 force=true 生成）；修复缩略图生成失败丢失封面问题
- [修改] 「板块单独执行」章节重写为「板块重执行（多选组合）」
- [修改] 删除 `runFullSection`，`run()` 统一调 `runSectionCombo`（全集/真子集同路径）；`workResp→startResp` 合并（文件名模板）补入 runSectionCombo 的 B 步，修复 A+B 子集资源文件名/路径缺作者的问题；移除无用的 `isFull()`

### 2026-06-17
- [修改] 任务启动/重试/恢复/崩溃恢复入口由单根 `loadAndStartTaskTree` 统一改为多根 `loadAndStartTaskTrees`（唯二入口 `startTaskTrees`/`resumeTaskTrees`，多根共享一次 ListTaskTree + 一次批量查重 + 重复执行保护）
- [新增] 「板块单独执行」章节：runMode 四值分流、A/C 非终态语义（不产生终态/不持久化/失败恢复执行前状态）、板块隔离备份、Redownload* Handler、`run()` 按 runMode 分流、`onStateChange` 跳过 A/C 持久化
- [修改] 事务 2 位置由 `run()` 步骤 4-6 更正为 `startDownload()`
- [新增] 补偿机制补充 `BackupStores(types...)` 板块隔离与 `BackupAllStores` 包装关系；`saveThumbnail` 解耦为 `saveThumbnailByWorkId`/`doSaveThumbnail(force)` 改造说明

### 2026-06-12
- [新增] 创建任务执行流程文档，覆盖完整生命周期、状态模型、事务边界、补偿机制、崩溃恢复
