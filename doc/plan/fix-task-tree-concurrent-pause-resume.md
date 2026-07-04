# 修复:任务树高频暂停/恢复下的状态机失稳

> **状态(2026-07-04):本计划为"四条纪律"补丁视角;纪律 1/2 已被 `refactor-task-single-goroutine-invariant.md` 的 dispatch 不变量重构吸收并实现(更治本)。纪律 3/4 是否仍需要,留待 stress 回归后据"重构后观察清单"判定 —— 预期 1/2/3 随不变量消失,纪律 4(已完成 role 过滤)与并发无关、大概率仍需单独做。**

## 背景

pixiv 资源损坏问题已通过 SDK push→pull 改造 + 续传以 `diskOffset` 为权威修复,详见 `doc/plan/fix-task-resume-data-corruption.md`。本次问题与 pull 改造**无关**——日志已验证 pull 链路对齐正确(`[ResumeMount] writeOffset == reader 建连 offset`、`[RetryReader] Read 调用方 = transport.serveSpecsPull`、257 续传 `writeOffset=2487633 == Range@2487633`)。

本次是 `taskManager` 在**高频暂停/恢复父任务**的 stress 场景下暴露的**状态机并发失稳**。实测复现:对含 7 子任务(作品 `Konbini interior`,父任务 254)的父任务,以约每秒 2–3 次的频率反复暂停/恢复,出现下述四个问题。

> 本文档已根据现状代码(`manager.go`/`model.go`/`persistent_store.go`)核对修正。核心结论:**四个问题的真正根因是"dispatch 阶段无树级互斥"(纪律 2)引发的双重 dispatch 竞态**;其余纪律是对竞态窗口的收口与防御。

## 任务树操作纪律(核心原则)

四条纪律,收口到 `taskManager`。**纪律 2 是根因,纪律 1/3/4 是对竞态窗口的堵截与边界处理。**

1. **executeTask 入口终态守卫**:Resume/Pause 的 dispatch 谓词已跳过终态子任务(`ResumeTaskTree` 仅 dispatch `Paused`/`Pausing`,`buildOrReuseChild` 的 `skipTerminal` 跳过 `Finished`/`Failed`/`PartlyFinished`),但 `executeTask` 入口缺终态守卫。补上:goroutine 真正执行时若任务已处 `Finished`/`Failed`/`PartlyFinished`,直接 return,不 `setState(Processing)`。作用是兜底纪律 2 竞态下"滞后 goroutine 把 Finished 拉回 Processing"。
2. **Resume/Pause/Stop dispatch 阶段树级互斥**:`ParentTask` 增加 per-parent 的 `dispatchMu`,`ResumeTaskTree`/`PauseTaskTree`/`StopTaskTree` 的 dispatch 阶段(遍历子任务 + `prepareForResume` + `tryDispatch` / `Pause` / `Stop`)持锁串行。dispatch 阶段是纯内存快操作,不影响下载并发;作用是杜绝同一任务被并发 dispatch 出多条 `executeTask` goroutine。配合修复 `removeFromQueue` 只清除首个匹配的缺陷(同名重复入队时残留)。
3. **中间态不响应 Resume**:Resume/Pause 遍历子任务时,`Pausing`/`Stopping` 直接跳过(当前 `ResumeTaskTree` 会 dispatch `Pausing` 态,是纪律 3 要堵的口)。这是用户在 `doc/todo.md#L30` 提出的思路。
4. **已完成 role 不进入下载**:Resume 返回 specs 后,丢弃 `streamOffsets[role] > 0 && streamOffsets[role] >= spec.Size` 的 spec(磁盘已写满即视为完成),不为其 mount/进入 downloadLoop,杜绝 `Range@满` → 416。

> 原"纪律 5:父任务 finished 基于 DB 终态查询"经核对**不实施**(详见下文"已排除的方案")。

---

## 问题清单与日志证据

### 问题 1:父任务终态延迟 / Finished 被拉回 Processing

**现象**:所有子任务进入终态后,父任务仍处于 `Processing`,经过较长的时间(或关闭任务页面再打开)后才渲染为终态;日志中出现过 `finished=2/7 → 1/7` 的计数倒退。

**日志证据**:
```
10:10:10.246  恢复任务树 254
10:10:10.488  恢复任务树 254   ← 242ms 内重复
10:10:11.138  恢复任务树 254
10:10:11.603  恢复任务树 254   ← 2.4 秒内 6 次树级 Resume
...
10:10:16.761  finished=2/7
10:10:18.790  executeTask 257(resumeFromDB=true); 257 Finished → Processing
10:10:18.790  finished=1/7      ← 计数倒退(2→1)
```

