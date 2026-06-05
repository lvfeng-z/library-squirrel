# Resource 接入 PersistentStore 重构计划

> **实施状态**：阶段一和阶段二已于 2026-06-05 完成实施，阶段三（清理废弃字段）待后续执行。
>
> **实施偏差**（相对于原计划的差异）：
> 1. taskManager 新增了 `StoreReader` 接口（计划中仅定义了 `StoreStreamer`），用于 resume 时查询 PersistentStore 记录和获取绝对路径
> 2. work.Service 新增了调用方定义的 `StoreBatchReader` 和 `StoreDeleter` 接口（计划中仅笼统描述"通过接口注入"）
> 3. backup 模块暂未完全改造为通过 PersistentStore 操作文件，当前对有 WorkStoreID 的资源仅做禁用（跳过文件移动）
> 4. 前端 `WorkCardItem` 兼容新旧数据格式（`resources` 数组 vs `resource` 单对象），待 bindings 重新生成后清理

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

将 Resource 的文件元数据和文件操作职责委托给 PersistentStore，使 Resource 实体仅保留业务字段。所有文件 I/O 统一通过 PersistentStore 执行。

### 核心设计决策

| # | 决策点 | 结论 |
|---|--------|------|
| 1 | Store 写入方式 | **StoreStream 返回 StoreWriter**：单个方法创建 DB 记录（未完成）+ 文件 + 返回 `StoreWriter`。调用方通过 StoreWriter 写入，完成后调用 `writer.Complete()` 自动标记已完成，失败时调用 `writer.Abort()` 自动清理 |
| 2 | 双写策略 | **不双写**。文件仅由 PersistentStore 写入，Resource 旧文件元数据字段不再写入 |
| 3 | 前端获取文件路径 | SDK 新增 `ResourceFullDTO`（含 `workStore`/`thumbnailStore`），`WorkFullDTO.Resources` 从 `[]*ResourceDTO` 改为 `*ResourceFullDTO` |
| 4 | backup 改造时机 | **阶段一同步改造**，backup 直接通过 PersistentStore 操作文件 |
| 5 | SDK TaskResourceDTO | 精简为仅保留 `Size`、`Type`、`Format`、`SuggestName`、`Continuable`，移除 `URL`/`LocalPath`/`RemotePath`/`Completeness`/`ResourceID` |
| 6 | PersistentStore 职责 | StoreWriter 封装完整的文件+DB 生命周期：创建、写入、完成确认、失败清理。调用方不直接操作文件句柄，不调用独立的 ConfirmStore/RemoveIncomplete |
| 7 | PersistentStore.FileSize | **移除**。该字段仅有写入侧（Store 时写入），无读取侧业务逻辑。文件大小由 Resource.ResourceSize 承担 |
| 8 | PersistentStore 状态 | 新增 `Status` 字段（0=未完成，1=完成）。HTTP 文件服务对未完成记录返回 404 |
| 9 | 旧数据兼容 | **不考虑旧数据兼容**，不需要迁移工具 |

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
    WorkStoreID       sql.NullInt64  `gorm:"column:work_store_id;index" json:"workStoreId"`           // 作品资源文件
    ThumbnailStoreID  sql.NullInt64  `gorm:"column:thumbnail_store_id" json:"thumbnailStoreId"`       // 封面/缩略图
    // file_path、file_name、filename_extension、resource_size、workdir 废弃不再使用
}
```

```go
// PersistentStore（重构后）
type PersistentStore struct {
    *model.BaseEntity
    FilePath          sql.NullString `gorm:"column:file_path;uniqueIndex" json:"filePath"`
    FileName          sql.NullString `gorm:"column:file_name" json:"fileName"`
    FilenameExtension sql.NullString `gorm:"column:filename_extension" json:"filenameExtension"`
    Status            int            `gorm:"column:status;default:0" json:"status"`          // 新增：0=未完成，1=完成
    // FileSize 已移除
}
```

## 阶段策略

采用**渐进式接入**，分三个阶段，每个阶段独立可发布。不考虑旧数据兼容，不提供迁移工具。

---

## 阶段一：Resource 接入 PersistentStore

**目标**：Resource 新增 `work_store_id`/`thumbnail_store_id` 字段，新下载的资源通过 PersistentStore.StoreStream 写入文件，backup 改为通过 PersistentStore 操作，前端通过 ResourceFullDTO 展示。

### 1.1 PersistentStore 实体变更

`backend/base/model/entity/persistent_store.go`：
- 新增 `Status int` 字段，`gorm:"column:status;default:0"`，常量定义：`StoreStatusIncomplete = 0`、`StoreStatusComplete = 1`
- 移除 `FileSize sql.NullInt64` 字段

### 1.2 PersistentStore Service 新增方法

#### StoreWriter 接口

```go
// StoreWriter 封装文件句柄和 DB 记录，实现完整的写入生命周期管理
//
// 生命周期：
//   写入中 → Write() + Sync()
//   暂停   → Close()          关闭文件句柄，保留未完成 DB 记录
//   成功   → Complete()       同步+关闭+更新 DB 为已完成
//   失败   → Abort()          关闭+删除文件+删除 DB 记录
type StoreWriter interface {
    io.Writer
    Sync() error    // 同步文件到磁盘
    Close() error   // 关闭文件句柄（暂停），DB 记录保持未完成
    Complete() error // 完成写入：同步+关闭+更新 DB 状态为已完成
    Abort() error   // 放弃写入：关闭+删除文件+删除 DB 记录
}
```

PersistentStore 包内实现 `storeWriter` 结构体，封装 `*os.File`、`storeId` 和 Service 引用。

#### StoreStream

```go
// StoreStream 创建 DB 记录（未完成）+ 目录 + 文件，返回 storeId 和 StoreWriter
// 单个方法完成全部初始化，调用方通过 StoreWriter 写入数据
// relPath: 相对于 {workDir}/store/ 的路径
// fileName: 原始文件名
func (s *Service) StoreStream(ctx context.Context, relPath string, fileName string) (storeId int64, writer StoreWriter, err error)
```

内部实现：
- 校验 relPath
- 创建 PersistentStore 记录（Status=未完成，FilePath=relPath，FileName=fileName，FilenameExtension=提取扩展名）
- 确保目录存在（os.MkdirAll）
- 创建文件（os.Create）
- 返回 storeId 和封装了 `*os.File` 的 StoreWriter

#### ResumeStream

```go
// ResumeStream 恢复存储，以 append 模式打开未完成文件，返回 StoreWriter
// storeId: StoreStream 返回的未完成记录 ID
func (s *Service) ResumeStream(ctx context.Context, storeId int64) (writer StoreWriter, err error)
```

内部实现：
- 根据 storeId 查询记录，确认 Status=未完成
- 获取绝对路径（GetAbsPath）
- 以 append 模式打开文件（O_WRONLY|O_APPEND）
- 返回 StoreWriter

#### StoreWriter.Complete 实现

- `file.Sync()` + `file.Close()`
- 更新 DB 记录 Status 为已完成

#### StoreWriter.Close 实现

- `file.Sync()` + `file.Close()`
- DB 记录保持不变（Status=未完成），供 ResumeStream 恢复

#### StoreWriter.Abort 实现

- `file.Close()`
- 删除磁盘文件
- 删除 DB 记录

### 1.3 PersistentStore HTTP Handler 调整

`backend/assetserver/store_handler.go`：
- GetById 或根据路径查询时，检查 `Status`，仅 `StoreStatusComplete` 的记录返回文件
- 未完成状态返回 404

### 1.4 PersistentStore DTO 变更

`backend/base/model/dto/persistent_store_dto.go`：
- 移除 `FileSize` 字段
- 新增 `Status int` 字段

SDK `dto/persistent_store_dto.go` 同步更新。

### 1.5 Resource 实体变更

`backend/base/model/entity/resource.go`：
- 新增 `WorkStoreID sql.NullInt64` 字段，`gorm:"column:work_store_id;index"`
- 新增 `ThumbnailStoreID sql.NullInt64` 字段，`gorm:"column:thumbnail_store_id"`

### 1.6 SDK TaskResourceDTO 精简

`sdk/dto/handler_dto.go` 中的 `TaskResourceDTO`：

```go
// TaskResourceDTO 任务处理器资源 DTO（精简后）
type TaskResourceDTO struct {
    Size         int64  `json:"size"`         // 远程文件大小
    Type         string `json:"type"`         // 资源类型
    Format       string `json:"format"`       // 文件格式/扩展名（如 "jpg"、"mp4"）
    SuggestName  string `json:"suggestName"`  // 插件建议文件名
    Continuable  *bool  `json:"continuable"`  // 是否支持续传
}
```

移除的字段：`ResourceID`、`URL`、`LocalPath`、`RemotePath`、`Completeness`。
`Format` 保留，`resolveLocalPath()` 中用于拼接文件扩展名。

### 1.7 SDK 新增 ResourceFullDTO

`sdk/dto/resource_dto.go`：

```go
// ResourceFullDTO 资源完整 DTO（包含作品资源和封面的 PersistentStore 信息）
type ResourceFullDTO struct {
    ID               int64              `json:"id"`
    WorkID           int64              `json:"workId"`
    TaskID           int64              `json:"taskId"`
    Enabled          bool               `json:"enabled"`
    SuggestName      *string            `json:"suggestName"`
    ResourceComplete int                `json:"resourceComplete"`
    WorkStoreID      *int64             `json:"workStoreId"`
    ThumbnailStoreID *int64             `json:"thumbnailStoreId"`
    WorkStore        *PersistentStoreDTO `json:"workStore,omitempty"`        // 作品资源文件
    ThumbnailStore   *PersistentStoreDTO `json:"thumbnailStore,omitempty"`   // 封面/缩略图
    CreateTime       int64              `json:"createTime"`
    UpdateTime       int64              `json:"updateTime"`
}
```

SDK `dto/work_dto.go` 的 `WorkFullDTO`：
- `Resources []*ResourceDTO` 改为 `Resource *ResourceFullDTO`（单对象，不再是数组）

### 1.8 内部 ResourceDTO 转换调整

`backend/base/model/dto/resource_dto.go`：
- 新增 `NewResourceFullDTO(resource *entity.Resource, workStore, thumbnailStore *entity.PersistentStore) *sdkdto.ResourceFullDTO`
- 旧的 `NewResourceDTO()` 保留用于插件侧等不需要 store 信息的场景

### 1.9 taskManager 修改（核心）

`backend/taskManager/model.go`：

#### 新增接口定义（在 taskManager 包内）

```go
// StoreStreamer 创建存储记录并返回 StoreWriter
type StoreStreamer interface {
    StoreStream(ctx context.Context, relPath string, fileName string) (storeId int64, writer persistentStore.StoreWriter, err error)
    ResumeStream(ctx context.Context, storeId int64) (writer persistentStore.StoreWriter, err error)
}
```

注意：`ConfirmStore` 和 `RemoveIncomplete` 不再需要，完成确认和失败清理由 StoreWriter.Complete/Abort 处理。

#### ManagedTask 新增字段

```go
type ManagedTask struct {
    // ... 现有字段 ...
    storeStreamer StoreStreamer
    workStoreId   int64           // StoreStream 返回的作品资源 store ID
    storeWriter   StoreWriter     // 当前写入的 StoreWriter（替代 currentFile）
}
```

`currentFile *os.File` 被 `storeWriter StoreWriter` 替代。不再需要 `storeCleaner`，失败清理由 `storeWriter.Abort()` 处理。

#### run() 新流程（步骤 4-7 重写）

```
当前流程：
  4. resolveLocalPath() → absPath={workDir}/resource/{author}/{file}, relPath={author}/{file}
  5. 构建 Resource{FilePath: relPath, ...} → resourceSaver.Save()
  6. 更新 task.PendingResourceID
  7. os.Create(localPath) → downloadLoop()

