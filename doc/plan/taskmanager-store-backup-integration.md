# 任务替换流程接入 PersistentStore 备份还原

> 前置计划：`doc/plan/persistentstore-backup-integration.md`（PersistentStore 接入 Backup 模块）
>
> 本计划在前置计划的基础上，将 taskManager 的替换流程从"禁用/启用 Resource"改为"移动备份/从备份还原 PersistentStore"

## 现状分析

### 当前 taskManager 替换流程

```
任务检测到作品已存在 → 用户确认替换
  → BackupAndDisable(workId)       // 禁用 Resource（Enabled=false），不处理 PersistentStore 文件
  → 创建新 Resource → 下载新资源到新 PersistentStore
  → 失败时 Restore(resourceIds)    // 重新启用旧 Resource（Enabled=true）
```

**问题**：旧的 PersistentStore 文件未被备份，替换后旧文件丢失无法还原。且创建新 Resource 是不必要的——Resource 的业务字段不变，变的是底层文件。

### 目标替换流程

**核心设计**：Resource 不随替换而禁用或新建，被替换的对象是 PersistentStore。Resource 沿用原有记录，仅更新 WorkStoreID。

```
任务检测到作品已存在 → 用户确认替换
  → 备份旧 PersistentStore 文件（移动备份）+ 删除旧 PersistentStore DB 记录
    （Resource 保持 Enabled=true，WorkStoreID 暂指向已删除的 store）
  → 下载新资源到新 PersistentStore → 更新 Resource.WorkStoreID 指向新 store
  → 失败时从备份还原：
      → 删除新 PersistentStore（如有）
      → 从备份移动文件回 store 目录 → 创建新 PersistentStore DB 记录
      → 更新 Resource.WorkStoreID 指向还原的 store
```

对比当前流程的变更：

| 环节 | 当前 | 目标 |
|------|------|------|
| 旧资源处理 | 禁用 Resource | 备份 PersistentStore 文件 + 删除 PersistentStore 记录 |
| 新资源创建 | 创建新 Resource | 沿用旧 Resource，仅更新 WorkStoreID |
| 失败还原 | 重新启用旧 Resource | 从备份还原 PersistentStore + 更新 WorkStoreID |
| Resource 状态 | Enabled 变为 false → true | 始终保持 Enabled=true |

## 前置计划实施内容

前置计划 `persistentstore-backup-integration.md` 需要完成的变更：

1. 移除 `persistentStore/handler.go`
2. PersistentStore.Service 新增 `FileMover` 依赖，`Delete(ctx, id, backup bool) (backupId, error)`
3. backup.Service 新增 `MoveToBackup` 方法
4. work.Service `StoreDeleter` 接口签名更新
5. app.go 依赖注入调整

以下所有内容均**假设前置计划已完成**。

## 本计划需要的变更

### 1. ResourceBackupOrchestrator 接口重新设计

**`backend/taskManager/model.go`**

当前接口：
```go
type ResourceBackupOrchestrator interface {
    BackupAndDisable(ctx context.Context, workId int64) []int64
    Restore(ctx context.Context, resourceIds []int64)
}
```

新接口 — 反映"备份 PersistentStore 并准备替换"的语义：
```go
// StoreBackupOrchestrator PersistentStore 备份编排器接口
// 封装替换场景下旧 PersistentStore 文件的备份和还原
type StoreBackupOrchestrator interface {
    // BackupStores 备份作品的 PersistentStore 文件并删除记录，返回受影响的资源 ID 列表
    // Resource 保持不变（不禁用），仅备份并删除关联的 PersistentStore
    BackupStores(ctx context.Context, workId int64) []int64
    // RestoreStores 从备份还原 PersistentStore 文件并更新 Resource.WorkStoreID
    RestoreStores(ctx context.Context, resourceIds []int64)
}
```

ManagedTask 中对应字段和引用同步重命名：
- `backupOrchestrator ResourceBackupOrchestrator` → `storeBackupOrchestrator StoreBackupOrchestrator`
- `backedUpResourceIds` → 含义不变（记录哪些 Resource 的 PersistentStore 已被备份）

### 2. ResourceBackupOrchestrator 实现重写

**`backend/backup/resource_orchestrator.go`**

重命名为 `store_backup_orchestrator.go`，完全重写。

#### 需要定义的接口

```go
// StoreDeleter PersistentStore 删除接口（支持备份）
type StoreDeleter interface {
    Delete(ctx context.Context, id int64, backup bool) (int64, error)
}

// StoreCreator PersistentStore 创建接口（从文件注册）
type StoreCreator interface {
    StoreFromFile(ctx context.Context, relPath string, fileName string, srcAbsPath string) (int64, error)
}

// ResourceUpdater Resource 更新接口
type ResourceUpdater interface {
    Update(ctx context.Context, resource *entity.Resource) error
}

// BackupRestorer 备份还原接口
type BackupRestorer interface {
    GetResourceBackups(ctx context.Context, resourceIds []int64) ([]*entity.Backup, error)
    GetBackupPath(backup *entity.Backup) string
    RestoreFile(ctx context.Context, backupPath string, targetPath string) error
    Delete(ctx context.Context, id int64) error
}
```

#### 构造函数

