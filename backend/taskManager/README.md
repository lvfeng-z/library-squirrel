# taskManager 模块说明

## 一句话职责

任务**运行时控制面**：在内存中管理任务树的生命周期（启动/暂停/恢复/停止/重试）、并发控制、状态机推进与进度推送。负责"任务怎么被调度控制"；"任务主体怎么执行"由按 task_type 注册的执行面策略承载（plugin-download 归 `backend/download`、share-receive 归 `backend/share`、export 归 `backend/export`）。

## 边界

- 与 **task**：`task` 负责任务**实体**的持久化 CRUD（创建、查询、删除、状态字段读写，属静态数据）；`taskManager` 负责任务**运行时**的调度控制（属动态）。task 写数据库，taskManager 跑内存状态机并把结果批量刷回数据库。
- 与 **download**：download 实现插件下载任务的执行面策略（板块组合/多轨下载/续传/替换链），taskManager 经策略表按 task_type 路由并经 `StrategyHandle` 与其交互（终态/进度/覆盖确认/跳过/恢复信号/软暂停广播）。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `StartTaskTrees(taskIds)` | 批量启动任务（全量执行） |
| `PauseTaskTrees` / `ResumeTaskTrees` | 批量暂停 / 恢复任务 |
| `StopTaskTrees` | 批量停止任务 |
| `RetryTaskTrees` | 批量重试任务（保留各任务已记录的执行模式） |
| `Redownload(taskIds, storeRoles, includeWorkInfo)` | 板块重执行两步编排：先把板块选择（资源 store_type 集合 + 是否含作品元数据；空资源集=仅作品信息）经 `SectionRecorder` 写入各任务的作品任务领域行（父任务请求展开到全部子成员），再整树启动 |
| `GetTaskState(taskId)` | 查询单任务状态（综合内存 + 数据库） |
| `GetTaskControlConfig()` | 任务控制操作防重入配置（`config.yaml` `task.operationCooldownMs`，前端操作按钮冷却依据；0=不启用） |
| `GetTaskTreeState(taskId)` | 查询任务状态：父任务返回聚合状态、叶子/独立任务返回自身状态 |
| `GetTaskSnapshot()` | 获取所有活跃任务的完整状态快照 |
| `IsIdle()` | 是否空闲（无运行中任务） |
| `CountActiveByPlugin(pluginPublicId)` | 统计插件名下运行中任务数（Processing/Pausing/Stopping/WaitingForInput）——供插件停用/换版拦截判据与确认框代价明示；插件身份在作品任务领域行，经 `WorkTaskProjector` 窄投影批量读取后内存过滤 |
| `ConfirmReplace` / `ConfirmReplaceBatch` | 用户确认重复作品的替换 / 跳过。replace 答复投递前同步预检等待时记录的冲突作品集合的分享拉取锁，命中返回 `shareLock.ErrWorkLocked` 且不投递（单条不摘条目；批量整体不投递），前端强制解锁后重发即放行；skip 答复不查锁 |

## 控制语义

