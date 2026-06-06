# 重构计划：backup 模块自行控制备份路径 & 清理冗余参数

## 目标

1. `backup.Service` 内部持有 `workDirGetter`，所有创建备份方法**移除 `workDir` 参数**，备份根路径完全由 backup 模块自行决定。
2. `CreateBackup` / `MoveBackup` **移除 `fileName` 参数**，内部从 `sourcePath` 推导（`filepath.Base(sourcePath)`），该参数在所有调用方中均为冗余。

## 当前状况

### 问题 1：workDir 受外部影响

backup.Service 的所有创建备份方法都接收外部传入的 `workDir` 参数：

| 方法 | workDir 来源 | 调用方 |
|------|-------------|--------|
| `CreateBackup` | 参数 | Handler (IPC)、CreatePluginBackup |
| `MoveBackup` | 参数 | MoveBackupForResource、MoveToBackup |
| `CreatePluginBackup` | 参数（透传给 CreateBackup） | Handler (IPC)、plugin.Service |
| `MoveBackupForResource` | 参数（透传给 MoveBackup） | 无调用方（死代码） |
| `MoveToBackup` | 内部 `util.RootPath()` 硬编码 | persistentStore.Service |

### 问题 2：fileName 参数冗余

所有调用方传入的 `fileName` 均为 `filepath.Base(sourcePath)`：

| 调用方 | fileName 取值 |
|--------|--------------|
| `Handler.Create` (IPC) | 前端传入，与 sourcePath 的文件名一致 |
| `plugin.Service.Install` → `CreatePluginBackup` | `filepath.Base(installDTO.PackagePath)` |
| `persistentStore.Service.Delete` → `MoveToBackup` | `record.FileName`，而 absPath 由 record 构造，文件名一致 |

## 重构方案

### Step 1: backup.Service 注入 workDirGetter

**文件**: `backend/backup/service.go`

1. `Service` 结构体新增字段 `workDirGetter func() string`
2. `NewService` 签名增加 `workDirGetter func() string` 参数
3. 新增私有方法 `getWorkDir() string`，内部调用 `s.workDirGetter()`

```go
type Service struct {
    repo          Repository
    workDirGetter func() string
}

func NewService(repo Repository, workDirGetter func() string) *Service {
    return &Service{repo: repo, workDirGetter: workDirGetter}
}

func (s *Service) getWorkDir() string {
    return s.workDirGetter()
}
```

### Step 2: 核心方法移除 workDir 和 fileName 参数

**文件**: `backend/backup/service.go`

#### 2.1 `CreateBackup` — 移除 `fileName`、`workDir`

签名变更：
```go
// 之前
CreateBackup(ctx, sourceType, sourceId, fileName, sourcePath, workDir)
// 之后
CreateBackup(ctx, sourceType, sourceId, sourcePath)
```

内部变更：
- `workDir` → `s.getWorkDir()`
- `fileName` → `filepath.Base(sourcePath)`

#### 2.2 `MoveBackup` — 移除 `fileName`、`workDir`

签名变更：
```go
// 之前
MoveBackup(ctx, sourceType, sourceId, fileName, sourcePath, workDir)
// 之后
MoveBackup(ctx, sourceType, sourceId, sourcePath)
```

内部变更同上。

#### 2.3 `CreatePluginBackup` — 移除 `fileName`、`workDir`

签名变更：
```go
// 之前
CreatePluginBackup(ctx, sourceId, fileName, sourcePath, workDir)
// 之后
CreatePluginBackup(ctx, sourceId, sourcePath)
```

透传调用同步调整。

#### 2.4 `MoveBackupForResource` — 移除 `fileName`、`workDir`

签名变更：
```go
// 之前
MoveBackupForResource(ctx, sourceId, fileName, sourcePath, workDir, originalFilePath, originalFileName, originalFilenameExtension)
// 之后
MoveBackupForResource(ctx, sourceId, sourcePath, originalFilePath, originalFileName, originalFilenameExtension)
```

> 注：当前无调用方，但保留方法以备后续使用。`originalFilePath`、`originalFileName`、`originalFilenameExtension` 是 PersistentStore 的领域元数据，backup 模块不应自行推导，保留由调用方显式传入。

#### 2.5 `MoveToBackup` — 移除 `fileName`，移除 `util.RootPath()` 硬编码

签名变更：
```go
// 之前
MoveToBackup(ctx, sourceId, fileName, absFilePath, originalFilePath, originalFileName, originalFilenameExtension)
// 之后
MoveToBackup(ctx, sourceId, absFilePath, originalFilePath, originalFileName, originalFilenameExtension)
```

