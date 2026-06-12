# 事务范围优化计划

## 目标

为任务执行过程中缺少事务保护的关键操作添加事务，确保数据一致性。

---

## P0-1: `DeleteWorkAndSurroundingData` 添加事务

### 问题

`work/service.go:375` 的删除操作分 5 步执行，无事务保护。部分失败导致不可恢复的数据不一致。

### 方案

将关联表删除 + Resource 删除 + Work 删除包裹在事务中。磁盘文件删除（PersistentStore）在事务成功后执行，事务回滚时文件保留（后续可清理或下次删除时重试）。

#### 修改文件

**1. `backend/work/service.go`**

- 重构 `DeleteWorkAndSurroundingData` 方法，使用 `s.transactor.ExecInTransaction` 包裹以下操作：
  1. 删除 reWorkTag 关联
  2. 删除 reWorkWorkSet 关联
  3. 删除 Resource
  4. 删除 Work
- 事务成功后，执行 PersistentStore 的磁盘文件删除（不在事务内）
- 事务回滚时，文件保留不删除

```go
func (s *Service) DeleteWorkAndSurroundingData(ctx context.Context, id int64) error {
    // 事务前：收集需要删除的 Store 信息（事务内不能保留文件 I/O）
    var storesToDelete []int64
    resources, err := s.resourceDeleter.ListByWorkId(ctx, id)
    if err != nil {
        return err
    }
    for _, res := range resources {
        if res.WorkStoreID.Valid {
            storesToDelete = append(storesToDelete, res.WorkStoreID.Int64)
        }
        if res.ThumbnailStoreID.Valid {
            storesToDelete = append(storesToDelete, res.ThumbnailStoreID.Int64)
        }
    }

    // 事务内：删除关联 + Resource + Work
    err = s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
        if err := s.reWorkTagWriter.DeleteByWorkId(txCtx, id); err != nil {
            return err
        }
        if err := s.reWorkWorkSetWriter.DeleteByWorkId(txCtx, id); err != nil {
            return err
        }
        if err := s.reWorkAuthorWriter.DeleteByWorkId(txCtx, id); err != nil {
            return err
        }
        if err := s.resourceDeleter.DeleteByWorkId(txCtx, id); err != nil {
            return err
        }
        return s.repo.Delete(txCtx, id)
    })
    if err != nil {
        return err
    }

    // 事务后：删除磁盘文件（尽力而为，失败不影响业务）
    if s.storeDeleter != nil {
        for _, storeId := range storesToDelete {
            s.storeDeleter.Delete(ctx, storeId, false)
        }
    }
    return nil
}
```

**注意**：当前 `DeleteWorkAndSurroundingData` 未删除 `reWorkAuthor` 关联，这是一个遗漏，需一并补充。

---

## P0-2: Resource Save + PendingResourceID 更新添加事务

### 问题

`taskManager/model.go` 中步骤 5（保存 Resource）和步骤 6（更新 PendingResourceID）是两个独立的 DB 操作。如果 Resource 保存成功但 PendingResourceID 更新失败，会产生孤儿 Resource。

### 方案

引入一个新的接口 `TransactionAwareStoreSetup`，将 Resource 保存和 PendingResourceID 更新合并到同一个事务中。StoreStream（涉及文件 I/O）不纳入事务。

#### 修改文件

**1. `backend/taskManager/model.go`**

- 新增接口定义：

```go
// TransactionAwareTransactor 事务执行器接口（用于步骤 5-6 的事务包裹）
type TransactionAwareTransactor interface {
    ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
```

- 在 `ManagedTask` 结构体中新增字段 `transactor TransactionAwareTransactor`
- 在 `NewManagedTask` 构造函数中接收该依赖
- 修改 `run()` 方法步骤 5-6 的代码，用事务包裹：

```go
// 步骤 4: StoreStream（不纳入事务）
storeId, writer, err := m.storeStreamer.StoreStream(m.ctx, relativePath, fileName)
// ...

// 步骤 5-6: 在事务内保存 Resource + 更新 PendingResourceID
var resourceId int64
err = m.transactor.ExecInTransaction(m.ctx, func(txCtx context.Context) error {
    // 保存 Resource（替换场景更新 / 新建场景创建）
    var saveErr error
    resourceId, saveErr = m.saveResourceInTx(txCtx, storeId, ...)
    if saveErr != nil {
        return saveErr
    }
    // 更新 PendingResourceID
    m.task.PendingResourceID = sql.NullInt64{Int64: resourceId, Valid: true}
    return m.pendingResourceUpdater.UpdatePendingResourceID(txCtx, m.taskId, m.task.PendingResourceID)
})
if err != nil {
    writer.Abort()
    // 错误处理...
}
```

