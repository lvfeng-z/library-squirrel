# 重构:建立"一任务一 goroutine"dispatch 不变量

> **状态:已实现(2026-07-04),高频启停回归于 2026-07-05 修复。** 两层守卫(创建层 `claimTask`/`claimParent` + 派发层 `dispatch` CAS + `pendingResume`)落地,`runExited`/`runMu`/5 秒等待补偿机制删除。taskManager 单测全绿。**高频启停 stress 曾复现"某子任务在其余皆暂停时单独 Processing"**(任务 257):根因是 `pendingResume` 不被后续 Pause 清除,卡在不可取消插件 Read 滞后退出的 goroutine 按陈旧标志重派发;修复为 `PauseTaskTree`/`StopTaskTree` 开头对所有子任务 `pendingResume.Store(false)`(用户最新意图作废陈旧 resume)。观察到的状态切换延迟系插件 gRPC 同步耦合 + 信号量排队,属"不在本次范围"。`-race` 待 CGO 可用环境补跑。

## 背景与根因

从 TypeScript 重构为 Go 之后,任务调度的状态机问题持续存在(高频暂停/恢复下:Finished 被拉回 Processing、Paused 任务自动完成、文件锁/秒级切换延迟等)。经架构分析确认**总根源是一条丢失的隐式不变量**。

**为什么 TS 版没有此问题**:JS 单线程事件循环天然串行化所有 dispatch,同一任务不可能有两条执行流并发。Go 重写引入了真并行,却未补上单线程模型免费提供的互斥。`tryDispatch`(manager.go:486)与 `dispatchFromQueue`(manager.go:561)两个启动点、四个调用入口(`loadAndStartTaskTrees`/`ResumeTaskTree`/`ConfirmReplace`/`ConfirmReplaceBatch`)均**无"该任务是否已有 goroutine 在跑/在队列"的检查**。

**当前用补偿代替禁止**:`runExited`/`runMu`/`prepareForResume` 的 5 秒等待 + `executeTask` 登记/关闭生命周期信号,这套机制的存在本身就是"允许重入、在转换边界善后"的证明。补偿有漏洞(`tryDispatch → go executeTask` 与 `runExited` 赋值之间的窗口)、有超时兜底(5 秒后强行继续 → 放弃互斥)、有队列无去重(`removeFromQueue` 只清首个)。本计划**正面建立不变量,删除补偿机制**。

## 本次范围(严格限定)

**只做两件事:**

1. 建立"一任务至多一条 `executeTask` goroutine"不变量(dispatch 互斥)。
2. 删除为重入兜底的补偿机制(`runExited`/`runMu`/5 秒等待)。

**不做任何"任务树操作纪律"相关的改动。** 既不保留 `fix-task-tree-concurrent-pause-resume.md` 里的纪律 1(executeTask 终态守卫)、纪律 3(中间态不响应),也不做纪律 4(已完成 role 过滤)。理由:这些纪律大概率是"不变量缺失"的派生症状,在不变量成立后应自然消失;**是否真的消失,留待重构后通过 stress 回归观察,而非在本计划里预先假定并捆绑实现**。这样把"架构根因"和"边界 case"分清楚,爆炸半径最小。

## 目标不变量

> **对任意 `ManagedTask`,任意时刻至多存在一条 `executeTask` goroutine。**

`executeTask` 内部派生的子 goroutine(`downloadLoop` 的 per-stream `copyLoop`、`drainUnselectedReaders`)属于该 goroutine 的执行内部,不计入"多条 dispatch"。

派生约束:
1. dispatch 是**互斥 claim**:`idle → queued → running → idle` 单一状态机,并发 dispatch 调用者通过 CAS 决出唯一 winner,输者立即 no-op(dispatch 幂等:同一任务 Resume 两次等于 Resume 一次)。
2. `waitingQueue` 中同一任务至多一条(由 claim 保证)。
3. 暂停期间到达的 Resume 不丢失(由"待处理 resume"标志 + goroutine 退出检查保证),**不需要任何"等待旧 goroutine"的逻辑**。

## 当前如何违反不变量(待消除)

