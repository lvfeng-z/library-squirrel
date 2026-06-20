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
| `Redownload(taskIds, sections)` | 板块重执行（多选组合：A=1/B=2/C=3，空数组视为全集） |
| `GetTaskState(taskId)` | 查询单任务状态（综合内存 + 数据库） |
| `GetTaskTreeState(taskId, isLeaf)` | 查询任务树聚合状态 |
| `GetTaskSnapshot()` | 获取所有活跃任务的完整状态快照 |
| `IsIdle()` | 是否空闲（无运行中任务） |
| `ConfirmReplace` / `ConfirmReplaceBatch` | 用户确认重复作品的替换 / 跳过 |

## 核心概念

- **ManagedTask / ParentTask**：内存中的运行任务与父任务聚合。
- **信号量**：`maxParallel` 控制全局并发数，超出则进 FIFO 等待队列。
- **板块执行模式**（runMode）：`Full` / `ResourceOnly` / `WorkInfo` / `Thumbnail`，支持按板块单独重执行。
- **进度推送器**（TaskProgressPusher）：两种实现——Wails 事件直推、快照模式（SnapshotPusher）。
- **批量状态写入**：状态 / 进度先攒在内存，由 `flushLoop` 定时批量刷库，避免逐条写入。

## 依赖关系

- 依赖：`Repository`（任务树查询 / 批量状态设置）、`task` 包（TaskStatusEnum 状态枚举）、`WorkDirProvider`、`FileNameFormatProvider`、`pluginExecFactory`（插件执行器，TaskExecutor）、`TaskProgressPusher`
- 被依赖：前端任务执行面板（操作栏）

## 关键设计

- **内存 + 数据库双轨状态**：运行态以内存为准（`GetTaskSnapshot` / `IsIdle`），查询态综合两者（`GetTaskState`）。
- **唯二执行入口**：`startTaskTrees`（开始 / 重试 / 板块重执行）与 `resumeTaskTrees`（恢复），其余操作均收敛到这两个入口。
- **依赖全部接口注入**：插件执行器、仓储、进度推送器均通过构造函数注入，`Manager` 不直接持有具体 Service。
