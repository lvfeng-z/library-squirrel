# 耗时操作接入任务模块 — 选型与起步方案

> 派生自 `multitrack-resource-lineage` 谱系（K 多轨合并节点讨论）：合并现状核查中发现合并**完全同步、无控制面**（并发叠加 bug / 阻塞 IPC / 无进度 / 无取消 / 无崩溃恢复 / 与任务模块零关系），而 taskManager 恰好具备这些能力 → 引出"是否接入、接入到何种程度"。
> 本文档是选型规划交付物：沉淀三路调研结论 → 逐档分析侵入面与收益 → 给出推荐路径与待决策点。**不纳入当前 bilibili（multitrack A）开发范围**，待将来排期。

## 一、三路调研结论

### 1.1 合并现状（merge 能力包 + 编排 + 缺口）

| 维度 | 现状 | 位置 |
|---|---|---|
| 能力包边界 | **纯能力**，`FFmpegMuxer.MergeRemux(ctx, video, audio, out) error`，输入输出全为文件绝对路径，不耦合 store/resource | `backend/merge/ffmpeg.go:56` |
| 业务编排 | `MergeService.MergeResource`：取两轨 → ffmpeg → `StoreFromFile` 落 merged → 事务挂 `resource_store(merged)` | `backend/resource/merge_service.go:83-168` |
| 执行入口 | 前端按钮 → IPC `Handler.MergeResource` → 同步阻塞 ffmpeg `cmd.Run()` | `backend/resource/handler.go:24`；`frontend/.../WorkDialog.vue:140-153` |
| provider 抽象 | `Merger` 接口已 provider-agnostic（`FFmpegMuxer` 唯一实现） | `merge_service.go:36-38` |

**五个缺口（不论是否接入都该修的技术债/bug）：**

1. **阻塞 IPC**：ffmpeg 在 IPC 调用 goroutine 内同步执行，大文件可达分钟级直接占用 IPC 线程（`doc/plan/merge-business.md:181` 已记风险）。
2. **并发叠加 bug**：`MergeResource` 全程**不查已有 merged store** 就重复合并。keep 模式下重复触发 → 累积孤儿 `resource_store(merged)` 行（磁盘文件被同路径覆盖只一份，但关联行累积）。后端**无任何并发锁/幂等守卫**，前端仅 `merging` ref 弱保护。
3. **无进度**：ffmpeg `stderrBuf` 仅用于失败诊断（截末尾 512 字节），不解析进度、不 emit。
4. **无主动取消**：仅 ctx 超时（10min）兜底，前端无法主动 cancel。
5. **无启动清理**：孤儿临时文件（`os.TempDir()` 的 `ls-merge-*`）+ 孤儿产物 store（落盘成功但挂 `resource_store` 事务未执行即崩溃）无清理；`app.go` 启动只清回收站。

> 附带偏差：merged 当前挂 `Generation=downloaded`（可续传语义），但 merged 是一次性派生产物，语义应为 `derived`（`resource_store.go:19-22`）。接入控制面时一并修正。

### 1.2 taskManager 控制面能力（已具备的治理能力）

| 能力 | 机制 | 位置 |
|---|---|---|
| 串行化（actor） | per-task `actorLoop` goroutine + `cmdCh` 串行命令；"一任务一 goroutine" + dispatch CAS 首派守卫 + 创建层 claim 唯一性 | `backend/taskManager/model.go:428,462-466`；`manager.go:470-480,859-891` |
| 并发限流 | `semaphore chan struct{}`（容量 `maxParallel`）+ `waitingQueue` FIFO | `manager.go:66-67,483`；`model.go:526-535,572-584` |
| 优雅暂停 | `cmdWatcher` 阶段感知：downloadLoop 走 `softPause` drain（排空在途 chunk）、setup 立即 cancel；2s drain 超时兜底 | `model.go:620-657,688-714` |
| 强制停止 | `handleStopCmd`：runCancel + 遍历 streams `abort()` + 通知插件 → Failed | `model.go:717-742`；`manager.go:582-628` |
| 进度推送 | `TaskProgressPusher`（9 方法）+ `SnapshotPusher`（50ms 防抖快照）+ `flushLoop`（200ms 批量刷盘） | `progress_pusher.go:11-21,263`；`manager.go:1186-1246` |
| 崩溃恢复 | 基于 DB 稳态 + `pending_resource_id` 续传；**非自动恢复**（启动不拉起 Paused，用户下次启动续传）；终态即时落盘 | `manager.go:140,307-330,1249-1283`；`model.go:1381-1582` |
| 续传 | Range offset（`os.Stat` 文件大小）+ `store.Status` 对齐 + 416 防护（`ResumeWriteOffset`） | `model.go:1420-1463,1530-1544` |
| 统一面板 | `TaskProgressTreeDTO` 树 + 内存状态覆写 + 增量/快照双通道 | `task/service.go:453-500,260-270`；`progress_pusher.go:46-53` |

