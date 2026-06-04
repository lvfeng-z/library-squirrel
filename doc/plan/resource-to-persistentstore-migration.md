# Resource 接入 PersistentStore 重构计划

## 现状分析

### Resource 模块当前职责

Resource 实体承载两类职责：
1. **业务字段**：`work_id`、`task_id`、`enabled`、`suggest_name`、`resource_complete` — 描述资源与作品/任务的业务关系
2. **文件元数据**：`file_path`、`file_name`、`filename_extension`、`resource_size`、`workdir` — 描述磁盘文件信息

### Resource 被使用的方式

| 调用方 | 使用的接口方法 | 用途 |
|--------|--------------|------|
| `work.Service` | `ListByWorkId`、`ListByWorkIds`、`DeleteByWorkId` | 作品详情展示资源、级联删除 |
| `search.Service` | `ListByWorkIds` | 搜索结果中展示封面 |
| `task.Service` | `Save` | 任务保存作品时创建资源记录 |
| `taskManager.Manager` | `Save`、`Update`、`GetById`、`ListByWorkId`、`GetEnabledByWorkId` | 下载时创建/更新记录、恢复时读取 |
| `backup.ResourceBackupOrchestrator` | `GetEnabledByWorkId`、`GetById`、`Update` | 备份/恢复资源文件 |
| `resource.Handler`（前端） | `Save`、`Delete`、`Update`、`GetById`、`ListByWorkId`、`DeleteByWorkId` | 前端 CRUD |

### 文件操作分散在多处

Resource 模块本身（service/repository）**不操作磁盘文件**。文件 I/O 分散在：
- `taskManager/model.go`：下载时写入文件、恢复下载时检查文件大小
- `backup/resource_orchestrator.go`：备份时移动文件、恢复时移回文件
- `assetserver/resource_handler.go`：HTTP 请求时读取文件
- `work/service.go`：删除作品时**仅删 DB 记录，不删磁盘文件**

## 重构目标

将 Resource 的文件元数据和文件操作职责委托给 PersistentStore，使 Resource 实体仅保留业务字段。

### 目标实体

```go
// Resource（重构后）
type Resource struct {
    *model.BaseEntity
    WorkID            int64          `gorm:"column:work_id;index:idx_resource_work_id" json:"workId"`
    TaskID            int64          `gorm:"column:task_id;index:idx_resource_task_id" json:"taskId"`
    Enabled           bool           `gorm:"column:enabled" json:"enabled"`
    SuggestName       sql.NullString `gorm:"column:suggest_name" json:"suggestName"`
    ResourceComplete  int            `gorm:"column:resource_complete" json:"resourceComplete"`
    StoreID           sql.NullInt64  `gorm:"column:store_id;index" json:"storeId"`          // 新增：指向 persistent_store.id
}
```

`file_path`、`file_name`、`filename_extension`、`resource_size`、`workdir` 五个字段逐步废弃。

## 阶段策略

采用**渐进式接入**，分三个阶段，每个阶段独立可发布：

---

## 阶段一：新增 `store_id` 字段（双写模式）

**目标**：Resource 新增 `store_id` 字段，新下载的资源同时写入 PersistentStore，旧数据不受影响。

### 1.1 Entity 变更

`backend/base/model/entity/resource.go`：
- 新增 `StoreID sql.NullInt64` 字段，`gorm:"column:store_id;index"`
- 保留所有旧文件元数据字段（不删除）

### 1.2 DTO 变更

- SDK `ResourceDTO` 新增 `StoreID *int64` 字段
- `NewResourceDTO()` / `ToResourceEntity()` 同步更新

### 1.3 Resource Service 新增方法

```go
// GetByStoreId 根据 store_id 获取资源
GetByStoreId(ctx context.Context, storeId int64) (*entity.Resource, error)
```

### 1.4 taskManager 修改（核心）

`backend/taskManager/model.go`：

**当前流程**：
1. `resolveLocalPath()` 计算路径 → `{workDir}/resource/{author}/{filename}`
2. 写文件到该路径
3. 创建 `Resource` 实体（含 file_path 等）
4. `resourceSaver.Save()` 保存

**新流程**：
1. `resolveLocalPath()` 计算路径 → `resource/{author}/{filename}`（相对于 store 根目录）
2. 文件写入 `{workDir}/store/resource/{author}/{filename}`（通过 PersistentStore.Service.Store）
3. 创建 `Resource` 实体，设置 `StoreID` 指向返回的 persistent_store.id
4. 同时保留旧字段写入（双写），兼容旧读取路径
5. `resourceSaver.Save()` 保存

