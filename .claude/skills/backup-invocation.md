# 备份功能调用技能

## 适用场景

涉及 backup 模块的调用、修改、扩展。典型触发词：
- "备份"、"还原"
- "备份路径"、"backup"
- "MoveToBackup"、"DeleteWithBackup"
- "替换链回滚时的备份还原"

## 架构概览

```
backend/backup/
  service.go                  — 核心 Service：备份创建（复制/移动）、还原、查询、删除
  repository.go               — backup 表仓储
```

backup 是纯文件保管能力包：清单行只记保管位置与时间，不记来源（MODULE_BOUNDARY_PURITY）——来源关联由发起方业务行内嵌记录（`persistent_store.backup_id` / `plugin.BackupID`）。替换/板块重执行的作品级软删与回滚编排归发起方（taskManager / download / share 经 resource.ReplacementService，见 `backend/resource/README.md`）。

### 备份路径规则

backup 模块**自行控制**备份根路径，通过注入的 `workDirGetter` 获取用户配置的 workDir。

```
最终路径 = {workDirGetter()} / backup / YYYY / MM / DD / {fileName}
```

- `workDir`：由 `settings.GetWorkDir()` 提供（如 `D:\LS`），每次调用时读取最新值
- `fileName`：从 `sourcePath` 的 `filepath.Base()` 推导，冲突时追加时间戳后缀
- 日期子目录：`backup/YYYY/MM/DD/`，由 backup 模块自行构建

### 数据库实体

`entity.Backup` 关键字段（纯保管清单——只记保管位置与时间）：

| 字段 | 说明 |
|------|------|
| `FileName` | 备份文件名（可能含冲突后缀） |
| `FilePath` | 保管文件相对路径（`backup/YYYY/MM/DD/fileName`，正斜杠） |
| `Workdir` | 备份时的 workDir 绝对路径（还原时拼接绝对路径的基准） |

## 三条业务调用链

### 1. 插件安装备份（复制，保留源文件）

```
plugin.Service.installCore
  → BackupProvider.CreateBackup(ctx, packagePath)
    → backup.Service.storeFile(copy=true)
      内部：workDir = s.getWorkDir(), fileName = filepath.Base(packagePath)
      操作：复制 packagePath → {workDir}/backup/YYYY/MM/DD/{fileName}
  → 清单行 ID 内嵌 plugin.BackupID（重装以备份文件为安装源；换版/卸载消费式删除旧备份）
```

特点：复制方式（保留原文件），通过 `BackupProvider` 接口解耦。

### 2. store 文件移入备份（替换前置软删）

```
download 提交点首步 / share 确认替换 / 策略任务
  → resource.ReplacementService.SoftDeleteWorkStoreRoles(ctx, workId, roles)
    → persistentStore.Service.DeleteWithBackup(ctx, storeId)
      → fileMover.MoveToBackup(ctx, absFilePath)     [backup.Service 实现 FileMover]
        → backup.Service.storeFile(copy=false)
          操作：移动 absFilePath → {workDir}/backup/YYYY/MM/DD/{fileName}（跨卷回退复制）
      → 记录软删 + 行内写 backup_id（与 deleted_at 同条 UPDATE）
    → 软删行入回收站文件条目，可经复原置换回滚
```

特点：移动方式（O(1)，删除源文件）；未完成行（partial 文件）不入备份直接废弃。`resource_store` 关联不摘——软删行经挂载链联作品。

### 3. 备份还原（替换失败回滚）

```
taskManager setFailed 单点（任务失败终态）
  → resource.ReplacementService.RestoreReplacedStores(ctx, scope)
    → 物理丢弃替换期新建的 store 行（行+文件+关联，释放旧代 file_path）
    → 对每个有备份 victim：
        backupReader.GetById(backupId) → 获取 Backup 清单行
        → backupReader.GetBackupPath(backup) → 拼接 {workdir}/{filePath}
        → backupReader.RestoreFile(ctx, backupAbs, storeAbs) → 移回 store/ 原路径（带操作抑制）
    → persistentStore.RestoreByIds → 批量复活软删行（清软删标志与 backup_id 双列）
    → backupReader.DeleteBackup(backupId) → 清理已还原备份（文件缺失容忍）
    → 重算 victim 所属资源完整度
```

顺序约束：先复活行清 backup_id、再删备份清单行——行内引用未清时删清单行会撞 `persistent_store.backup_id` 外键拒绝。

## Service 方法签名速查

```go
// 备份创建（backup.Service 自行决定 workDir 和 fileName）
CreateBackup(ctx, sourcePath)             // 复制备份（保留源文件；插件安装包备份）
MoveToBackup(ctx, absFilePath)            // 移动备份（O(1)；实现 persistentStore.FileMover）

// 查询方法
GetById(ctx, id)
GetByFilePath(ctx, filePath)
ListByPathPrefix(ctx, prefix)
ListCreatedBefore(ctx, beforeMs)
ListAllIDs(ctx) / ListAllInWorkDir(ctx)
PageBackups(ctx, pageNumber, pageSize, includeIDs, excludeIDs)

// 还原/路径
GetBackupPath(backup)                     // 拼接 {workdir}/{filePath}
ResolveBackupPathById(ctx, backupId)      // 按清单行 ID 解析备份绝对路径
RestoreFile(ctx, backupPath, targetPath)  // 文件还原（移回目标路径）

// 删除方法
DeleteBackup(ctx, id)         // 删备份磁盘文件与清单行（文件缺失容忍；消费式删除）
DeleteBackupFile(ctx, id)     // 仅删磁盘文件（保留清单行；回收站两阶段清理用）
DeleteBackupRecord(ctx, id)   // 仅删清单行（不删文件）

// 维护
UpdateFilePath(ctx, id, newFilePath)  // 目录改名前缀同步
NormalizeFilePaths(ctx)              // file_path 反斜杠规范化
```

## 接口依赖关系

```
backup.Service 实现的接口：
  - persistentStore.FileMover   (MoveToBackup)
  - plugin.BackupProvider       (CreateBackup、GetById、DeleteBackup)

backup.Service 依赖的接口：
  - backup.Repository           (数据库操作)

消费方（编排归发起方，backup 不驻留业务编排器）：
  - plugin          (BackupProvider——安装包备份，行内 BackupID 内嵌引用)
  - persistentStore (FileMover——DeleteWithBackup 移文件入 backup)
  - resource        (ReplaceBackupRestorer——替换链失败回滚还原文件，接口定义于 resource/replacement.go)
  - recycleBin      (BackupReader——复原置换还原；两阶段 DeleteBackupFile/DeleteBackupRecord)
  - backupGovernance (BackupReferencer 引用集对账的清列配合方，登记义务见 backend/backup/README.md)
```

## 关键设计约束

1. **workDir 由 backup 模块内部决定**：所有创建备份的方法不接受 workDir 参数，通过注入的 `workDirGetter` 获取
2. **fileName 从 sourcePath 推导**：不接受 fileName 参数，内部 `filepath.Base(sourcePath)`
3. **清单行不记来源**：来源关联由发起方业务行内嵌记录（`persistent_store.backup_id` / `plugin.BackupID`）；引用 backup 清单行的业务方须登记进 backupGovernance 的 `BackupReferencer`（app.go 装配处）
4. **FilePath 存储相对路径**：`backup/YYYY/MM/DD/fileName`，不含 workDir 前缀
5. **Workdir 字段记录备份时的 workDir**：用于后续 `GetBackupPath` 还原绝对路径
6. **MoveBackup 跨文件系统自动降级**：`os.Rename` 失败时回退为 `CopyFile + Remove`