**任务类型接入点（选型最关键）：**
- **当前无 `type`/`kind` 字段**（`entity/task.go:10-28`，全仓零命中 `TaskType`）。任务行为完全由 `(PluginPublicID, PluginExtensionID)` → registry → `TaskHandler` 决定。
- `TaskExecutor` 接口（`model.go:138-154`）：`CreateWorkInfo/Start/Pause/Stop/Resume`，其中 `Start/Resume` 返回 `[]*StoreSpec`（带 `ReadCloser` 的**流**）。
- 接入装配：`pluginExecFactory`（`app.go:871`）单一 `TaskExecutorImpl`，从 task 实体读 `(publicId, extensionId)` 查 registry。

### 1.3 任务模型与 store 关系（数据层契合度）

- **task 实体无类型维度**：状态机 9 态（Created→…→Finished/Failed/PartlyFinished）+ 瞬态 `WaitingForInput`。`Resource.TaskID` 外键；task→work 经 `SiteID+SiteWorkID` 查重，无直接外键。
- **resource_store 开放枚举**：`StoreType`（main/thumbnail/videoTrack/audioTrack/merged，新增加常量不改表）× `Generation`（downloaded/derived）。
- **universe 机制已就绪**：`InvolvedRoles`（涉及板块，创建期声明）+ `StoreRoles`（按次所选子集）双字段。原则：**声明=hint，Start 产出=truth**，挂载按 role 隔离（`mountResourceStores` 先 `DeleteByResourceIdAndTypes` 只清同 role 旧行）。
- **核心契合点**：「任务只动自身声明板块」使"合并任务产 merged"与"下载任务产 main"**在数据模型上完全对称、无需新表**——下载任务声明 `[main,videoTrack,audioTrack]` 只动这三个 role，合并任务声明 `[merged]` 只动 merged。

**两个独立调研收敛出的同一结论（最大架构债）：**
> taskManager 执行链 `runSectionCombo → startDownload → downloadLoop → copyLoop` **深度耦合"下载流"语义**——假设任务产出 `[]*StoreSpec`（带 ReadCloser 流）、reader→writer 拷贝、stream 字节算进度、Range 续传。合并是"本地两文件→一文件 remux"，**无 reader 流、无 Range 续传**，无法直接套用。

---

## 二、逐档分析

| 维度 | D 异步化 | A 接入控制面 | B 加 type 维度 | C 执行策略可插拔 |
|---|---|---|---|---|
| **做法** | MergeResource 包 goroutine + ffmpeg 进度回调 + cancel 接口，**不碰 taskManager** | 合并作为 taskManager 一种执行器/任务类型，复用 actor/信号量/暂停/进度/崩溃恢复/面板 | task 加 `Type` 字段，`run()` 按 type 分流（runDownload vs runMerge），TaskExecutor 接口拆分 | 控制面/执行面剥离：taskManager 保留调度内核，执行面外提为可注入 `ExecutionStrategy`，下载/合并/未来各自实现（开闭原则） |
| **解决缺口** | IPC 阻塞 ✅ 进度 ✅ 取消 ✅ | 全部 ✅（含崩溃恢复/统一面板） | 全部 ✅ + 未来操作标准化 | 全部 ✅ + 未来操作标准化（与 B 同，因 C ⊃ B） |
| **不解决** | 崩溃恢复、统一面板、信号量限流（UX 割裂，"二等公民"） | — | — | — |
| **侵入面** | 低（merge 包加 progress callback + handler 异步化 + 前端迷你 UI） | 中高（需泛化进度模型、为合并走独立执行分支） | 大（执行器分发、runMode 派生、进度模型、恢复策略、创建路径全触及，但执行面仍内嵌 ManagedTask） | 最大（执行面从 ManagedTask 剥离外提 + 定义 ExecutionStrategy 接口；但控制面 actor/信号量/状态机/进度/恢复保留不动） |
| **核心障碍** | 需额外补幂等守卫（异步化反而放大并发面） | TaskExecutor 的"流"假设 + runSectionCombo 流式链路 | taskManager 刚稳定（多轨重构/actor 不变量），分流改造回归风险 | 剥离触及刚稳定的多轨/续传/drain（回归风险最高）；控制面/执行面协调边界（暂停/进度/恢复下发）设计难 |
| **数据模型改动** | 无 | 复用 universe（声明 `[merged]`）+ 可能需放宽 `Resource.TaskID` 单值约束 | task 加 `Type` 字段（GORM AutoMigrate 自动加列，老任务默认 download） | task 加 `Type` + 执行策略元数据（同 B） |

