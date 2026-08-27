# taskManager 模块说明

## 一句话职责

任务**运行时调度引擎**：在内存中管理任务树的生命周期（启动/暂停/恢复/停止/重试）、并发控制、状态机推进与进度推送。负责"任务怎么跑起来"。

## 边界

- 与 **task**：`task` 负责任务**实体**的持久化 CRUD（创建、查询、删除、状态字段读写，属静态数据）；`taskManager` 负责任务**运行时**的调度执行（属动态）。task 写数据库，taskManager 跑内存状态机并把结果批量刷回数据库。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `StartTaskTrees(taskIds)` | 批量启动任务（全量执行） |
| `PauseTaskTrees` / `ResumeTaskTrees` | 批量暂停 / 恢复任务 |
| `StopTaskTrees` | 批量停止任务 |
| `RetryTaskTrees` | 批量重试任务（保留各任务已记录的执行模式） |
| `Redownload(taskIds, storeRoles, includeWorkInfo)` | 板块重执行（资源 store_type 集合 + 是否含作品元数据；空资源集 + 不含元数据 = 全集） |
| `GetTaskState(taskId)` | 查询单任务状态（综合内存 + 数据库） |
| `GetTaskTreeState(taskId)` | 查询任务状态：父任务返回聚合状态、叶子/独立任务返回自身状态 |
| `GetTaskSnapshot()` | 获取所有活跃任务的完整状态快照 |
| `IsIdle()` | 是否空闲（无运行中任务） |
| `CountActiveByPlugin(pluginPublicId)` | 统计插件名下运行中任务数（Processing/Pausing/Stopping/WaitingForInput）——供插件停用/换版拦截判据与确认框代价明示 |
| `ConfirmReplace` / `ConfirmReplaceBatch` | 用户确认重复作品的替换 / 跳过 |

## 控制语义

- **操作范围按对象类型**：操作**父任务**→作用于整棵树（全部子任务）；操作**叶子/独立任务**→仅作用于自身，不扩散兄弟。前端单行操作传 `[id]`、批量操作传多 id，统一走批量接口。
- **整树加载**：任务运行时整棵树加载到内存（`ParentTask.children` 含全部子任务，含未启动的兄弟），供父状态聚合与完成判定，避免「整树/非整树」分支。
- **单独 Start 叶子**：`processParentUnit` 的 `leafSet` 参数控制——整树仍加载到 children，但只 dispatch 被请求的叶子，其余兄弟保持 Created 未 dispatch。
- **运行中父单元重纳终态子任务**：父任务运行中对终态（Finished/Failed）子任务发起开始/重试/恢复时，`processParentUnit` 检测到父单元已 claim（`claimParent` 输者），复用已有父单元、经 `reinjectLeaves` 把终态子任务重建（旧对象 actor 已退出）并重新 dispatch，而非静默跳过；执行模式记录（recordMode）只写实际纳入调度的任务，不波及运行中兄弟。非终态子任务不纳入——Paused 的恢复走 `ResumeTaskTrees`→`resolveTargets` 内存路径，本不经此。
- **未启动兄弟守卫**：Pause/Stop/Resume 对 `!actorStarted && Created` 的兄弟（整树加载驻留但未 dispatch）跳过不投命令（避免误推 Paused/Failed）；`cleanupStoppedTree` 对这些兄弟 `cancel()` 退出 actor 防泄漏。
- **跳过解耦**：用户在 `ConfirmReplace` 选跳过的子任务置 `skipped` 标志（内存态、不持久化、仅本次 Start/Retry 执行有效，Resume 不读），`AllChildrenTerminal` 据此视为终态让父正常清理——不再把 Created 当终态（避免误伤未启动兄弟）。崩溃重启当作未跳过重新执行。
- **静默跳过**：Pause/Stop/Resume 对不在内存的 taskId（`!ok`）静默跳过 return nil（控制操作幂等）。

## 状态与生命周期不变量

- **手动触发，无自动执行/恢复**：任务创建后停留 Created，等用户手动"开始"才进 Processing；app 重启后 Paused 任务**不自动恢复**，需手动"恢复"。无启动钩子自动跑任务——`resumeFromDB`/跨重启续传仅在手动 `ResumeTaskTree` 触发时执行，启动只做 `InstallBundledPlugins` 等基建。
- **状态以资源实际状态为准**：续传/重下/完成判定依据资源实际状态（文件 `os.Stat` + `persistent_store.status`），不盲信 `PendingResourceID`（该字段可能与 store 实际脱节，如 recycleBin/backup 还原重建 store 后 task 仍持旧值）。`resumeFromPersistedState` 等决策点：文件存在 + Complete → Finished、文件缺失 → 重下、Incomplete → 续传；PendingResourceID 仅用于定位 resource，不作"是否续传"的决定因素。

