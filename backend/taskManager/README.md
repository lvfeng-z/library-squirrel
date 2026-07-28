# taskManager 模块说明

## 一句话职责

任务**运行时调度引擎**：在内存中管理任务树的生命周期（启动/暂停/恢复/停止/重试）、并发控制、状态机推进与进度推送。负责"任务怎么跑起来"。

## 边界

- 与 **task**：`task` 负责任务**实体**的持久化 CRUD（创建、查询、删除、状态字段读写，属静态数据）；`taskManager` 负责任务**运行时**的调度执行（属动态）。task 写数据库，taskManager 跑内存状态机并把结果批量刷回数据库。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `StartTaskTree(taskId, isLeaf)` | 启动任务树（isLeaf=true 时仅启动叶子节点） |
| `PauseTaskTree` / `ResumeTaskTree` | 暂停 / 恢复任务树 |
| `StopTaskTree` | 停止任务树 |
| `RetryTaskTree` | 重试任务树 |
| `Redownload(taskIds, storeRoles, includeWorkInfo)` | 板块重执行（资源 store_type 集合 + 是否含作品元数据；空资源集 + 不含元数据 = 全集） |
| `GetTaskState(taskId)` | 查询单任务状态（综合内存 + 数据库） |
| `GetTaskTreeState(taskId, isLeaf)` | 查询任务树聚合状态 |
| `GetTaskSnapshot()` | 获取所有活跃任务的完整状态快照 |
| `IsIdle()` | 是否空闲（无运行中任务） |
| `ConfirmReplace` / `ConfirmReplaceBatch` | 用户确认重复作品的替换 / 跳过 |

## 核心概念

- **ManagedTask / ParentTask**：内存中的运行任务与父任务聚合。
- **信号量**：`maxParallel` 控制全局并发数，超出则进 FIFO 等待队列。
- **板块执行模式**（runMode）：`{workInfo, storeRoles}`——`workInfo` 为作品元数据独立板块，`storeRoles` 为所选资源 store_type 子集（main/thumbnail/...）。mode 不再由调用方传参，而从 task 实体的 `StoreRoles`/`IncludeWorkInfo` 字段派生（`runModeFromTask`），故暂停 / 跨重启恢复时保持原板块选择，不退化为全量。`Redownload` 入口负责写入这两个字段后启动。
- **进度推送器**（TaskProgressPusher）：两种实现——Wails 事件直推、快照模式（SnapshotPusher）。
- **状态落盘**：终态（Finished/Failed/PartlyFinished）即时同步写库，进程崩溃也不丢失；非终态（Paused）状态、进度、pending_resource_id 仍攒在内存，由 `flushLoop` 每 200ms 批量刷库，避免高频写放大。终态即时写与 `doFlush` 的批量 status 写都在 `pendingMu` 临界区内，互斥执行，杜绝批量通道的过时快照回写覆盖终态。

## 依赖关系

- 依赖：`Repository`（任务树查询 / 批量状态设置）、`task` 包（TaskStatusEnum 状态枚举）、`WorkDirProvider`、`FileNameFormatProvider`、`pluginExecFactory`（插件执行器，TaskExecutor）、`TaskProgressPusher`
- 被依赖：前端任务执行面板（操作栏）

## 关键设计

- **内存 + 数据库双轨状态**：运行态以内存为准（`GetTaskSnapshot` / `IsIdle`），查询态综合两者（`GetTaskState`）。
- **per-task actor 模型**：每个 `ManagedTask` 持一条常驻 goroutine(`actorLoop`) + 命令通道(`cmdCh`),任务级可变状态只在 actor goroutine 内修改。外部操作(`Pause`/`Resume`/`Stop`/`ConfirmReplace`/`dispatch`)退化为向 `cmdCh` 非阻塞投递命令(`postCmd`,投递路径不持 `m.mu` 防死锁),actor 串行处理(`handleRunCmd`/`handlePauseCmd`/`handleStopCmd`)。命令队列天然记忆(无丢失唤醒)且保证时序——pause 排在 resume 之后最终生效,从结构上消除滞后 goroutine 按陈旧标志重派发。创建层 `claimTask`/`claimParent`(`m.mu` 下 insert-or-get)保证同一 taskId 只有一个对象;`actorStarted` CAS 保证一任务一 actor。长任务(downloadLoop)执行期间 `cmdWatcher` 并发监听 `cmdCh`,收到 pause/stop 立即 `runCancel` 中断在途(经项一的 reader 响应 ctx 传到 copyLoop);`runCtx.Done` 统一中断 copyLoop(B 阶段已删 `pauseCh` 双轨)。
- **执行入口与信号量**:`startTaskTrees`(开始/重试/板块重执行)与 `resumeTaskTrees`(恢复)从 DB 加载任务树后调 `dispatch`;`ResumeTaskTree`(内存内恢复)与 `ConfirmReplace` 直接 `postCmd`。`dispatch` 是首启入口(`actorStarted` CAS + 投 `cmdStart`/`cmdResume`)。信号量槽位获取移入 actor 内部(`handleRunCmd` 中 `select semaphore`,取不到则 `enqueueSelf` 入 `waitingQueue`);槽位释放后 `dispatchFromQueue` 向队首投 `cmdResume` 唤醒。`PauseTaskTree`/`StopTaskTree` 对子任务并行投命令(各 actor 独立处理,Stop 带 ack 等待终态后 `cleanupStoppedTree`)。`Redownload` 先持久化板块选择到 task 再调 `startTaskTrees`。
- **依赖全部接口注入**：插件执行器、仓储、进度推送器均通过构造函数注入，`Manager` 不直接持有具体 Service。
- **store 处理范围限定在插件本次声明的板块**：任务执行（首次下载/重试/Redownload/重启续传）全程只动插件 Start/Resume 返回的 StoreSpec 对应板块——挂载删旧走 `DeleteByResourceIdAndTypes(store_type IN 本次 specs 的 role)`（`mountResourceStores`）、板块选择走 `filterSpecsByRoles`（按 storeRoles 过滤插件 specs）、重启续传 `resumeFromPersistedState` 读全集但已 Complete 者进 `completedRoles` 跳过。`videoMain`（分离流场景由用户「合并」操作挂载、不来自插件 specs 时）对任务流程透明：不被删/覆盖/失效，重下源轨道后旧 videoMain 去留由用户决定；本地导入插件可直接声明 videoMain（来自插件 specs，任务正常处理）。勿新增"重下/重置时清空该 Resource 全部 store"之类逻辑。