### 关键判断：A 是 B 的半成品；C 是 B 的彻底剥离版（同向不同深度）

两路调研独立得出：**A 的深度实现必然演化为 B**（合并进 taskManager 要么套下载型 actor 空操作、要么按 type 分流到 `runMerge`，后者即 B），A 是中途态、不值得作为独立终点。

B 与 C **不是并列独立方案，而是同方向的两个深度档**：
- **B**（最小分流）：task 加 `Type`，`run()` 里 `switch type` 分流到 `runDownload`/`runMerge`。新增类型要改 `run()` 内核。
- **C**（彻底剥离）：`ManagedTask` 持有 `ExecutionStrategy` 接口，下载/合并各自实现并注入。新增类型只加一个 strategy 实现，不改内核（开闭原则）。C 是 B 把"执行面剥离"做到位的产物。

所以**真正的选项谱是连续的**：D（不碰 taskManager）→ B（最小分流，新增类型改内核）→ C（彻底剥离，开闭原则）。选 B 还是 C，取决于"未来是否确定要接多种耗时操作"。下文先详评 C，再给推荐。

### C 方案详评：执行策略可插拔（控制面/执行面剥离）

> 用户认可的方向："任务控制与实施最解耦"。目标是让 taskManager 调度内核稳定，执行逻辑按类型可插拔，未来转码/导出/批量重处理接入不改内核。

**剥离边界**（控制面保留 / 执行面外提）：

| 层 | 归属 | 内容 |
|---|---|---|
| 控制面（保留不动） | Manager/ManagedTask 内核 | actorLoop/cmdCh 串行、semaphore/waitingQueue 限流、state 状态机、dispatch CAS/claim 守卫、`TaskProgressPusher` 推送、flushLoop 批量刷盘、崩溃恢复框架（loadAndStartTaskTrees/buildOrReuseChild）、终态即时落盘、cmdWatcher 阶段感知调度骨架 |
| 执行面（外提为 ExecutionStrategy） | 各策略实现 | streamController、startDownload、downloadLoop、copyLoop、handleEOF、resumeFromPersistedState、drain、reportProgress 的 stream 汇总 |

**ExecutionStrategy 接口雏形**（草案，实施时细化）：
- 执行策略职责：跑完任务 + 自报进度 `(total, finished)` + 声明自身能力（能否优雅暂停 / 能否续传）。
- 控制面职责：何时跑（信号量/队列）、状态机流转、进度推送、暂停/停止协调下发、崩溃恢复触发。
- 关键契约分歧（下载型 vs 合并型）：下载型支持 drain 优雅暂停 + Range 续传；合并型只支持 cancel 重来 + 无续传。接口需暴露能力位（如 `SupportsGracefulPause()/SupportsResume()`），或分「流式策略」「派生策略」两类接口。
- 现有 `TaskExecutor` 接口（`model.go:138-154`）是执行策略抽象的**半成品雏形**——C 是把它做完整 + 把下载执行面从 ManagedTask 剥离。

