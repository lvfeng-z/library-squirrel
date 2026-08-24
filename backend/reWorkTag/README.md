# reWorkTag 模块说明

## 一句话职责

作品 ↔ 标签关联：管理 `re_work_tag` 关联表，按 tagType 区分本地标签与站点标签，支持前端直接增删（Link / Unlink）。属于 reWork* 系列模块。

## 命名渊源

见 [reWorkAuthor 模块说明](../reWorkAuthor/README.md) 的"命名渊源"：`reWork*` 对应 `re_work_*` 关联表（relation of work）。

## 边界

- 与 **reWorkAuthor**：结构同构（作品关联表），但交互方式不同——
  - **reWorkTag 暴露写入**（`Link` / `Unlink`）：标签由用户在作品详情页交互式增删；
  - **reWorkAuthor 只读**（Handler 无写入）：作者关联由 work 在保存作品时按 SITE 删后重建、LOCAL 增量保留。
- 与 **localTag / siteTag**：标签实体由 localTag / siteTag 管理；本模块只管"作品关联了哪些标签"这层关系，tagType=LOCAL 填 LocalTagID，tagType=SITE 填 SiteTagID。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Link(tagType, tagIds, namespaces, workId)` | 批量链接标签到作品（upsert：同 work+tag 已存在则更新 namespace，否则新增；namespace 来源按 tagType 区分，见下） |
| `Unlink(tagType, tagIds, workId)` | 批量从作品移除标签 |
| `ListByWorkId(workId)` | 查询作品关联的所有标签 |
| `ListLocalTagIdsByWorkId(workId)` | 查询作品关联的本地标签ID |
| `ListSiteTagIdsByWorkId(workId)` | 查询作品关联的站点标签ID |

## 核心概念

- **tagType 双层**：`constant.LOCAL`（本地标签，填 LocalTagID）/ `constant.SITE`（站点标签，填 SiteTagID），同一 ReWorkTag 记录二选一。
- **批量增删**：Link / Unlink 接收标签ID数组，对应 `LinkBatchToWork` / `RemoveBatchFromWork`。
- **namespace（关联级属性）**：`re_work_tag.namespace` 挂在关联上（非 tag 实体身份）。Link 时 local 关联用前端传的 namespaces（用户自设，与 tagIds 等长配对，空串→NULL）；site 关联由后端按所指 `site_tag.namespace` 镜像（忽略前端传值）。
- **upsert 落库**：`LinkBatchToWork` → `UpsertBatch`（`clause.OnConflict`），按 (work_id, tag_id) 唯一约束冲突时 UPDATE namespace，否则 INSERT——支持「已绑定 tag 改 namespace 重新确认」（前端编辑 ns 后移入待确认缓冲区，确认走 upsert 更新而非重复插入）。

## 依赖关系

- 依赖：`localTag` / `siteTag` 实体（通过 tagType 关联）；**siteTag**（注入 `SiteTagNamespaceReader` 接口，site 关联镜像 `site_tag.namespace`，复用其 `ListBySiteTagIds`）
- 被依赖：前端作品详情（Link / Unlink 交互）、**work**（通过 `ReWorkTagWriter` 读取关联快照、按 work 删除）、**localTag**（删除编排注入 `DeleteByLocalTagId` 清理被删标签挂载的全部关联——`re_work_tag.local_tag_id` 有外键，未清即删标签行被拒）、**siteTag**（删除编排注入 `DeleteBySiteTagId` 清理被删站点标签挂载的全部关联——`re_work_tag.site_tag_id` 有外键，未清即删标签行被拒）
