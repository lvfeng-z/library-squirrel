# 任务替换流程：PersistentStore 备份还原重构

> 前置计划：`doc/plan/persistentstore-backup-integration.md`（PersistentStore 接入 Backup 模块）
>
> 本计划在前置计划的基础上，将 taskManager 的替换流程从"禁用/启用 Resource"改为"备份/还原 PersistentStore"，
> 并为 Resource 未来扩展更多 Store 字段打下基础。

## 现状分析

### 当前 taskManager 替换流程

```
任务检测到作品已存在 → 用户确认替换
  → BackupAndDisable(workId)       // 禁用 Resource（Enabled=false），不处理 PersistentStore 文件
  → 创建新 Resource → 下载新资源到新 PersistentStore
  → 失败时 Restore(resourceIds)    // 重新启用旧 Resource（Enabled=true）
```

**问题**：
1. 旧的 PersistentStore 文件未被备份，替换后旧文件丢失无法还原
2. 创建新 Resource 是不必要的——Resource 的业务字段不变，变的是底层文件

### 目标替换流程

**核心设计**：Resource 不随替换而禁用或新建，被替换的对象是 PersistentStore。Resource 沿用原有记录，仅更新 Store 字段。

```
任务检测到作品已存在 → 用户确认替换
  → BackupAllStores(workId)        // 备份 PersistentStore 文件 + 删除 PersistentStore 记录
    （Resource 保持 Enabled=true，Store 字段暂指向已删除的 store）
  → 下载新资源到新 PersistentStore → 更新 Resource.WorkStoreID 指向新 store
  → 失败时 RestoreAllStores(备份清单)：
      → 从备份移动文件回 store 目录 → 注册 PersistentStore DB 记录
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

## 核心设计：备份清单（Backup Manifest）

替换场景中，Resource 可能关联多个 Store（WorkStore、ThumbnailStore 等）。
备份时由编排器遍历 Resource 的所有 Store 字段，生成结构化清单返回给调用方；
还原时调用方直接传入清单，按 BackupID 定位备份——数据流是**直通的**，不依赖数据库间接查询。

### 数据结构

定义在 `backend/taskManager/model.go`（调用方定义接口和数据类型）：

```go
// StoreType 标识 Resource 上不同类型的 Store 字段
// 扩展方式：在 Resource 新增 Store 字段时，在此追加常量
type StoreType int

const (
    StoreTypeWork      StoreType = iota + 1 // WorkStoreID — 作品主资源
    StoreTypeThumbnail                       // ThumbnailStoreID — 封面/缩略图
)

// StoreBackupItem 单个 Store 的备份条目
type StoreBackupItem struct {
    ResourceID int64     // 所属 Resource ID
    BackupID   int64     // Backup 记录 ID（0 = 未备份，直接删除了）
    StoreType  StoreType // Store 字段类型
}
```

语义约定：
- `BackupID > 0`：该 Store 文件已移动到备份目录，可还原
- `BackupID == 0`：该 Store 被直接删除（当前缩略图不备份），不可还原

### 接口设计

```go
// StoreBackupOrchestrator 资源存储备份编排器接口
// 封装替换场景下作品 Resource 全部 PersistentStore 的一站式备份和还原
// 当前业务中一个 Work 恰好对应一条 Resource，接口以 workId 为入参
type StoreBackupOrchestrator interface {
    // BackupAllStores 备份作品 Resource 的全部 Store，返回备份清单
    // Resource 的每个 Store 字段（WorkStoreID、ThumbnailStoreID 等）都会产生一条 StoreBackupItem
    BackupAllStores(ctx context.Context, workId int64) []*StoreBackupItem
    // RestoreAllStores 从备份清单还原所有 Store 并更新对应 Resource
    // 仅还原 BackupID > 0 的条目；BackupID == 0 的条目跳过（对应 Resource 字段保持 null）
    RestoreAllStores(ctx context.Context, items []*StoreBackupItem)
}
```

## 实现设计

### 1. StoreBackupOrchestrator 实现

**`backend/backup/store_backup_orchestrator.go`**（新文件，替代原 `resource_orchestrator.go`）

#### 依赖接口

```go
// ResourceProvider 资源查询
type ResourceProvider interface {
    GetEnabledByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
    GetById(ctx context.Context, id int64) (*entity.Resource, error)
}

// StoreDeleter Store 删除（支持备份）
type StoreDeleter interface {
    Delete(ctx context.Context, id int64, backup bool) (int64, error)
}

