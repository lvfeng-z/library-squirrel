# backup 模块说明

## 一句话职责

纯文件仓库：将文件收入 `backup/YYYY/MM/DD/` 保管并维护保管清单，支持按清单行 ID 取回与清理。核心是 `StoreBackupOrchestrator`——在资源替换场景下，一站式备份并还原作品 Resource 的全部 PersistentStore（还原目标由其内存清单快照承载）。

## 边界

- **纯保管定位（2026-08-20 纯化裁决）**：backup 是领域无关能力包（MODULE_BOUNDARY_PURITY），表结构**只记保管位置与时间**（file_name/file_path/workdir），**禁止任何来源/归属信息**（曾有的 source_type/source_id/original_* 五列已删列退役）。来源关联一律内嵌发起方业务行：`persistent_store.backup_id`、`plugin.BackupID`（引用保管清单行 ID）。
- 与 **persistentStore**：persistentStore 管源资源文件与 DB 记录（行内 `backup_id` 引用备份）；backup 管保管副本与清单。persistentStore 经其自定义的 `FileMover` 接口消费 backup（app 装配注入），`HardDelete(id, backup=true)`（物理删除联动留档）与 `DeleteWithBackup(id)`（作品软删除链的文件侧：移文件入 backup、行内写 backup_id 并软删）都经此移动文件。
- 与 **StoreBackupOrchestrator**：Service 提供单文件保管/取回/清理；Orchestrator 编排"作品级"的多 Store 批量备份 / 还原（板块隔离）。

## 对外接口（Service）

| 方法 | 作用 |
| --- | --- |
| `CreateBackup(sourcePath)` | 复制源文件入备份目录并建清单行（保留源文件；插件安装包备份） |
| `MoveToBackup(absFilePath) (id, err)` | 移动源文件入备份目录并建清单行，返回清单行 ID（store 软删链） |
| `GetById(id)` | 按清单行 ID 查询（对业务行的唯一查询面） |
| `GetBackupPath(backup)` / `ResolveBackupPathById(id)` | 取备份文件绝对路径 |
| `RestoreFile(backupPath, targetPath)` | 从备份路径还原文件到目标绝对路径 |
| `DeleteBackup(id)` | 删除备份的磁盘文件与清单行（文件缺失容忍） |

> 无 Wails Handler（前端零消费，已随纯化退役）。作品 store 的批量备份 / 还原由 taskManager（替换 / 板块重执行场景）经 TaskDeps 持有的 `StoreBackupOrchestrator` 调用。

## 核心概念

- **保管清单行**：backup 表行 = 一份被保管的文件（位置 + 时间），无来源信息；被业务行引用（backup_id）即为有主，无引用即为无主（治理对象，见任务图 backup 治理分支的双向对账模型）。
- **RestoreFile**：从备份绝对路径还原文件到目标绝对路径（存在则覆盖、跨盘回退复制）；目标在 store/ 监控白名单内的 fsmonitor 操作抑制由调用方负责（抑制键为相对路径，与绝对路径不同构）。
- **StoreBackupOrchestrator**：封装作品 Resource 全部 PersistentStore 的一站式备份和还原。
  - `BackupStores(workId, types...)`：按 StoreType 板块隔离备份（如仅资源文件、不触及缩略图）；删行前对 file_path/file_name 做内存快照存入 `StoreBackupItem`（物理删行后行内信息无处可查，还原目标由内存清单承载）。
  - `RestoreAllStores(items) (restored, skipped)`：按内存快照路径还原；`BackupID<=0`（备份未成功）或快照缺失的条目计入 `skipped` 并告警。还原成功后在各 `StoreBackupItem.NewStoreID` 回填新 store ID，供调用方重挂 `resource_store`——**backup 只还原文件、不感知 resource_store**。
- **StoreType**：标识 Resource 上不同类型的 Store 字段（资源、缩略图等），新增字段时追加常量。

## 依赖关系

- 依赖：persistentStore（StoreDeleter / StoreImporter）、resource（StoreResourceProvider）
- 被依赖：persistentStore（FileMover：`HardDelete(backup=true)`/`DeleteWithBackup` 的文件移动）、taskManager（StoreBackupOrchestrator：替换 / 板块重执行的批量备份还原）、recycleBin（BackupReader：复原与彻底删除按行内 backup_id 定位备份）、plugin（BackupProvider：安装包备份 + Reinstall 按 BackupID 直查）、assetserver（BackupPathResolver：`/store/` 已删记录按行内 backup_id 定位备份文件服务）

## 关键设计

- **备份时重命名释放占用**：备份原文件前先重命名，快速释放原名称占用，再写入新文件。
- **板块隔离**：BackupStores 按 StoreType 选择性备份，支持板块重执行只备份相关 store。