| 路径 | 现状 | 违反点 |
|---|---|---|
| 并发 `ResumeTaskTree` | 两者都见子任务 Paused → 各调 `prepareForResume + tryDispatch` | 两发 |
| `tryDispatch` 入队后再次 dispatch | 无 claim 检查,可重复 append `waitingQueue` | 队列重复条目 |
| `dispatchFromQueue` 退出时 | 取队首 `go executeTask`,不检查该任务是否已有 goroutine | 重复启动 |
| `prepareForResume` 等 `runExited` | `tryDispatch` 已 `go` 但 goroutine 未执行到赋值 `runExited` → 读到 nil 不等 | 窗口 → 双 goroutine |
| 5 秒超时兜底 | 超时后强行继续 resume | 放弃互斥 |
| `removeFromQueue` | 匹配首个即 return | 残留重复条目 |
| **开始路径对象创建**(`loadAndStartTaskTrees`) | "重复执行保护"基于快照(182–191),两个并发调用都快照于彼此 `addTask` 之前 → 都通过 → 各自 `buildOrReuseChild + newManagedTask + addTask`(`taskMap[id]=task` 直接覆盖,963)→ 创建出**两个 ManagedTask 对象** | 派发层 CAS 活在对象上,两对象各自命中,**挡不住** → 每子任务双 goroutine |

## 设计:dispatch 生命周期状态机

### 状态

`ManagedTask` 增加 `dispatchState atomic.Int32`:

| 状态 | 含义 | 持信号量 | 在 waitingQueue |
|---|---|---|---|
| `dsIdle` | 无 goroutine | 否 | 否 |
| `dsQueued` | 已 claim,等槽位 | 否 | 是 |
| `dsRunning` | executeTask goroutine 存活 | 是 | 否 |

辅以 `pendingResume atomic.Bool`:**仅当 Resume 命中"goroutine 正在跑(CAS 输)"时置位**,表示"当前 goroutine 退出后应重新 dispatch(恢复)"。它是丢失唤醒的补救,**不是阻塞等待**。

### 两层守卫(缺一不可)

`dispatchState` 活在 `ManagedTask` 对象上,只能保证"**同一对象**只派发一条 goroutine"(恢复路径:操作 parentMap 里的现有对象,直接生效)。但**开始路径会创建对象**,若同一 taskId 能创建出两个对象,派发层 CAS 在两对象上各自命中,无从比对。故不变量需要两层互补守卫:

| 层 | 守卫 | 机制 | 适用路径 |
|---|---|---|---|
| **创建层** | 同一 taskId 只有一个 `ManagedTask`/`ParentTask` 对象 | `claimParent`/`claimTask`:`m.mu` 下 insert-or-get,输者复用赢者的对象并跳过 | 开始/重试/重下(从 DB 创建对象) |
| **派发层** | 同一对象只派发一条 goroutine | `dispatchState` CAS(idle→queued) | 恢复(操作现有对象);也是创建层之后的第二道闸 |

### 跃迁与 owner

| 跃迁 | 触发者 | 机制 |
|---|---|---|
| `idle → queued` | `tryDispatch`(唯一 dispatch 漏斗) | `dispatchState.CompareAndSwap(dsIdle, dsQueued)`,输者 no-op |
| `queued → running` | `tryDispatch`(即时获得槽位)或 `dispatchFromQueue`(出队获得槽位) | 获得信号量后 `Store(dsRunning)`,再 `go executeTask` |
| `running → idle` | `executeTask` 退出 defer | 持 `m.mu`:`Store(dsIdle)`,随后 `dispatchFromQueue` |
| `running → queued` | `executeTask` 退出 defer(发现 `pendingResume` 且任务处于可恢复态) | 持 `m.mu`:重置可变状态 → `Store(dsQueued)` → 重新走槽位获取 |

**锁顺序**:`m.mu` → `dispatchState`。`dispatchState` 的 `running → idle/queued` 跃迁在持 `m.mu` 时 `Store`(与现有 `dispatchFromQueue`/`removeFromQueue` 都持 `m.mu` 对齐);`idle → queued` 用 CAS 不获取 `m.mu`;任何持 `dispatchState` 读取的路径都不反向获取 `m.mu`,杜绝循环。

### 创建层 claim(开始路径)

`claimParent` / `claimTask` 把现有"快照判重(182–191)+ 末尾覆盖式 `parentMap[id]=...`/`taskMap[id]=...`(302–304/963)"合并为**一次原子 insert-or-get**:

```
claimParent(id, name) (*ParentTask, created bool):
    m.mu.Lock(); defer m.mu.Unlock()
    if existing, ok := m.parentMap[id]; ok { return existing, false }
    p := NewParentTask(id, name)
    m.parentMap[id] = p
    return p, true

claimTask(t *domain.Task) (*ManagedTask, created bool):
    m.mu.Lock(); defer m.mu.Unlock()
    if existing, ok := m.taskMap[t.ID]; ok { return existing, false }
    mt := m.newManagedTask(t)   // 内部不再 addTask
    m.taskMap[mt.taskId] = mt
    return mt, true
```

`loadAndStartTaskTrees` 处理每个单元时**先 claim**:`created=false`(并发赢家已创建)→ 直接 skip 该单元(由赢家负责 build children + dispatch);`created=true` → 赢家继续 `buildOrReuseChild`(改为内部 `claimTask`,输者复用)/`processParentUnit` 与派发。**快照式"重复执行保护"删除**,由 claim 取代。

注意:终态清理(`cleanupFinishedTask`/`cleanupStoppedTree` 删 parentMap/taskMap)在 claim 之后——任务终态后从 map 移除,下次开始/重试的 claim 拿不到现成对象、正常创建新的,不受影响。

### 单一 dispatch 入口

**所有任务进入执行流程都经 `dispatch(task) bool` 这一个函数**(开始/确认/从 DB 恢复/内存恢复统一入口)。`pendingResume` 的 resume 意图只有一个调用方需要(内存内 `ResumeTaskTree`),直接内联在那一个循环里,不再单设 `requestResume`/`tryClaim`,避免"谁是入口"的命名歧义:

```
dispatch(task) bool:                // 唯一统一入口
    if !task.dispatchState.CAS(dsIdle, dsQueued):
        return false                // 已 dispatched(幂等)
    select {
    case m.semaphore <- struct{}{}:
        task.dispatchState.Store(dsRunning)
        go m.executeTask(task)
    default:
        m.mu.Lock()
        m.waitingQueue.append(task) // CAS 已保证不重复入队
        m.mu.Unlock()
        task.setState(TaskStateWaiting)
    }
    return true
```

各调用方:
- **开始/重试/重下/从 DB 恢复**(`loadAndStartTaskTrees`)/ **确认替换**(`ConfirmReplace`/Batch):`m.dispatch(task)`,返回值可不关心(对象刚创建/刚释放槽位,CAS 理论上必赢)。
- **内存内恢复**(`ResumeTaskTree` 循环体,唯一需要 resume 意图处):
  ```
  for child:
      if child.GetState() != Paused && != Pausing: continue
      if !m.dispatch(child):              // 已在跑(暂停退出过渡窗口)
          child.pendingResume.Store(true) // 退出契约据此重派发,不丢唤醒
  ```

`dispatchFromQueue` 出队时 `Store(dsRunning)` 后 `go executeTask`。`removeFromQueue` 移除时同步 `task.dispatchState.Store(dsIdle)`(把 `queued` 任务拉回 `idle`,否则 Pause 一个 queued 任务会永久卡死 dispatch)。`executeTask` 退出契约的"重派发"分支不走 `dispatch`(已持 `m.mu`、直接 `Store(dsQueued)` 再取槽位),与入口函数解耦。

### executeTask 退出契约(吸收"丢失唤醒")

```
executeTask(task):
    defer:
        recover...
        <-m.semaphore                              // 先释放槽位
        m.mu.Lock()
        pending := task.pendingResume.Swap(false)  // 原子读清
        s := task.GetState()
        if pending && (s == Paused || s == Pausing):
            task.prepareForResume()                // 此时无并发 goroutine,纯字段重置
            task.dispatchState.Store(dsQueued)     // 重新 claim(已持 m.mu)
            m.mu.Unlock()
            m.acquireSlotAndRun(task)              // 获取槽位 → go executeTask(可能再入队)
            return
        task.dispatchState.Store(dsIdle)
        m.mu.Unlock()
        m.dispatchFromQueue()
    // ...原有 run()/resumeFromPersistedState() 主体不变...
```

`prepareForResume` **只保留"重置可变字段"(cancel + 重建 ctx、`closeStreamReaders`、`streams=nil`、重建 `pauseCh`、按 `PendingResourceID` 设 `resumeFromDB`)**,**删除等待旧 goroutine 整段**。重置在退出契约持 `m.mu` 时进行,无并发 goroutine 访问这些字段,无需任何等待。

## 改动点

### `backend/taskManager/model.go`