// StoreRegistrar Store 注册（为已在 store 目录中的文件创建 DB 记录）
type StoreRegistrar interface {
    RegisterExistingStore(ctx context.Context, relPath string, fileName string) (int64, error)
}

// ResourceUpdater Resource 更新
type ResourceUpdater interface {
    Update(ctx context.Context, resource *entity.Resource) error
}

// BackupReader 备份查询与文件操作
type BackupReader interface {
    GetById(ctx context.Context, id int64) (*entity.Backup, error)
    GetBackupPath(backup *entity.Backup) string
    RestoreFile(ctx context.Context, backupPath string, targetPath string) error
    Delete(ctx context.Context, id int64) error
}
```

#### 构造函数

```go
func NewStoreBackupOrchestrator(
    resourceProvider ResourceProvider,
    storeDeleter     StoreDeleter,
    storeRegistrar   StoreRegistrar,
    resourceUpdater  ResourceUpdater,
    backupReader     BackupReader,
) *StoreBackupOrchestrator
```

#### BackupAllStores 实现

```
1. 查询作品的启用资源 GetEnabledByWorkId(workId)（当前业务下恰好一条）
2. 遍历该资源的 Store 字段（按 StoreType 枚举）：
   a. WorkStoreID.Valid →
      storeDeleter.Delete(ctx, storeId, backup=true) → backupId
      追加 StoreBackupItem{resourceId, backupId, StoreTypeWork}
   b. ThumbnailStoreID.Valid →
      storeDeleter.Delete(ctx, storeId, backup=false) → backupId=0
      追加 StoreBackupItem{resourceId, 0, StoreTypeThumbnail}
      （当前缩略图不备份，未来可改为 backup=true）
3. Resource 记录不变（不禁用，保持 Enabled=true）
4. 返回完整清单
```

#### RestoreAllStores 实现

```
1. 按 BackupID > 0 过滤清单，仅处理有备份的条目
2. 对每个有备份的条目：
   a. backupReader.GetById(backupId) → 获取 Backup 记录
   b. 从 Backup.OriginalFilePath / OriginalFileName / OriginalFilenameExtension
      还原原始路径信息
   c. 计算还原目标绝对路径：{workDir}/store/{OriginalFilePath}
   d. backupReader.RestoreFile(backupAbsPath, targetAbsPath)
      → 移动备份文件到 store 目录
   e. storeRegistrar.RegisterExistingStore(relPath, fileName)
      → 创建 PersistentStore DB 记录，得到新 storeId
   f. resourceProvider.GetById(resourceId) → 获取 Resource
   g. 根据 StoreType 更新 Resource 对应字段：
      - StoreTypeWork      → resource.WorkStoreID = newStoreId
      - StoreTypeThumbnail → resource.ThumbnailStoreID = newStoreId
   h. resourceUpdater.Update(ctx, resource)
   i. backupReader.Delete(ctx, backupId) → 清理备份记录
3. BackupID == 0 的条目（如当前缩略图）：无需还原
   → Resource 的对应字段在步骤 5 中会被新值覆盖（或保持 null）
```

**关键设计**：步骤 2e 使用 `RegisterExistingStore` 而非 `StoreFromFile`。
文件已由 RestoreFile 移动到 store 目录中的正确位置，只需创建 DB 记录指向它。
`StoreFromFile` 会以 `os.Open` 打开同一文件、再由 `os.Create` 截断，导致内容丢失。

### 2. PersistentStore.Service 新增方法

`backend/persistentStore/service.go` 新增：

```go
// RegisterExistingStore 为 store 目录中已存在的文件创建 PersistentStore DB 记录
// 不涉及文件操作（文件已在正确位置），仅注册数据库记录
// relPath: 相对于 {workDir}/store/ 的路径
// fileName: 原始文件名
func (s *Service) RegisterExistingStore(ctx context.Context, relPath string, fileName string) (int64, error)
```

实现逻辑：
1. `validatePath(relPath)` 校验路径合法性
2. 确认文件存在：`os.Stat(absPath)`
3. 从 fileName 提取扩展名
4. 创建 PersistentStore 记录（Status = Complete）
5. 返回记录 ID

### 3. taskManager/model.go 变更

#### 接口与数据类型

新增 `StoreType`、`StoreBackupItem`（见上方"数据结构"节）。

当前接口替换：
```go
// 旧
type ResourceBackupOrchestrator interface {
    BackupAndDisable(ctx context.Context, workId int64) []int64
    Restore(ctx context.Context, resourceIds []int64)
}

