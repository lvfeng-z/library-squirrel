# reWorkAuthor 模块说明

## 一句话职责

作品 ↔ 作者关联的**存取中枢**：管理多对多关联表 `re_work_author`，同时覆盖本地作者与站点作者两条链路，支持角色（role_name）与排序（sort_order）。本模块负责"关联怎么存取"，"何时建立关联"由 work 模块决定。

## 命名渊源

`reWork*` 系列模块对应数据库关联表 `re_work_*`（relation of work），即"作品与其他实体的关系表"。共三个同构模块：

- **reWorkAuthor**：作品 ↔ 作者（本模块）
- **reWorkTag**：作品 ↔ 标签，Handler 暴露 `Link`/`Unlink` 写入，前端直接调用
- **reWorkWorkSet**：作品 ↔ 作品集，仅 repository 无 Handler，被其他模块内部调用，含 is_cover 封面标记

## 边界

- 与 **localAuthor / siteAuthor**：作者**实体**（名称、头像等）由 `localAuthor`/`siteAuthor` 管理；本模块只管"作品关联了哪些作者"这层关系，作者详情到对应作者模块取。
- 与 **work**：work 决定"什么时候建立关联"（入库链按来源窄域重建：SITE 插件来源关联删后重建、LOCAL 关联增量保留），本模块提供存取能力。与 reWorkTag 对称，用户手动挂联（`Link`/`Unlink`）由前端直接调用。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Link(authorType, authorIds, roleNames, workId)` | 用户手动挂联作者（可选角色，与 authorIds 等长配对，空串角色落 NULL），冲突不翻转来源 |
| `Unlink(authorType, authorIds, workId)` | 批量从作品移除作者关联 |
| `ListByWorkId(workId)` | 获取单个作品关联的本地 + 站点作者 |
| `ListByWorkIds(workIds)` | 批量获取多个作品的作者关联 |
| `ListLocalAuthorsByWorkId(workId)` | 查询作品关联的本地作者 |
| `ListSiteAuthorsByWorkId(workId)` | 查询作品关联的站点作者 |
| `ListRankedLocalAuthorWithWorkIdByWorkIds(workIds)` | 批量查询本地作者（带作品ID） |
| `ListRankedSiteAuthorWithWorkIdByWorkIds(workIds)` | 批量查询站点作者（带作品ID） |

> `Link`/`Unlink` 走用户手动挂联链（source=MANUAL，批量 upsert 冲突刷 role_name/sort_order 不含 source）；入库链写入（`DeleteByWorkId` / `DeletePluginSiteByWorkId` / `SaveBatchOnConflict`）不暴露给前端，由 work 通过 `ReWorkAuthorWriter` 接口调用；`DeleteByLocalAuthorId` 由 localAuthor 删除编排调用（删本地作者时清其全部作品关联）；`DeleteBySiteAuthorId` 由 siteAuthor 删除编排调用（删站点作者时清其全部作品关联）。

## 核心概念

- **本地作者 / 站点作者双层**：同一作品可同时关联本地作者（用户体系）与站点作者（pixiv 等），DTO 分别为 `RankedLocalAuthor` / `RankedSiteAuthor`。
- **role_name**：作者在本作品中的角色（如原作、系列作者）。
- **sort_order**：作者在作品中的展示排序。
- **增量同步（权威随来源）**：work 入库链写入时，SITE 轨按来源窄域重建——删「SITE 且 source=PLUGIN」的关联（`DeletePluginSiteByWorkId`）后按本次声明重建（`SaveBatchOnConflict`——插件元数据对同一作者产出多条同 ID DTO 时批内重复折叠为单条关联），LOCAL 关联增量保留（已存在跳过），用户手动挂的关联（source=MANUAL，`Link`/`Unlink`）不在清理域、重拉不动；冲突 upsert（`UpsertBatch`）不写 source 列，先建行者的来源保持不变。`(work_id, local_author_id)` / `(work_id, site_author_id)` 唯一索引是增量去重的约束保障（SQLite NULL 不参与唯一性，LOCAL/SITE 两类互不冲突）。

## 依赖关系

- 依赖：`localAuthor`、`siteAuthor` 实体（通过关联表 JOIN 查询作者信息）
- 被依赖：**work**（通过 `ReWorkAuthorWriter` / `ReWorkAuthorReader` 接口写入与快照采集）、**localAuthor**（删除本地作者时通过 `DeleteByLocalAuthorId` 清作品关联）、**siteAuthor**（删除站点作者时通过 `DeleteBySiteAuthorId` 清作品关联）、前端作品详情展示（通过 Handler 查询）
