# 任务状态查询：综合内存与数据库状态

## 问题

`taskManager.Manager` 仅将稳定态（Paused/Finished/Failed/PartlyFinished）持久化到数据库，瞬态（Created/Waiting/Processing/Pausing/Stopping）仅存在于内存中。导致 `TaskQueryDTO.Status` 条件翻译为 SQL `WHERE status = ?` 时，对瞬态状态的查询永远返回空结果。

此外，任务进入内存后（如 Paused → Processing 恢复），数据库中的旧状态可能已过时，稳态查询也可能返回不准确的结果。

## 方案

查库之前先查询内存中的任务状态，获取符合条件的 ID，将这些 ID 作为附加条件融入原有查询条件，复用现有分页机制。

**核心思路**：
- **瞬态状态查询**：从内存收集匹配 ID → 清除 Status 条件 → 添加 `id IN (匹配IDs)` → 查询 DB
- **稳态状态查询**：从内存收集不匹配 ID → 保留 Status 条件 → 追加 `id NOT IN (不匹配IDs)` → 查询 DB
- **无状态查询**：直接查 DB，不做额外处理

## 改动清单

### 1. `task` 包：定义接口

**文件**：`backend/task/service.go`

新增 `MemoryStateProvider` 接口（调用方定义，提供方实现，符合 SERVICE_DEPENDENCY_VIA_INTERFACE 规则）：

```go
// MemoryStateProvider 内存任务状态提供者接口
// 由 taskManager.Manager 实现，用于查询时综合内存中的实时状态
type MemoryStateProvider interface {
    // GetTaskStates 获取所有内存中任务的当前状态快照
    // 返回 map[taskId]status，包含父任务和子任务
    GetTaskStates() map[int64]int
}
```

`Service` 结构体新增字段和 setter：

```go
type Service struct {
    // ... 现有字段
    memoryProvider MemoryStateProvider  // 新增
}

func (s *Service) SetMemoryProvider(provider MemoryStateProvider) {
    s.memoryProvider = provider
}
```

新增辅助函数：

```go
// isTransientStatus 判断状态是否为瞬态（不会出现在数据库中）
func isTransientStatus(status int) bool {
    switch TaskStatusEnum(status) {
    case TaskStatusCreated, TaskStatusWaiting, TaskStatusProcessing,
        TaskStatusPausing, TaskStatusStopping:
        return true
    default:
        return false
    }
}
```

### 2. `task` 包：修改查询方法

**文件**：`backend/task/service.go`

新增核心方法 `buildPageOptionWithMemory`，供 `QueryParentPage`、`Page`、`QueryChildrenTaskPage` 三个分页方法共用：

```go
// buildPageOptionWithMemory 构建 PageOption，综合内存中的任务状态调整查询条件
func (s *Service) buildPageOptionWithMemory(query TaskQueryDTO, page, pageSize int) (*database.PageOption, error) {
    if query.Status.Value == nil || s.memoryProvider == nil {
        // 无状态过滤或无内存提供者：标准转换
        conv := querypkg.NewConverter(entity.Task{})
        return conv.ToPageOption(query, page, pageSize, nil)
    }

    targetStatus := int(*query.Status.Value)
    states := s.memoryProvider.GetTaskStates()

    if isTransientStatus(targetStatus) {
        // 瞬态：收集内存中匹配的 ID
        var matchingIDs []int64
        for id, state := range states {
            if state == targetStatus {
                matchingIDs = append(matchingIDs, id)
            }
        }

        // 清除 Status 条件（DB 中不存在瞬态）
        query.Status.Value = nil

        conv := querypkg.NewConverter(entity.Task{})
        opt, err := conv.ToPageOption(query, page, pageSize, nil)
        if err != nil {
            return nil, err
        }

        if len(matchingIDs) > 0 {
            opt.Conditions = append(opt.Conditions, clause.IN{
                Column: clause.Column{Name: "id"},
                Values: toInterfaceSlice(matchingIDs),
            })
        } else {
            // 无匹配任务，返回永假条件
            opt.Conditions = append(opt.Conditions, clause.Eq{
                Column: clause.Column{Name: "id"}, Value: -1,
            })
        }
        return opt, nil
    }

    // 稳态：收集内存中状态不同的 ID（需排除，防止 DB 旧状态干扰）
    var excludeIDs []interface{}
    for id, state := range states {
        if state != targetStatus {
            excludeIDs = append(excludeIDs, id)
        }
    }

    conv := querypkg.NewConverter(entity.Task{})
    opt, err := conv.ToPageOption(query, page, pageSize, nil)
    if err != nil {
        return nil, err
    }

    if len(excludeIDs) > 0 {
        opt.Conditions = append(opt.Conditions, clause.Not(clause.IN{
            Column: clause.Column{Name: "id"},
            Values: excludeIDs,
        }))
    }
    return opt, nil
}
```