**工作量与风险**（公允，非"重写"）：
- 工作量大：downloadLoop/copyLoop/streamController/续传是 taskManager 最复杂、刚经多轨重构稳定的部分，剥离外提触及面广。
- 设计风险：控制面/执行面协调边界（暂停/停止下发、进度上报、恢复触发）需仔细设计——剥得太净可能丢失下载型 drain 语义，剥得不净又重新纠缠。这正是调研指出的"剥离边界不清"。
- **但不是重写**：控制面内核（actor/信号量/状态机/进度/恢复框架）保留不动，工作量集中在执行面外提 + 接口抽象。

**适用判定**：未来**确定**接多种耗时操作（转码/导出/批量重处理）→ C 值得（一次剥离 + 终身开闭）；只有合并一种 → C 过度设计（YAGNI），B 足够。

### 横切必做项（任何方案的前置）

1. **并发叠加去重**：`MergeResource` 入口先查 `GetByType(resourceId, StoreTypeMerged)`，存在则跳过/复用（或提供"强制重合并"显式入口）。
2. **Generation 标注修正**：merged 挂 `derived` 而非 `downloaded`。
3. **启动清理**：app 启动扫描 `os.TempDir()` 的 `ls-merge-*` 残留 + 孤儿产物 store（落盘成功但无 `resource_store` 关联）。

---

## 三、推荐路径

考虑你对 C 解耦价值的认可，给出两条路径供选，核心分歧是"未来是否确定要接多种耗时操作"。

### 路径一（保守·D→B 视需求）：未来操作种类不确定

阶段0 必修 → 阶段1 D 止血 → 阶段2 视真实反馈决定是否上 B（最小分流）。
- 适合：不确定未来是否接转码/导出等，先让用户用上异步合并、验证"进统一面板"是否真需求，再决定。
- 若阶段1 后无统一治理反馈，则长期停在 D。

### 路径二（解耦优先·分阶段达成 C）：认可未来接多种操作 + 解耦投资

阶段0 必修 → 阶段1 D 止血（顺带为"进度自报"铺路）→ 阶段2 执行面剥离 + 定义 `ExecutionStrategy`（下载先迁过去，验证内核不回归）→ 阶段3 合并作为第二个 `ExecutionStrategy` 接入（验证开闭——接入不改内核）。
- 适合：确定未来要接多种耗时操作，愿意为开闭原则做执行面剥离的一次性投资。
- **核心风险**：阶段2 剥离刚稳定的多轨/续传/drain 是全方案回归风险最高的环节，**必须独立成一个有充分测试保护的重构前置**，不能和合并接入混做。即：先剥离下载执行面、确认下载全流程不回归，再让合并轻量接入（此时才体现 C 的开闭价值）。

### 我的修正推荐

鉴于你认可 C 的解耦价值，倾向**路径二（解耦优先）**，但把阶段2（执行面剥离）作为独立前置。这样 C 的解耦终态可达，又把回归风险最高的剥离环节单独隔离、充分测试，不和合并接入耦合。

若你对"未来接多种操作"尚不确定，则路径一更稳——但注意路径一的阶段2 若上 B，未来再接第三种操作时仍要改内核（届时会想把 B 升级为 C），存在返工。**两条路径的阶段0/阶段1 完全一致**，分歧只在阶段2 往哪走，可先做阶段0/1 再定。

### 反方观点

- 若未来**只有合并一种**耗时操作，路径二的执行面剥离是纯过度设计（YAGNI），路径一甚至仅 D 更经济。
- 若**已确定**要统一治理且未来接多种操作，路径二的阶段2 剥离值得尽早做（越晚积累的下载执行面逻辑越多，剥离越难）。

### 路径二选定后的 C 必要性再确认（基于未来操作清单·2026-07-16）

用户补充未来操作：导出、翻译、查重、AI。调研结论：

**事实**：
- 四种操作当前**全部零代码**（纯设想）。查重仅 URL 级（`site_id+site_work_id` 唯一索引），无图像哈希/感知哈希。
- taskManager 的"下载流"假设在**四个层面**系统性排斥非下载型操作：① 产出契约 `StoreSpec` 强制 ReadCloser 文件流（`handler_dto.go:68-77`）；② 执行循环 `copyLoop` reader→writer 字节拷贝（`model.go:1239`）；③ 进度模型字节累加、无自定义维度口子（`model.go:1716-1730`、`progress_pusher.go:14`）；④ 恢复语义 Range offset 续传、纯计算无续传（`model.go:1420-1463`）。
- 这四种操作**全部非下载型**（文件打包/网络文本/纯计算/API 调用），全部撞墙——不只合并。
- 翻译/AI 大概率插件，但 SDK `TaskHandler.Start` 同样强制返回 `[]*StoreSpec`（`task_handler.go:7-26`），逼插件伪造文件流。