新流程：
  4. resolveLocalPath() → relPath=resource/{author}/{file}（相对于 store 根目录）
  4.1 storeStreamer.StoreStream(ctx, relPath, fileName) → (workStoreId, storeWriter)
  5. 构建 Resource{WorkStoreID: 0（暂空）, ResourceSize: startResp.Resource.Size, ...} → resourceSaver.Save()
     注意：不再写入 FilePath/FileName/FilenameExtension/Workdir
  6. 更新 task.PendingResourceID
  7. downloadLoop()  → 使用 storeWriter 替代 os.Create 创建的 *os.File
```

注意：Resource 先保存（获取 resourceId），WorkStoreID 在 StoreStream 后立即设置（DB 记录已创建）。

#### downloadLoop() 修改

downloadLoop 结构与当前基本一致，核心变化是 `*os.File` 替换为 `StoreWriter`：

```go
func (m *ManagedTask) downloadLoop() runResult {
    buf := make([]byte, 32*1024)
    defer m.currentReader.Close()
    // 注意：不 defer storeWriter.Close/Abort，由各分支显式处理

    for {
        select {
        case <-m.pauseCh:
            m.storeWriter.Sync()
            m.storeWriter.Close()      // 关闭文件句柄，DB 记录保持未完成
            return runResultPaused

        case <-m.ctx.Done():
            m.storeWriter.Abort()       // 清理文件和 DB 记录
            return m.fail(...)

        default:
            n, readErr := m.currentReader.Read(buf)
            if n > 0 {
                if _, writeErr := m.storeWriter.Write(buf[:n]); writeErr != nil {
                    m.storeWriter.Abort()
                    return m.fail(writeErr)
                }
                m.totalWritten += int64(n)
                m.onProgress(m.totalWritten, ...)
            }
            if readErr == io.EOF {
                m.storeWriter.Complete() // 同步+关闭+更新 DB 为已完成
                m.clearPendingResourceID()
                m.setState(TaskStateFinished)
                return runResultDone
            }
            if readErr != nil {
                // 处理暂停中的读取错误...
            }
        }
    }
}
```

与当前 downloadLoop 的差异对照：

| 变化点 | 当前 | 新方案 |
|--------|------|--------|
| 文件创建 | `os.Create(localPath)` | `storeStreamer.StoreStream()` |
| 文件写入 | `m.currentFile.Write(buf)` | `m.storeWriter.Write(buf)` |
| 文件同步 | `m.currentFile.Sync()` | `m.storeWriter.Sync()` |
| 暂停关闭 | `m.currentFile.Close()` | `m.storeWriter.Close()` |
| 完成确认 | 无 | `m.storeWriter.Complete()` |
| 失败清理 | 无 | `m.storeWriter.Abort()` |

#### 暂停流程

```
用户点击暂停
  → state = Pausing
  → downloadLoop 检测到 pauseCh
    → storeWriter.Sync()              // PersistentStore 的 StoreWriter 处理
    → return runResultPaused
