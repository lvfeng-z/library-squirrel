# recycleBin 模块说明

## 一句话职责

回收站（已删实体通用管理器）：软删除模型下管理三类条目——**作品条目**（work 已删行聚合其内：列表查询、复原、彻底删除）、**文件条目**（persistent_store 已删行且非「作品已删」聚合形态：列表查询、版本回滚复原、彻底清理）与**作品集条目**（work_set 已删行：列表查询、复原、彻底删除），以及共享保留期的 TTL 自动清理。

## 边界

- 与 **work**：work 实现 `WorkRestorer` 接口（软删/复原清标志/冲突查询/物理级联删除），recycleBin 负责复原与彻底删除的**编排**（业务键冲突裁决、文件还原顺序、backup 清理）。软删除入口 `work.SoftDeleteWork` 由 work 模块直接暴露给前端，不经本模块。
- 与 **workSet**：workSet 实现 `WorkSetRestorer` 接口（软删/已删查询/冲突查询/清标志/级联删除）；作品集条目复原=冲突裁决+清标志（无文件无从属复活段），软删入口 `workSet.SoftDeleteWorkSet` 由 workSet 模块直接暴露。
- 与 **search**：作品条目查询转发 `QueryRecycleWorkPage`（条件体系与作品搜索同构，基线 `deleted_at > 0`）；文件条目查询转发 `QueryRecycleStorePage` + `ListRecycleStoreIdsDeletedBefore`（TTL 圈定）——文件域条件体系（文件名/路径模糊、媒体类型、备份状态、作品名、删除时间）与作品条目的 SearchCondition 标签体系**分轨**，筛选器按前端 tab 切换查询模型；作品集条目查询转发 `QueryRecycleWorkSetPage`（作品集域平铺条件：名称/站点/删除时间，与另两类分轨）。
- 与 **persistentStore**：文件条目清理链经 `StoreCleaner` 接口（GetDeletedStore 查已删行 / CleanupFile 尽力删文件 / DeleteUnscopedByIds 物理删行），复原置换链经 `StoreRestorer` 接口（GetById/GetByFilePath 活行查询 / DeleteWithBackup 与 SoftDeleteAndDiscardFile 置换软删 / RestoreByIds 复活）。
- 与 **resource**：复原置换后经 `ResourceRecomputer` 重算资源完整度（角色构成可能变化，如合并回滚补回轨道）；文件条目置换的作品锁守卫经 `ResourceWorkReader` 反查挂载资源所属作品。
- 与 **shareLock**：复原覆盖转移与文件条目置换前置经 `WorkLockChecker` 接口查作品锁（shareLock.ShareLockRegistry 实现），作品正被分享拉取持有时拒绝执行；强制解锁 `ForceUnlockWork` 由 Handler 直通注册中心。
- 与 **backup**：软删除时资源文件移入 backup/（persistent_store 行内 backup_id 引用保管清单行），复原时经 `RestoreFile` 还原回 store/ 原路径并删备份记录，彻底删除/清理时**两阶段**（先 `DeleteBackupFile` 删文件、再 `DeleteBackupRecord` 删记录）消费式删备份——文件删不动即中止、记录保留，由前端询问用户仅删记录或放弃。作品集条目无文件无备份面。

## 对外接口（Handler）

方法名显式表达条目实体归属（Works=作品条目、Stores=文件条目、WorkSets=作品集条目）。

