# 前端自定义 Task DTO/Entity 迁移到 Wails Bindings 计划

## 背景

前端 `model/model/dto/` 和 `model/model/entity/` 中存在 6 个自定义类型，其中 5 个与 Wails 自动生成的 bindings 存在重叠，可迁移到 bindings。`TaskScheduleDTO` 因其增量更新语义无法被 binding 类型替代，予以保留。

## 当前类型对照

| 自定义类型 | 文件 | 处理方式 |
|---|---|---|
| `Task` (entity) | `model/model/entity/Task.ts` | → 删除，用 `TaskDTO` (bindings) 替代 |
| `TaskProgressDTO` | `model/model/dto/TaskProgressDTO.ts` | → 删除，用 `TaskProgressDTO` (bindings) 替代 |
| `TaskScheduleDTO` | `model/model/dto/TaskScheduleDTO.ts` | **保留** — 增量模式专用适配器 |
| `TaskProgressMapTreeDTO` | `model/model/dto/TaskProgressMapTreeDTO.ts` | → 删除（仅做类型断言） |
| `TaskProgressTreeDTO` | `model/model/dto/TaskProgressTreeDTO.ts` | → 删除，用 `TaskProgressTreeDTO` (bindings) 替代 |
| `TaskTreeDTO` | `model/model/dto/TaskTreeDTO.ts` | → 删除（过窄的类型注解） |

## Binding 类型结构（已有的）

```
TaskDTO (bindings)                ← 替代自定义 Task entity
├── id, hasChild, pid, taskName, siteId, siteWorkId, url
├── status, pendingResourceId, continuable
├── pluginPublicId, pluginContributionId, pluginData, errorMessage
└── createTime, updateTime

TaskProgressDTO (bindings)        ← 替代自定义 TaskProgressDTO
├── task: TaskDTO | null          ← 嵌套而非继承
├── total: number | null
├── finished: number | null
├── siteName: string | null
└── schedule: number | null

TaskProgressTreeDTO (bindings)    ← 替代自定义 TaskProgressTreeDTO
├── taskProgress: TaskProgressDTO | null
├── children: (TaskProgressTreeDTO | null)[]
├── hasChildren: boolean | null
└── isLeaf: boolean | null

taskSnapshotItem (bindings)       ← 快照模式使用
├── id, taskName, status, total, finished
```

## 核心差异：继承 vs 组合

**自定义类型**使用继承：`TaskProgressDTO extends Task`，字段直接平铺（`row.id`、`row.status`）。

**Binding 类型**使用组合：`TaskProgressDTO.task: TaskDTO`，需要嵌套访问（`row.task?.id`、`row.task?.status`）。

**关键发现**：TaskDialog.vue、TaskManage.vue、TaskOperationBarActive.vue 等组件**已经在使用 binding 的组合格式**（`row.taskProgress?.task?.id`），说明页面层的迁移实际已经完成。残留的自定义类型主要在 **Store 层和 IPC 监听层**。

## 为什么保留 `TaskScheduleDTO`

### 两条数据通道的差异

后端有两条推送路径，使用不同的 DTO 和更新语义：

| | 增量模式（`WailsTaskProgressPusher`） | 快照模式（`SnapshotPusher`） |
|---|---|---|
| 后端 DTO | `taskStateDTO` / `taskScheduleDTO`（私有类型） | `taskSnapshotDTO` / `taskSnapshotItem` |
| 推送方式 | `Events.Emit("task-events"/"parent-events")` | `Events.Emit("task-snapshot")` |
| 更新语义 | **部分字段更新**，缺失字段 = 不更新 | **全量替换** |
| Binding 暴露 | ❌ 不出现在 handler 方法签名中 | ✅ `GetTaskSnapshot()` 返回值间接引用 |

### 不能用 `taskSnapshotItem` 替代的原因

增量模式的后端 `taskScheduleDTO` 字段为非指针值类型（`int64`），Go 零值不会序列化到 JSON：

```
PushProgress → JSON: {"id":123, "total":100, "finished":50}
                             ↑ 没有 taskName、status 字段 → 前端为 undefined
```

前端通过 `notNullish()` 判断字段是否存在来决定是否更新，`undefined` 跳过，`0` 不跳过。