**taskManager 需要的依赖注入**：
- 新增对 `persistentStore.Service` 的依赖
- 通过接口注入，定义在 `taskManager` 包内

### 1.5 ResourceHandler（HTTP）兼容

`backend/assetserver/resource_handler.go` 保持不变。

当 `Resource.StoreID` 有效时，前端可通过 `buildStoreUrl()` 访问文件；当 `StoreID` 为空时，仍走旧的 `/resource/` 路由。前端可优先使用 `StoreID`，回退到旧路径。

### 1.6 前端 DTO 变更

`WorkFullDTO` 中的 `Resources` 自动包含 `storeId` 字段。前端显示资源时：
- 如果 `storeId` 存在 → 使用 `buildStoreUrl(store.FilePath)` 构建 URL
- 否则 → 使用 `buildResourceUrl(resource.filePath)` 构建 URL

### 1.7 恢复下载兼容

`taskManager/model.go` 恢复下载时：
- 如果 `Resource.StoreID` 有效 → 从 PersistentStore 获取文件路径
- 否则 → 使用旧的 `resource.FilePath`（向后兼容）

### 修改文件清单

| 文件 | 变更 |
|------|------|
| `backend/base/model/entity/resource.go` | 新增 `StoreID` 字段 |
| SDK `dto/resource_dto.go` | 新增 `StoreID` 字段 |
| `backend/base/model/dto/resource_dto.go` | 转换函数同步更新 |
| `backend/persistentStore/service.go` | 新增 `StoreFromReader(ctx, relPath, fileName, reader) (int64, error)`（支持 io.Reader + 返回写入字节数） |
| `backend/taskManager/model.go` | 下载流程改用 PersistentStore 写入，双写旧字段 |
| `backend/taskManager/interfaces.go` | 新增 PersistentStore 接口定义 |
| `app.go` | taskManager 初始化时注入 PersistentStoreService |
| 前端资源展示组件 | 优先使用 storeId 构建 URL |

---

## 阶段二：文件操作统一委托 PersistentStore

**目标**：所有 Resource 相关的文件操作（备份、恢复、删除）通过 PersistentStore 执行。

### 2.1 backup 模块改造

`backend/backup/resource_orchestrator.go`：
- 备份：调用 `PersistentStore.Delete(storeId)` 删除文件和记录，由 backup 模块自行管理备份文件的复制
- 恢复：调用 `PersistentStore.Store(relPath, fileName, reader)` 重新存入文件，更新 `Resource.StoreID`

### 2.2 work 级联删除改造

`backend/work/service.go`：
- 删除作品时，查询关联 Resource 的 `StoreID`
- 批量调用 `PersistentStore.Delete(storeId)` 清理文件
- 再删除 Resource DB 记录

### 2.3 taskManager 恢复下载改造

- 恢复时完全依赖 `StoreID` 获取文件信息，不再读取旧文件字段

---

## 阶段三：废弃旧字段

**目标**：移除 Resource 实体中的旧文件元数据字段。

### 3.1 前置条件

- 阶段一、二已稳定运行
- 提供数据迁移工具：扫描所有 Resource，为缺少 `StoreID` 的记录在 PersistentStore 中注册文件，回填 `StoreID`

### 3.2 清理工作

- 移除 `Resource` 的 `file_path`、`file_name`、`filename_extension`、`resource_size`、`workdir` 字段
- 移除 `assetserver/resource_handler.go`（统一使用 `/store/` 路由）
- 移除前端 `buildResourceUrl()`，统一使用 `buildStoreUrl()`
- 更新所有 Resource DTO 转换

---

## 风险与注意事项

1. **双写期间数据一致性**：阶段一中 file_path 和 store_id 指向不同位置，需确保两条路径都可访问
2. **大文件处理**：PersistentStore.Store 使用 io.Reader，taskManager 的下载流本身就是 Reader，无需额外内存开销
3. **恢复下载**：阶段一中需要同时兼容两种路径模式，逻辑复杂度增加
4. **备份/恢复流程**：阶段二需要重新设计备份策略，当前是文件移动，改为通过 PersistentStore 删除+重存
5. **旧数据迁移**：阶段三需要提供迁移工具，扫描旧 Resource 文件注册到 PersistentStore

## 建议

**推荐从阶段一开始实施**，理由：
- 改动范围可控，核心变更集中在 taskManager 的下载流程
- 不影响现有功能和旧数据
- 为后续阶段奠定基础

