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
  3. 删除 reWorkAuthor 关联（当前遗漏，需补充）
  4. 删除 Resource
  5. 删除 Work
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

---

## P0-2: StoreStream(DB 记录) + Resource Save + PendingResourceID 更新添加事务

### 问题

`taskManager/model.go` 中步骤 4-6 逐步执行，无事务保护：
- 步骤 4：StoreStream 创建文件 + PersistentStore DB 记录
- 步骤 5：保存 Resource（关联 workId → storeId）
- 步骤 6：更新 Task.pending_resource_id

如果步骤 5 或 6 失败，会产生孤儿 DB 记录（PersistentStore、Resource）。

### 方案

将 **StoreStream 的 DB 记录创建 + Resource Save + PendingResourceID 更新** 合并到一个事务中。文件操作保持在事务外，事务失败时显式清理文件。

#### 关键约束

`storeWriter.Abort()` 内部通过 DB 查询获取文件路径再删除文件（`storeWriter.Abort()` → `repo.GetById()` → `os.Remove()`）。如果事务回滚，DB 记录不存在，Abort() 会静默跳过文件删除，导致孤儿文件。

因此需要：
1. 文件操作在事务外（创建文件）
2. DB 操作在事务内（PersistentStore 记录 + Resource + PendingResourceID）
3. 事务失败时通过 `writer.Close()` 关闭文件句柄 + 显式删除文件（不依赖 Abort）

#### 修改文件

**1. `backend/persistentStore/service.go`**

新增 `CleanupFile` 公共方法，供事务失败时显式清理磁盘文件：

```go
// CleanupFile 清理指定相对路径的磁盘文件（用于事务回滚后的文件清理）
func (s *Service) CleanupFile(relPath string) {
    absPath := filepath.Join(s.getWorkDir(), relPath)
    if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
        logger.Log.Warn("清理文件失败", zap.String("path", absPath), zap.Error(err))
    }
}
```

**2. `backend/taskManager/model.go`**

新增接口和字段：

```go
// Transactor 事务执行器接口
type Transactor interface {
    ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// PendingResourceUpdater 任务 pending_resource_id 更新接口
type PendingResourceUpdater interface {
    UpdatePendingResourceID(ctx context.Context, taskId int64, resourceID sql.NullInt64) error
}

// StoreFileCleaner 事务失败时清理磁盘文件
type StoreFileCleaner interface {
    CleanupFile(relPath string)
}
```

- `ManagedTask` 新增字段：`transactor Transactor`、`pendingResourceUpdater PendingResourceUpdater`
- `StoreStreamer` 接口新增方法：无需修改（StoreStream 本身已支持事务 context，因为底层 repository 通过 `DBFromContext` 自动获取事务 DB）
- `NewManagedTask` 构造函数新增参数
- 修改 `run()` 方法，将步骤 4-6 重构为事务内操作：

```go
// 步骤 4: 解析文件保存路径
_, relativePath, fileName := m.resolveLocalPath(startResp)

// 步骤 4-6: 事务 2（DB 操作原子化，文件操作在事务外）
var writer persistentStore.StoreWriter
var storeId int64
var resourceId int64

err = m.transactor.ExecInTransaction(m.ctx, func(txCtx context.Context) error {
    // 4. StoreStream：创建文件（事务外行为） + 创建/更新 PersistentStore DB 记录（事务内）
    var txErr error
    storeId, writer, txErr = m.storeStreamer.StoreStream(txCtx, relativePath, fileName)
    if txErr != nil {
        return txErr
    }

    // 5. 保存/更新 Resource
    resourceId, txErr = m.saveResource(txCtx, workId, storeId, startResp)
    if txErr != nil {
        return txErr
    }

    // 6. 更新 pending_resource_id
    m.task.PendingResourceID = sql.NullInt64{Int64: resourceId, Valid: true}
    return m.pendingResourceUpdater.UpdatePendingResourceID(txCtx, m.taskId, m.task.PendingResourceID)
})
if err != nil {
    // 事务回滚：DB 记录已全部回滚，需显式清理文件
    if writer != nil {
        writer.Close()  // 仅关闭文件句柄，不使用 Abort()（DB 记录已不存在）
    }
    m.storeFileCleaner.CleanupFile(relativePath)  // 删除磁盘文件
    if m.abortedByPause() {
        return runResultPaused
    }
    m.setFailed(fmt.Sprintf("创建资源失败: %v", err))
    return runResultDone
}
```

- 新增 `saveResource` 私有方法：将当前步骤 5 中的替换/新建 Resource 逻辑抽取为独立方法，接收 `txCtx` 参数。底层 `ResourceSaver`/`ResourceUpdater` 通过 BaseRepository 的 `getDb(ctx)` 自动参与事务。
- 新增 `storeFileCleaner StoreFileCleaner` 字段，指向 `persistentStore.Service` 实例

**3. `backend/taskManager/manager.go`**

- 在构建 `ManagedTask` 时传入 `transactor`、`pendingResourceUpdater`、`storeFileCleaner` 参数

**4. `app.go`**

- 将 `dbTransactorAdapter` 传递给 `TaskManager` 构建链
- 将 `task.Repository` 的 `UpdatePendingResourceID` 方法包装为 `PendingResourceUpdater` 接口
- 将 `persistentStore.Service` 作为 `StoreFileCleaner` 传入

#### StoreStream 事务兼容性分析

`StoreStream` 底层的 repository 方法：
- `s.repo.GetByFilePath(ctx, relPath)` — 通过 BaseRepository，自动参与事务 ✅
- `s.repo.Update(ctx, existing)` — 通过 BaseRepository，自动参与事务 ✅
- `s.repo.Save(ctx, store)` — 通过 BaseRepository，自动参与事务 ✅

传入 `txCtx` 后，所有 DB 操作自动在事务内执行，无需修改 `StoreStream` 本身的代码。

#### 事务失败时的文件清理保障

| 场景 | DB 状态 | 文件状态 | 清理机制 |
|------|---------|---------|---------|
| StoreStream 文件创建失败 | 无记录 | 无文件 | 无需清理 |
| StoreStream DB 失败 | 事务回滚 | 文件存在，StoreStream 内部已清理 | StoreStream 自身 `file.Close() + os.Remove()` |
| Resource Save 失败 | 事务回滚 | 文件存在 | `writer.Close()` + `CleanupFile()` |
| PendingResourceID 失败 | 事务回滚 | 文件存在 | `writer.Close()` + `CleanupFile()` |
| 进程崩溃 | 取决于时机 | 取决于时机 | 孤儿文件可通过定期扫描清理（DB 层面无孤儿记录） |

#### `resumeFromPersistedState` 路径分析

`resumeFromPersistedState` 中调用 `StoreStream`（不可续传场景）后直接进入 `downloadLoop`，不涉及 Resource 创建或 PendingResourceID 更新（这些在首次 `run()` 中已完成）。因此无需事务保护。

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
2. **P0-2**: StoreStream(DB 记录) + Resource Save + PendingResourceID 事务化
3. **P1**: 任务创建事务化
4. 每步完成后运行 `go test ./...` 确认无回归
5. 手动验证：创建任务 → 下载 → 删除作品的完整流程

## 清理

- `task.Service.SaveWorkInfo`（简化版，`task/service.go:511`）无调用方，确认后可删除
