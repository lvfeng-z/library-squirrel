# 修复：独立单任务（pid=0, hasChild=false）无法执行

## 问题描述

当插件返回的单个子任务被创建为独立任务（`pid=0`、`hasChild=false`）后，开始或重试该任务时，任务不会执行任何下载操作，直接被标记为完成（status=6）。

## 根因分析

`loadAndStartTaskTree`（`manager.go:145`）的任务分类逻辑存在盲区：

| 任务类型 | pid | hasChild | 当前行为 | 正确行为 |
|---|---|---|---|---|
| 叶子任务 | >0 | false | ✅ 正确识别为叶子 | 同左 |
| 父任务 | 0 | true | ✅ 正确识别为父 | 同左 |
| **独立单任务** | **0** | **false** | ❌ 被当作无子任务的父任务 | **应直接作为子任务执行** |

**执行链**：`isLeaf` 判断要求 `Pid > 0` → 独立任务 `isLeaf=false` → 被当作 `actualParentId` → 查找 `pid == taskId` 的子任务 → 找不到 → 零子任务退出 → `computeParentFinalState` 返回 `TaskStateFinished`。

## 受影响的函数

| 函数 | 文件 | 影响 |
|---|---|---|
| `loadAndStartTaskTree` | `manager.go:145` | 核心：独立任务被当作空父任务 |
| `computeParentFinalState` | `manager.go:267` | `total=0` 时返回 Finished |
| `resolveParentKey` | `manager.go:824` | `isLeaf=false` → 查 `parentMap` → 找不到 |
| `GetTaskTreeState` | `manager.go:565` | 依赖 `resolveParentKey`，同样失败 |
| `PauseTaskTree` / `ResumeTaskTree` / `StopTaskTree` | `manager.go` | 依赖 `resolveParentKey`，同样失败 |
| 前端 `isLeafTask` | `TaskManage.vue:231` | `pid=0` → 返回 `false` |

## 修复方案

### 核心思路

在 `loadAndStartTaskTree` 中识别"独立单任务"（`pid=0`、`hasChild=false`），将其直接包装为 `ManagedTask` 并派发，跳过父子关系构建逻辑。

### 修改清单

#### 1. `backend/taskManager/manager.go` — `loadAndStartTaskTree`

在 `isLeaf` 判断之后（约 line 171），增加"独立单任务"检测：

```go
// 判断是否为独立单任务（无父无子）
isStandalone := rootTask != nil &&
    (!rootTask.Pid.Valid || rootTask.Pid.Int64 == 0) &&
    (!rootTask.HasChild.Valid || !rootTask.HasChild.Bool)
```

当 `isStandalone=true` 时，直接构建 `ManagedTask` 并派发，不走父子关系逻辑：

```go
if isStandalone {
    child := m.buildOrReuseChild(rootTask, skipTerminal)
    if child == nil {
        // 已终态，直接计算状态
        finalState := TaskState(rootTask.Status)
        if isStableState(finalState) {
            m.addToPending(taskId, task.TaskStatusEnum(finalState), "")
        }
        return nil
    }
    // 不加入 parentMap，因为独立任务没有父任务
    m.tryDispatch(child)
    return nil
}
```

注意：独立任务的 `ManagedTask.parentId=0`，`cleanupFinishedTask` 中 `mt.parentId != 0` 分支不会进入，清理逻辑直接移除自身即可。这与现有行为兼容。

#### 2. `backend/taskManager/manager.go` — `resolveParentKey`

增加独立单任务的处理路径。当前逻辑：

```go
func (m *Manager) resolveParentKey(taskId int64, isLeaf bool) (int64, bool) {
    if !isLeaf {
        return taskId, true
    }
    // ...leaf path...
}
```

对于独立任务，`isLeaf=false` 会返回 `(taskId, true)`，随后调用方会查 `parentMap[taskId]` → 找不到 → 报错。需要让独立任务走 `taskMap` 查找路径。修改为：

```go
func (m *Manager) resolveParentKey(taskId int64, isLeaf bool) (int64, bool) {
    if !isLeaf {
        // 非叶子：检查是否在 parentMap 中（真正的父任务）
        m.mu.RLock()
        _, inParent := m.parentMap[taskId]
        m.mu.RUnlock()
        if inParent {
            return taskId, true
        }
        // 不在 parentMap：可能是独立单任务，当作叶子处理（查 taskMap）
    }
    // 叶子 / 独立任务：从 taskMap 获取 parentId
    m.mu.RLock()
    mt, ok := m.taskMap[taskId]
    m.mu.RUnlock()
    if !ok {
        return 0, false
    }
    if mt.parentId == 0 {
        return taskId, true
    }
    return mt.parentId, true
}
```

这样 `PauseTaskTree`、`ResumeTaskTree`、`StopTaskTree`、`GetTaskTreeState` 都能正确处理独立任务——它们通过 `taskMap` 找到 `ManagedTask`，而独立任务的 `parentId=0`，最终返回 `taskId` 自身作为查找键。

#### 3. `backend/taskManager/manager.go` — `GetTaskTreeState`

当前实现：

```go
func (m *Manager) GetTaskTreeState(taskId int64, isLeaf bool) (TaskState, error) {
    parentKey, ok := m.resolveParentKey(taskId, isLeaf)
    if !ok {
        return TaskStateCreated, ErrTaskTreeNotFound
    }
    parent, ok := m.parentMap[parentKey]
    if !ok {
        return TaskStateCreated, ErrTaskTreeNotFound
    }
    return parent.GetState(), nil
}
```

修改 `resolveParentKey` 后，独立任务返回 `(taskId, true)`，但 `parentMap[taskId]` 仍不存在。需要在查 `parentMap` 之前先查 `taskMap`：

```go
func (m *Manager) GetTaskTreeState(taskId int64, isLeaf bool) (TaskState, error) {
    parentKey, ok := m.resolveParentKey(taskId, isLeaf)
    if !ok {
        return TaskStateCreated, ErrTaskTreeNotFound
    }

    // 优先查 parentMap（父任务场景）
    if parent, ok := m.parentMap[parentKey]; ok {
        return parent.GetState(), nil
    }

    // 回退查 taskMap（独立单任务场景）
    m.mu.RLock()
    mt, ok := m.taskMap[parentKey]
    m.mu.RUnlock()
    if !ok {
        return TaskStateCreated, ErrTaskTreeNotFound
    }
    return mt.getState(), nil
}
```

#### 4. 前端 `isLeafTask` — 无需修改

前端的 `isLeafTask` 对独立任务返回 `false`（当作非叶子），这与后端新逻辑兼容：后端 `loadAndStartTaskTree` 不再依赖前端传入的 `isLeaf` 参数来识别独立任务，而是通过 DB 字段自行判断。`resolveParentKey` 也已改为 fallback 查 `taskMap`。因此前端无需改动。

### 不修改的部分

| 项目 | 原因 |
|---|---|
| `cleanupFinishedTask` | 独立任务 `parentId=0`，走 `else` 分支直接从 `taskMap` 删除，逻辑正确 |
| `buildOrReuseChild` | 已支持无 `parentId` 的任务，无需修改 |
| 前端 `isLeafTask` | 后端自行判断独立任务，前端无需感知 |
| `computeParentFinalState` | 独立任务不再走到此函数 |

## 修改文件清单

| 文件 | 修改点 |
|---|---|
| `backend/taskManager/manager.go` | `loadAndStartTaskTree` 增加独立任务分支；`resolveParentKey` 增加 taskMap fallback；`GetTaskTreeState` 增加 taskMap fallback |