// 新
type StoreBackupOrchestrator interface {
    BackupAllStores(ctx context.Context, workId int64) []*StoreBackupItem
    RestoreAllStores(ctx context.Context, items []*StoreBackupItem)
}
```

#### 新增 ResourceUpdater 接口

替换场景的步骤 5 中需要更新已有 Resource（而非创建新 Resource），
因此 taskManager 需要新增此接口：

```go
// ResourceUpdater Resource 更新接口（用于替换场景更新已有 Resource 的 Store 字段）
type ResourceUpdater interface {
    Update(ctx context.Context, resource *entity.Resource) error
}
```

#### ManagedTask 字段变更

```go
// 旧
backupOrchestrator     ResourceBackupOrchestrator   // 资源备份编排器
backedUpResourceIds []int64                         // 已备份的资源 ID 列表（用于任务失败时还原）

// 新
storeBackupOrchestrator  StoreBackupOrchestrator    // 资源存储备份编排器
storeBackupItems       []*StoreBackupItem           // 备份清单（用于任务失败时还原）
```

`NewManagedTask` 构造函数参数和赋值同步更新。

#### run() 流程变更

**步骤 0.1**（替换确认后的备份）：

```go
// 旧：
m.backedUpResourceIds = m.backupOrchestrator.BackupAndDisable(m.ctx, m.existingWorkId)

// 新：
m.storeBackupItems = m.storeBackupOrchestrator.BackupAllStores(m.ctx, m.existingWorkId)
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

ThumbnailStoreID 在替换后由插件决定是否重新下载。

**defer 失败还原**：

```go
// 旧：
if m.backedUpResourceIds != nil {
    m.backupOrchestrator.Restore(m.ctx, m.backedUpResourceIds)
}

// 新：
if m.storeBackupItems != nil {
    m.storeBackupOrchestrator.RestoreAllStores(m.ctx, m.storeBackupItems)
}
```

### 4. app.go 依赖注入

```go
storeBackupOrchestrator := backup.NewStoreBackupOrchestrator(
    app.ResourceService,         // ResourceProvider + ResourceUpdater
    app.PersistentStoreService,  // StoreDeleter
    app.PersistentStoreService,  // StoreRegistrar
    app.ResourceService,         // ResourceUpdater
    app.BackupService,           // BackupReader
)
```

初始化顺序：
1. `BackupService`
2. `PersistentStoreService`（注入 BackupService 作为 FileMover）
3. `StoreBackupOrchestrator`（注入各 Service）
4. `TaskManager`（注入 StoreBackupOrchestrator）

`NewManagedTask` 传参中 `backupOrchestrator` 替换为 `storeBackupOrchestrator`。

## 可扩展性分析

### 新增 Store 字段（如 SubtitleStoreID）

1. 在 `StoreType` 枚举追加 `StoreTypeSubtitle`
2. 在 `BackupAllStores` 的遍历逻辑中添加新字段处理
3. 在 `RestoreAllStores` 的 switch 中添加新字段 → Resource 字段映射
4. 无需修改接口签名，无破坏性变更

### 新增 Resource 与 Work 的对应关系

当前业务中 Work 与 Resource 是 1:1 关系，`BackupAllStores` 以 workId 为入参，
内部通过 `GetEnabledByWorkId` 获取资源（恰好一条）。
若未来 Work 需要关联多条 Resource（如视频+缩略图各一条），
`GetEnabledByWorkId` 已返回切片，BackupAllStores 遍历逻辑无需修改即可天然支持多条资源。

## 调整文件清单

| 文件 | 变更 |
|------|------|
| `backend/backup/resource_orchestrator.go` | 删除（被新文件替代） |
| `backend/backup/store_backup_orchestrator.go` | **新增**：StoreBackupOrchestrator 实现 |
| `backend/taskManager/model.go` | 新增 StoreType/StoreBackupItem、接口替换为 StoreBackupOrchestrator、新增 ResourceUpdater、ManagedTask 字段变更、run() 步骤 5 改为更新而非创建 |
| `backend/persistentStore/service.go` | 新增 `RegisterExistingStore` 方法 |
| `app.go` | NewStoreBackupOrchestrator 注入 + NewManagedTask 传参更新 |

## 验证

- 替换场景：旧 WorkStore 文件移动到备份目录，新文件下载后 Resource.WorkStoreID 指向新 store
- 替换失败场景：备份文件移回 store 目录，新 PersistentStore 记录创建，Resource.WorkStoreID 指向还原的 store
- Resource 始终保持 Enabled=true
- 缩略图 Store 被直接删除（不备份），Resource.ThumbnailStoreID 由后续步骤处理
- `CGO_ENABLED=0 go vet ./backend/...` 后端编译通过
