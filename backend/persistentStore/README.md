# persistentStore 模块说明

## 一句话职责

资源文件的**持久化存储**：管理 `persistent_store` 记录与对应的磁盘文件，提供提交点建行 / 文件写入、按路径查询、删除（可联动备份）。本模块无 Handler，不直接暴露前端，由 task 下载流程、backup、recycleBin 等内部调用。

## 边界

- 与 **resource**：resource 管 Resource 实体（作品的资源元数据抽象）；persistentStore 管具体的文件落盘与 `persistent_store` 记录。
- 与 **backup**：persistentStore 的 `Delete(id, backup)` 可联动 backup 创建备份；StoreBackupOrchestrator 调用本模块的导入 / 删除接口。
- 与 **storeRegistry**：所有存储路径必须以已注册子目录开头（白名单单一源在 storeRegistry，见下），禁止裸路径。

## 对外能力（无 Handler，供内部调用）

| 方法 | 作用 |
| --- | --- |
| `CommitStore` | 提交点建行（下载暂存模式）：文件已由 download 提交点 rename 到最终路径，本方法只建/复用完整行（completed_at 即时置位 + 宽高/头指纹按最终路径提取 + 哈希双列由调用方传入）；同路径已有活行时复用该行全字段覆写（Save 含零值清旧哈希） |
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

## 核心概念

- **已注册子目录**（`storeRegistry`）：路径必须以下列前缀开头——
  `store/resource`（作品资源，含缩略图与合并产物，命名见 `doc/store-naming-convention.md`）、`store/avatar/local`（本地作者头像）、`store/avatar/site`（站点作者头像）。
- **路径基准（PATH_SEPARATOR_DISCIPLINE 两域模型）**：所有相对路径（relPath 域）基于 workDir 且**正斜杠**——写入口（CommitStore/Store/StoreFromExternal）入口处 `ToSlash` 规范化一次，查旧/抑制登记/落库全程与 DB 基准一致；absPath（`filepath.Join(workDir, rel)`）仅存在于 os.* 调用点不回流。禁止 `../`、`./` 或绝对路径。

## 依赖关系

- 依赖：workDir 提供者（根目录）
- 被依赖：**task**（下载资源落盘）、**work**（软删除经 `DeleteWithBackup` 移文件并软删记录，复原经 `RestoreByIds` 复活，purge 经 `DeleteUnscopedByIds` 物理删行）、**taskManager**（替换前置软删 StoreReplacer / 失败回滚派生与复活 StoreBackupReader）、**recycleBin**（复原编排 StoreRestorer + 文件条目清理 StoreCleaner）、**resource**、**fsmonitor**（StoreReader 对账 + 裁决失效经 MarkInvalid）、**assetserver**（/store/ 状态路由 ResolveFileState）、**backupGovernance**（BackupReferencer：引用集投影 `ListReferencedBackupIDs`（Unscoped 含已删行）、悬空清列 `ClearBackupRefsByBackupIDs`、非法活行防御清列）、**extension**（插件库查询：仓储 `ListByIds` 批量读 store 行组装活行摘要，软删行经 scope 自动排除）

## 关键设计

- **记录-文件不变量（2026-08-19 修复，2026-08-20 补 backup_id）**：状态字段如实反映物理世界——`completed_at`（落盘完成时刻，0=未完成）+ `deleted_at`（软删：文件移 backup 或 fsmonitor 外部裁决不复从，复原/裁决链清除）+ `backup_id`（备份清单行引用，软删链与 deleted_at 单条 UPDATE 同生共死写入、复原双列同清，外部删除失效行保持 0）。/store/ 文件服务按状态路由：软删记录按行内 backup_id 查 backup 保管清单定位文件（同路径多代各行指各的，代次隔离）。
- **行只在提交点建（必然完整）**：下载链走暂存模式——内容先写 `{workDir}/task-staging/`（fsmonitor 零感知），全部写满后由 download 提交点 rename 到最终路径并经 `CommitStore` 建行，`completed_at` 即时置位，不产生未完成中间态行。`completed_at=0` 的未完成行为历史中间态遗留（存量库可能有）：/store/ 读取层按 completed 状态路由（未完成 404 防半成品）、对账基线 `ListValidComplete` 仅收 completed_at>0、其磁盘文件列 Untracked 提示，由 recycleBin/replacement 的未完成分流分支与删除链处置。
- **路径强校验**：`storeRegistry.ValidatePath` 拒绝未注册子目录，统一正斜杠比较以兼容 Windows。
- **操作抑制登记（suppression）**：各 Create/Remove/Rename 落盘点（`Store`/`StoreFromExternal`/`Delete`/`DeleteWithBackup`/`CleanupFile`/`CleanupFileResult`）在磁盘操作前 `storeRegistry.Suppress(relPath)` + `defer Release`，让 fsmonitor 把自身写入与外部操作区分开。`CleanupFileResult` 为 `CleanupFile` 的返回错误变体（供删除流「先文件后记录」Phase A 判定文件删除是否真实失败）。backup 模块的 `MoveBackup` 亦在汇点自登记（覆盖所有移入 backup 的调用方）；download 提交点的 rename 窗口由 download 自登记（操作属主纪律）。
- **fsmonitor 查询的软删排除**：store 行软删后经 GORM 自动 scope 从 StoreReader 三方法（`GetByFingerprint`/`GetByFilePathComplete`/`ListValidComplete`）排除——文件软删期间位于 backup/（监控白名单外）。曾用消费侧 JOIN work 的 `notDeletedWorkCond`（persistentStore 越界感知业务实体），已随软删落地删除。fsmonitor 外部裁决不复从经 `MarkInvalid`（软删）实现，原 invalid_at 列退役。
- **图像宽高提取**：`CommitStore`/`Store`/`StoreFromExternal` 落盘后，若是图片（`util.IsImageExt`）则用 `image.DecodeConfig` 读头部解码填入 `Width`/`Height`（供前端瀑布流精准布局）。解码失败仅记日志、留 0，不阻断入库。