阶段二、三可视实际需要决定是否继续推进。

---

## 新会话实现上下文

以下信息供新会话理解现有代码模式，无需重新探索。

### 关键代码位置

| 文件 | 说明 |
|------|------|
| `backend/taskManager/model.go` | **阶段一核心修改文件**，936 行。包含 ManagedTask、run()、downloadLoop()、resolveLocalPath()、resumeFromPersistedState() |
| `backend/taskManager/manager.go` | Manager 编排器，NewManager() 构造函数签名定义了所有依赖注入 |
| `backend/backup/resource_orchestrator.go` | **阶段二核心修改文件**，215 行。BackupAndDisable / Restore 流程 |
| `backend/work/service.go` | work.Service 的接口定义（ResourceReader / ResourceBatchReader / ResourceDeleter 等） |
| `app.go`（项目根目录） | 依赖注入组装，taskManager 初始化在 `initAdvancedServices()` |

### taskManager 接口定义（model.go:87-142）

taskManager 包内定义了以下接口，由外部服务实现后注入：

```go
// ResourceSaver（model.go:113）— 由 resourceSaverAdapter 实现
type ResourceSaver interface {
    Save(ctx context.Context, resource *entity.Resource) (int64, error)
    Update(ctx context.Context, resource *entity.Resource) error
}

// ResourceReader（model.go:126）— 由 ResourceService 直接实现
type ResourceReader interface {
    ListByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
    GetEnabledByWorkId(ctx context.Context, workId int64) ([]*entity.Resource, error)
    GetById(ctx context.Context, id int64) (*entity.Resource, error)
}

// ResourceBackupOrchestrator（model.go:137）— 由 backup.ResourceBackupOrchestrator 实现
type ResourceBackupOrchestrator interface {
    BackupAndDisable(ctx context.Context, workId int64, workDir string) []int64
    Restore(ctx context.Context, resourceIds []int64, workDir string)
}

// WorkDirProvider（manager.go:31）— 由 SettingsService 实现
type WorkDirProvider interface {
    GetWorkDir() string
}

// FileNameFormatProvider（manager.go:36）— 由 SettingsService 实现
type FileNameFormatProvider interface {
    GetFileNameFormat() string
}
```

### taskManager 依赖注入链

**NewManager 签名**（manager.go:96）：
```go
func NewManager(
    maxParallel int,
    workDirProvider WorkDirProvider,        // → SettingsService
    fileNameFormatProvider FileNameFormatProvider, // → SettingsService
    repo Repository,                         // → task.TaskRepository
    pusher TaskProgressPusher,               // → WailsTaskProgressPusher / NoopProgressPusher
    pluginExecFactory func(string) (TaskExecutor, error), // → extension.NewTaskExecutor
    workInfoSaver WorkInfoSaver,             // → WorkService
    resourceSaver ResourceSaver,             // → resourceSaverAdapter（包装 ResourceService）
    workChecker WorkChecker,                 // → WorkService
    resourceReader ResourceReader,           // → ResourceService
    backupOrchestrator ResourceBackupOrchestrator, // → backup.ResourceBackupOrchestrator
) *Manager
```

**app.go 组装**（`initAdvancedServices()`，约 815-833 行）：
```go
resourceSaverAdapter := &resourceSaverAdapter{svc: app.ResourceService}
resourceBackupOrchestrator := backup.NewResourceBackupOrchestrator(app.ResourceService, app.BackupService)

app.TaskManagerService = taskManager.NewManager(
    app.SettingsService.GetSettings().ImportSettings.MaxParallelImport,
    app.SettingsService,              // WorkDirProvider + FileNameFormatProvider
    app.SettingsService,
    app.taskRepo,
    taskManagerPusher,
    pluginExecFactory,
    app.WorkService,                   // WorkInfoSaver
    resourceSaverAdapter,              // ResourceSaver（包装 ResourceService）
    app.WorkService,                   // WorkChecker
    app.ResourceService,              // ResourceReader
    resourceBackupOrchestrator,        // ResourceBackupOrchestrator
)
```

**阶段一需新增**：在 NewManager 参数中添加 `persistentStore.Service`（通过新接口 `StoreWriter` 注入），并在 app.go 传入 `app.PersistentStoreService`。

### run() 核心流程（model.go:237-396）

