# backup 模块说明

## 一句话职责

纯文件仓库：将文件收入 `backup/YYYY/MM/DD/` 保管并维护保管清单，支持按清单行 ID 取回与清理。

## 边界

- **纯保管定位（2026-08-20 纯化裁决）**：backup 是领域无关能力包（MODULE_BOUNDARY_PURITY），表结构**只记保管位置与时间**（file_name/file_path/workdir），**禁止任何来源/归属信息**（曾有的 source_type/source_id/original_* 五列已删列退役）。来源关联一律内嵌发起方业务行：`persistent_store.backup_id`、`plugin.BackupID`（引用保管清单行 ID）。
- 与 **persistentStore**：persistentStore 管源资源文件与 DB 记录（行内 `backup_id` 引用备份）；backup 管保管副本与清单。persistentStore 经其自定义的 `FileMover` 接口消费 backup（app 装配注入），`HardDelete(id, backup=true)`（物理删除联动留档）与 `DeleteWithBackup(id)`（软删链的文件侧：移文件入 backup、行内写 backup_id 并软删）都经此移动文件。
- 替换/板块重执行的**作品级多 store 软删与回滚编排归发起方 taskManager**（ORCHESTRATION_BY_CALLER）：经 TaskDeps 持久 StoreReplacer/StoreBackupReader/BackupFileRestorer 组合本模块与 persistentStore 能力自行串联；本模块不驻留业务编排器（曾有的 `StoreBackupOrchestrator` 随替换链软删化退役——内存清单快照形态被「软删行本身即持久还原点」取代）。

## 对外接口（Service）

| 方法 | 作用 |
| --- | --- |
| `CreateBackup(sourcePath)` | 复制源文件入备份目录并建清单行（保留源文件；插件安装包备份） |
| `MoveToBackup(absFilePath) (id, err)` | 移动源文件入备份目录并建清单行，返回清单行 ID（store 软删链） |
| `GetById(id)` | 按清单行 ID 查询（对业务行的唯一查询面） |
| `GetBackupPath(backup)` / `ResolveBackupPathById(id)` | 取备份文件绝对路径 |
| `RestoreFile(backupPath, targetPath)` | 从备份路径还原文件到目标绝对路径（源端 backup/ 的 fsmonitor 操作抑制在本方法内登记） |
| `DeleteBackup(id)` | 删除备份的磁盘文件与清单行（文件缺失容忍、行不存在幂等；文件端抑制在本方法内登记） |
| `ListCreatedBefore(beforeMs)` | 查询创建时间早于阈值的清单行（实现 backupGovernance.BackupCatalog：正向无主候选） |
| `ListAllIDs()` | 全量投影清单行 ID（实现 backupGovernance.BackupCatalog：反向现存集） |
| `PageBackups(pageNumber, pageSize, includeIDs, excludeIDs)` | 分页查保管清单（create_time 倒序；实现 backupGovernance.BackupCatalog：备份管理面板清单分页。ID 集过滤，引用态语义由治理方折算，本模块只做纯过滤——大集分块避 SQLite 参数上限） |
| `GetByFilePath(filePath)` / `ListByPathPrefix(prefix)` / `ListAllInWorkDir()` | 按当前工作目录的保管路径查询面（精确/前缀/全量；实现 fsmonitor.BackupReader——backup 域文件缺失感知的关联数据源，工作目录迁移前旧行不在监控树被排除） |
| `UpdateFilePath(id, newFilePath)` | 更新清单行保管路径（实现 fsmonitor.BackupRepairer：backup 域移动同步） |

> 无 Wails Handler（前端零消费，已随纯化退役）。无主备份的治理（双向对账/清理调度）由 backupGovernance 编排，本模块只提供目录查询面与删除能力。

## 核心概念

- **保管清单行**：backup 表行 = 一份被保管的文件（位置 + 时间），无来源信息；被业务行引用（backup_id）即为有主，无引用即为无主（backupGovernance 双向对账的治理对象：无主且超保留期→清理，悬空引用→清列）。
- **RestoreFile**：从备份绝对路径还原文件到目标绝对路径（存在则覆盖、跨盘回退复制）；目标在 store/ 监控白名单内的 fsmonitor 操作抑制由调用方负责（抑制键为相对路径，与绝对路径不同构）。
- **StoreType**：标识 Resource 上不同类型的 Store 字段（资源、缩略图等），新增字段时追加常量。

## 依赖关系

- 依赖：无业务模块依赖（纯文件能力）；仅引用 storeRegistry（备份根单一源 `BackupDirPath` + 操作抑制登记）
- 被依赖：persistentStore（FileMover：`HardDelete(backup=true)`/`DeleteWithBackup` 的文件移动）、taskManager（BackupFileRestorer：替换失败回滚的文件还原）、recycleBin（BackupReader：复原与彻底删除按行内 backup_id 定位备份）、plugin（BackupProvider：安装包备份 + Reinstall 按 BackupID 直查）、assetserver（BackupPathResolver：`/store/` 已删记录按行内 backup_id 定位备份文件服务）、backupGovernance（BackupCatalog：无主对账的清单目录面）、fsmonitor（BackupReader/BackupRepairer：backup 域文件缺失感知的查询与修复，经 app.go 适配器注入）

## 关键设计

- **备份时重命名释放占用**：备份原文件前先重命名，快速释放原名称占用，再写入新文件。
- **backup/ 文件操作的操作抑制登记归本模块**：`RestoreFile`（源端移出触发 backup 域 Remove 事件，行删除在还原之后有竞态窗口）与 `DeleteBackup`（文件先删行后删的窗口）在方法内登记抑制——单一登记点覆盖全部调用方（recycleBin/taskManager/backupGovernance/plugin），调用方无须感知；storeFile 目的端是 Create 事件，backup 域不报 Create，无需登记。