```

与当前暂停流程完全一致，仅将 `currentFile.Sync()` 替换为 `storeWriter.Sync()`。

#### 恢复流程

```
用户点击恢复
  → resumeFromPersistedState()
  → resource = GetById(pendingResourceID)
  → store = GetById(resource.WorkStoreID)          // 获取未完成记录
  → 已写入字节数 = os.Stat(GetAbsPath(store)).Size()
  → pluginExec.Resume(downloadedBytes)              // 获取续传 reader
  → storeStreamer.ResumeStream(ctx, storeId)        // 获取 append 模式 StoreWriter
  → downloadLoop()
```

#### resolveLocalPath() 修改

```go
// 无模板模式
relativePath = filepath.Join("resource", fileName)
absSavePath = ""  // 不再使用，文件由 StoreStream 内部创建

// 模板模式
relativePath = filepath.Join("resource", authorDir, fileName)
absSavePath = ""  // 同上
```

返回值 `absSavePath` 不再使用，仅 `relativePath` 和 `fileName` 有效。

#### resumeFromPersistedState() 修改

```
当前：localPath = filepath.Join(workDir, "resource", resource.FilePath.String)
改为：通过 WorkStoreID 查询 PersistentStore 记录 → GetAbsPath 获取文件路径 → os.Stat 获取已下载字节数
      然后调用 pluginExec.Resume(downloadedBytes) 获取续传 reader
      最后调用 ResumeStream(ctx, storeId) 获取 append 模式 StoreWriter