| 方法 | 作用 |
| --- | --- |
| `PageWorks(page, query)` | 分页查询作品条目（query.conditions 为 SearchCondition 条件体系 + sortBy/sortOrder） |
| `RestoreWork(workId, overwrite)` | 复原已软删作品（overwrite 控制冲突时占位作品转入回收站） |
| `PurgeWork(workId)` | 彻底删除已软删作品（不可恢复，级联清从属行与备份）。**两阶段**：先删文件（store 原路径 + backup 文件），任一真实失败即返回「文件删除失败（记录已保留）」、记录未动；成功才删记录（`DeleteWorkAndSurroundingData` + 备份记录）。前端据此询问「仅删记录或放弃」（`PurgeWorkRecords`） |
| `PurgeWorkRecords(workId)` | 仅删除作品条目记录（不动磁盘文件）——文件删除失败后用户明确选择「仅删记录」的降级路径 |
| `PageStores(page, query)` | 分页查询文件条目（query 为 RecycleStorePageQuery 文件域条件体系） |
| `RestoreStore(storeId)` | 复原文件条目（版本回滚置换：行内备份还原为当前版本，被置换的当前活行转入回收站） |
| `PurgeStore(storeId)` | 彻底删除文件条目（不可恢复，条目单位=store 行）。**两阶段**：先删文件（行 file_path + 行内备份文件），任一真实失败即返回「文件删除失败（记录已保留）」、记录未动；成功才事务删行（先摘 resource_store 关联再物理删行）+ 备份记录。前端据此询问「仅删记录或放弃」（`PurgeStoreRecords`） |
| `PurgeStoreRecords(storeId)` | 仅删除文件条目记录（不动磁盘文件）——文件删除失败后用户明确选择「仅删记录」的降级路径 |
| `PageWorkSets(page, query)` | 分页查询作品集条目（query 为 RecycleWorkSetPageQuery 作品集域平铺条件体系） |
| `RestoreWorkSet(workSetId, overwrite)` | 复原已软删作品集（overwrite 控制冲突时占位作品集转入回收站；本地手建集键 NULL 无冲突） |
| `PurgeWorkSet(workSetId)` | 彻底删除已软删作品集（不可恢复，级联清成员关联与父子关联行） |
| `ForceUnlockWork(workId)` | 强制解锁作品锁——清除该作品的全部分享拉取会话引用。被 `shareLock.ErrWorkLocked` 拒绝的操作（复原覆盖转移、文件条目置换、替换前置软删）在用户知情确认强制继续后调用本方法，重试原操作即放行 |

> 逻辑删除入口 `work.SoftDelete` / `workSet.SoftDeleteWorkSet`（handler）由各模块暴露；TTL 自动清理由后台 goroutine 内部调用 PurgeWork/PurgeStore/PurgeWorkSet，不经 Handler。快照时代的 recycle_bin 表与仓储已随快照体系整体移除（软件未发布，无兼容保留）。

## 核心概念

- **作品条目 = work 已删行**：work.deleted_at（毫秒时间戳）> 0 即作品条目，挂载链软删 store 聚合其内；删除时间、TTL 过期判定、删除时间排序均由该列承担。从属行（resource/resource_store/re_work_*）与 persistent_store 记录在软删期间原地保留，复原无需重建。列表 DTO 含剩余天数（service 按 TTL 设置组装）与预览图路径（与作品卡片同优先级：缩略图优先、图片资源主图回退；软删期间文件在 backup/，经 /store/ 的 backup 兜底仍可访问）。
- **文件条目 = persistent_store 已删行 ∧ 非「作品已删」聚合形态**（`work 不可达 ∨ work 存活`）：条目单位是 store 行非作品，TTL/过期按行自身 deleted_at，作品仅提供上下文展示。三类来源——MarkInvalid 失效行（外部裁决失效，backup_id=NULL，由此获得可见性与 TTL 终态）、离链孤儿（挂载链断的历史残迹，自愈落入）、替换/merge 软删残留（J' 软删化落地后的例行态）。**备份圈定按行内引用**：行内 backup_id 定位备份（同路径多代互不干扰）。
- **可复原性状态**：`CanRestore = 行内引用备份（backup_id 非空）∧ 挂载链可达（活作品）`。`RestoreStore`（版本回滚置换）据此守卫：同键 (resource,role,seq) 活行先软删入回收站（可再复原——回滚即一次替换，机制单态；未完成占位行走废弃分支不入备份），再还原行内备份到原路径、复活本行（双列清）、删备份清单行、重算资源完整度。关联零操作：本行关联保留（复活即挂载回位）、被置换行关联保留成死（双关联标准形态）。
- **复活集=同键最新死代**：作品条目复原按 (resource_id, store_type, store_seq) 键取最新死代复活（`ListRevivableWorkStores` 与 work 共用派生）——关联保留形态下同键多代无差别复活会令双活行同 file_path 撞部分唯一索引、备份文件还原互相覆盖；更早死代保持死态留在文件条目。
- **复原冲突**（作品条目）：删除后重新下载同作品会占用业务键（部分唯一索引 `idx_work_site_site_work_active` 仅约束活行，已删行释放键）。复原时检测到活占位行 → 放弃（报 ErrRestoreConflict）或覆盖（占位作品转入回收站，文件移 backup 让出 store/ 路径，反悔可再复原）。
- **作品集条目 = work_set 已删行**：删除时间、TTL 按 work_set.deleted_at；成员关联（re_work_work_set）与父子关联（re_work_set_work_set）行**原地保留**——复原零成本（清标志即全恢复，层级/成员/封面全在），彻底删除才级联清理。活成员数按 work 活行计数（已删成员不计，作品复原后自动回位）。**复原冲突**：业务键被同键新活集占位（重新下载场景，三列唯一索引下活行占键）→ 放弃或覆盖（占位集转回收站）；本地手建集（键 NULL）不参与唯一性、无冲突可能。**注意三列索引的毫秒约束**：同键两代死行删除时刻互异（同毫秒双删撞索引报错，显式失败非静默损坏，现实操作无同毫秒产道）。
- **文件还原的操作抑制**：还原目标在 store/ 监控白名单内，逐文件 storeRegistry.Suppress/Release 登记避免被 fsmonitor 误报为外部变更。
- **作品锁守卫（分享拉取中）**：复原覆盖转移与文件条目置换会移走作品的活行 store 文件，作品正被分享拉取持有时在途拉取会读到源文件消失，两链前置查锁、命中返回 `shareLock.ErrWorkLocked`（覆盖转移按占位作品、置换经挂载资源反查所属作品）。锁为防误触软防护：资源行缺失或反查异常时告警放行，不因守卫自身故障阻断复原。

