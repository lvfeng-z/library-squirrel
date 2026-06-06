# 备份功能调用技能

## 适用场景

涉及 backup 模块的调用、修改、扩展。典型触发词：
- "备份"、"还原"
- "备份路径"、"backup"
- "StoreBackupOrchestrator"、"MoveToBackup"
- "替换作品时的备份"

## 架构概览

```
backend/backup/
  service.go                  — 核心 Service：备份创建、移动、还原、查询
  handler.go                  — Wails IPC Handler（Create、CreatePluginBackup、GetById、GetPluginBackup）
  store_backup_orchestrator.go — 替换场景编排器：BackupAllStores / RestoreAllStores
```

### 备份路径规则

backup 模块**自行控制**备份根路径，通过注入的 `workDirGetter` 获取用户配置的 workDir。

```
最终路径 = {workDirGetter()} / backup / YYYY / MM / DD / {fileName}
```

- `workDir`：由 `settings.GetWorkDir()` 提供（如 `D:\LS`），每次调用时读取最新值
- `fileName`：从 `sourcePath` 的 `filepath.Base()` 推导，冲突时追加时间戳后缀
- 日期子目录：`backup/YYYY/MM/DD/`，由 backup 模块自行构建

### 数据库实体

`entity.Backup` 关键字段：

| 字段 | 说明 |
|------|------|
| `SourceType` | 来源类型：1=插件、2=资源、3=PersistentStore |
| `SourceID` | 来源记录 ID |
| `FileName` | 备份文件名（可能含冲突后缀） |
| `FilePath` | 相对路径（`backup/YYYY/MM/DD/fileName`） |
| `Workdir` | 备份时的 workDir 绝对路径 |
| `OriginalFilePath` | 原始相对路径（用于还原定位，仅 Resource/PersistentStore 类型） |
| `OriginalFileName` | 原始文件名 |
| `OriginalFilenameExtension` | 原始扩展名 |

## 三条业务调用链

### 1. 插件安装备份

```
plugin.Service.Install
  → BackupProvider.CreatePluginBackup(ctx, pluginId, packagePath)
    → Service.CreateBackup(ctx, SourceTypePlugin, pluginId, packagePath)
      内部：workDir = s.getWorkDir(), fileName = filepath.Base(packagePath)
      操作：复制 packagePath → {workDir}/backup/YYYY/MM/DD/{fileName}
```

特点：复制方式（保留原文件），通过 `BackupProvider` 接口解耦。

### 2. PersistentStore 备份（替换作品场景）

```
taskManager (替换作品)
  → StoreBackupOrchestrator.BackupAllStores(ctx, workId)
    → resourceProvider.GetEnabledByWorkId → 遍历 Resource
      → storeDeleter.Delete(ctx, storeId, backup=true)     [persistentStore.Service]
        → fileMover.MoveToBackup(ctx, storeId, absPath, originalFilePath, originalFileName, ext)
          → Service.MoveBackup(ctx, SourceTypePersistentStore, storeId, absPath)
            内部：workDir = s.getWorkDir(), fileName = filepath.Base(absPath)
            操作：移动 absPath → {workDir}/backup/YYYY/MM/DD/{fileName}
          → 记录 OriginalFilePath / OriginalFileName / OriginalFilenameExtension
      → storeDeleter.Delete(ctx, thumbnailStoreId, backup=false)  [缩略图直接删除]
```

特点：移动方式（O(1)，删除源文件），ThumbnailStore 不备份直接删除。

### 3. PersistentStore 还原（替换失败回滚）

```
StoreBackupOrchestrator.RestoreAllStores(ctx, items)
  → 遍历 items（仅 BackupID > 0）
    → backupReader.GetById(backupId) → 获取 Backup 记录
    → 从 Backup 记录读取 OriginalFilePath / OriginalFileName
    → backupReader.GetBackupPath → 拼接 {workdir}/{filePath}
    → storeImporter.StoreFromExternal(ctx, backupAbsPath, originalFilePath, originalFileName)
      操作：移动备份文件 → {workDir}/store/{originalFilePath}，创建新 PersistentStore 记录
    → resourceUpdater.Update → 更新 Resource 的 StoreID 字段
    → backupReader.Delete → 清理备份记录
```

## Service 方法签名速查

```go
// 核心方法（backup.Service 自行决定 workDir 和 fileName）
CreateBackup(ctx, sourceType, sourceId, sourcePath)             // 复制备份
MoveBackup(ctx, sourceType, sourceId, sourcePath)               // 移动备份（高效）
CreatePluginBackup(ctx, sourceId, sourcePath)                   // 插件备份（委托 CreateBackup）
MoveBackupForResource(ctx, sourceId, sourcePath, originalFilePath, originalFileName, originalExt) // 资源移动备份
MoveToBackup(ctx, sourceId, absFilePath, originalFilePath, originalFileName, originalExt)          // PersistentStore 移动备份

// 查询方法
GetById(ctx, id)
GetPluginBackup(ctx, sourceId)
GetResourceBackup(ctx, resourceId)
GetResourceBackups(ctx, resourceIds)
GetBackupPath(backup)                  // 拼接 {workdir}/{filePath}

// 还原方法
RestoreFile(ctx, backupPath, targetPath) // 通用文件还原

// 删除方法
Delete(ctx, id)                         // 删除备份记录（仅 DB，不删文件）
```

## 接口依赖关系

```
backup.Service 实现的接口：
  - persistentStore.FileMover  (MoveToBackup)
  - plugin.BackupProvider       (CreatePluginBackup, GetPluginBackup)

backup.Service 依赖的接口：
  - backup.Repository           (数据库操作)

StoreBackupOrchestratorImpl 的依赖（通过构造函数注入）：
  - StoreResourceProvider       (resource.Service)
  - StoreDeleter                (persistentStore.Service)
  - StoreImporter               (persistentStore.Service)
  - ResourceUpdater             (resource.Service)
  - BackupReader                (backup.Service)
```

## 关键设计约束

1. **workDir 由 backup 模块内部决定**：所有创建备份的方法不接受 workDir 参数，通过注入的 `workDirGetter` 获取
2. **fileName 从 sourcePath 推导**：不接受 fileName 参数，内部 `filepath.Base(sourcePath)`
3. **OriginalFilePath/FileName/Extension 由调用方传入**：这些是调用方的领域元数据，backup 模块不应自行推导
4. **FilePath 存储相对路径**：`backup/YYYY/MM/DD/fileName`，不含 workDir 前缀
5. **Workdir 字段记录备份时的 workDir**：用于后续 `GetBackupPath` 还原绝对路径
6. **MoveBackup 跨文件系统自动降级**：`os.Rename` 失败时回退为 `CopyFile + Remove`