## 核心概念

- **执行面策略（ExecutionStrategy，内置任务类型）**：控制面（actor 循环/信号量/状态机/进度/持久化/恢复调度）留在 taskManager，「任务主体怎么执行」外提为可插拔接口——`task.task_type` 非空的任务经按类型注册的策略执行（Manager 构造时注入策略表，app.go 装配；如 share-receive 归 share 模块实现），策略经 `StrategyHandle` 上报终态（Finish/Fail）与进度，RunCtx 取消（暂停/停止）即中断信号、终态由控制面接管；未注册策略的类型不可构建。`StrategyHandle` 另提供执行内挂起等待覆盖确认（`WaitReplaceConfirm`——置 WaitingForInput、逐条推冲突事件、等待期间释放信号量槽位，复用现有 `ConfirmReplace(taskId, action)` 整体答复）与终态回滚登记（`SetTerminalRollback`——失败/停止时由 setFailed 单点触发复活软删行）。插件任务（task_type 空）维持既有执行路径（板块组合 + 多轨下载/续传），其执行面的物理外提归 longops D 阶段。
- **ManagedTask / ParentTask**：内存中的运行任务与父任务聚合。
- **信号量**：`maxParallel` 控制全局并发数，超出则进 FIFO 等待队列。
- **板块执行模式**（runMode）：`{workInfo, storeRoles}`——`workInfo` 为作品元数据独立板块，`storeRoles` 为所选资源 store_type 子集（main/thumbnail/...）。mode 不再由调用方传参，而从 task 实体的 `StoreRoles`/`IncludeWorkInfo` 字段派生（`runModeFromTask`），故暂停 / 跨重启恢复时保持原板块选择，不退化为全量。`Redownload` 入口负责写入这两个字段后启动。
- **进度推送器**（TaskProgressPusher）：两种实现——Wails 事件直推、快照模式（SnapshotPusher）。
- **状态落盘**：终态（Finished/Failed/PartlyFinished）即时同步写库，进程崩溃也不丢失；非终态（Paused）状态、进度、pending_resource_id 仍攒在内存，由 `flushLoop` 每 200ms 批量刷库，避免高频写放大。终态即时写与 `doFlush` 的批量 status 写都在 `pendingMu` 临界区内，互斥执行，杜绝批量通道的过时快照回写覆盖终态。

## 依赖关系

- 依赖：`Repository`（任务树查询 / 批量状态设置）、`task` 包（TaskStatusEnum 状态枚举）、`WorkDirProvider`、`FileNameFormatProvider`、`pluginExecFactory`（插件执行器，TaskExecutor）、`TaskProgressPusher`、内置任务类型执行面策略表（task_type → ExecutionStrategy，构造注入）
- 被依赖：前端任务执行面板（操作栏）、share（实现 share-receive 执行面策略）

## 关键设计

