# task 模块说明

## 一句话职责

任务**实体**的持久化层：管理任务核心行（task）与任务类型领域行（work_task / share_task）的增删改查与状态字段读写，以及"URL → 插件 → 任务"的创建路由。负责"任务记录长什么样、存在哪"，"任务怎么执行"由 taskManager 负责。

## 边界

- 与 **taskManager**：task 是**静态数据**（实体 CRUD、状态枚举的定义方）；taskManager 是**动态调度**（消费 task 定义的状态枚举驱动运行时状态机）。task 写数据库，taskManager 读 task 的状态枚举来推进。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `CreateTask(req)` | 创建任务（含父子任务树） |
| `CreateTaskByURL(req)` | URL → 路由到处理该 URL 的插件扩展点 → 创建任务。`CreateTaskByURLRequest{URL, ChosenPluginPublicId, ChosenExtensionId}`：两显选键为空即未显选（程序化触发恒为此态）；交互触发命中多候选且未显选时返回冲突载荷 `Conflict=true` + `ConflictCandidates`（未调用任何插件），前端选择后带键重发 |
| `Save` / `Update` | 保存 / 更新任务（DTO 组装/拆向覆盖核心行与领域行双表） |
| `DeleteTask(ids)` | 批量删除任务（含子任务；运行态编排——树内含 Processing/Waiting 行先经 `RunningStopper` 停止并有界等待终态，超时拒绝删除，非运行态直通；事务内先清 resource.task_id 引用（置 NULL=非任务产，resource 行保留）、删 work_task / share_task 领域行，再删任务核心行；提交后清理被删任务（含子任务）的任务暂存目录） |
| `RefreshStatus(taskId)` | 刷新任务状态 |
| `SetTreeStatus(taskIds, status, includeStatus)` | 设置任务树状态 |
| `GetById` / `QueryPage` | 单查 / 分页查询 |
| `QueryParentPage` / `QueryChildrenTaskPage` | 父任务 / 子任务分页（带站点名） |
| `ListChildrenTask` / `ListTaskTree` / `ListTaskTreeCore` | 子任务列表 / 任务树列表（核心行+领域行双查，查询面用）/ 任务树核心行列表（任务运行时控制面树加载用，只查核心行） |
| `ListTasksBySiteAndSiteWorkID` | 按站点 + 站点作品ID查关联任务（板块执行选任务） |
| `QueryTreeDataPage` | 任务树数据分页 |
| `ListStatus` / `ListSchedule` | 状态 / 进度列表 |

## 核心概念

- **TaskStatusEnum**：任务状态枚举，与 taskManager.TaskState 保持一致。
  `Created(0) / Waiting(1) / Processing(2) / Pausing(3) / Paused(4) / Stopping(5) / Finished(6) / Failed(7) / PartlyFinished(8)`