- **操作范围按对象类型**：操作**父任务**→作用于整棵树（全部子任务）；操作**叶子/独立任务**→仅作用于自身，不扩散兄弟。前端单行操作传 `[id]`、批量操作传多 id，统一走批量接口。
- **整树加载**：任务运行时整棵树加载到内存（`ParentTask.children` 含全部子任务，含未启动的兄弟），供父状态聚合与完成判定，避免「整树/非整树」分支。
- **单独 Start 叶子**：`processParentUnit` 的 `leafSet` 参数控制——整树仍加载到 children，但只 dispatch 被请求的叶子，其余兄弟保持 Created 未 dispatch。
- **运行中父单元重纳终态子任务**：父任务运行中对终态（Finished/Failed）子任务发起开始/重试/恢复时，`processParentUnit` 检测到父单元已 claim（`claimParent` 输者），复用已有父单元、经 `reinjectLeaves` 把终态子任务重建（旧对象 actor 已退出）并重新 dispatch，而非静默跳过；板块选择写行只覆盖请求的任务（重下载入口在启动前已写行），不波及运行中兄弟。非终态子任务不纳入——Paused 的恢复走 `ResumeTaskTrees`→`resolveTargets` 内存路径，本不经此。
- **未启动兄弟守卫**：Pause/Stop/Resume 对 `!actorStarted && Created` 的兄弟（整树加载驻留但未 dispatch）跳过不投命令（避免误推 Paused/Failed）；`cleanupStoppedTree` 对这些兄弟 `cancel()` 退出 actor 防泄漏。
- **跳过收口（Skip）**：用户在覆盖确认选跳过、或仅作品信息板块完成时，执行面经 `StrategyHandle.Skip` 上报——控制面释放槽位、置 `skipped` 收口标志（内存态、不持久化，`AllChildrenTerminal` 据此视为终态）、状态回任务行加载时 DB 快照（执行前状态，不产生终态）并单次清理。崩溃重启当作未跳过重新执行。
- **静默跳过**：Pause/Stop/Resume 对不在内存的 taskId（`!ok`）静默跳过 return nil（控制操作幂等）。
- **停止即收口**：停止命令处理（`handleStopCmd`）置 Failed 后在同一处完成内存清理（taskMap 摘除、父任务全部子任务终态时连带清 parentMap、前端移除推送），应答后于清理——`StopTaskTrees` 返回即已收口。叶子/独立任务停止后可立即重试（重试重建对象重新执行）；不清理则对象滞留 taskMap（IsIdle 恒假）且重试复用滞留对象后被 dispatch 幂等丢弃。

## 状态与生命周期不变量

- **手动触发，无自动执行/恢复**：任务创建后停留 Created，等用户手动"开始"才进 Processing；app 重启后 Paused 任务**不自动恢复**，需手动"恢复"。无启动钩子自动跑任务——跨重启续传仅在手动恢复触发时经执行面（download）按恢复信号执行。
- **状态以资源实际状态为准**（续传/重下/完成判定归执行面）：执行面决策点以暂存文件 `os.Stat` 与已提交 store 行为准；控制面只承载状态机与调度。

## 核心概念

- **执行面策略（ExecutionStrategy）**：控制面（actor 循环/信号量/状态机/进度/持久化/恢复调度）留在 taskManager，「任务主体怎么执行」外提为可插拔接口——**全部任务类型经按 `task.task_type` 注册的策略表执行**（Manager 构造时注入，app.go 装配）：`'plugin-download'` → download 模块的插件下载策略（板块组合 + 多轨下载/续传 + 替换链）；`'share-receive'` → share 模块的收件拉取策略；`'export'` → export 模块的导出打包策略（领域行直查取选择参数 → Collect/Plan → 打包 zip，恰用 Task/RunCtx/Fail/Finish/ReportProgress 五方法）；空类型/未注册类型拒启。策略经 `StrategyHandle` 上报终态（Finish/Fail）、跳过收口（Skip）与进度；RunCtx 取消（暂停/停止）即中断信号、终态由控制面接管。`StrategyHandle` 另提供：执行内挂起等待覆盖确认（`WaitReplaceConfirm`——置 WaitingForInput、逐条推冲突事件、记录冲突作品集合供替换答复前置锁预检、等待期间释放信号量槽位，复用 `ConfirmReplace(taskId, action)` 整体答复）、终态回滚登记（`SetTerminalRollback`——受害者清单与替换期新建行清单两载荷合并累积；失败/停止时由 setFailed 单点触发：丢弃新建行、复活软删行）、可排空阶段上报（`MarkDrainPhase`——命令监听据此分流暂停处置：可排空阶段走软暂停排空在途再停，其余阶段立即取消）、软暂停广播（`SoftPauseSignal`——控制面进入软暂停时 close，执行面收尾在途读取落盘）、恢复信号（`ResumeRequested`——执行进入策略前任务实时内存状态==Paused 时置位，执行面据此分叉跨重启续传与全新执行）。暂停/停止的插件 RPC 转发经可选能力 `InterruptNotifier`（控制面按类型断言调用，download 实现）。
- **ManagedTask / ParentTask**：内存中的运行任务与父任务聚合。ManagedTask 持任务核心行（加载时 DB 快照——跳过收口回执行前状态的回退基准）；运行态经 atomic state 字段承载，冷加载构造时按 DB 行 status 初始化（进程重启后任务不在内存，DB 行是上一会话落定的执行前稳态、仅稳定态落库——Paused 行带该状态进入执行即命中恢复信号，跨重启续传分叉可达）。
- **信号量**：`maxParallel` 控制全局并发数，超出则进 FIFO 等待队列。
- **板块执行模式**：`{workInfo, storeScope}` 三态——持久化在作品任务领域行（`StoreRoles`/`IncludeWorkInfo`，板块模式唯一源），执行面（download）每次执行查行派生；重下载入口（Handler.Redownload）负责写行后启动；终态不清空（重试按原板块再来一次，后续重下/开始覆盖）。
- **进度推送器**（TaskProgressPusher）：两种实现——Wails 事件直推、快照模式（SnapshotPusher）。
- **状态落盘**：终态（Finished/Failed/PartlyFinished）即时同步写库，进程崩溃也不丢失；非终态（Paused）状态与进度攒在内存，由 `flushLoop` 每 200ms 批量刷库，避免高频写放大。终态即时写与 `doFlush` 的批量 status 写都在 `pendingMu` 临界区内，互斥执行，杜绝批量通道的过时快照回写覆盖终态。

