# reWorkTag 模块说明

## 一句话职责

作品 ↔ 标签关联：管理 `re_work_tag` 关联表，按 tagType 区分本地标签与站点标签，支持前端直接增删（Link / Unlink）。属于 reWork* 系列模块。

## 命名渊源

见 [reWorkAuthor 模块说明](../reWorkAuthor/README.md) 的"命名渊源"：`reWork*` 对应 `re_work_*` 关联表（relation of work）。

## 边界

- 与 **reWorkAuthor**：结构同构（作品关联表），交互对称——两者均暴露 `Link` / `Unlink`（另含 `UnlinkDimension` 精确摘维度关联行）供前端作品详情交互式增删，维度列分别为 namespace / role_name。
- 与 **localTag / siteTag**：标签实体由 localTag / siteTag 管理；本模块只管"作品关联了哪些标签"这层关系，tagType=LOCAL 填 LocalTagID，tagType=SITE 填 SiteTagID。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Link(tagType, tagIds, namespaces, workId)` | 批量链接标签到作品（用户手动挂联，建行落 source=MANUAL；upsert：同 work+tag+namespace 冲突仅刷 update_time，否则新增；namespaces 与 tagIds 等长配对，local/site 关联均用调用方传值） |
| `Unlink(tagType, tagIds, workId)` | 批量从作品移除标签（该标签的全部 ns 关联行——维度盲删） |
| `UnlinkDimension(tagType, tagIds, namespaces, workId)` | 精确摘除维度关联行：只删 (work, tag, ns) 命中行、不波及同标签其他 ns 行（改 ns 的旧值行删除入口，新值行走 Link） |
| `ListByWorkId(workId)` | 查询作品关联的所有标签 |
| `ListLocalTagIdsByWorkId(workId)` | 查询作品关联的本地标签ID |
| `ListSiteTagIdsByWorkId(workId)` | 查询作品关联的站点标签ID |

## 核心概念

- **tagType 双层**：`constant.LOCAL`（本地标签，填 LocalTagID）/ `constant.SITE`（站点标签，填 SiteTagID），同一 ReWorkTag 记录二选一。
- **source（关联写入来源）**：`source` 列记关联由谁建立（PLUGIN=插件声明 / MANUAL=用户手动）——work 入库链重拉时只窄域重建插件来源的 SITE 关联（`DeletePluginSiteByWorkId` 删「SITE 且 source=PLUGIN」），用户手动挂的 SITE/LOCAL 关联永不触碰；LOCAL 关联（含插件按名声明挂的）不删、增量追加。
- **批量增删**：Link / Unlink 接收标签ID数组，对应 `LinkBatchToWork` / `RemoveBatchFromWork`。
- **namespace（关联级维度）**：`re_work_tag.namespace` 挂在关联行上（非 tag 实体身份，local_tag/site_tag 实体表无 ns 列），`string not null default ''`、空串=无 ns；唯一键含 ns——同作品同标签多 ns 可达（e-hentai `female:tagA`+`male:tagA` 落两条关联）。Link 的 local/site 关联均用调用方传值（site 轨开放用户自设）；维度值归一化（TrimSpace + 小写折叠）后写入，并同事务登记 `tag_namespace` 清单行（find-or-create + origin 升级 + last_use 刷新，origin=user）。
- **upsert 落库**：`LinkBatchToWork` → `UpsertBatch`（`clause.OnConflict`），按 (work_id, tag_id, namespace) 唯一约束冲突仅刷 update_time，否则 INSERT。冲突更新列不含 source：先建行者的来源保持不变，插件再声明用户手动挂的关联不翻转来源、不重复建行。「已绑定 tag 改 ns」= 新 ns 行走 `Link` + 旧 ns 行走 `UnlinkDimension`（前端 diff 组合两调用），同标签新旧 ns 并存为两条关联。

## 依赖关系

- 依赖：`localTag` / `siteTag` 实体（通过 tagType 关联）；**tagNamespace**（注入 `NamespaceInventoryWriter` 接口，关联写入时同事务登记 ns 清单行）
- 被依赖：前端作品详情（Link / Unlink 交互）、**work**（通过 `ReWorkTagWriter` 读取关联快照、按 work 删除）、**localTag**（删除编排注入 `DeleteByLocalTagId` 清理被删标签挂载的全部关联——`re_work_tag.local_tag_id` 有外键，未清即删标签行被拒）、**siteTag**（删除编排注入 `DeleteBySiteTagId` 清理被删站点标签挂载的全部关联——`re_work_tag.site_tag_id` 有外键，未清即删标签行被拒）