```

由于不考虑旧数据兼容，resume 逻辑可简化为**必须通过 WorkStoreID 获取路径**，如果 WorkStoreID 无效则视为异常。

#### Format 字段

`resolveLocalPath()` 使用 `res.Format` 拼接文件扩展名，`Format` 已保留在 `TaskResourceDTO` 中，无需替代方案。

### 1.10 依赖注入链更新

**Manager 新增字段**：
```go
type Manager struct {
    // ... 现有字段 ...
    storeStreamer StoreStreamer
}
```

**NewManager 新增参数**：
```go
func NewManager(
    // ... 现有参数 ...
    storeStreamer StoreStreamer,  // → PersistentStoreService
) *Manager
```

**app.go 组装更新**：
```go
app.TaskManagerService = taskManager.NewManager(
    // ... 现有参数 ...
    app.PersistentStoreService,    // StoreStreamer
)
```

注意：不再需要 StoreCleaner 参数，失败清理由 StoreWriter.Abort() 内部处理。

### 1.11 backup 模块改造

`backend/backup/resource_orchestrator.go`：

#### BackupAndDisable 新流程

```
当前：查询资源 → 移动文件到备份目录 → 清空 Resource 文件字段 → 禁用资源 → 保存备份记录
改为：查询资源 → 通过 WorkStoreID/ThumbnailStoreID 获取 PersistentStore 记录
     → 移动文件到备份目录
     → 调用 PersistentStore.Delete(workStoreId) 和 PersistentStore.Delete(thumbnailStoreId) 清理 DB 记录
     → 清空 Resource.WorkStoreID 和 Resource.ThumbnailStoreID（不改变 Enabled 状态）
     → 保存备份记录（记录 store 信息以便恢复）