1. `ManagedTask` 增加 `dispatchState atomic.Int32`、`pendingResume atomic.Bool`;**删除 `runExited chan struct{}` 与 `runMu sync.Mutex`** 及相关注释(343–349)。
2. `prepareForResume`(1219–1247):删除"等待旧 goroutine 退出"整段(1223–1232),仅保留可变字段重置。

### `backend/taskManager/manager.go`

1. `executeTask`(496–548):重写 defer 为上文"退出契约";**删除 `runExited` 登记/关闭**(497–512)。主体 `run()`/`resumeFromPersistedState()` 调用不变。
2. `tryDispatch`(483–493)**重命名为 `dispatch(task) bool`**(唯一统一入口):顶部 `dispatchState.CAS(dsIdle, dsQueued)` claim,失败返回 false;获得槽位后 `Store(dsRunning)` 并返回 true;未得槽位则入队、`setState(Waiting)`、返回 true。**不新增 `requestResume`/`tryClaim`**——resume 意图由调用方内联处理(见 5)。
3. `dispatchFromQueue`(551–566):出队后 `queued → running`(`Store(dsRunning)`)。
4. `ResumeTaskTree`(660–667):循环体改为 `if !m.dispatch(child) { child.pendingResume.Store(true) }`(仅对 `Paused`/`Pausing` 子任务);去掉原 `prepareForResume + tryDispatch` 直调,`prepareForResume` 的字段重置移到 executeTask 退出契约的重派发分支。
5. `removeFromQueue`(569–580):移除时同步 `task.dispatchState.Store(dsIdle)`。顺手遍历清除全部匹配条目(防御性,CAS 生效后理论上不出现重复)。
7. **创建层 claim(开始路径,补缺)**:新增 `claimParent(id,name) (*ParentTask, created bool)` 与 `claimTask(t) (*ManagedTask, created bool)`,均 `m.mu` 下 insert-or-get。
   - `processParentUnit`(275–307):开头 `claimParent`;`created=false` 直接返回 nil(并发赢家负责),`created=true` 才 build children。删除末尾覆盖式 `m.parentMap[actualParentId] = parentTask`(302–304)。
   - `buildOrReuseChild`(312–345):内部 `newManagedTask`+`addTask` 改为 `claimTask`;输者返回赢家对象(后续 dispatch 走同一对象的派发层 CAS)。`addTask`(961–965)改为 insert-or-get 语义或被 `claimTask` 取代。
   - `loadAndStartTaskTrees`(142–271):**删除快照式"重复执行保护"**(182–191 的 `runningTasks`/`runningParents` 快照 + 221–232 的快照判重),由 claim 在创建时原子保证。独立任务的 standalone 分支同样走 `claimTask`。
   - 注意:RetryTaskTree/Redownload 也经 `loadAndStartTaskTrees`,一并受益。

### `backend/taskManager/model_multi_stream_test.go`

- 删除 `TestPrepareForResume_WaitsForGoroutineExit`(649–678):新设计**显式不等待**。替换为新回归:`executeTask` 运行期间置 `pendingResume=true` 并使任务进入 Paused,验证 goroutine 退出后自动重新 dispatch,且全程**至多一条 goroutine**(goroutine 计数器断言)。
- 新增"开始路径对象唯一性"回归:并发两次 `StartTaskTree(同一 taskId)`,断言 `taskMap`/`parentMap` 中该任务只有一份对象、全程至多一条 goroutine。

## 不在本次范围

以下均**显式延后**,本计划不触及。其中前四项是原"四条纪律",它们的去留取决于不变量成立后是否还有残留现象,应**在本次重构落地、stress 回归通过后单独观察**再决定是否需要:

- **纪律 1**(executeTask 终态守卫):不变量成立后无滞后 goroutine,大概率不需要。
- **纪律 2**(树级互斥):被 per-task CAS 完全吸收,不需要。
- **纪律 3**(中间态不响应 Resume):`pendingResume` 对 Pausing 任务给出"退出后恢复"的语义,比"直接忽略"更不丢用户意图;原纪律大概率不需要,甚至与之冲突(忽略会丢 Resume)。待观察。
- **纪律 4**(已完成 role 过滤,`resumeFromPersistedState` 内):独立的数据正确性问题(store.Status 与磁盘不一致 → 416),与并发无关。**大概率仍需要**,但单独评估、单独实现,不混入本次。
- **插件 gRPC 同步耦合**(`executeTask` 持信号量调插件、`Pause()` 状态转换中调插件 → 卡住钉死槽位):独立计划,建议给插件调用加 context 超时。
- **终态批量落盘**(终态走 200ms flush):独立计划,建议终态状态写即时落盘。
- **per-task actor 全重构**:本计划的 CAS+pendingResume 是"轻量 actor",保证不变量而不重写执行模型。若后续发现 CAS 路径仍有边界问题,可升级为常驻 goroutine + command channel(真 actor),届时 `dispatchState` 退化为 actor 的 `started` 标志。