**结论（分层）**：
1. **方向上 C 必要、B 不可接受**：5 种非下载型操作（合并+导出+翻译+查重+AI）若都进 taskManager，B 会让 `run()` 有 5 个差异巨大的执行分支（每加一种重写进度/恢复/暂停），开闭原则彻底失效。C 的 `ExecutionStrategy` 是唯一不污染内核的方案。
2. **时机上需求驱动、不预先剥离**：5 种操作零代码，"系统性排斥"是接口推断非已观察痛点。剥离触及刚稳定的多轨/续传/drain（高风险），**不应为设想预先投入**。
3. **成本修正**：C 不只主程序剥离，还可能需扩展 SDK `TaskHandler` 接口（让插件声明非流式策略，因翻译/AI 是插件）——工作量比初评更大，但价值覆盖插件生态。

**修正认知：方向锁 C、时机需求驱动。** 阶段0/1 先做（合并必修+异步化），用合并验证"非下载型操作进控制面"是否真需全套（崩溃恢复/统一面板）；阶段2（执行面剥离）**推迟到合并或查重等真正需要进控制面时启动**，一旦启动走 C（非 B）。

### 2026-07-20 重评（unpark·合并已有真实使用需求前提）

**触发**：用户确认合并已有真实日常使用需求（非测试），按 park 时定的恢复动作①重评 C 必要性。

**输入**：
- 痛点：合并"偶尔顺畅"+"崩溃要重来/无法取消"（理念担忧 > 频发实际痛点；`log/` 零 merge 痕迹佐证低频使用）
- 战略顾虑：担心现在不解耦，将来新操作接入时下载执行面耦合过深"剥不动"

**判断**：
1. "将来剥不动"的真实风险点**不在时间推移**，而在"非下载型操作强行塞进当前耦合体"——下载执行面（`streamController`/`downloadLoop`/`copyLoop`/续传/drain）多轨重构后已定型；bilibili 专栏仍属下载型走下载流；下一个非下载型操作接入前不会显著增长。
2. 路径二「接入前先剥（阶段2）→ 再接入（阶段3）」原则已内化对该顾虑的防护——坚守新操作接入前先剥、不走 B 的内核分流，耦合态不累积非下载逻辑。
3. 合并痛点（崩溃要重来/无法取消）→ 阶段1 异步化止血即可解决主要困扰（cancel/进度/不卡 IPC），崩溃恢复需控制面但属低频"锦上添花"。

**结论（用户拍板）**：
- 方向锁 C 不变。
- **阶段1（异步化止血·D）紧随必修开工**。
- **阶段2（执行面剥离·C 核心）推迟到"下一个非下载型操作要接入控制面时"启动**（用户选"推迟·等临近再启动"），届时按路径二先剥再接。
- "剥不动"顾虑由路径二"接入前先剥"原则防护，记录在案。

### 2026-07-20 补充（rich-article 分析·派生文件产出不撞墙）

**触发**：评估 rich-article-resource（专栏正文 `.md` + 内嵌图）实现是否会加深 taskManager 执行面/控制面耦合——直接检验上文"下载流假设系统性排斥非下载型操作"的适用边界。

**代码证据**（taskManager 执行面对 derived 已是一等公民）：
1. `streamController` 注释明确"管理单个 store 的传输(**downloaded/derived 通用**:reader→store 拷贝)"（`model.go:298`）——derived 与 downloaded 共用同一 reader→writer 拷贝路径。
2. **缩略图（thumbnail）已是 `derived + io.NopCloser` 的现有案例**（`model_multi_stream_test.go:189,309,504` 大量覆盖）——rich-article 的 `.md` 走的是缩略图同款路径。
3. `copyLoop` 唯一的 generation 分支 `if s.generation == entity.GenerationDownloaded`（`model.go:1318`）只用于"downloaded 完整性校验"（是否下完），derived 跳过——不是"为 derived 加分支"，而是"downloaded 才需要的校验"。
4. derived 未完成的重产逻辑**已存在**（`model.go:1489-1501`：Resume 时 derived 整轨重产，调 `Start`）。

