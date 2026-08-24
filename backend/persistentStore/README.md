# persistentStore 模块说明

## 一句话职责

资源文件的**持久化存储**：管理 `persistent_store` 记录与对应的磁盘文件，提供流式 / 文件写入、按路径查询、删除（可联动备份）。本模块无 Handler，不直接暴露前端，由 task 下载流程、backup、recycleBin 等内部调用。

## 边界

- 与 **resource**：resource 管 Resource 实体（作品的资源元数据抽象）；persistentStore 管具体的文件落盘与 `persistent_store` 记录。
- 与 **backup**：persistentStore 的 `Delete(id, backup)` 可联动 backup 创建备份；StoreBackupOrchestrator 调用本模块的导入 / 删除接口。
- 与 **storeRegistry**：所有存储路径必须以已注册子目录开头（白名单单一源在 storeRegistry，见下），禁止裸路径。

## 对外能力（无 Handler，供内部调用）

| 方法 | 作用 |
| --- | --- |
| `StoreStream` / `ResumeStream` | 流式写入（支持断点续传，StoreWriter） |
| `Store` / `StoreFromFile` / `StoreFromExternal` | 从 Reader / 本地文件 / 外部文件写入；`StoreFromExternal` 导入前先清同 `file_path` 旧记录（避免 UNIQUE 冲突） |
| `Delete(id, backup)` | 删除记录与文件（backup=true 联动备份，仅已完成文件、失败降级直接删除）；记录不存在视为已删除，返回 `(0,nil)` 而非错误 |
| `HardDelete(id, backup)` | 物理删记录 + 删文件（查行走 GetById 受软删 scope 保护——**对已软删行静默跳过**，适用于活行） |
| `DeleteUnscopedByIds(ids)` | 批量物理删行（单条 DELETE IN，dbFromCtx 模式可入事务；对已软删行的直删通路——purge 链专用） |
| `GetDeletedStore(id)` | 按 ID 获取已软删行（nil = 不存在或非已删态；回收站文件条目清理链入口校验） |
| `DeleteWithBackup(id)` | 软删链的文件侧：移文件入 backup + **记录软删**（与文件移动同生共死）；不看完成状态、失败返回错误（契约与 `HardDelete` 的区别见方法注释）。作品软删除、替换前置、merge overwrite、回收站复原置换共用 |
| `SoftDeleteAndDiscardFile(id)` | 软删记录并废弃其文件（未完成行进入软删产道的分支：partial 文件无复原价值不入备份，尽力删+抑制登记；软删仍经 SoftDeleteWithBackup 单点写入 backup_id=NULL） |
| `DeleteByFilePath` | 按路径删除（物理删） |
| `MarkInvalid(id)` / `RestoreByIds(ids)` / `ResolveFileState(relPath)` | 外部裁决失效（软删）/ 批量复活（复原/失败回滚/置换链，清软删标志与 backup_id 双列）/ 状态解析（/store/ 路由，含删口径） |
| `GetById` / `GetByIds` / `GetByFilePath` | 查询 |
| `Exists` | 存在性校验（/store/ 状态路由用 `ResolveFileState`，含删口径） |
| `ResolveStorePath` / `GetAbsPath` | 路径解析（relPath → 绝对路径） |
| `BuildVariantPath(sourceRelPath, suffix)` | 路径变换：从源 store relPath 派生变体路径（同目录 + 文件名追加 suffix + 保扩展 + 净化，正斜杠入库），供合并在已有 store 旁派生产物路径 |

## 核心概念

- **StoreWriter**：流式写入句柄，支持 `Complete`（写 completed_at 完成时刻）/ `Abort`（中止清理：删文件 + 物理删记录）/ `Sync`。流式写入直接落到最终路径，DB 记录在写入开始时即创建（completed_at=0 未完成），`Complete` 置完成时刻。
- **已注册子目录**（`storeRegistry`）：路径必须以下列前缀开头——
  `store/resource`（作品资源）、`store/thumbnail`（视频缩略图）、`store/avatar/local`（本地作者头像）、`store/avatar/site`（站点作者头像）。