- **内存 + 数据库双轨状态**：运行态以内存为准（`GetTaskSnapshot` / `IsIdle`），查询态综合两者（`GetTaskState`）。
- **per-task actor 模型**：每个 `ManagedTask` 持一条常驻 goroutine(`actorLoop`) + 命令通道(`cmdCh`),任务级可变状态只在 actor goroutine 内修改。外部操作(`Pause`/`Resume`/`Stop`/`ConfirmReplace`/`dispatch`)退化为向 `cmdCh` 非阻塞投递命令(`postCmd`,投递路径不持 `m.mu` 防死锁),actor 串行处理(`handleRunCmd`/`handlePauseCmd`/`handleStopCmd`)。命令队列天然记忆(无丢失唤醒)且保证时序——pause 排在 resume 之后最终生效,从结构上消除滞后 goroutine 按陈旧标志重派发。创建层 `claimTask`/`claimParent`(`m.mu` 下 insert-or-get)保证同一 taskId 只有一个对象;`actorStarted` CAS 保证一任务一 actor。长任务(downloadLoop)执行期间 `cmdWatcher` 并发监听 `cmdCh`,收到 pause/stop 立即 `runCancel` 中断在途(经项一的 reader 响应 ctx 传到 copyLoop);`runCtx.Done` 统一中断 copyLoop(B 阶段已删 `pauseCh` 双轨)。
- **执行入口与信号量**:`startTaskTrees`(开始/重试/板块重执行)与 `resumeTaskTrees`(恢复)从 DB 加载任务树后调 `dispatch`;`ResumeTaskTrees`(批量恢复:内存命中直接 postCmd,未命中收集走 `resumeTaskTrees` 从 DB 加载)与 `ConfirmReplace`(按任务类型分流——插件任务直接 `postCmd`,策略任务投执行内挂起等待的确认通道)。`dispatch` 是首启入口(`actorStarted` CAS + 投 `cmdStart`/`cmdResume`)。信号量槽位获取移入 actor 内部(`handleRunCmd` 中 `select semaphore`,取不到则 `enqueueSelf` 入 `waitingQueue`);槽位释放后 `dispatchFromQueue` 向队首投 `cmdResume` 唤醒。策略任务确认挂起期间 `WaitReplaceConfirm` 自行释放槽位、答复后重新取槽(`releaseSlot` 按 slotHeld 守卫防重复释放)。`PauseTaskTrees`/`StopTaskTrees` 批量循环 `resolveTargets` 后对目标并行投命令(各 actor 独立处理,Stop 带 ack 等待终态后对去重 parent `cleanupStoppedTree`)。`Redownload` 先持久化板块选择到 task 再调 `startTaskTrees`。
- **依赖全部接口注入**：插件执行器、仓储、进度推送器均通过构造函数注入，`Manager` 不直接持有具体 Service。
- **store 处理范围限定在插件本次声明的板块**：任务执行（首次下载/重试/Redownload/重启续传）全程只动插件 Start/Resume 返回的 StoreSpec 对应板块——挂载删旧走 `DeleteByResourceIdAndTypes`（`mountResourceStores`，**只摘指向活行 store 的关联**——软删行关联保留）、板块选择走 `filterSpecsByRoles`（按 storeRoles 过滤插件 specs）、重启续传 `resumeFromPersistedState` 读全集但已 Complete 者进 `completedRoles` 跳过（关联行先按 store 活性过滤——软删行关联不是续传对象）。`videoMain`（分离流场景由用户「合并」操作挂载、不来自插件 specs 时）对任务流程透明：不被删/覆盖/失效，重下源轨道后旧 videoMain 去留由用户决定；本地导入插件可直接声明 videoMain（来自插件 specs，任务正常处理）。勿新增"重下/重置时清空该 Resource 全部 store"之类逻辑。
- **覆盖确认门槛为行级**：所选板块角色（runMode.storeRoles）与已有作品**活行** store 的 store_type 集合求交（软删残留代不算「作品拥有该角色」——merge overwrite 轨道残留、替换残留不再触发弹窗），**交集非空才弹覆盖确认**（含 thumbnail 行——覆盖缩略图同样需要用户知情）；空交集或已有作品零活行则不弹窗，但仍保留 `existingWorkId` 供替换定位（门槛与替换语义解耦）。板块为空（插件自决全量）时已有任意活行即弹。仅作品信息任务（fetchStores=false）不参与查重。行级角色查询失败时保守退回弹窗（宁多弹不漏弹），载荷 `conflictRoles=nil` 表示板块信息不可得；非 nil 时弹窗展示将覆盖的板块明细。
- **替换链软删化（委派 resource 替换能力）**：替换前置经 `softDeleteReplaceTargets` 委派 `ReplaceStoreOps.SoftDeleteWorkStoreRoles`（resource 模块实现，输入 `(workId, roles)` 纯领域参数）软删所选板块的旧 store——已完成行 `DeleteWithBackup`（移文件入 backup + 行内 backup_id 同生共死），未完成行 `SoftDeleteAndDiscardFile`（废弃 partial 文件不备份），历史残留死行不动，`resource_store` 关联不摘（软删行经挂载链可联作品、随作品级联净化）。成功/中断残留入回收站文件条目由 TTL 收尾。
- **失败回滚复活原行（`restoreReplaceTargets`，委派 resource 替换能力）**：任务失败/停止（含跨重启续传后）回滚到替换前状态——作品已软删则守卫跳过（两代归回收站作品条目）；本模块先摘新建 store 的关联（`DeleteByStoreIds`）并物理删行（释放 file_path），再委派 `ReplaceStoreOps.RestoreReplacedStores`（`WorkID` 数据驱动）按**同键最新死代**圈定 victim（(resource_id, store_type, store_seq) 维度，argmax deleted_at——多代残留只回滚本代），逐行 `RestoreFile`（suppress 登记）+ 删备份清单行后 `RestoreByIds` 复活（双列清）并重算完整度。victim 关联从未摘除，复活即挂载回位，无重挂步骤。**策略任务经 `SetTerminalRollback` 登记钩子在同一单点触发**（`triggerTerminalRollback` 按执行器登记的受害者显式清单复活，多作品由执行器自持；未登记即无软删行可复活，直接让位）。