```go
func NewStoreBackupOrchestrator(
    resourceProvider ResourceProvider,   // GetEnabledByWorkId + GetById
    storeDeleter     StoreDeleter,       // Delete(ctx, id, backup)
    storeCreator     StoreCreator,       // StoreFromFile
    resourceUpdater  ResourceUpdater,    // Update(ctx, resource)
    backupRestorer   BackupRestorer,     // GetResourceBackups + RestoreFile + Delete
) *StoreBackupOrchestrator
```

#### BackupStores 实现

```
1. 查询作品所有启用资源 GetEnabledByWorkId(workId)
2. 对每个资源：
   a. WorkStoreID.Valid → storeDeleter.Delete(ctx, storeId, true) → 得到 backupId
      → 记录 backupStoreMap[resourceId] = backupId（内部维护，用于后续还原）
   b. ThumbnailStoreID.Valid → 同上（不需要还原缩略图，直接删除 backup=false）
   c. Resource 记录不变（不禁用）
3. 返回受影响的资源 ID 列表
```

#### RestoreStores 实现

```
1. 根据 resourceIds 查询备份记录 GetResourceBackups(resourceIds)
2. 对每个有备份记录的资源：
   a. 从 Backup.OriginalFilePath/OriginalFileName 恢复原始 store 路径
   b. 计算还原目标绝对路径：{workDir}/store/{OriginalFilePath}
   c. backupRestorer.RestoreFile(backupAbsPath, targetAbsPath) → 移动备份文件回 store
   d. storeCreator.StoreFromFile(ctx, relPath, fileName, targetAbsPath) → 注册新 PersistentStore → 得到新 storeId
   e. resource.WorkStoreID = 新 storeId
   f. resourceUpdater.Update(ctx, resource) → 更新 Resource
   g. backupRestorer.Delete(ctx, backupId) → 清理备份记录
3. 对无备份记录的资源（如仅有 ThumbnailStoreID）：无需处理
```

### 3. taskManager/model.go run() 流程调整

**步骤 0.1**（替换确认后的备份）：

```go
// 旧：
m.backedUpResourceIds = m.backupOrchestrator.BackupAndDisable(m.ctx, m.existingWorkId)

// 新：
m.backedUpResourceIds = m.storeBackupOrchestrator.BackupStores(m.ctx, m.existingWorkId)
```

**步骤 5**（保存 Resource）：

当前逻辑是创建新 Resource：
```go
resource := entity.NewResource()
resource.WorkID = workId
resource.TaskID = m.task.GetID()
resource.Enabled = true
resource.WorkStoreID = sql.NullInt64{Int64: storeId, Valid: true}
resourceId, err := m.resourceSaver.Save(m.ctx, resource)
```

改为查询已有 Resource 并更新 WorkStoreID：
```go
// 查询该作品已有的启用资源（替换场景下应只有一条）
resources, err := m.resourceReader.GetEnabledByWorkId(m.ctx, workId)
if err != nil || len(resources) == 0 { ... }
existingResource := resources[0]
existingResource.WorkStoreID = sql.NullInt64{Int64: storeId, Valid: true}
existingResource.SuggestName = sql.NullString{String: startResp.Resource.SuggestName, Valid: ...}
existingResource.ResourceComplete = 0
err = m.resourceUpdater.Update(m.ctx, existingResource)
```

需要新增 `ResourceUpdater` 接口到 taskManager：
```go
type ResourceUpdater interface {
    Update(ctx context.Context, resource *entity.Resource) error
}
```

**defer 失败还原**：

```go
// 旧：
m.backupOrchestrator.Restore(m.ctx, m.backedUpResourceIds)

// 新：
m.storeBackupOrchestrator.RestoreStores(m.ctx, m.backedUpResourceIds)
```

### 4. app.go 依赖注入

```go
storeBackupOrchestrator := backup.NewStoreBackupOrchestrator(
    app.ResourceService,         // ResourceProvider
    app.PersistentStoreService,  // StoreDeleter
    app.PersistentStoreService,  // StoreCreator
    app.ResourceService,         // ResourceUpdater
    app.BackupService,           // BackupRestorer
)
```

初始化顺序：
1. `BackupService`
2. `PersistentStoreService`（注入 BackupService 作为 FileMover）
3. `StoreBackupOrchestrator`（注入 PersistentStoreService + BackupService + ResourceService）
4. `TaskManager`（注入 StoreBackupOrchestrator）

## 调整文件清单

| 文件 | 变更 |
|------|------|
| `backend/backup/resource_orchestrator.go` | 重命名为 `store_backup_orchestrator.go`，完全重写 BackupStores/RestoreStores |
| `backend/taskManager/model.go` | 接口重命名 `StoreBackupOrchestrator`、新增 `ResourceUpdater`、run() 步骤 5 改为更新而非创建 |
| `app.go` | `NewStoreBackupOrchestrator` 注入 + `NewManagedTask` 传参更新 |

## 验证

- 替换场景：确认旧 PersistentStore 文件被移动到备份目录，新文件下载后 Resource.WorkStoreID 指向新 store
- 替换失败场景：确认备份文件被移回 store 目录，新 PersistentStore 记录创建，Resource.WorkStoreID 指向还原的 store
- Resource 始终保持 Enabled=true
- `CGO_ENABLED=0 go vet ./backend/...` 后端编译
