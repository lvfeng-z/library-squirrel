# work 模块说明

## 一句话职责

作品**核心实体**的业务编排层：管理作品记录的增删改查，并作为关联写入中枢——插件入库链（`SaveWorkInfo`）按来源窄域重建插件声明的关联（SITE 标签/作者轨与作品集成员轨删「source=PLUGIN 且本次未声明」再按本次声明重建，用户来源关联永不触碰；LOCAL 关联增量追加），经接口驱动 reWorkAuthor / reWorkTag / reWorkWorkSet。

## 边界

- 与 **reWorkAuthor / reWorkTag / reWorkWorkSet**：reWork 系列只管"关联怎么存取"；work 决定"什么时候建立关联"（入库链窄域重建），通过 `ReWorkAuthorWriter` / `ReWorkTagReader` 等接口调用，不持有具体 Service。
- 与 **persistentStore / resource**：work 管作品实体与关联编排；资源文件由 persistentStore 存储，Resource 实体由 resource 模块管理。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Save(work)` | 保存作品（记录本身，不碰关联——关联由入库链/前端 Link 入口各自驱动） |
| `Update(work)` | 更新作品（结构体 Updates 跳零值字段，用户字段依赖跳零值不被清） |
| `Delete(id)` | 删除作品记录（裸删，不级联） |
| `SoftDelete(id)` | 软删除（进回收站：文件移 backup + work 行打 deleted_at 标志，从属行原地保留） |
| `GetById` / `QueryPage` | 单查 / 分页 |
| `GetFullWorkInfoByIds(ids)` | 批量获取完整作品信息（含关联） |
| `GetBySiteAndSiteWorkID` | 按站点 + 站点作品ID查询 |
| `ListRankedLocalAuthorWithWorkIdByWorkIds` | 批量查作品关联的本地作者 |
| `UpdateLastUsed(ids)` | 更新最后使用时间 |

## 核心概念

- **关联写入中枢（权威随来源）**：work 持有 reWork 系列的 Writer / Reader 接口。重拉时按关联行 source 列窄域重建插件来源关联——SITE 作者/标签轨删「source=PLUGIN 的 SITE 关联」（`DeletePluginSiteByWorkId`）后按本次声明重建（同 ID 元数据多条 DTO 批内折叠），作品集成员轨删「source=PLUGIN 且本次未声明」（`DeletePluginByWorkIdExcluding`）；用户来源（手动挂联、物理纳入复制）永不触碰，LOCAL 关联增量追加（插件与用户挂的并存）。重建行按声明直写关联级维度（标签 ns / 作者 role，归一化后写入、直写关联行不写实体表），并在同一事务登记维度清单（`tagNamespace`/`authorRole` 的 `EnsureUsedBatch`，origin=plugin）；upsert 冲突（与既有行三元组重合，含用户手动挂的同值关联）仅刷 update_time、不翻转来源。`buildWorkSetLinks` 按各 workSet 当前最大 sort_order +1 续排（纠正维度错位，避免集内塌 0）。
- **重拉行面白名单**：重拉对周边行的更新收口为插件独占权威字段（siteTag/siteAuthor/workSet 仓储 upsert 冲突列白名单）；work 行用户策展字段（nick_name/last_view/local_author_id）依赖「插件不填 + 结构体 Updates 跳零值」双层约定保留（`saveOrUpdateWork` 注释锚定）。
- **周边声明按站点键分轨**：`SaveWorkInfo` 处理站点作者/站点标签/作品集声明时，DTO 的 `siteKey` 缺省（空）或等于作品站点键 = 本站声明，批量 upsert（建行/改行）；声明其他站点键 = 跨站引用，站点键经 site `GetByKey` 按键寻址解析（站点表为 identity 注册表投影，未注册键无行报错），行按 `(站点, 站点侧 ID)` find-only 查回挂联——不写行不造行（键即身份，站点域不按名寻址），行不存在报错并携带站点键与站点侧 ID。
- **原站序拉取编排**：`SaveWorkInfo` 作品入库事务提交后，异步经 `WorkSetOrderFetcher`（plugin 提供，`SetWorkSetOrderFetcher` 延迟注入）拉取作品所属作品集的原站序，映射 siteWorkId→work.id 写 `re_work_work_set.site_sort_order`（ORCHESTRATION_BY_CALLER：编排归入库发起方 work，获取能力归 plugin）。网络调用须事务外（`MaxOpenConns=1` 死锁），故事务提交后异步派发。
- **作品集父集关系拉取编排**：同窗口异步经 `WorkSetRelationFetcher`（plugin 提供，`SetWorkSetRelationFetcher` 延迟注入）拉取作品所属作品集的父集关系，upsert 父集 + 建立父子关系（事务内 `CollectAncestorWorkSetIds` 环路检测）+ 写 `re_work_set_work_set.site_sort_order`（对齐原站序拉取范式）。初始本地序 `sort_order` 取原站序，`SaveRelation` 的 OnConflict DoNothing 保证重复拉取不覆盖用户后续拖拽。
- **软删除（聚合根单表标志）**：`SoftDeleteWork` = 停关联任务 → 事务外逐 store 移文件进 backup（backup.work_id 归属）→ 事务内 work 一条软删 UPDATE（`deleted_at` 毫秒时间戳，soft_delete 插件改写）。从属行（resource / resource_store / re_work_*）与 persistent_store 记录原地保留——复原仅需文件还原 + 清标志，无需重建。业务键唯一性由部分索引 `idx_work_site_site_work_active`（WHERE deleted_at = 0）承担：已删行释放键，删除后可重新下载同作品，复原撞占位作品走「放弃/覆盖」。软删前置查作品锁（`WorkLockChecker`，shareLock 实现）：作品正被分享拉取持有时拒绝（`shareLock.ErrWorkLocked`，强制解锁后重试）。
- **物理删除内部**：`DeleteWorkAndSurroundingData`（首步清 work_set.cover_work_id 指向本作品的封面引用——外键删除防线前置，经 CoverReferenceClearer 由 workSet 仓储实现、覆盖含软删集行 + 级联删从属行 + store 记录行 + work 行）为内部方法，供 recycleBin 彻底删除链调用；对已删行操作须走 `DeleteUnscoped`（GORM 软删 scope 会挡住普通 Delete）。store 记录行在事务内经 `StoreDeleter.DeleteUnscopedByIds` 批量物理删——purge 目标行全为软删行，`HardDelete` 的 GetById 受软删 scope 保护会静默跳过（NotFound 提前返回），早期版本事务后走 HardDelete 曾致每次 Purge 遗留离链孤儿行；原路径文件已移 backup/，文件清理由调用方按行内 backup_id 承担。
- **WorkRestorer**：work 实现 recycleBin 的 WorkRestorer 接口（`SoftDeleteWork` / `GetDeletedWork` / `RestoreDeletedWork` / `ListRevivableWorkStores` / `ListDeletedBefore` / `DeleteWorkAndSurroundingData` / `GetBySiteAndSiteWorkID`），回收站复原/彻底删除/TTL 的原子能力提供方。
- **复活集=同键最新死代**：`RestoreWorkStores`/`ListRevivableWorkStores`（内部 `deriveRevivableStores`）按挂载键 (resource_id, store_type, store_seq) 取最新死代（argmax deleted_at）——关联保留形态下同键多代（替换/merge 残留）无差别复活会令双活行同 file_path 撞部分唯一索引、备份文件还原互相覆盖；作品软删链最后处置的一代即删除时活代，更早代保持死态归回收站文件条目。`ListWorkStoresIncludeDeleted`（全量含删）保留给彻底删除链的备份收集。

## 依赖关系

- 依赖：reWorkAuthor / reWorkTag / reWorkWorkSet（Writer / Reader 接口）、reWorkSetWorkSet（WorkSetRelationWriter：父子关系写入 + 环路检测）、tagNamespace / authorRole（维度清单写入：`TagNamespaceInventoryWriter` / `AuthorRoleInventoryWriter`，入库链关联写入同事务登记）、persistentStore（Store 删除/带归属备份删除）、resource（Resource 保存/删除/resource_store 级联删除）、localTagFindOrCreator、site（SiteReader：周边声明站点键解析——作品站点键判定 + 跨站键 `GetByKey`）、plugin（WorkSetOrderFetcher：原站序获取；WorkSetRelationFetcher：父集关系获取，均延迟注入）、shareLock（WorkLockChecker：软删除前置作品锁守卫）
- 被依赖：前端作品库（列表 / 详情 / 编辑）、taskManager（任务执行后落库作品，`WorkInfoSaver` 接口——`SaveWorkInfo(task, workTask, workResp)` 核心行+作品任务领域行两参）、recycleBin（WorkRestorer：软删/复原/彻底删除原子能力）、search（作品搜索：查询经 BaseRepository 自动排除已删行）、localAuthor（WorkAuthorMirrorClearer：删除本地作者时清 `work.local_author_id` 镜像列——仓储 `ClearLocalAuthorOnWorks` 原生 UPDATE 覆盖含软删行，外键拦截不分行态）、site（WorkSiteRefCounter：站点删除守卫的作品引用计数，仓储 `CountBySiteId` 活行/软删行分别计数）、extension（插件库查询 WorkQuerySource：id/复合键直查 + `PageByFilter` 过滤分页——名称/作者名/标签名/时间范围过滤经 EXISTS 关联子查询落在仓储，作品过滤语义的域内单一落点）