**根因**(经核对修正):不是 ResumeTree 直接 dispatch 了 Finished 任务(dispatch 谓词已跳过),而是**双重 dispatch 竞态**(纪律 2):257 在 `Paused` 态被并发的 ResumeTree 各自 dispatch;其中一个 goroutine 把 257 跑到 `Finished` 后,另一个滞后的 goroutine 仍执行 `resumeFromPersistedState → setState(Processing)`,把 `Finished` 拉回 `Processing`。`ParentTask.RefreshState` 每次都是即时重新计数(非累加),倒退是 257 内存态被错误翻转的**症状**,不是计数方法的问题。父任务长时间停在 `Processing` 是因为 `RefreshState` 看到 257 仍 `Processing`;重开页面从 DB 重读才正确(DB 中 257 已 Finished)。

**修复**:纪律 2(根因)+ 纪律 1(executeTask 入口终态守卫,兜底滞后的 goroutine)。

### 问题 2:暂停子任务自动完成

**现象**:某次恢复父任务后,一个子任务仍处于 `Paused`,在用户无操作的情况下,它自行进入 `Finished`(资源正常)。

**日志证据**:
```
10:10:13.200  prepareForResume 259(PendingResourceID=37); executeTask 259
10:10:16.352  259 Processing → Finished
```
此前 259 处于 `Paused`(被 10:10:13.042 暂停树置 Paused)。

**根因**:竞态下 259 被多次入队 `waitingQueue`(并发 ResumeTree 各 dispatch 一次);用户暂停树时,`PauseTaskTree` 第一阶段对 `Waiting` 态调用 `removeFromQueue`,但 `removeFromQueue` 匹配首个即 return,**残留的重复队列条目**在信号量释放后由 `dispatchFromQueue` 取出再次执行。"无操作"是表象,实际是前序累积的重复 dispatch 在跑。资源正常是因为 pull 对齐生效。

**修复**:纪律 2(dispatch 互斥,源头不再产生重复入队)+ 修复 `removeFromQueue` 清除全部同名条目。

### 问题 3:状态切换延迟

**现象**:子任务在 `Paused` / `Processing` / `Waiting` 之间切换有较高延迟(秒级)。

**日志证据**:
```
10:10:14.403  WARN 删除旧文件失败 ... The process cannot access the file because it is being used by another process.
10:10:28.468  downloadLoop 结束 361 elapsed=10.48s
10:10:39.806  downloadLoop 结束 360 elapsed=20.49s
```

**根因**:同一任务被并发 dispatch 多条 `downloadLoop`,争抢同一目标文件(锁失败);downloadLoop 在竞争 + 等待被拉长到 10–20 秒;`prepareForResume` 还要等待旧 goroutine 退出。状态切换自然迟钝。

**修复**:纪律 2(从源头消除并发叠加)+ 纪律 3(中间态跳过)。

### 问题 4:已完成 role 续传导致 416 Failed

**现象**:某子任务资源已下载完整,却在某次恢复时 Resume 失败(`416 Range Not Satisfiable`),被置 `Failed`。

**日志证据**:
```
10:10:39.694  任务 261 跨重启续传 offsets=map[main:2347191]   ← 等于完整 size
10:10:41.199  路径一 Probe Range@2347191 → 416; 走路径二
10:10:42.395  路径二 SetValidBytes(2347191) → 请求失败 416
10:10:42.395  任务 261 跨重启 Resume 失败 → 261 Processing → Failed
```

**根因**:261 的 main 资源磁盘文件已写满(`os.Stat` = 2347191),但 `persistent_store.Status` 仍为 `Incomplete`(0)(写满字节的 goroutine 未走到 `storeWriter.Complete()`,或竞态下状态未对齐)。`resumeFromPersistedState` 的 streamOffsets 计算只按 `Status==Complete` 排除,于是把 `info.Size()` 放入 `streamOffsets`,插件路径一/二都以 `Range@满` 请求,服务端必然 416。

**修复**:纪律 4(Resume 返回后按 `streamOffsets[role] >= spec.Size` 过滤已完成 role)。

---

## 修复方案(代码改动点)

### 1. dispatch 阶段树级互斥(纪律 2,根因)— `model.go` / `manager.go`