```
run()
  ├─ 0. 检查 workdir 是否已配置
  ├─ 0.0 检查作品是否已存在（重复检测）
  ├─ 0.1 替换确认后备份已有资源文件
  │     └─ backupOrchestrator.BackupAndDisable(ctx, existingWorkId, workDir)
  ├─ 1. pluginExec.CreateWorkInfo(ctx, task) → WorkResponse
  ├─ 2. workInfoSaver.SaveWorkInfo(ctx, task, workResp) → workId
  ├─ 3. pluginExec.Start(ctx, task, workId) → (reader, StartResponse, error)
  ├─ 4. resolveLocalPath(startResp) → (absPath, relativePath, fileName)
  │     └─ 返回:
  │         absSavePath = filepath.Join(workDir, "resource", authorDir, fileName)
  │         relativePath = filepath.Join(authorDir, fileName)
  │         fileName = 文件名（含扩展名）
  ├─ 4.1 os.MkdirAll(filepath.Dir(localPath), 0755)
  ├─ 5. 创建 Resource 实体并保存 ★核心修改点★
  │     └─ 当前: 直接构建 entity.Resource{FilePath: relativePath, ...}
  │        调用 resourceSaver.Save(ctx, resource) → resourceId
  ├─ 6. 更新 task.PendingResourceID
  ├─ 7. os.Create(localPath) → file
  └─ downloadLoop()
```

**阶段一修改点（步骤 4-7）**：
- 步骤 4：`resolveLocalPath()` 的 `absSavePath` 改为指向 `{workDir}/store/resource/...`
- 步骤 5：先通过 PersistentStore.Store 写入文件获取 storeId，再创建 Resource 并设置 StoreID
- 步骤 7：**不再手动 os.Create**，由 PersistentStore.Store 内部处理文件创建和写入

**关键问题**：当前 downloadLoop 需要持有 `*os.File` 进行流式写入，但 PersistentStore.Store 接受 `io.Reader`。需要新增一个 Store 方法支持返回文件路径但不写入内容（延迟写入），或者改为在 downloadLoop 完成后一次性 Store。推荐方案：

```
方案 A（推荐）: 新增 PersistentStore.Service.ReserveStore(ctx, relPath, fileName) (storeId int64, absPath string, error)
  - 仅创建 DB 记录 + 确保目录存在，返回绝对路径供 taskManager 创建文件
  - downloadLoop 正常使用 os.Create + Write
  - 下载完成后调用 PersistentStore.Service.ConfirmStore(ctx, storeId, fileSize) 更新 fileSize

方案 B: 在 downloadLoop 完成后调用 PersistentStore.StoreFromFile
  - 需要先下载到临时路径，再 StoreFromFile 移入
  - 多一次文件复制，不推荐
```

### resumeFromPersistedState() 流程（model.go:493-587）

```
resumeFromPersistedState()
  ├─ 1. 通过 pending_resource_id 加载 Resource 实体
  │     └─ resourceReader.GetById(ctx, task.PendingResourceID)
  ├─ 2. 计算本地文件绝对路径
  │     └─ 当前: localPath = filepath.Join(workDir, "resource", resource.FilePath.String)
  │     阶段一兼容: 如果 resource.StoreID 有效 → 使用 PersistentStore 路径
  │                 否则 → 使用旧路径
  ├─ 3. os.Stat(localPath) → downloadedBytes
  ├─ 4. 构建 TaskResParam 调用 pluginExec.Resume()
  └─ 5. 根据 Continuable 标志选择追加/截断模式打开文件 → downloadLoop()
```

**阶段一修改点**：
- 步骤 2 需要兼容 StoreID：如果 StoreID 有效，从 PersistentStore 获取绝对路径

### resolveLocalPath() 实现（model.go:748-777）

```go
func (m *ManagedTask) resolveLocalPath(startResp *sdkdto.WorkResponse) (absSavePath, relativePath, fileName string) {
    // 模板为空时使用插件建议的文件名
    if tpl == "" {
        fileName = m.buildSuggestedFileName(res)
        relativePath = fileName
        absSavePath = filepath.Join(workDir, "resource", fileName)  // ★ 修改为 "store", "resource" ★
        return
    }
    // 模板模式
    relativePath = filepath.Join(authorDir, fileName)
    absSavePath = filepath.Join(workDir, "resource", authorDir, fileName)  // ★ 同上 ★
    return
}
```

**阶段一修改**：将 `filepath.Join(workDir, "resource", ...)` 改为 `filepath.Join(workDir, "store", "resource", ...)`。对应的 relativePath 需加 `"resource/"` 前缀以匹配 PersistentStore 的 `resource` 已注册子目录。