如果改用 `taskSnapshotItem`（同样是非指针值类型），未设置字段会以 Go 零值序列化：

```
PushProgress → JSON: {"id":123, "taskName":"", "status":0, "total":100, "finished":50}
                                         ↑ 零值出现，会覆盖真实数据
```

- `notNullish(0)` → `true` → 用 0（PENDING）覆盖了真实的 `status`
- `notNullish("")` → `true` → 用空字符串覆盖了真实的 `taskName`

不能加 `omitempty`，因为快照模式中 `total: 0`、`status: 0` 是合法业务值。两种模式的更新语义本质不同，不应共用同一个类型。

因此 `TaskScheduleDTO` 作为后端 `taskScheduleDTO` 的前端映射保留，注释中已标注对应关系。

---

## 迁移步骤

### 第一步：删除 `TaskTreeDTO`（无影响）

**文件**：`model/model/dto/TaskTreeDTO.ts`

**使用位置**：仅 `TaskOperationBar.vue` 的 `buttonClicked` 回调参数类型注解。

**问题**：`buttonClicked` 的实际参数是 `TaskProgressTreeDTO`（来自 `row` prop），`TaskTreeDTO` 类型注解过窄。

**操作**：
1. `TaskOperationBar.vue:12` — 将 `buttonClicked: (row: TaskTreeDTO, ...)` 改为 `buttonClicked: (row: TaskProgressTreeDTO, ...)`
2. `TaskOperationBar.vue` 中将 `import TaskProgressTreeDTO from '@renderer/model/model/dto/TaskProgressTreeDTO.ts'` 改为从 bindings 导入
3. 移除 `import TaskTreeDTO`
4. 删除 `TaskTreeDTO.ts`

### 第二步：删除 `TaskProgressMapTreeDTO`（无影响）

**文件**：`model/model/dto/TaskProgressMapTreeDTO.ts`

**使用位置**：仅 `MainIpcListener.ts` 中两处 `as TaskProgressMapTreeDTO[]` 类型断言。

**问题**：这两处类型断言实际接收的父任务 store 存的是 `TaskProgressDTO`，`TaskProgressMapTreeDTO` 的 `children: Map<...>` 字段从未被使用。

**操作**：
1. `MainIpcListener.ts:40` — `data as TaskProgressMapTreeDTO[]` → `data as TaskProgressDTO[]`
2. `MainIpcListener.ts:58` — `data as TaskProgressMapTreeDTO[]` → `data as TaskProgressDTO[]`
3. 移除 `import TaskProgressMapTreeDTO`
4. 删除 `TaskProgressMapTreeDTO.ts`

### 第三步：迁移 Store 中的 `TaskProgressDTO`（自定义） → `TaskProgressDTO`（binding）

**这是最关键的一步**。Store 层目前使用自定义 `TaskProgressDTO`（继承自 `Task`，字段平铺），需要切换到 binding `TaskProgressDTO`（组合结构 `task: TaskDTO`）。

#### 3a. Store 数据结构变更

**`UseTaskStore.ts`**：

当前：
```typescript
tasks: Map<number, { task: TaskProgressDTO, notificationId?: string }>
// 平铺访问: task.id, task.taskName, task.status, task.total, task.finished
```

迁移后：
```typescript
import { TaskProgressDTO } from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto'
import { TaskDTO } from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto'

tasks: Map<number, { task: TaskProgressDTO, notificationId?: string }>
// 嵌套访问: task.task?.id, task.task?.taskName, task.task?.status, task.total, task.finished
```

**`UseParentTaskStore.ts`**：

当前：
```typescript
parentTasks: Map<number, TaskProgressDTO>
```

迁移后：
```typescript
parentTasks: Map<number, TaskProgressDTO>  // 同样切换到 binding TaskProgressDTO
```

#### 3b. Store 方法改造要点

**`loadSnapshot` 方法**（两 Store 共有）：

当前从 `taskSnapshotItem` 手动构造自定义 `TaskProgressDTO`：
```typescript
const taskDTO = new TaskProgressDTO()
taskDTO.id = item.id
taskDTO.taskName = item.taskName
taskDTO.status = item.status
taskDTO.total = item.total
taskDTO.finished = item.finished
```