- **任务三表形态**：task 核心控制行只承载生命周期与树形关系（status / pid / has_child / task_type 等 9 列）；插件下载领域字段在 **work_task**（1:1 共享主键——主键 id 恒 = 所属 task.id，无独立外键列；行集=全部 `'plugin-download'` 任务行，含树形父行与子行）；分享接收领域字段在 **share_task**（同 1:1 形态，仅收件子任务行）。领域行仓储：share_task 归本模块（`ShareTaskRepository`，私有组合 BaseRepository——通用写方法不外漏，写路径收口到 `CreateForTask`）；work_task 仓储归 download 模块（执行面持有），本模块建树写行与树双查读行经窄接口 `WorkTaskWriter` / `WorkTaskReader` 注入消费（app.go 装配）；工厂 `NewWorkTask(taskID)` / `NewShareTask(taskID)` 对非正 id panic fail-fast（零值主键插入会被 SQLite 静默按 rowid 分配新值，破坏 1:1 同值约束）。
- **任务类型（task_type）**：恒有值——插件下载任务写 `'plugin-download'`（常量 `entity.TaskTypePluginDownload`，work_task 领域行持插件身份，download 模块实现其执行面策略）；其他取值为内置类型（`'share-receive'` / `'export'`，经 taskManager 注册的执行面策略执行，领域字段在各自领域表）。内置类型创建时经任务类型注册表校验，未知类型拒绝创建。查询过滤经 `TaskQueryDTO.TaskType`（`task.task_type`，eq/ne 算子——类型专属视图圈定与任务面板排除）。
- **任务树**：父任务聚合子任务，父任务状态由子任务聚合得出（PartlyFinished 为父任务聚合态）。
- **内置任务树建树**（Service 层能力，非 Handler 暴露；经调用方定义的能力接口注入使用，如 share 的 `BuiltinTaskControl`，app.go 装配）：
  - `CreateBuiltinTask(taskType, taskName)`：创建内置类型独立任务（创建后停留 Created，运行控制与插件任务一致）。
  - `CreateBuiltinTaskTree(taskType, parentName, children)`：事务原子创建整树——1 个父容器（has_child=true、pid=NULL、task_type 落值）+ N 个子任务（pid=父ID、has_child=false、task_type 落值），父 ID 在事务内回填子 pid，任一步失败整体回滚。
  - `CreateBuiltinTaskParent` + `CreateBuiltinTaskChildren`：两段式建树——先建父容器拿 parentID（子任务入参依赖父 ID 的场景，如子任务领域行经父目录路径定位共享清单），再补建子任务；子任务创建非事务，失败由调用方显式 `DeleteTask` 删树回滚。
  - 入参 `BuiltinTaskChild{TaskName}`（children 顺序即子任务展示顺序；领域数据由建树调用方持子任务 ID 后写入自有领域表，本模块不感知）；错误：子任务为空 `ErrBuiltinTaskNoChildren`、父 ID 无效 `ErrBuiltinTaskChildrenNoParent`。父容器为纯聚合节点（无执行面、无领域行），子任务各自独立执行。