- 新增 `saveResourceInTx` 私有方法，将当前步骤 5 中的替换/新建逻辑抽取出来，接收 `txCtx` 参数
- 新增接口 `PendingResourceUpdater`：

```go
// PendingResourceUpdater 任务 pending_resource_id 更新接口
type PendingResourceUpdater interface {
    UpdatePendingResourceID(ctx context.Context, taskId int64, resourceID sql.NullInt64) error
}
```

**2. `backend/taskManager/manager.go`**（TaskManager 构造 ManagedTask 的地方）

- 在 `NewManagedTask` 调用时传入 `transactor` 实例（复用 `work.Service` 使用的同一个 `dbTransactorAdapter`）

**3. `app.go`**

- 将 `dbTransactorAdapter` 实例也传递给 `TaskManager` 的构建链

#### 替换场景的 ResourceUpdater

当前替换场景使用 `m.resourceUpdater.Update(ctx, existingResource)` 更新已有 Resource。此操作也应接收 `txCtx` 以参与事务。需要确认 `ResourceUpdater` 的底层 repository 是否支持 context 事务传递（通过 BaseRepository 的 `getDb(ctx)` 自动支持）。

---

## P1: 任务创建（父任务 + 子任务）添加事务

### 问题

`task/service.go:629` 中 `handleCreateTaskArray` 逐条创建任务，父任务和子任务之间无事务保护。

### 方案

将父任务创建 + 全部子任务创建包裹在事务中。

#### 修改文件

**1. `backend/task/service.go`**

- 引入 `Transactor` 接口依赖（同 `work.Service` 的模式）：

```go
type Transactor interface {
    ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
```

- 在 `Service` 结构体中添加 `transactor Transactor` 字段
- 在 `NewService` 构造函数中接收 `transactor`
- 修改 `handleCreateTaskArray`，将每个父任务 + 其子任务的创建包裹在事务中：

```go
// 多个子任务：事务内创建父任务 + 子任务
err = s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
    if err := s.repo.CreateTask(txCtx, parentTask); err != nil {
        return err
    }
    parentId := parentTask.GetID()
    for _, childResp := range children {
        childTask := &entity.Task{...}
        assignTask(childTask, ..., parentId)
        if err := s.repo.CreateTask(txCtx, childTask); err != nil {
            return err
        }
    }
    return nil
})
```

- `handleCreateTaskStream` 中的批量创建也做类似处理

**2. `backend/task/repository.go`**

- `TaskRepository` 的自定义方法（`CreateTask` 等）目前通过 `r.GORM()` 而非 `DBFromContext` 操作。需要添加 `dbFromCtx` 辅助方法，让自定义 SQL 方法支持事务 context：

```go
func (r *TaskRepository) dbFromCtx(ctx context.Context) *gorm.DB {
    return database.DBFromContext(ctx, r.BaseRepository.GORM())
}
```

- 将 `CreateTask` 等方法中的 `r.GORM()` 替换为 `r.dbFromCtx(ctx)`

**3. `app.go`**

- 在 `task.NewService` 调用中增加 `transactor` 参数

#### 事务粒度选择

- 方案 A（推荐）：每个父任务 + 其子任务一个事务。粒度适中，失败时只影响当前父任务的子任务组
- 方案 B：所有任务创建一个大事务。粒度过大，单个子任务失败导致整批回滚

---

## 实施顺序

1. **P0-1**: `DeleteWorkAndSurroundingData` 事务化
2. **P0-2**: Resource Save + PendingResourceID 事务化
3. **P1**: 任务创建事务化
4. 每步完成后运行 `go test ./...` 确认无回归
5. 手动验证：创建任务 → 下载 → 删除作品的完整流程

## 清理

- `task.Service.SaveWorkInfo`（简化版，`task/service.go:511`）无调用方，确认后可删除