迁移为使用 binding `TaskProgressDTO`：
```typescript
const taskDTO = new TaskProgressDTO()
taskDTO.task = new TaskDTO()
taskDTO.task.id = item.id
taskDTO.task.taskName = item.taskName
taskDTO.task.status = item.status
taskDTO.total = item.total
taskDTO.finished = item.finished
```

**`updateTask` 方法**（`UseTaskStore`）：

当前：`new TaskProgressDTO()` 创建空对象，然后 `copyIgnoreUndefined(taskStoreObj.task, task)` 合并。
迁移：`new TaskProgressDTO()` + `new TaskDTO()` 初始化，合并逻辑不变（`copyIgnoreUndefined` 是浅拷贝，对组合结构同样有效）。

**`updateTaskSchedule` 方法**（两 Store 共有）：

当前：`task.status = scheduleDTO.status`（平铺）。
迁移：`task.task.status = scheduleDTO.status`（嵌套）。需要注意 `task.task` 可能为 null 的保护。

**`setTask` / `createNotificationItem`**：

访问 `task.taskName` → `task.task?.taskName`。

#### 3c. Store 外部访问适配

Store 外部通过 `getTask()` 获取 `TaskProgressDTO`，迁移后字段访问路径变化。需要检查所有调用方：

- `MainIpcListener.ts`：通过 store 方法间接访问，无需改
- 各 Vue 组件：已使用 binding 格式（`row.taskProgress?.task?.id`），不受影响

### 第四步：删除 `TaskProgressDTO`（自定义）和 `Task` (entity)

**前提**：第二步、第三步完成后。

**文件**：
- `model/model/dto/TaskProgressDTO.ts`
- `model/model/entity/Task.ts`

**操作**：
1. 确认无残留 import
2. 删除两个文件

### 第五步：删除 `TaskProgressTreeDTO`（自定义）

**文件**：`model/model/dto/TaskProgressTreeDTO.ts`

**使用位置**：`TaskOperationBar.vue` 中一处 import（第一步已替换为 binding import）。

**操作**：
1. 确认 `TaskOperationBar.vue` 不再 import 自定义版本
2. 删除 `TaskProgressTreeDTO.ts`

### 第六步：清理 `MainIpcListener.ts`

将 `TaskProgressDTO` 的 import 从自定义版本切换到 binding 版本，确认 `TaskScheduleDTO` 保留。

---

## 文件变更清单

| 文件 | 操作 |
|---|---|
| `model/model/entity/Task.ts` | 删除 |
| `model/model/dto/TaskProgressDTO.ts` | 删除 |
| `model/model/dto/TaskProgressMapTreeDTO.ts` | 删除 |
| `model/model/dto/TaskProgressTreeDTO.ts` | 删除 |
| `model/model/dto/TaskTreeDTO.ts` | 删除 |
| `model/model/dto/TaskScheduleDTO.ts` | **保留**（已添加后端映射注释） |
| `MainIpcListener.ts` | 改 import，`TaskProgressMapTreeDTO` → `TaskProgressDTO`(binding) |
| `UseTaskStore.ts` | 改 import，Store 内部访问路径从平铺→嵌套 |
| `UseParentTaskStore.ts` | 改 import，Store 内部访问路径从平铺→嵌套 |
| `TaskOperationBar.vue` | 移除 `TaskTreeDTO`/`TaskProgressTreeDTO` 自定义 import，改回调类型 |

## 风险点

1. **`copyIgnoreUndefined` 兼容性**：该方法做浅拷贝，对 binding 的组合结构（`TaskProgressDTO.task: TaskDTO`）是安全的，因为只复制第一层引用。但需确认 `task.task` 已初始化为 `TaskDTO` 实例后再合并。

2. **`TaskScheduleDTO` 构造器适配**：`TaskScheduleDTO` 构造器兼容 binding 格式和 IPC 事件格式两种数据。Store 中 `instanceof TaskScheduleDTO` 检查和 `new TaskScheduleDTO(rawData)` 构造保留不变。

3. **IPC 事件格式无编译期约束**：后端 `taskStateDTO` 和 `taskScheduleDTO` 是私有类型，字段格式不通过 binding 约束。`TaskScheduleDTO` 已在注释中标注对应后端类型作为契约文档。