## 依赖关系

- 依赖：work（WorkRestorer：含 ListRevivableWorkStores 复活集派生）、workSet（WorkSetRestorer：软删/复原/级联）、backup（BackupReader：GetById/GetBackupPath/RestoreFile/DeleteBackup/DeleteBackupFile/DeleteBackupRecord）、search（RecycleWorkQuerier：QueryRecycleWorkPage；RecycleStoreQuerier：QueryRecycleStorePage/ListRecycleStoreIdsDeletedBefore/GetRecycleStoreMount/GetAliveStoreIdByKey；RecycleWorkSetQuerier：QueryRecycleWorkSetPage）、persistentStore（StoreCleaner + StoreRestorer）、resource（ResourceRecomputer + ResourceWorkReader=Service 行查询反查所属作品 + StoreAssociationCleaner=ResourceStoreRepository 关联摘除）、database（Transactor 事务执行器）、settings（TTL 配置 + workDir）、shareLock（WorkLockChecker 作品锁守卫，Handler 另直通 ShareLockRegistry 供 ForceUnlockWork）
- 被依赖：前端回收站页面（作品/文件/作品集三 tab）

## 关键设计

- **编排归发起方**：复原/彻底删除/清理的流程编排（校验→冲突裁决→文件→DB）在本模块，原子能力经接口注入（work/workSet/backup/persistentStore 提供）。
- **查询完全复用**：列表不自带查询实现，转发 search（作品条目=EXISTS 条件体系 + 精简投影；文件条目=谓词 `deleted_at>0 ∧ NOT EXISTS(挂载链指向已软删作品)` + 行本体投影 + 挂载上下文二段批查组装 CanRestore/作品字段；作品集条目=谓词 `deleted_at>0` + 活成员数子查询）。
- **TTL 三轮清理**：第一轮过期作品条目（PurgeWork 级联），第二轮过期文件条目（圈定与列表同谓词——「作品已删」聚合行不被圈定，保护作品条目复原能力），第三轮过期作品集条目（PurgeWorkSet 级联清关联行）；三类条目共享 `settings.recycleBin.retentionDays` 保留期。
- **文件条目清理的终态清理义务**：事务内先摘指向该行的 resource_store 关联再物理删行（外键强制下的删除前置义务；关联摘除经 resource 的 ResourceStoreRepository `DeleteByStoreIds` 能力注入）+ 消费式删备份 + 尽力删行内 file_path 指向的文件（正常软删行扑空无害；file_path 指向 backup/ 域的残迹行——无保管清单行的散落文件——随行清除，不删即不可见垃圾）。不产生「删行不清备份」的通路（backupGovernance 引用集语义不受破坏）。
- **best-effort 文件链**：文件还原在清标志前、失败警告后继续（部分复原语义）；彻底删除的级联链内 store 原路径文件删除对已移 backup 的文件扑空无害。
