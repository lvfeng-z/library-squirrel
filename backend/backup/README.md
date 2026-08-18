# backup 模块说明

## 一句话职责

备份管理：对作品资源文件、插件数据等创建备份副本并记录元数据，支持还原。核心是 `StoreBackupOrchestrator`——在资源替换场景下，一站式备份并还原作品 Resource 的全部 PersistentStore。

## 边界

- 与 **persistentStore**：persistentStore 管源资源文件与 DB 记录；backup 管备份副本与备份记录。persistentStore 经其自定义的 `FileMover` 接口消费 backup（app 装配注入），`Delete(id, backup=true)`（物理删除联动备份）与 `DeleteWithBackup(id, workId)`（作品软删除链的文件侧：移文件入 backup 并写 work_id 归属，persistent_store 记录原地保留）都经此移动文件。
- 与 **StoreBackupOrchestrator**：通用 Handler 处理单文件备份；Orchestrator 编排"作品级"的多 Store 批量备份 / 还原（板块隔离）。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Create(sourceType, sourceId, sourcePath)` | 创建备份 |
| `CreatePluginBackup(sourceId, sourcePath)` | 创建插件数据备份 |
| `GetById(id)` | 按ID查询备份 |
| `GetPluginBackup(sourceId)` | 查询插件的备份 |

> 作品资源 store 的批量备份 / 还原由 taskManager（替换 / 板块重执行场景）经 TaskDeps 持有的 `StoreBackupOrchestrator` 调用，不经 Handler。

## 核心概念

- **sourceType**：备份来源类型（作品资源 / 插件数据等）。
- **work_id 归属**：作品软删除链逐 store 建备份时写入 `backup.work_id`——复原（`ListByWorkId` 聚合取文件清单 → `RestoreFile` 还原回 store/ 原路径）与彻底删除（清文件与记录）都按此关联；任务板块重执行等其他来源的备份无归属（NULL），不在软删除链范围。
- **RestoreFile**：从备份绝对路径还原文件到目标绝对路径（存在则覆盖、跨盘回退复制）；目标在 store/ 监控白名单内的 fsmonitor 操作抑制由调用方负责（抑制键为相对路径，与绝对路径不同构）。
- **StoreBackupOrchestrator**：封装作品 Resource 全部 PersistentStore 的一站式备份与还原。
  - `BackupStores(workId, types...)`：按 StoreType 板块隔离备份（如仅资源文件、不触及缩略图）。
  - `RestoreAllStores(items) (restored, skipped)`：从备份清单还原；`BackupID<=0`（备份未成功）的条目计入 `skipped` 并告警，避免静默丢失，调用方据 `skipped` 上报。还原成功后在各 `StoreBackupItem.NewStoreID` 回填新 store ID，供调用方重挂 `resource_store`——**backup 只还原文件、不感知 resource_store**（模块边界：resource_store 的清理本次新建 store 与还原后重挂均归 taskManager 还原分支）。
- **StoreType**：标识 Resource 上不同类型的 Store 字段（资源、缩略图等），新增字段时追加常量。

## 依赖关系

- 依赖：persistentStore（StoreDeleter / StoreImporter）、resource（ResourceUpdater / StoreResourceProvider）
- 被依赖：persistentStore（FileMover：`Delete`/`DeleteWithBackup` 的文件移动与 work_id 归属写入）、taskManager（StoreBackupOrchestrator：替换 / 板块重执行的批量备份还原）、recycleBin（BackupReader：ListByWorkId/RestoreFile/GetBackupPath/Delete——复原与彻底删除的文件链）、assetserver（BackupPathResolver：`/store/` 原路径文件缺失时按 original_file_path 反查兜底服务，软删作品文件在 backup/ 期间可访问）

## 关键设计

- **备份时重命名释放占用**：备份原文件前先重命名，快速释放原名称占用，再写入新文件。
- **板块隔离**：BackupStores 按 StoreType 选择性备份，支持板块重执行只备份相关 store。