> 注意：`clause.Not` 和 `clause.IN` 来自 `gorm.io/gorm/clause`。如果 `clause.Not` 的签名不兼容，可改用 `clause.Expr{SQL: "id NOT IN ?", Vars: []interface{}{excludeIDs}}`。

修改三个分页方法，替换 `conv.ToPageOption` 为 `buildPageOptionWithMemory`：

- `QueryParentPage`：调用 `buildPageOptionWithMemory(query, page.PageNumber, page.PageSize)`
- `Page`：同上
- `QueryChildrenTaskPage`：同上

### 3. `taskManager` 包：实现接口

**文件**：`backend/taskManager/manager.go`

在 `Manager` 上实现 `MemoryStateProvider` 接口：

```go
// GetTaskStates 获取所有内存中任务的当前状态快照
func (m *Manager) GetTaskStates() map[int64]int {
    m.mu.RLock()
    defer m.mu.RUnlock()

    states := make(map[int64]int)

    // 子任务状态
    for id, mt := range m.taskMap {
        states[id] = int(mt.GetState())
    }

    // 父任务状态
    for id, pt := range m.parentMap {
        states[id] = int(pt.GetState())
    }

    // 等待确认的任务
    m.waitingForInputMu.Lock()
    for id, mt := range m.waitingForInputMap {
        states[id] = int(mt.GetState())
    }
    m.waitingForInputMu.Unlock()

    return states
}
```

### 4. `app.go`：注入依赖

在 `app.go` 的初始化流程中，`TaskManagerService` 创建之后调用 setter 注入：

```go
// 现有：app.TaskManagerService = taskManager.NewManager(...)

// 新增：将 TaskManager 注入到 TaskService 作为内存状态提供者
app.TaskService.SetMemoryProvider(app.TaskManagerService)
```

### 5. 可选增强：查询结果状态覆写

**文件**：`backend/task/service.go`

在分页查询返回结果后，用内存状态覆写 DB 中的旧状态，使首次加载即显示正确状态：

```go
// overlayMemoryStates 用内存中的实时状态覆写查询结果中的状态
func (s *Service) overlayMemoryStates(tasks []*entity.Task) {
    if s.memoryProvider == nil {
        return
    }
    states := s.memoryProvider.GetTaskStates()
    for _, task := range tasks {
        if state, ok := states[task.GetID()]; ok {
            task.Status = state
        }
    }
}
```

在三个分页方法返回前调用 `overlayMemoryStates(result.Data)`。

## 状态场景对照表

| 查询状态 | 内存情况 | DB 情况 | 处理方式 | 结果 |
|---|---|---|---|---|
| Processing（瞬态） | tasks [1,5] 是 Processing | 旧状态 Created/Paused | `id IN (1,5)`，无 status 条件 | 正确返回 [1,5] |
| Processing（瞬态） | 无匹配 | — | `id = -1`（永假） | 返回空 |
| Finished（稳态） | task [3] 刚 Finished 未刷盘 | status=2(Processing) | `status=6 AND id NOT IN (其他内存任务)` | task 3 会被排除（200ms 窗口），但刷盘后自动修正 |
| Paused（稳态） | task [7] 已 Resume 为 Processing | status=4(Paused) | `status=4 AND id NOT IN (7)` | 正确排除 [7] |
| 无状态过滤 | — | — | 标准查询 + 可选覆写 | 全量返回 |

## 文件改动汇总

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `backend/task/service.go` | 修改 | 新增接口、辅助方法、修改分页查询 |
| `backend/taskManager/manager.go` | 修改 | 新增 `GetTaskStates()` 方法 |
| `app.go` | 修改 | 新增 `SetMemoryProvider` 调用 |

## 风险评估

- **内存数据量**：`taskMap` + `parentMap` 大小受信号量（maxParallel）和等待队列限制，通常 < 100，`GetTaskStates()` 性能无忧
- **200ms 刷盘延迟**：稳态查询可能漏掉刚进入稳态但未刷盘的任务，可接受
- **锁竞争**：`GetTaskStates()` 使用 `RLock`，不阻塞 `executeTask` 等写操作
