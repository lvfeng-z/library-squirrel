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
- **任务树**：父任务聚合子任务，父任务状态由子任务聚合得出（PartlyFinished 为父任务聚合态）。
- **CreateTaskByURL 路由**：URL 匹配插件的 URL 监听器，路由到对应插件创建任务。
- **leaf/独立任务（pid=NULL 根级）创建高回归区**：无 Children 响应 → 独立 leaf、有 Children → parent+children（不折叠），统一经 `planCreateResponse` 单点判定（stream/array 共用，消除双路径不对称）。根级任务 pid 落 NULL（外键引用 task.id，无 id=0 行，写 0 必违约），子任务 pid=父 ID。改创建路径须保 leaf(pid=NULL) 覆盖，回归测试 `backend/task/service_create_test.go`（fakeRepo 8 例 + OpenTestDB 外键库落盘锚定 1 例）。契约见 `doc/plugin-dev-guide.md`「Create 返回的任务结构契约」。

## 依赖关系

- 依赖：URL 监听器（`urlListener.ListListener`，由插件提供）、站点 / 作品集查询、事务执行器（Transactor，删除链编排用）、resource.task_id 引用清理（repository 层原生 UPDATE，删任务前置义务）
- 被依赖：**taskManager**（消费 TaskStatusEnum）、前端任务管理页（CRUD + 查询）、site（TaskSiteRefCounter：站点删除守卫的任务引用计数，仓储 `CountBySiteId`）