- **路径基准（PATH_SEPARATOR_DISCIPLINE 两域模型）**：所有相对路径（relPath 域）基于 workDir 且**正斜杠**——三个写入口（StoreStream/Store/StoreFromExternal）入口处 `ToSlash` 规范化一次，查旧/抑制登记/落库全程与 DB 基准一致；absPath（`filepath.Join(workDir, rel)`）仅存在于 os.* 调用点不回流。禁止 `../`、`./` 或绝对路径。

## 依赖关系

- 依赖：workDir 提供者（根目录）
- 被依赖：**task**（下载资源落盘）、**work**（软删除经 `DeleteWithBackup` 移文件并软删记录，复原经 `RestoreByIds` 复活，purge 经 `DeleteUnscopedByIds` 物理删行）、**taskManager**（替换前置软删 StoreReplacer / 失败回滚派生与复活 StoreBackupReader）、**recycleBin**（复原编排 StoreRestorer + 文件条目清理 StoreCleaner）、**resource**、**fsmonitor**（StoreReader 对账 + 裁决失效经 MarkInvalid）、**assetserver**（/store/ 状态路由 ResolveFileState）、**backupGovernance**（BackupReferencer：引用集投影 `ListReferencedBackupIDs`（Unscoped 含已删行）、悬空清列 `ClearBackupRefsByBackupIDs`、非法活行防御清列）

## 关键设计

- **记录-文件不变量（2026-08-19 修复，2026-08-20 补 backup_id）**：状态字段如实反映物理世界——`completed_at`（落盘完成时刻，0=未完成）+ `deleted_at`（软删：文件移 backup 或 fsmonitor 外部裁决不复从，复原/裁决链清除）+ `backup_id`（备份清单行引用，软删链与 deleted_at 单条 UPDATE 同生共死写入、复原双列同清，外部删除失效行保持 0）。/store/ 文件服务按状态路由：软删记录按行内 backup_id 查 backup 保管清单定位文件（同路径多代各行指各的，代次隔离）。
- **直接落最终路径 + completed_at 占位**：流式写入直接落到最终路径（不经过临时文件），DB 记录以 `completed_at=0` 起步，`Complete` 时置完成时刻。未完成文件不靠临时后缀隔离，而是由读取层 `StoreFileHandler` 依据 completed 状态校验——未完成记录的 `/store/` 请求直接返回 404，从而避免半成品被读取。零值写入注意：续传重置回未完成须经 `ResetCompleted` 显式列更新（GORM Updates 跳零值）。
- **路径强校验**：`storeRegistry.ValidatePath` 拒绝未注册子目录，统一正斜杠比较以兼容 Windows。
- **操作抑制登记（suppression）**：各 Create/Remove/Rename 落盘点（`Store`/`StoreStream`/`StoreFromExternal`/`Delete`/`DeleteWithBackup`/`CleanupFile`/`storeWriter.Abort`）在磁盘操作前 `storeRegistry.Suppress(relPath)` + `defer Release`，让 fsmonitor 把自身写入与外部操作区分开（`Complete`/`ResumeStream` 无 fsnotify Create/Remove 事件，不登记）。backup 模块的 `MoveBackup` 亦在汇点自登记（覆盖所有移入 backup 的调用方）。
- **fsmonitor 查询的软删排除**：store 行软删后经 GORM 自动 scope 从 StoreReader 三方法（`GetByFingerprint`/`GetByFilePathComplete`/`ListValidComplete`）排除——文件软删期间位于 backup/（监控白名单外）。曾用消费侧 JOIN work 的 `notDeletedWorkCond`（persistentStore 越界感知业务实体），已随软删落地删除。fsmonitor 外部裁决不复从经 `MarkInvalid`（软删）实现，原 invalid_at 列退役。
- **图像宽高提取**：`Complete`/`Store`/`StoreFromExternal` 落盘后，若是图片（`util.IsImageExt`）则用 `image.DecodeConfig` 读头部解码填入 `Width`/`Height`（供前端瀑布流精准布局）。解码失败仅记日志、留 0，不阻断入库。