内部变更：
- `util.RootPath()` → `s.getWorkDir()`
- `fileName` → `filepath.Base(absFilePath)`

### Step 3: 更新 persistentStore.Service（FileMover 接口 + 调用方）

**文件**: `backend/persistentStore/service.go`

1. `FileMover` 接口移除 `fileName` 参数：

```go
// 之前
MoveToBackup(ctx, sourceId, fileName, absFilePath, originalFilePath, originalFileName, originalFilenameExtension) (int64, error)
// 之后
MoveToBackup(ctx, sourceId, absFilePath, originalFilePath, originalFileName, originalFilenameExtension) (int64, error)
```

2. `Delete` 方法中调用 `MoveToBackup` 时移除 `fileName` 参数（第 446 行）：

```go
// 之前
backupId, err = s.fileMover.MoveToBackup(ctx, id, fileName, absPath, record.FilePath.String, originalFileName, originalFilenameExtension)
// 之后
backupId, err = s.fileMover.MoveToBackup(ctx, id, absPath, record.FilePath.String, originalFileName, originalFilenameExtension)
```

同时可移除 `Delete` 方法中为 `fileName` 准备的局部变量（第 437-439 行）。

### Step 4: 更新 backup.Handler（IPC 层）

**文件**: `backend/backup/handler.go`

- `Handler.Create`: 移除 `fileName`、`workDir` 参数
- `Handler.CreatePluginBackup`: 移除 `fileName`、`workDir` 参数

### Step 5: 更新 plugin.Service 的 BackupProvider 接口及调用

**文件**: `backend/plugin/service.go`

1. `BackupProvider` 接口 `CreatePluginBackup` 方法签名移除 `fileName`、`workDir` 参数：

```go
type BackupProvider interface {
    CreatePluginBackup(ctx context.Context, sourceId int64, sourcePath string) (*entity2.Backup, error)
    GetPluginBackup(ctx context.Context, sourceId int64) (*entity2.Backup, error)
}
```

2. `plugin.Service.Install` 中调用 `CreatePluginBackup` 时移除 `fileName`、`workDir` 参数和相关的 `getWorkDir()` / fallback 逻辑。

3. `plugin.Service` 移除 `WorkDirProvider` 依赖和 `workDirProvider` 字段。

经确认：`plugin.Service` 中 `workDirProvider` 仅用于 `Install` 方法中的备份调用（第 396-400 行），移除后 `WorkDirProvider` 依赖和接口定义可以一并删除。

### Step 6: 更新 app.go 初始化

**文件**: `app.go`

1. `initBaseServices` 中 `backup.NewService` 调用增加 `workDirGetter` 参数：

```go
app.BackupService = backup.NewService(backupRepo, func() string {
    return app.SettingsService.GetWorkDir()
})
```

2. `plugin.NewService` 调用移除 `app.SettingsService` 参数（不再需要 WorkDirProvider）：

```go
app.PluginService = plugin.NewService(pluginRepo, app.BackupService)
```

### Step 7: 重新生成 TypeScript bindings

运行 `wails3 generate bindings -ts`，`Handler.Create` 和 `Handler.CreatePluginBackup` 的签名变化会自动反映到 bindings。

检查并同步更新前端 wrapper（如有引用 `fileName`/`workDir` 参数的地方）。

## 影响范围总结

| 文件 | 变更类型 |
|------|---------|
| `backend/backup/service.go` | 核心重构：注入 workDirGetter，移除 workDir/fileName 参数 |
| `backend/backup/handler.go` | 移除 2 个方法的 fileName/workDir 参数 |
| `backend/plugin/service.go` | BackupProvider 接口移除 fileName/workDir；移除 WorkDirProvider 依赖 |
| `backend/persistentStore/service.go` | FileMover 接口移除 fileName；Delete 方法调用处同步调整 |
| `app.go` | 调整 backup.NewService 和 plugin.NewService 的参数 |
| `frontend/bindings/...` | 自动重新生成 |
| `frontend/src/apis/http/wrappers/` | 移除对应的 fileName/workDir 参数传递（如有） |

## 不受影响的部分

- `backup/store_backup_orchestrator.go` — 通过 `StoreDeleter` 接口调用，不直接传 workDir/fileName
- `taskManager/` — 不直接调用 backup 方法，通过 orchestrator 间接调用

## 风险点

- **前端 bindings 变更**：`Handler.Create` 和 `CreatePluginBackup` 签名变更后，前端若有调用需同步更新
- **plugin.Service 构造函数变更**：移除 `workDirProvider` 参数后，所有 `NewService` 调用点需同步修改
