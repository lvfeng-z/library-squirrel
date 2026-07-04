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
- **批量状态写入**：状态 / 进度先攒在内存，由 `flushLoop` 定时批量刷库，避免逐条写入。

## 依赖关系

- 依赖：`Repository`（任务树查询 / 批量状态设置）、`task` 包（TaskStatusEnum 状态枚举）、`WorkDirProvider`、`FileNameFormatProvider`、`pluginExecFactory`（插件执行器，TaskExecutor）、`TaskProgressPusher`
- 被依赖：前端任务执行面板（操作栏）

## 关键设计

- **内存 + 数据库双轨状态**：运行态以内存为准（`GetTaskSnapshot` / `IsIdle`），查询态综合两者（`GetTaskState`）。
- **一任务一 goroutine dispatch 不变量**：任意时刻每个任务至多存在一条 `executeTask` goroutine。两层守卫：创建层 `claimTask`/`claimParent`（`m.mu` 下 insert-or-get）保证同一 taskId 只有一个 `ManagedTask`/`ParentTask` 对象（取代快照式判重，消除 TOCTOU）；派发层 `dispatch` 用 `dispatchState`（`dsIdle`/`dsQueued`/`dsRunning`）CAS-claim 保证同一对象只派发一条 goroutine（输者幂等 no-op）；`pendingResume` 标志兜住内存内 Resume 在 pause-exit 窗口的丢失唤醒（executeTask 循环顶消费并 `prepareForResume` 重置）。`removeFromQueue`/`releaseSlotAndIdle` 负责把 `queued`/`running` 回退到 `idle`。
- **唯二执行入口**：`startTaskTrees`（开始 / 重试 / 板块重执行）与 `resumeTaskTrees`（恢复）从 DB 加载任务树并派发；`ResumeTaskTree`（内存内恢复）与 `ConfirmReplace` 直接走 `dispatch`。所有派发统一经 `dispatch` 单一漏斗（含 CAS 不变量）。`Redownload` 先持久化板块选择到 task 再调 `startTaskTrees`。
- **依赖全部接口注入**：插件执行器、仓储、进度推送器均通过构造函数注入，`Manager` 不直接持有具体 Service。
