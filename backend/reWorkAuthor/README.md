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
| `Link(authorType, authorIds, roleNames, workId)` | 用户手动挂联作者（roleNames 与 authorIds 等长配对，空串=无 role；同作品同作者不同 role 落独立关联行——身兼数职），冲突不翻转来源 |
| `Unlink(authorType, authorIds, workId)` | 批量从作品移除作者关联（该作者的全部 role 关联行——维度盲删） |
| `UnlinkDimension(authorType, authorIds, roleNames, workId)` | 精确摘除维度关联行：只删 (work, author, role) 命中行、不波及同作者其他 role 行（改 role 的旧值行删除入口，新值行走 Link） |
| `ListByWorkId(workId)` | 获取单个作品关联的本地 + 站点作者 |
| `ListByWorkIds(workIds)` | 批量获取多个作品的作者关联 |
| `ListLocalAuthorsByWorkId(workId)` | 查询作品关联的本地作者 |
| `ListSiteAuthorsByWorkId(workId)` | 查询作品关联的站点作者 |
| `ListRankedLocalAuthorWithWorkIdByWorkIds(workIds)` | 批量查询本地作者（带作品ID） |
| `ListRankedSiteAuthorWithWorkIdByWorkIds(workIds)` | 批量查询站点作者（带作品ID） |

> `Link`/`Unlink`/`UnlinkDimension` 走用户手动挂联链（source=MANUAL，批量 upsert 按 (work, author, role) 冲突仅刷 sort_order/update_time、不含 source）；入库链写入（`DeleteByWorkId` / `DeletePluginSiteByWorkId` / `SaveBatchOnConflict`）不暴露给前端，由 work 通过 `ReWorkAuthorWriter` 接口调用；`DeleteByLocalAuthorId` 由 localAuthor 删除编排调用（删本地作者时清其全部作品关联）；`DeleteBySiteAuthorId` 由 siteAuthor 删除编排调用（删站点作者时清其全部作品关联）。

## 核心概念

- **本地作者 / 站点作者双层**：同一作品可同时关联本地作者（用户体系）与站点作者（pixiv 等），DTO 分别为 `RankedLocalAuthor` / `RankedSiteAuthor`。
- **role_name（关联级维度）**：作者在本作品中的角色（如原作、脚本、声优），`string not null default ''`、空串=无 role——与 reWorkTag 的 namespace 同构（维度挂关联行、不进作者实体表）。维度值归一化（TrimSpace + 小写折叠）后写入，并同事务登记 `author_role` 清单行（find-or-create + origin 升级 + last_use 刷新，origin=user）。
- **sort_order**：作者在作品中的展示排序。
- **增量同步（权威随来源）**：work 入库链写入时，SITE 轨按来源窄域重建——删「SITE 且 source=PLUGIN」的关联（`DeletePluginSiteByWorkId`）后按本次声明重建（`SaveBatchOnConflict`——插件元数据对同一作者产出多条同 ID DTO 时批内重复折叠为单条关联），LOCAL 关联增量保留（已存在跳过），用户手动挂的关联（source=MANUAL，`Link`/`Unlink`/`UnlinkDimension`）不在清理域、重拉不动；冲突 upsert（`UpsertBatch`）不写 source 列，先建行者的来源保持不变。`(work_id, local_author_id, role_name)` / `(work_id, site_author_id, role_name)` 唯一索引（含 role）是增量去重与「同作品同作者多 role」的约束保障。

## 依赖关系

- 依赖：`localAuthor`、`siteAuthor` 实体（通过关联表 JOIN 查询作者信息）；**authorRole**（注入 `RoleInventoryWriter` 接口，关联写入时同事务登记 role 清单行）
- 被依赖：**work**（通过 `ReWorkAuthorWriter` / `ReWorkAuthorReader` 接口写入与快照采集）、**localAuthor**（删除本地作者时通过 `DeleteByLocalAuthorId` 清作品关联）、**siteAuthor**（删除站点作者时通过 `DeleteBySiteAuthorId` 清作品关联）、前端作品详情展示（通过 Handler 查询）
