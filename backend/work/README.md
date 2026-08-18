# work 模块说明

## 一句话职责

作品**核心实体**的业务编排层：管理作品记录的增删改查，并作为关联写入中枢——在保存 / 更新 / 删除作品时，通过接口驱动 reWorkAuthor / reWorkTag / reWorkWorkSet 完成关联的全量替换。

## 边界

- 与 **reWorkAuthor / reWorkTag / reWorkWorkSet**：reWork 系列只管"关联怎么存取"；work 决定"什么时候建立关联"（保存 / 更新作品时全量替换），通过 `ReWorkAuthorWriter` / `ReWorkTagReader` 等接口调用，不持有具体 Service。
- 与 **persistentStore / resource**：work 管作品实体与关联编排；资源文件由 persistentStore 存储，Resource 实体由 resource 模块管理。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Save(work)` | 保存作品（含重建关联） |
| `Update(work)` | 更新作品（全量替换关联） |
| `Delete(id)` | 删除作品记录（裸删，不级联） |
| `SoftDelete(id)` | 软删除（进回收站：文件移 backup + work 行打 deleted_at 标志，从属行原地保留） |
| `GetById` / `QueryPage` | 单查 / 分页 |
| `GetFullWorkInfoByIds(ids)` | 批量获取完整作品信息（含关联） |
| `GetBySiteAndSiteWorkID` | 按站点 + 站点作品ID查询 |
| `ListRankedLocalAuthorWithWorkIdByWorkIds` | 批量查作品关联的本地作者 |
| `UpdateLastUsed(ids)` | 更新最后使用时间 |

## 核心概念

- **关联写入中枢**：work 持有 reWork 系列的 Writer / Reader 接口，保存作品时全量替换关联（DeleteByWorkId + SaveBatch）。`buildWorkSetLinks` 按各 workSet 当前最大 sort_order +1 续排（纠正维度错位，避免集内塌 0）。
- **原站序拉取编排**：`SaveWorkInfo` 作品入库事务提交后，异步经 `WorkSetOrderFetcher`（plugin 提供，`SetWorkSetOrderFetcher` 延迟注入）拉取作品所属作品集的原站序，映射 siteWorkId→work.id 写 `re_work_work_set.site_sort_order`（ORCHESTRATION_BY_CALLER：编排归入库发起方 work，获取能力归 plugin）。网络调用须事务外（`MaxOpenConns=1` 死锁），故事务提交后异步派发。
- **作品集父集关系拉取编排**：同窗口异步经 `WorkSetRelationFetcher`（plugin 提供，`SetWorkSetRelationFetcher` 延迟注入）拉取作品所属作品集的父集关系，upsert 父集 + 建立父子关系（事务内 `CollectAncestorWorkSetIds` 环路检测）+ 写 `re_work_set_work_set.site_sort_order`（对齐原站序拉取范式）。初始本地序 `sort_order` 取原站序，`SaveRelation` 的 OnConflict DoNothing 保证重复拉取不覆盖用户后续拖拽。
- **软删除（聚合根单表标志）**：`SoftDeleteWork` = 停关联任务 → 事务外逐 store 移文件进 backup（backup.work_id 归属）→ 事务内 work 一条软删 UPDATE（`deleted_at` 毫秒时间戳，soft_delete 插件改写）。从属行（resource / resource_store / re_work_*）与 persistent_store 记录原地保留——复原仅需文件还原 + 清标志，无需重建。业务键唯一性由部分索引 `idx_work_site_site_work_active`（WHERE deleted_at = 0）承担：已删行释放键，删除后可重新下载同作品，复原撞占位作品走「放弃/覆盖」。
- **物理删除内部**：`DeleteWorkAndSurroundingData`（级联删从属行 + work 行 + store 文件）为内部方法，供 recycleBin 彻底删除链调用；对已删行操作须走 `DeleteUnscoped`（GORM 软删 scope 会挡住普通 Delete）。
- **WorkRestorer**：work 实现 recycleBin 的 WorkRestorer 接口（`SoftDeleteWork` / `GetDeletedWork` / `RestoreDeletedWork` / `ListDeletedBefore` / `DeleteWorkAndSurroundingData` / `GetBySiteAndSiteWorkID`），回收站复原/彻底删除/TTL 的原子能力提供方。

## 依赖关系

- 依赖：reWorkAuthor / reWorkTag / reWorkWorkSet（Writer / Reader 接口）、reWorkSetWorkSet（WorkSetRelationWriter：父子关系写入 + 环路检测）、persistentStore（Store 删除/带归属备份删除）、resource（Resource 保存/删除/resource_store 级联删除）、localTagFindOrCreator、plugin（WorkSetOrderFetcher：原站序获取；WorkSetRelationFetcher：父集关系获取，均延迟注入）
- 被依赖：前端作品库（列表 / 详情 / 编辑）、task（任务完成后落库作品）、recycleBin（WorkRestorer：软删/复原/彻底删除原子能力）、search（作品搜索：查询经 BaseRepository 自动排除已删行）