- **CreateTaskByURL 路由（多候选）**：候选发现 = 插件 Activate 期注册的 URL 监听器（正则匹配），**候选粒度 = 扩展点**（插件 × `extensionId`）；监听器缺插件 PublicID 属注册数据缺陷而非插件运行结果，剔除不入候选。候选经 `backend/route` 基座按**候选全键**（插件 ID 与扩展点 ID 以 NUL 拼接——NUL 不在标识符取值域内，`("a","bc")` 与 `("ab","c")` 不撞键）字典序**稳定排序**后逐个调用插件 create，首个产出任务的候选即路由终点。本消费面无协议级不适配信号（正则命中即预期处理），故一切失败（处理器不可用 / 传输层错误 / 零任务）皆真失败即终止并点名插件，不尝试后续候选。
- **两入口分工**：`CreateTaskByURL`（程序化面）恒按候选全键字典序、不问冲突——宿主 `CreateTask`、插件经宿主建任务走此入口；`CreateTaskByURLWithChoice`（交互面，前端手动建任务，Handler 暴露的即此）带显选键时该扩展点候选经排序键（置首前缀 "0"）排在其他候选之前，未带显选键且候选 ≥2 时返回冲突载荷（`Conflict=true` + `ConflictCandidates`，按候选全键字典序、首位即默认选中项）且**不调用任何插件**，由前端选择后带键重发。显选键经**两键联合校验**（插件 ID + 扩展点 ID 同时候选集命中），未命中报 `ErrChosenCandidateInvalid`。
- **运行态删除编排**（`Service.DeleteTask`）：删除即放弃执行——无条件先经 `RunningStopper` 窄接口（本模块定义、taskManager 实现，`SetRunningStopper` 延迟注入解决装配时序）执行「停止 + 有界等待终态」（等待上限 35s，与控制命令 ack 有界等待同量级）。运行态是瞬态、内存权威、不落库（运行中任务 DB 行恒 Created），运行判定由停止器按内存目标集自查，非运行任务（不在内存或已终态，含建树回滚删 Created 态树）在其内部快速直通；超时拒绝删除（「任务停止超时，请稍后重试删除」），不强行删除——删行但执行继续属不可预期态。暂存即时清理等收尾自然落在停止完成后。
- **任务暂存基建**（`staging.go`）：任务属主暂存作用域（下载与收件任务）的派生、创建与清理薄封装，底层经 staging 能力包——`DownloadStagingPath`/`ReceiveStagingPath` 派生 `{workDir}/staging/{download|share-receive}/{taskID}/`（absPath 域，workDir 未配置由调用侧守卫链拦截）；`ReceiveManifestRelPath` 派生收件共享清单相对路径（`manifest_path` 列值）；`EnsureDownloadScope`/`EnsureReceiveScope` 确保作用域存在（首建经 staging 原子入口写 scope.json 自证描述，恢复场景复用既有目录）；`StagingFileName(role, storeSeq, ext)` 生成 `role_seq` 派生键暂存文件名（续传定位不依赖元数据重解析，最终名由下载执行面在执行前解析派生）；`CleanupStagingByTaskIds(workDir, taskIds)` 按任务 ID 集合对两属主根各回收一次（任务删除链 `Service.DeleteTask` 提交后即调——被删全量 ID 含子任务；work 删除链治理经 taskManager `StagingCleaner` 注入适配）；`NewStagingOwnerAlive` 构造启动清扫的归属判活谓词（task 表全量 ID 一次装载，逐作用域 O(1) 判定，app.go 装配注入 staging 包统一清扫）。作用域键恒为任务 ID，两根各承载一种内容形态——下载根为 `role_seq` 键暂存文件，收件根父作用域含共享 manifest.json、子作用域为按清单路径镜像命名的暂存文件（形态说明见 `staging.go` 头注）；暂存总根不在 store/ 白名单与 backup/ 域内，fsmonitor 零感知。
- **leaf/独立任务（pid=NULL 根级）创建高回归区**：无 Children 响应 → 独立 leaf、有 Children → parent+children（不折叠），统一经 `planCreateResponse` 单点判定（stream/array 共用，消除双路径不对称）。根级任务 pid 落 NULL（外键引用 task.id，无 id=0 行，写 0 必违约），子任务 pid=父 ID。改创建路径须保 leaf(pid=NULL) 覆盖，回归测试 `backend/task/service_create_test.go`（fakeRepo 8 例 + OpenTestDB 外键库落盘锚定 1 例）。契约见 `doc/plugin-dev-guide.md`「Create 返回的任务结构契约」。

## 依赖关系

- 依赖：URL 监听派生索引（`urlListener.ListListener`，派生源为插件清单 `extensions.taskHandlers[].urlPatterns`，宿主激活期按清单条目派生登记、条目级粒度＝插件公开 ID + 条目 id）、多候选路由基座（`backend/route`，URL 监听器候选经其按全键字典序逐个尝试）、站点 / 作品集查询、事务执行器（Transactor，删除链编排用）、workDir 读取（`workDirGetter func() string`，删除链清下载暂存目录用）、resource.task_id 引用清理（repository 层原生 UPDATE，删任务前置义务）
- 被依赖：**taskManager**（消费 TaskStatusEnum 与任务树核心行查询 ListTaskTreeCore，并实现本模块定义的 `RunningStopper` 运行态停止器；板块选择写行/活跃插件计数投影经 download 模块仓储、work 删除链下载暂存清理经本模块暂存基建 `CleanupStagingByTaskIds` 适配）、前端任务管理页（CRUD + 查询）、site（TaskSiteRefCounter：站点删除守卫的任务引用计数，仓储 `CountBySiteId`）、share（收件侧经 `BuiltinTaskControl` 能力接口创建/启动内置任务树 + `ShareTaskStore` 窄接口读写 share_task 领域行，app.go 装配）、export（经 `TaskControl` 能力接口两步创建/启动/回滚删除导出任务，app.go 装配）、download（`SectionRecorder` 写行展开子成员用 `ListChildrenTask`，app.go 装配）