### backup 模块关键流程

**BackupAndDisable**（resource_orchestrator.go:57-104）：
- 查询 workId 的启用资源
- 遍历资源：检查文件存在 → 移动到备份目录 → 禁用资源（清空 FilePath/FileName/FilenameExtension）
- 记录原始路径到 Backup 实体的 `original_file_path` / `original_file_name` / `original_filename_extension`

**Restore**（resource_orchestrator.go:109-146）：
- 查询备份记录
- 移动备份文件回原始路径（`{workDir}/resource/{originalFilePath}`）
- 重新启用资源，恢复 FilePath/FileName/FilenameExtension

**阶段一兼容**：backup 模块暂不改。备份的资源如果有 StoreID，备份后 StoreID 仍指向 PersistentStore 中已删除的记录。阶段二统一处理。

### resourceSaverAdapter（app.go:890-897）

```go
type resourceSaverAdapter struct {
    svc *resource.Service
}

func (a *resourceSaverAdapter) Save(ctx context.Context, resource *entity.Resource) (int64, error) {
    if err := a.svc.Save(ctx, resource); err != nil {
        return 0, err
    }
    return resource.GetID(), nil
}

func (a *resourceSaverAdapter) Update(ctx context.Context, resource *entity.Resource) error {
    return a.svc.Update(ctx, resource)
}
```

这个适配器将 ResourceService 适配为 taskManager 的 ResourceSaver 接口。阶段一不需要修改此适配器，Resource 实体的 Save/Update 逻辑不变。

### ManagedTask 构造函数（model.go:212-234）

```go
func NewManagedTask(
    taskId, parentId int64,
    task *entity.Task,
    pluginExec TaskExecutor,
    workInfoSaver WorkInfoSaver,
    resourceSaver ResourceSaver,
    workDirProvider WorkDirProvider,
    fileNameFormatProvider FileNameFormatProvider,
    workChecker WorkChecker,
    resourceReader ResourceReader,
    backupOrchestrator ResourceBackupOrchestrator,
    pusher TaskProgressPusher,
) *ManagedTask
```

**阶段一需新增参数**：添加 `storeWriter StoreWriter` 接口参数。

### Manager.newManagedTask 工厂（manager.go:846-927）

```go
func (m *Manager) newManagedTask(t *domain.Task) *ManagedTask {
    // ...
    mt := NewManagedTask(t.GetID(), parentId, t, pluginExec,
        m.workInfoSaver, m.resourceSaver, m.workDirProvider,
        m.fileNameFormatProvider, m.workChecker, m.resourceReader,
        m.backupOrchestrator, m.pusher)
    // ...设置回调...
    return mt
}
```

Manager 持有所有依赖接口的字段，通过 newManagedTask 传递给每个 ManagedTask。新增 StoreWriter 需要在 Manager 结构体和 NewManager 构造函数中同步添加。

### PersistentStore 已有接口

```go
// backend/persistentStore/service.go
type Service interface {
    Store(ctx context.Context, relPath string, fileName string, reader io.Reader) (int64, error)
    StoreFromFile(ctx context.Context, relPath string, fileName string, srcAbsPath string) (int64, error)
    GetById(ctx context.Context, id int64) (*entity.PersistentStore, error)
    GetByFilePath(ctx context.Context, filePath string) (*entity.PersistentStore, error)
    Delete(ctx context.Context, id int64) error
    DeleteByFilePath(ctx context.Context, filePath string) error
    Exists(ctx context.Context, id int64) bool
}

// 已注册子目录包含 "resource"，所以资源路径 "resource/{author}/{file}" 可通过校验
// GetAbsPath(store) 获取记录对应文件的绝对路径
// ResolveStorePath(relPath) 解析相对路径为绝对路径（静态方法）
```

### work.Service 中与 Resource 相关的接口

```go
// ResourceReader（work/service.go:55）— 读取作品关联资源
type ResourceReader interface {
    ListByWorkId(ctx context.Context, workId int64) ([]*entity2.Resource, error)
}

// ResourceBatchReader（work/service.go:93）— 批量读取
type ResourceBatchReader interface {
    ListByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*entity2.Resource, error)
}

// ResourceDeleter（work/service.go:193）— 级联删除
type ResourceDeleter interface {
    DeleteByWorkId(ctx context.Context, workId int64) error
}
```

work.Service 在构造时接收这三个接口的实现。阶段二需要在 `DeleteWorkAndSurroundingData` 中增加 PersistentStore 文件清理逻辑。