```

注意：不再将 Resource 置为禁用，仅清空 store_id 字段。备份记录需要存储 PersistentStore 的信息（relPath、fileName、filenameExtension），以便恢复时重新注册。Backup 实体可能需要新增字段来存储这些信息。

#### Restore 新流程

```
当前：查询备份记录 → 移动文件回原路径 → 恢复 Resource 文件字段 → 启用资源
改为：查询备份记录 → 移动作品资源文件到 store 目录
     → 调用 PersistentStore.StoreFromFile 注册文件 → 获取新 workStoreId
     → 如有封面，同样移动封面文件 → 获取新 thumbnailStoreId
     → 更新 Resource.WorkStoreID 和 ThumbnailStoreID（Resource 保持原 Enabled 状态不变）
```

#### backup 依赖注入更新

`ResourceBackupOrchestrator` 需要新增对 `persistentStore.Service` 的依赖。

### 1.12 search 模块 SQL 改造

`backend/search/repository.go` 的 `QueryWorkPage`：

resources 子查询新增 LEFT JOIN persistent_store 两次（workStore 和 thumbnailStore），构建 ResourceFullDTO：

```sql
(SELECT JSON_OBJECT(
    'id', r.id, 'workId', r.work_id, 'taskId', r.task_id,
    'enabled', IIF(r.enabled, json('true'), json('false')),
    'suggestName', r.suggest_name, 'resourceComplete', r.resource_complete,
    'workStoreId', r.work_store_id, 'thumbnailStoreId', r.thumbnail_store_id,
    'workStore', CASE WHEN ws.id IS NOT NULL THEN JSON_OBJECT(
        'id', ws.id, 'filePath', ws.file_path, 'fileName', ws.file_name,
        'filenameExtension', ws.filename_extension, 'status', ws.status,
        'createTime', ws.create_time, 'updateTime', ws.update_time)
    END,
    'thumbnailStore', CASE WHEN ts.id IS NOT NULL THEN JSON_OBJECT(
        'id', ts.id, 'filePath', ts.file_path, 'fileName', ts.file_name,
        'filenameExtension', ts.filename_extension, 'status', ts.status,
        'createTime', ts.create_time, 'updateTime', ts.update_time)
    END,
    'createTime', r.create_time, 'updateTime', r.update_time)
FROM resource r
LEFT JOIN persistent_store ws ON r.work_store_id = ws.id
LEFT JOIN persistent_store ts ON r.thumbnail_store_id = ts.id
WHERE t1.id = r.work_id) AS resource
```

注意：从 `JSON_GROUP_ARRAY` 改为 `JSON_OBJECT`（单资源），WorkFullDTO.Resource 从数组变为单对象。

### 1.13 work.Service DTO 组装改造

`backend/work/service.go` 的 `GetFullWorkInfoByIds`：

Phase 4（批量查询资源）需要扩展：
- 查询 Resource 实体后，收集所有有效的 WorkStoreID 和 ThumbnailStoreID
- 批量查询 PersistentStore 记录，构建 map
- 调用 `NewResourceFullDTO(resource, workStore, thumbnailStore)` 组装
- `fullDTO.Resources = []*ResourceDTO{...}` 改为 `fullDTO.Resource = resourceFullDTO`

work.Service 需要新增对 `persistentStore.Service` 的依赖（通过接口注入）。

### 1.14 前端变更

- `WorkFullDTO.Resources` 从 `Resource[]` 改为单个 `ResourceFullDTO`（含 `workStore`/`thumbnailStore`）
- `WorkFullDTO.ts` 构造函数中不再需要从 resources 数组中筛选活跃资源的逻辑
- 资源展示组件（WorkCard、WorkSetCard、WorkDialog）：
  - 图片/文件展示使用 `resource.workStore?.filePath` + `buildStoreUrl()`
  - 封面/缩略图使用 `resource.thumbnailStore?.filePath` + `buildStoreUrl()`
- `buildResourceUrl()` 保留但逐步废弃

### 修改文件清单

| 文件 | 变更 |
|------|------|
| `backend/base/model/entity/persistent_store.go` | 新增 `Status` 字段，移除 `FileSize` |
| `backend/base/model/entity/resource.go` | 新增 `WorkStoreID`、`ThumbnailStoreID` 字段 |
| `backend/persistentStore/service.go` | 新增 `StoreStream`、`ResumeStream` 方法和 `StoreWriter` 接口（含 Complete/Close/Abort） |
| `backend/persistentStore/dto.go` | 同步 Status/FileSize 变更 |
| `backend/assetserver/store_handler.go` | 未完成记录返回 404 |
| SDK `dto/persistent_store_dto.go` | 同步 Status/FileSize 变更 |
| SDK `dto/handler_dto.go` | 精简 `TaskResourceDTO`（移除 URL/LocalPath/RemotePath/Completeness/ResourceID，保留 Format） |
| SDK `dto/resource_dto.go` | 新增 `ResourceFullDTO`（workStore/thumbnailStore） |
| SDK `dto/work_dto.go` | `WorkFullDTO.Resources` 改为 `Resource *ResourceFullDTO` |
| `backend/base/model/dto/resource_dto.go` | 新增 `NewResourceFullDTO` |
| `backend/base/model/dto/persistent_store_dto.go` | 同步 Status/FileSize 变更 |
| `backend/taskManager/model.go` | 核心改造：run/downloadLoop/resume/resolveLocalPath，`*os.File` 替换为 `StoreWriter` |
| `backend/taskManager/manager.go` | NewManager 新增 StoreStreamer 依赖参数 |
| `app.go` | taskManager 和 backup 初始化时注入 PersistentStoreService |
| `backend/backup/resource_orchestrator.go` | 改为通过 PersistentStore 操作文件 |
| `backend/work/service.go` | GetFullWorkInfoByIds 组装 ResourceFullDTO |
| `backend/search/repository.go` | SQL LEFT JOIN persistent_store |
| `frontend/src/model/model/dto/ResourceFullDTO.ts` | 新增 ResourceFullDTO 类型 |
| `frontend/src/components/common/WorkCard.vue` | 使用 workStore.filePath + buildStoreUrl |
| `frontend/src/components/common/WorkSetCard.vue` | 同上 |
| `frontend/src/components/dialogs/WorkDialog.vue` | 同上 |
| `frontend/src/model/model/dto/WorkFullDTO.ts` | resources 数组逻辑改为单 ResourceFullDTO |

---

## 阶段二：级联删除接入 PersistentStore

**目标**：删除作品时通过 PersistentStore 清理磁盘文件。

### 2.1 work 级联删除改造

`backend/work/service.go`：
- `DeleteWorkAndSurroundingData` 中，删除 Resource 记录前：
  - 查询关联 Resource 的 `WorkStoreID` 和 `ThumbnailStoreID`
  - 批量调用 `PersistentStore.Delete()` 清理文件和 DB 记录（包括 workStore 和 thumbnailStore）
- 再删除 Resource DB 记录

---

## 阶段三：清理废弃字段

**目标**：移除 Resource 实体中的旧文件元数据字段，移除旧 HTTP handler。

### 3.1 清理工作

- 移除 `Resource` 的 `file_path`、`file_name`、`filename_extension`、`resource_size`、`workdir` 字段
- 移除 `assetserver/resource_handler.go`（统一使用 `/store/` 路由）
- 移除前端 `buildResourceUrl()`，统一使用 `buildStoreUrl()`
- 清理 SDK `ResourceDTO` 中对应的废弃字段
- 清理所有 DTO 转换函数中的旧字段映射

---

## 风险与注意事项

1. **暂停期间的状态**：暂停时文件可能不完整，PersistentStore 记录保持未完成状态，不可清理。ResumeStream 通过 storeId 查找未完成记录并 append 模式打开文件
2. **大文件处理**：StoreWriter 封装 *os.File，调用方使用 32KB 缓冲区分片写入，无额外内存开销
3. **backup 原子性**：backup 先移动文件再调用 PersistentStore.Delete，如果 Delete 失败，文件已移走但 DB 记录仍在，需要日志告警
4. **search SQL 性能**：LEFT JOIN persistent_store 增加查询复杂度，需关注大数据量下的性能表现
5. **SDK 插件兼容**：TaskResourceDTO 精简后移除了 URL/LocalPath/RemotePath/Completeness/ResourceID，需同步更新所有插件的 Start/Resume 返回值
6. **封面（thumbnailStore）机制**：Resource 实体新增 `thumbnail_store_id` 字段，但封面数据的来源和传递机制（插件提供）不在本计划中定义，留到后续计划（如 video-thumbnail-plan）实现。本次仅预留字段
7. **StoreWriter.Complete 失败**：文件已完整写入但 DB 状态更新失败时，文件存在但记录为未完成。需要后续修复机制
8. **StoreWriter.Abort 失败**：删除文件或 DB 记录失败时可能留下孤儿文件或记录，需要日志告警和后续修复机制

## 建议

**推荐从阶段一开始实施**，理由：
- 一次到位地完成核心改造（下载、backup、前端展示），不留中间态
- 不考虑旧数据兼容，逻辑简单清晰
- downloadLoop 结构不变，仅替换底层文件操作，改动风险可控
- 阶段二、三改动较小，可快速跟进
