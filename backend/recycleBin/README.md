# recycleBin 模块说明

## 一句话职责

回收站（已删实体通用管理器）：软删除模型下管理两类条目——**作品条目**（work 已删行聚合其内：列表查询、复原、彻底删除）与**文件条目**（persistent_store 已删行且非「作品已删」聚合形态：列表查询、版本回滚复原、彻底清理），以及共享保留期的 TTL 自动清理。

## 边界

- 与 **work**：work 实现 `WorkRestorer` 接口（软删/复原清标志/冲突查询/物理级联删除），recycleBin 负责复原与彻底删除的**编排**（业务键冲突裁决、文件还原顺序、backup 清理）。软删除入口 `work.SoftDeleteWork` 由 work 模块直接暴露给前端，不经本模块。
- 与 **search**：作品条目查询转发 `QueryRecycleWorkPage`（条件体系与作品搜索同构，基线 `deleted_at > 0`）；文件条目查询转发 `QueryRecycleStorePage` + `ListRecycleStoreIdsDeletedBefore`（TTL 圈定）——文件域条件体系（文件名/路径模糊、媒体类型、备份状态、作品名、删除时间）与作品条目的 SearchCondition 标签体系**分轨**，筛选器按前端 tab 切换查询模型。
- 与 **persistentStore**：文件条目清理链经 `StoreCleaner` 接口（GetDeletedStore 查已删行 / CleanupFile 尽力删文件 / DeleteUnscopedByIds 物理删行），复原置换链经 `StoreRestorer` 接口（GetById/GetByFilePath 活行查询 / DeleteWithBackup 与 SoftDeleteAndDiscardFile 置换软删 / RestoreByIds 复活）。
- 与 **resource**：复原置换后经 `ResourceRecomputer` 重算资源完整度（角色构成可能变化，如合并回滚补回轨道）。
- 与 **backup**：软删除时资源文件移入 backup/（persistent_store 行内 backup_id 引用保管清单行），复原时经 `RestoreFile` 还原回 store/ 原路径并删备份记录，彻底删除/清理时按行内 backup_id 消费式删备份文件与记录。

## 对外接口（Handler）

方法名显式表达条目实体归属（Works=作品条目、Stores=文件条目），为作品集条目（WorkSets）的并列扩展留语义空间。

| 方法 | 作用 |
| --- | --- |
| `PageWorks(page, query)` | 分页查询作品条目（query.conditions 为 SearchCondition 条件体系 + sortBy/sortOrder） |
| `RestoreWork(workId, overwrite)` | 复原已软删作品（overwrite 控制冲突时占位作品转入回收站） |
| `PurgeWork(workId)` | 彻底删除已软删作品（不可恢复，级联清从属行与备份） |
| `PageStores(page, query)` | 分页查询文件条目（query 为 RecycleStorePageQuery 文件域条件体系） |
| `RestoreStore(storeId)` | 复原文件条目（版本回滚置换：行内备份还原为当前版本，被置换的当前活行转入回收站） |
| `PurgeStore(storeId)` | 彻底删除文件条目（不可恢复，条目单位=store 行，含消费式删备份） |

> 逻辑删除入口 `work.SoftDelete`（handler）由 work 模块暴露；TTL 自动清理由后台 goroutine 内部调用 PurgeWork/PurgeStore，不经 Handler。快照时代的 recycle_bin 表与仓储已随快照体系整体移除（软件未发布，无兼容保留）。

## 核心概念