## 验证

| 场景 | 预期 |
|---|---|
| 高频暂停/恢复父任务(每秒 2–3 次,持续 10 秒) | 同一任务任意时刻至多一条 `executeTask` goroutine(goroutine 计数断言);无文件锁;状态切换百毫秒级;`Finished` 不回退 |
| Resume 在 pause-exit 窗口到达 | `pendingResume` 生效,goroutine 退出后自动重新 dispatch,Resume 不丢失 |
| 并发开始同一任务(连点 Start / Start+Retry / 双 Resume 从 DB 加载) | `claimParent`/`claimTask` 保证 map 中只有一份对象,全程至多一条 goroutine |
| Pause 命中 `queued` 任务 | `removeFromQueue` + `dispatchState → idle`,后续 dispatch 可重新 claim |
| `go test ./backend/taskManager/...` | 全绿(含改写后的 prepareForResume 回归) |
| `go test -race ./backend/taskManager/...` | 无 data race |

### 不变量断言(建议)

- `ManagedTask` 增加 `atomic.Int32 liveGoroutines`,executeTask 入口 `+1`、defer `-1`;stress 测试任意时刻断言 `≤ 1`。

## 重构后的观察清单(决定纪律去留)

落地 + stress 回归通过后,针对原四个问题现象复查:

1. 父任务终态延迟 / Finished 被拉回 Processing —— 应随不变量消失。若残留 → 重新评估纪律 1。
2. Paused 子任务自动完成 —— 应随不变量消失(无滞后 goroutine 把它拉回)。
3. 状态切换延迟 + 文件锁 —— 应随不变量消失(无并发 downloadLoop)。
4. 已完成 role 续传 416 —— **预期仍存在**(与并发无关)→ 此时单独做纪律 4。

观察结果决定 `fix-task-tree-concurrent-pause-resume.md` 的处置:若 1/2/3 消失,该文档缩减为只剩纪律 4(或并入本计划后续),其余删掉。

## 风险

| 风险 | 缓解 |
|---|---|
| `dispatchState` 与 `m.mu` 锁顺序引入新死锁 | 统一 `m.mu` → `dispatchState`,CAS 路径不获取 `m.mu`,反向不获取;审查全部跃迁点 |
| `pendingResume` 误置/漏消费 | 仅 `requestResume` CAS 输时置位;仅 executeTask defer 用 `Swap` 原子读清;`loadAndStartTaskTrees`/`ConfirmReplace` 不置位 |
| 信号量槽位在 `running → queued` 重 dispatch 路径泄漏 | 退出契约先 `<-m.semaphore` 释放,再走 claim+获取;槽位进出严格配对,review 核对 |
| 删除 `runExited` 破坏其他依赖 | 已确认仅 `prepareForResume` 与一个测试依赖,无外部引用 |
| `prepareForResume` 移到退出契约后,首次 Resume 的字段重置时机 | 首次 dispatch(从 idle claim)不经退出契约,无需重置(任务本就是干净状态);只有"重 dispatch"分支才需要重置,该分支已调用 `prepareForResume` |

## 涉及文件

- `backend/taskManager/manager.go`:`tryDispatch`/`requestResume`/`dispatchFromQueue`/`executeTask`/`removeFromQueue`/`ResumeTaskTree`
- `backend/taskManager/model.go`:`ManagedTask` 字段增删、`prepareForResume` 瘦身
- `backend/taskManager/model_multi_stream_test.go`:替换 `TestPrepareForResume_WaitsForGoroutineExit`

## 关联

- 现状校验与"四条纪律"(待观察后处置):`doc/plan/fix-task-tree-concurrent-pause-resume.md`
- 资源损坏修复(已完成):`doc/plan/fix-task-resume-data-corruption.md`
- 用户原始思路:`doc/todo.md#L30`(中间态不响应,可能被 pendingResume 取代)