## 依赖关系

- 依赖：`Repository`（任务树核心行查询 `ListTaskTreeCore`/批量状态设置/按站点作品反查）、`task` 包（TaskStatusEnum 状态枚举）、`WorkTaskProjector`（活跃插件计数的作品任务领域行窄投影，download 仓储实现）、`StagingCleaner`（work 删除链清下载暂存，task 模块暂存基建适配实现）、`SectionRecorder`（重下载板块选择写行，download 实现）、`TaskProgressPusher`、任务类型执行面策略表（task_type → ExecutionStrategy，构造注入；plugin-download/share-receive 均经此）、**shareLock**（WorkLockChecker——替换确认投递前置作品锁守卫）、resource 替换链复活能力（终态回滚单点）
- 被依赖：前端任务执行面板（操作栏）、task（运行态任务删除编排的 `RunningStopper` 窄接口实现——`StopAndWaitTerminal(taskIds, timeout)` 停止任务树并按内存目标集轮询等待全部目标离开运行态〔目标解析复用 `resolveTargets`，运行集=可停集：非终态且非未派发的 Created；运行态瞬态不落库，DB 行判定恒漏识别〕，超时返回错误令调用方拒绝删除（删行但执行继续属不可预期态）；不在内存的行（未启动、已终态清理或崩溃残留）无 actor 可停可等，视为非运行直通）、download（实现 plugin-download 执行面策略）、share（实现 share-receive 执行面策略）、export（实现 export 执行面策略）

## 关键设计