- **作品条目 = work 已删行**：work.deleted_at（毫秒时间戳）> 0 即作品条目，挂载链软删 store 聚合其内；删除时间、TTL 过期判定、删除时间排序均由该列承担。从属行（resource/resource_store/re_work_*）与 persistent_store 记录在软删期间原地保留，复原无需重建。列表 DTO 含剩余天数（service 按 TTL 设置组装）与预览图路径（与作品卡片同优先级：缩略图优先、图片资源主图回退；软删期间文件在 backup/，经 /store/ 的 backup 兜底仍可访问）。
- **文件条目 = persistent_store 已删行 ∧ 非「作品已删」聚合形态**（`work 不可达 ∨ work 存活`）：条目单位是 store 行非作品，TTL/过期按行自身 deleted_at，作品仅提供上下文展示。三类来源——MarkInvalid 失效行（外部裁决失效，backup_id=0，由此获得可见性与 TTL 终态）、离链孤儿（挂载链断的历史残迹，自愈落入）、替换/merge 软删残留（J' 软删化落地后的例行态）。**备份圈定按行内引用**：行内 backup_id 定位备份（同路径多代互不干扰）。
- **可复原性状态**：`CanRestore = backup_id>0 ∧ 挂载链可达（活作品）`。`RestoreStore`（版本回滚置换）据此守卫：同键 (resource,role,seq) 活行先软删入回收站（可再复原——回滚即一次替换，机制单态；未完成占位行走废弃分支不入备份），再还原行内备份到原路径、复活本行（双列清）、删备份清单行、重算资源完整度。关联零操作：本行关联保留（复活即挂载回位）、被置换行关联保留成死（双关联标准形态）。
- **复活集=同键最新死代**：作品条目复原按 (resource_id, store_type, store_seq) 键取最新死代复活（`ListRevivableWorkStores` 与 work 共用派生）——关联保留形态下同键多代无差别复活会令双活行同 file_path 撞部分唯一索引、备份文件还原互相覆盖；更早死代保持死态留在文件条目。
- **复原冲突**（作品条目）：删除后重新下载同作品会占用业务键（部分唯一索引 `idx_work_site_site_work_active` 仅约束活行，已删行释放键）。复原时检测到活占位行 → 放弃（报 ErrRestoreConflict）或覆盖（占位作品转入回收站，文件移 backup 让出 store/ 路径，反悔可再复原）。
- **文件还原的操作抑制**：还原目标在 store/ 监控白名单内，逐文件 storeRegistry.Suppress/Release 登记避免被 fsmonitor 误报为外部变更。

## 依赖关系

- 依赖：work（WorkRestorer：含 ListRevivableWorkStores 复活集派生）、backup（BackupReader：GetById/GetBackupPath/RestoreFile/DeleteBackup）、search（RecycleWorkQuerier：QueryRecycleWorkPage；RecycleStoreQuerier：QueryRecycleStorePage/ListRecycleStoreIdsDeletedBefore/GetRecycleStoreMount/GetAliveStoreIdByKey）、persistentStore（StoreCleaner + StoreRestorer）、resource（ResourceRecomputer）、settings（TTL 配置 + workDir）
- 被依赖：前端回收站页面（作品/文件双 tab）

## 关键设计

- **编排归发起方**：复原/彻底删除/清理的流程编排（校验→冲突裁决→文件→DB）在本模块，原子能力经接口注入（work/backup/persistentStore 提供）。
- **查询完全复用**：列表不自带查询实现，转发 search（作品条目=EXISTS 条件体系 + 精简投影；文件条目=谓词 `deleted_at>0 ∧ NOT EXISTS(挂载链指向已软删作品)` + 行本体投影 + 挂载上下文二段批查组装 CanRestore/作品字段）。
- **TTL 两轮清理**：第一轮过期作品条目（PurgeWork 级联），第二轮过期文件条目（圈定与列表同谓词——「作品已删」聚合行不被圈定，保护作品条目复原能力）；两类条目共享 `settings.recycleBin.retentionDays` 保留期。
- **文件条目清理的终态清理义务**：物理删行 + 消费式删备份 + 尽力删行内 file_path 指向的文件（正常软删行扑空无害；file_path 指向 backup/ 域的残迹行——无保管清单行的散落文件——随行清除，不删即不可见垃圾）。不产生「删行不清备份」的通路（backupGovernance 引用集语义不受破坏）。
- **best-effort 文件链**：文件还原在清标志前、失败警告后继续（部分复原语义）；彻底删除的级联链内 store 原路径文件删除对已移 backup 的文件扑空无害。