**认知修正**：本文档上方第①层排斥（"StoreSpec 强制 ReadCloser 文件流"）需分两类定性——

| 操作类型 | 例子 | 是否撞 StoreSpec 流契约 |
|---|---|---|
| **派生文件产出**（有文件，非流式） | `.md`、缩略图、合并产物 | **不撞**——`NopCloser` 是 derived 的标准产出方式（缩略图在用），走 derived 通用路径 |
| **纯计算/API 型**（无文件产出） | 翻译、AI、查重 | **真撞**——无自然文件流，得伪造 |

即 `NopCloser` 不是"伪装/撞墙"，而是 StoreSpec 契约**既已包容**的 derived 产出方式。

**对阶段2 触发条件的收窄**：rich-article 这类派生文件产出能干净走现有 derived 路径，不触发阶段2。真正触发阶段2（执行面剥离）的是**纯计算/API 型操作要接入控制面时**（翻译/AI/查重——它们无文件产出可塞 StoreSpec），而非笼统的"非下载型操作"。此修正进一步支持"阶段2 推迟"的结论。

---

## 四、待决策点

1. **选型方向**：路径一（保守 D→B 视需求）/ 路径二（解耦优先，分阶段达成 C，修正推荐）/ 仅 D（合并长期作轻量操作，未来不接其他耗时操作）？
2. **未来操作预期**：除合并外，是否**确定**会接入转码/导出/批量重处理等其他耗时操作？（这是 B vs C 的决定性判据——确定→路径二，不确定→路径一）
3. **阶段 0 是否独立先行**：三项必修是否先于选型确认就开工（建议是——纯 bug 修复，无架构争议；且两条路径阶段0/1 一致，先做不阻塞后续选型）？
4. **进度推送复用边界**：阶段 1 复用 `TaskProgressPusher` 通道但合并不进 taskManager，是否可接受"合并进度走 task topic 但无 task 记录"的轻量方案？（或合并用独立 topic）

---

## 五、不变量与约束（守住勿回退）

- **merge 包保持纯能力**（`MergeRemux` 只接文件路径），落盘/挂 store/板块声明归 resource（`MODULE_BOUNDARY_PURITY`）。
- **不论选型，并发叠加去重 + 启动清理必做**——异步化（D）反而放大并发面，去重是前置。
- **universe + resource_store 开放枚举不新增表**：合并产 merged 复用现有模型，不建合并专用表。
- **taskManager 现有不变量不破坏**（一任务一 goroutine、dispatch CAS 首派、actor 作用域边界）——若上 B，分流不得削弱这些守卫。
- **不纳入 bilibili（multitrack A）开发范围**：独立成树、不阻塞 bilibili；bilibili 产出多轨数据后合并照常走当前同步路径，本任务待将来排期。

---

## 六、实施记录·阶段0 必修（2026-07-20）

| 必修 | 状态 | 实施 |
|---|---|---|
| 1. 并发叠加去重 | ✅ 完成 | `MergeResource` 入口加去重守卫：`GetByType(resourceId, StoreTypeMerged)` 命中则幂等返回已有 store，避免 keep 模式重复合并累积孤儿 `resource_store(merged)` 关联。未加 force 参数（YAGNI）；历史已累积多行不在此清理 |
| 2. Generation 修正 | ✅ 完成 | merged 挂 `GenerationDerived`（一次性派生、不可续传），原误标 `GenerationDownloaded` |
| 3. 启动清理 | ⚠️ A 完成 / B 降级延后 | **A 临时文件**：提取 `mergeTempFilePrefix = "ls-merge-"` 常量（创建与清理共用），`MergeService.CleanupResidualTempFiles` 扫 `os.TempDir()` 删前缀残留，`app.go` 启动调用。**B 孤儿 store 降级**：`PersistentStore` 无 `store_type` 字段，无法精确区分 merged 孤儿 vs 下载孤儿；事务失败已有补偿（`MergeResource` 事务失败分支删产物 store）、崩溃残留窗口极小、贸然清理有误删风险 → 延后到孤儿 store 痛点显现时再做（届时需给 `PersistentStore` 加类型维度或做通用无关联 store LEFT JOIN 清理），记为延后分支 F |