- **内存 + 数据库双轨状态**：运行态以内存为准（`GetTaskSnapshot` / `IsIdle`），查询态综合两者（`GetTaskState`）。
- **程序关闭阻断点（`ShutdownGate`）**：程序退出的「等待全部任务真正暂停完成+落库后再继续退出」阻断封装——`Manager` 构造时创建 gate 并自注册优雅关闭（`GracefulShutdown`：暂停全部瞬态任务→等稳态→终刷落盘）为阻断项，app 在窗口销毁后（`Run()` 返回后）、关闭插件/数据库前调用 `WaitAll` 有界等待（40s=单任务暂停应答上界 35s+排空 2s+收口余量，超时记录后继续退出兜底）。不挂在窗口关闭事件上：wails v3 窗口事件监听器并发 fire-and-forget 分发、内建监听无条件销毁窗口，事件处理器内的等待拦不住 `Run()` 返回与进程退出。其他模块将来需要阻断程序关闭时经同一 gate `Register` 注册，不各自另写等待。
- **per-task actor 模型**：每个 `ManagedTask` 持一条常驻 goroutine(`actorLoop`) + 命令通道(`cmdCh`),任务级可变状态只在 actor goroutine 内修改。外部操作(`Pause`/`Resume`/`Stop`/`ConfirmReplace`/`dispatch`)退化为向 `cmdCh` 非阻塞投递命令(`postCmd`,投递路径不持 `m.mu` 防死锁),actor 串行处理(`handleRunCmd`/`handlePauseCmd`/`handleStopCmd`)。带应答命令(`Pause`/`Stop`)的等待有界：队列满丢弃即回写错误、actor 已退出立返错误、应答超时(默认 35s,覆盖中断通知 30s 上界)返回错误——控制操作不无限阻塞。actor 退出时取消任务 ctx 并以善后消费接管 `cmdCh`(消费残留命令后随 ctx 取消退出,不随任务对象常驻)。命令队列天然记忆(无丢失唤醒)且保证时序——pause 排在 resume 之后最终生效,从结构上消除滞后 goroutine 按陈旧标志重派发。创建层 `claimTask`/`claimParent`(`m.mu` 下 insert-or-get)保证同一 taskId 只有一个对象;`actorStarted` CAS 保证一任务一 actor。策略主体执行期间 `cmdWatcher` 并发监听 `cmdCh`,收到 pause/stop 按阶段分流（可排空阶段〔`drainPhase`，执行面经 MarkDrainPhase 上报〕走软暂停广播 + 2 秒排空超时兜底强制取消 vs 其余阶段立即 `runCancel` 中断在途）；`runCtx.Done` 统一中断执行面。
- **执行入口与信号量**:`startTaskTrees`(开始/重试)与`resumeTaskTrees`(恢复)从 DB 加载任务树（只查核心行）后调 `dispatch`;`ResumeTaskTrees`(批量恢复:内存命中直接 postCmd,未命中收集走 `resumeTaskTrees` 从 DB 加载)与`ConfirmReplace`(向执行内挂起等待的确认通道投递答复)。`dispatch` 是首启入口(`actorStarted` CAS + 投 `cmdStart`/`cmdResume`)。信号量槽位获取移入 actor 内部(`handleRunCmd` 中 `select semaphore`,取不到则 `enqueueSelf` 入 `waitingQueue`);槽位释放后 `dispatchFromQueue` 向队首投 `cmdResume` 唤醒。策略任务确认挂起期间 `WaitReplaceConfirm` 自行释放槽位、答复后重新取槽(`releaseSlot` 按 slotHeld 守卫防重复释放)。`PauseTaskTrees`/`StopTaskTrees` 批量循环 `resolveTargets` 后对目标并行投命令(各 actor 独立处理,Stop 带 ack 有界等待收口后对去重 parent `cleanupStoppedTree`——子任务经停止命令自清理可能已连带清树,该清理幂等跳过已清树防重复推送)。优雅关闭(`GracefulShutdown`)的暂停阶段同样按任务并行发起——单任务的中断通知等待时长不叠加进整体关闭耗时(经 `ShutdownGate` 作程序退出阻断项,见关键设计)。
- **依赖全部接口注入**：策略表、仓储、窄投影、进度推送器均通过构造函数注入，`Manager` 不直接持有具体 Service。
- **替换链终态回滚单点**：执行面替换软删成功后经 `SetTerminalRollback` 登记受害者清单（多次软删合并去重）；执行面在 store 行创建事务提交后登记新建行清单（跨执行轮次并集——停止/暂停恢复后失败等中断路径不经会话收口，台账须在控制面存活；当前两载荷登记方为 share-receive——plugin-download 替换链坍缩到提交窗口，只登记受害者、提交事务原子性兜底建行段）。任务失败/停止（含暂停态直接 Stop）统一经 `setFailed` 单点触发 `triggerTerminalRollback`：先按登记丢弃新建行（行+文件+关联，释放旧代 file_path）、再按受害者清单复活；受害者为空即未发生替换，新建行清单随登记作废（非替换失败保留已下载成果）。Finish 清空登记——重试从空态重新登记，不复活历史软删行。软删/复活/丢弃的能力实现见 resource 模块 README 替换链能力节。
