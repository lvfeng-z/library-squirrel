# work 模块说明

## 一句话职责

作品**核心实体**的业务编排层：管理作品记录的增删改查，并作为关联写入中枢——在保存 / 更新 / 删除作品时，通过接口驱动 reWorkAuthor / reWorkTag / reWorkWorkSet 完成关联的全量替换与快照采集。

## 边界

- 与 **reWorkAuthor / reWorkTag / reWorkWorkSet**：reWork 系列只管"关联怎么存取"；work 决定"什么时候建立关联"（保存 / 更新作品时全量替换），通过 `ReWorkAuthorWriter` / `ReWorkTagReader` 等接口调用，不持有具体 Service。
- 与 **persistentStore / resource**：work 管作品实体与关联编排；资源文件由 persistentStore 存储，Resource 实体由 resource 模块管理。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Save(work)` | 保存作品（含重建关联） |
| `Update(work)` | 更新作品（全量替换关联） |
| `Delete(id)` | 删除作品记录（裸删，不级联） |
| `SoftDelete(id)` | 软删除（进回收站，采集关联快照供恢复） |
| `GetById` / `QueryPage` | 单查 / 分页 |
| `GetFullWorkInfoByIds(ids)` | 批量获取完整作品信息（含关联） |
| `GetBySiteAndSiteWorkID` | 按站点 + 站点作品ID查询 |
| `ListRankedLocalAuthorWithWorkIdByWorkIds` | 批量查作品关联的本地作者 |
| `UpdateLastUsed(ids)` | 更新最后使用时间 |

## 核心概念

- **关联写入中枢**：work 持有 reWork 系列的 Writer / Reader 接口，保存作品时全量替换关联（DeleteByWorkId + SaveBatch）。
- **关联快照**：软删除 / 板块重执行前，采集作品的作者 / 标签 / 作品集关联快照（含 role_name / sort_order / is_cover），用于恢复。
- **逻辑删除对外、物理删除内部**：`SoftDelete`（Handler 暴露）软删除进回收站；物理删除 `DeleteWorkAndSurroundingData` / `HardDeleteWork` 为内部方法，不经 Handler 暴露，供 recycleBin 复原覆盖分支调用。
- **WorkRestorer**：work 实现 recycleBin 的 WorkRestorer 接口（`HardDeleteWork` / `RestoreWorkFromSnapshot` / `GetBySiteAndSiteWorkID`），支撑回收站复原（含引用校验、关联重建）。

## 依赖关系

- 依赖：reWorkAuthor / reWorkTag / reWorkWorkSet（Writer / Reader 接口）、persistentStore（Store 删除/读取）、resource（Resource 保存/删除）、localTagFindOrCreator、backup（移动备份）、recycleBin（RecycleItemSaver：写回收站快照）
- 被依赖：前端作品库（列表 / 详情 / 编辑）、task（任务完成后落库作品）、recycleBin（WorkRestorer：复原重建 / 覆盖删除）
