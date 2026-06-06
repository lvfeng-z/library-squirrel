# StoreBackupOrchestrator 重构：一站式备份与还原

> 本计划是对 `doc/plan/taskmanager-store-backup-integration.md` 的修正方案，
> 解决其严重问题 1（查询不匹配）和严重问题 2（StoreFromFile 截断），
> 并为 Resource 未来扩展更多 Store 字段（如 SubtitleStore）打下基础。

## 问题回顾

原计划的 `RestoreStores` 通过 `GetResourceBackups(resourceIds)` 查询备份记录。
但实际备份由 `PersistentStore.Delete → MoveToBackup` 创建，记录的 `SourceType=3 (PersistentStore)`、`SourceID=storeId`，
与查询条件 `SourceType=2 (Resource)`、`SourceID=resourceId` **完全不匹配**，导致还原永远返回空。

## 设计目标

1. **彻底消除查询不匹配**：不依赖数据库间接查询，备份方直接返回结构化清单
2. **一站式覆盖**：一次调用备份 Resource 关联的所有 Store（WorkStore、ThumbnailStore 及未来新增字段）
3. **可扩展的 Store 字段**：Resource 未来会扩展更多 Store 字段（如 SubtitleStore），StoreType 枚举可增量添加
4. **顺带解决 StoreFromFile 截断问题**（严重问题 2）：还原时文件已在 store 目录，应直接注册 DB 记录

## 核心设计：备份清单（Backup Manifest）

不再通过数据库查询关联备份。`BackupAllStores` 返回结构化清单，调用方持有清单，
还原时直接传入——数据流是**直通的**，不存在间接查询。

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

对比原计划的接口变更：

| 维度 | 原计划 | 本方案 |
|------|--------|--------|
| 返回值 | `[]int64`（resource IDs） | `[]*StoreBackupItem`（结构化清单） |
| 还原入参 | `resourceIds`（需间接查询数据库） | `[]*StoreBackupItem`（直接持有备份 ID） |
| 覆盖范围 | 仅 WorkStore | WorkStore + ThumbnailStore + 可扩展 |
| StoreType | 无 | 枚举，标识 Resource 的哪个字段 |

## 实现设计

### 实现文件

`backend/backup/store_backup_orchestrator.go`（新文件，替代原 `resource_orchestrator.go`）

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

与原计划对比：
- **删除** `StoreCreator`（`StoreFromFile`）→ 替换为 `StoreRegistrar`（`RegisterExistingStore`），解决截断问题
- **删除** `BackupRestorer.GetResourceBackups` → 替换为 `BackupReader.GetById`（按 backup ID 直接查）
- 新增 `StoreType` 使还原时知道更新 Resource 的哪个字段

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

**关键改进**：步骤 2e 使用 `RegisterExistingStore` 而非 `StoreFromFile`。
文件已由 RestoreFile 移动到 store 目录中的正确位置，只需创建 DB 记录指向它。
`StoreFromFile` 会以 `os.Open` 打开同一文件、再由 `os.Create` 截断，导致内容丢失。

### PersistentStore.Service 新增方法

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

### ManagedTask 变更

`backend/taskManager/model.go`：

```go
// 旧
backupOrchestrator     ResourceBackupOrchestrator
backedUpResourceIds []int64

// 新
storeBackupOrchestrator  StoreBackupOrchestrator
storeBackupItems       []*StoreBackupItem
```

### taskManager/model.go run() 流程变更

**步骤 0.1**（替换确认后的备份）：
```go
m.storeBackupItems = m.storeBackupOrchestrator.BackupAllStores(m.ctx, m.existingWorkId)
```

**defer 失败还原**：
```go
if m.storeBackupItems != nil {
    m.storeBackupOrchestrator.RestoreAllStores(m.ctx, m.storeBackupItems)
}
```

**步骤 5**（保存 Resource）不变：仍为查询已有 Resource 并更新 WorkStoreID。
ThumbnailStoreID 在替换后由插件决定是否重新下载（与原计划一致）。

### app.go 依赖注入

```go
storeBackupOrchestrator := backup.NewStoreBackupOrchestrator(
    app.ResourceService,         // ResourceProvider + ResourceUpdater
    app.PersistentStoreService,  // StoreDeleter
    app.PersistentStoreService,  // StoreRegistrar
    app.ResourceService,         // ResourceUpdater
    app.BackupService,           // BackupReader
)
```

初始化顺序不变：
1. `BackupService`
2. `PersistentStoreService`（注入 BackupService 作为 FileMover）
3. `StoreBackupOrchestrator`（注入各 Service）
4. `TaskManager`（注入 StoreBackupOrchestrator）

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

## 原计划中已解决的问题

| 原问题 | 解决方式 |
|--------|----------|
| 严重问题 1：查询不匹配 | 不再查询，由清单直通 BackupID |
| 严重问题 2：StoreFromFile 截断 | 改用 RegisterExistingStore |
| 中等问题 3：backupStoreMap 生命周期 | 删除局部 map，清单作为返回值传递 |
| 中等问题 4：ThumbnailStoreID 悬空 | 清单中 BackupID=0 标识不可还原，显式跳过 |

## 调整文件清单

| 文件 | 变更 |
|------|------|
| `backend/backup/resource_orchestrator.go` | 删除（被新文件替代） |
| `backend/backup/store_backup_orchestrator.go` | **新增**：StoreBackupOrchestrator 实现 |
| `backend/taskManager/model.go` | 新增 StoreType/StoreBackupItem、接口重命名、ManagedTask 字段变更 |
| `backend/persistentStore/service.go` | 新增 `RegisterExistingStore` 方法 |
| `app.go` | NewStoreBackupOrchestrator 注入 + NewManagedTask 传参更新 |

## 验证

- 替换场景：旧 WorkStore 文件移动到备份目录，新文件下载后 Resource.WorkStoreID 指向新 store
- 替换失败场景：备份文件移回 store 目录，新 PersistentStore 记录创建，Resource.WorkStoreID 指向还原的 store
- Resource 始终保持 Enabled=true
- 缩略图 Store 被直接删除（不备份），Resource.ThumbnailStoreID 由后续步骤处理
- `CGO_ENABLED=0 go vet ./backend/...` 后端编译通过
