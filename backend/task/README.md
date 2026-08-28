# task 模块说明

## 一句话职责

任务**实体**的持久化层：管理任务记录的增删改查与状态字段读写，以及"URL → 插件 → 任务"的创建路由。负责"任务记录长什么样、存在哪"，"任务怎么执行"由 taskManager 负责。

## 边界

- 与 **taskManager**：task 是**静态数据**（实体 CRUD、状态枚举的定义方）；taskManager 是**动态调度**（消费 task 定义的状态枚举驱动运行时状态机）。task 写数据库，taskManager 读 task 的状态枚举来推进。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `CreateTask(req)` | 创建任务（含父子任务树） |
| `CreateTaskByURL(url)` | URL → 查询监听该 URL 的插件 → 创建任务 |
| `CreateBuiltinTask(taskType, taskName, payload)` | 创建内置类型任务（`task_type` 非空、非插件执行；payload 为该类型执行面自有 JSON 载荷，本模块不解析；创建后停留 Created，运行控制与插件任务一致）。亦作为 `share.BuiltinTaskControl` 能力的实现方之一（app.go 装配） |
| `Save` / `Update` | 保存 / 更新任务 |
| `DeleteTask(ids)` | 批量删除任务（含子任务；事务内先清 resource.task_id 引用再删行，引用置 NULL=非任务产，resource 行保留） |
| `RefreshStatus(taskId)` | 刷新任务状态 |
| `SetTreeStatus(taskIds, status, includeStatus)` | 设置任务树状态 |
| `GetById` / `QueryPage` | 单查 / 分页查询 |
| `QueryParentPage` / `QueryChildrenTaskPage` | 父任务 / 子任务分页（带站点名） |
| `ListChildrenTask` / `ListTaskTree` | 子任务列表 / 任务树列表 |
| `ListTasksBySiteAndSiteWorkID` | 按站点 + 站点作品ID查关联任务（板块执行选任务） |
| `QueryTreeDataPage` | 任务树数据分页 |
| `ListStatus` / `ListSchedule` | 状态 / 进度列表 |

## 核心概念

- **TaskStatusEnum**：任务状态枚举，与 taskManager.TaskState 保持一致。
  `Created(0) / Waiting(1) / Processing(2) / Pausing(3) / Paused(4) / Stopping(5) / Finished(6) / Failed(7) / PartlyFinished(8)`
- **任务类型（task_type）**：NULL/空=插件任务（plugin_public_id 路由执行器）；非空=内置类型（share-host/share-receive 等，经 taskManager 注册的执行面策略执行，`payload` 列承载该类型自有 JSON 载荷）。
- **任务树**：父任务聚合子任务，父任务状态由子任务聚合得出（PartlyFinished 为父任务聚合态）。
- **内置任务树建树**（Service 层能力，非 Handler 暴露；经调用方定义的能力接口注入使用，如 share 的 `BuiltinTaskControl`，app.go 装配）：
  - `CreateBuiltinTaskTree(taskType, parentName, children)`：事务原子创建整树——1 个父容器（has_child=true、pid=NULL、task_type 落值）+ N 个子任务（pid=父ID、has_child=false、task_type/payload 落值），父 ID 在事务内回填子 pid，任一步失败整体回滚。
  - `CreateBuiltinTaskParent` + `CreateBuiltinTaskChildren`：两段式建树——先建父容器拿 parentID（子任务入参依赖父 ID 的场景，如子任务载荷引用父任务目录下的共享文件路径），再补建子任务；子任务创建非事务，失败由调用方显式 `DeleteTask` 删树回滚。
  - 入参 `BuiltinTaskChild{TaskName, Payload}`（children 顺序即子任务展示顺序；payload 为执行面自有 JSON，本模块不解析）；错误：子任务为空 `ErrBuiltinTaskNoChildren`、父 ID 无效 `ErrBuiltinTaskChildrenNoParent`。父容器为纯聚合节点（无执行面、无 payload），子任务各自独立执行。
- **CreateTaskByURL 路由**：URL 匹配插件的 URL 监听器，路由到对应插件创建任务。
- **leaf/独立任务（pid=NULL 根级）创建高回归区**：无 Children 响应 → 独立 leaf、有 Children → parent+children（不折叠），统一经 `planCreateResponse` 单点判定（stream/array 共用，消除双路径不对称）。根级任务 pid 落 NULL（外键引用 task.id，无 id=0 行，写 0 必违约），子任务 pid=父 ID。改创建路径须保 leaf(pid=NULL) 覆盖，回归测试 `backend/task/service_create_test.go`（fakeRepo 8 例 + OpenTestDB 外键库落盘锚定 1 例）。契约见 `doc/plugin-dev-guide.md`「Create 返回的任务结构契约」。

## 依赖关系

- 依赖：URL 监听器（`urlListener.ListListener`，由插件提供）、站点 / 作品集查询、事务执行器（Transactor，删除链编排用）、resource.task_id 引用清理（repository 层原生 UPDATE，删任务前置义务）
- 被依赖：**taskManager**（消费 TaskStatusEnum）、前端任务管理页（CRUD + 查询）、site（TaskSiteRefCounter：站点删除守卫的任务引用计数，仓储 `CountBySiteId`）、share（收件侧经 `BuiltinTaskControl` 能力接口创建/启动内置任务树，app.go 装配）