- `ParentTask`(`model.go:1602`)增加 `dispatchMu sync.Mutex`。
- `ResumeTaskTree`/`PauseTaskTree`/`StopTaskTree`(`manager.go:642`/`583`/`673`)在遍历子任务 + dispatch 前持有对应 parent 的 `dispatchMu`,dispatch 阶段结束释放。注意:仅序列化 dispatch 阶段,`executeTask`/`downloadLoop` 仍在锁外异步执行。
- 三处的 parent 解析(`resolveParentKey`)后,取到 `ParentTask` 即 `parent.dispatchMu.Lock()`,defer 解锁。
- 修复 `removeFromQueue`(`manager.go:569`):不要 `return`,遍历清除全部匹配 `taskId` 的条目。

### 2. executeTask 入口终态守卫(纪律 1,兜底)— `manager.go:519`

- `executeTask` 在 `ctx.Done` 检查之后、`run()`/`resumeFromPersistedState()` 之前,增加:`if s := task.GetState(); s == Finished || s == Failed || s == PartlyFinished { log + return }`。
- 不覆盖已设的终态,滞后的 goroutine 直接退出。

### 3. 中间态跳过(纪律 3)— `manager.go:660`(Resume 遍历)

- `ResumeTaskTree` 遍历子任务:`state != Paused`(去掉对 `Pausing` 的 dispatch);`Pausing`/`Stopping` 直接 `continue`。
- `PauseTaskTree` 已基本满足(`Pause()` 对非 `Processing` 返回错误),保持现状。

### 4. 已完成 role 过滤(纪律 4)— `model.go:1096` 附近(`resumeFromPersistedState`)

- 在插件 `Resume` 返回 specs 之后、mount 之前,过滤:对每个 returned spec,若 `streamOffsets[spec.Role] > 0 && streamOffsets[spec.Role] >= spec.Size`(`spec.Size > 0`),视为已完成,从 specs 中剔除(并入 `completedRoles`)。
- 过滤后若 specs 为空,走已有的"无未完成轨道 → Finished"分支(`model.go:1096`)。
- `PersistentStore` 无 size 字段、且 spec.Size 在 Resume 返回前不可得,故判定**只能后置**在 Resume 返回后(不放在 streamOffsets 计算处)。
- 插件侧补充(可选):Resume 收到 `offset >= 完整 size` 的 role 返回空 spec。

### 已排除的方案

- **原纪律 5(父任务 finished 基于 DB 终态查询)**:经核对 `ParentTask.RefreshState`(`model.go:1650`)已每次调用即时重计 `fc`,**非 goroutine 累加**。倒退是纪律 2 竞态下 257 内存态被翻转的症状,修好纪律 2 即消除。改为查 DB 会给每次状态刷新(flushLoop/快照)引入 DB 查询,增加延迟与连接压力,**不实施**。

## 涉及文件

- `backend/taskManager/manager.go`:`ResumeTaskTree`/`PauseTaskTree`/`StopTaskTree`(纪律 2 dispatch 互斥)、`executeTask`(纪律 1 终态守卫)、Resume 遍历(纪律 3)、`removeFromQueue`(纪律 2 配套修复)
- `backend/taskManager/model.go`:`ParentTask` 增加 `dispatchMu`(纪律 2)、`resumeFromPersistedState` 的已完成 role 过滤(纪律 4)

## 验证

| 场景 | 预期 |
|---|---|
| 高频暂停/恢复父任务(每秒 2–3 次,持续 10 秒) | 无并发 dispatch(同一任务同时只有一条 downloadLoop);`Finished` 不回退 `Processing`;状态切换在百毫秒级;无文件锁告警 |
| 子任务已下完整后恢复 | 该 role 不进入下载(无 416);子任务保持终态 |
| 所有子任务终态后 | 父任务立即刷新为终态(`Finished`/`PartlyFinished`/`Failed`),无需重开页面 |
| Paused 子任务在仅暂停(不恢复)时 | 保持 Paused,不自行进入 Processing/Finished |

## 关联

- 资源损坏修复(已完成):`doc/plan/fix-task-resume-data-corruption.md`
- 用户原始思路:`doc/todo.md#L30`(中间态不响应)——本计划将其作为纪律 3 收入,并补齐 dispatch 互斥(纪律 2,根因)、executeTask 终态守卫(纪律 1)、已完成 role 过滤(纪律 4)
