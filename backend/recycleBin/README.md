# recycleBin 模块说明

## 一句话职责

作品回收站：管理逻辑删除作品的快照与生命周期，支持从快照 100% 复原作品（含作者/标签/作品集关联与资源文件）、彻底删除、以及超过保留时长的自动清理。

## 边界

- 与 **work**：work 执行逻辑删除（采集快照 + 删原记录）并实现复原能力（WorkRestorer 接口）；recycleBin 负责快照存储、复原编排、彻底删除、TTL 清理。两者通过接口双向协作，构造上 recycleBin 在 work 之后创建以注入 WorkRestorer。
- 与 **backup**：recycleBin 复原时从 backup 还原资源文件（StoreFromExternal），彻底删除时删 backup 文件与记录；快照本身（recycle_bin 表）只存关联元数据，资源文件级信息由 backup 记录承载。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Page(page, query)` | 分页查询回收站列表（query：删除/创建时间范围、站点、作者、标签筛选 + 时间列排序） |
| `Restore(id, overwrite)` | 从回收站复原作品（overwrite 控制冲突覆盖） |
| `Purge(id)` | 彻底删除回收站条目（不可恢复） |

> 逻辑删除入口 `work.SoftDelete` 由 work 模块暴露，写入回收站；TTL 自动清理由后台 goroutine 内部调用 Purge，不经 Handler。

## 核心概念

- **快照（WorkRecycleSnapshot）**：逻辑删除时序列化作品的全部关联元数据（work 字段 + 作者/标签/作品集关联 + resource 业务字段 + backup id 映射），存入 recycle_bin.snapshot。复原只依赖快照。作品集关联快照（`WorkSetSnapshot`）含 `is_cover` / `sort_order`（本地序）/ `site_sort_order`（原站序）三字段，复原时一并还原。`WorkSnapshot.CreateTime`（作品入库时间）为后加字段：采集之前的旧快照无此值（NULL），复原时保持 Create 填充的复原时刻。
- **列表筛选的混合过滤**：时间范围（delete_time/work_create_time）与站点是 recycle_bin 冗余列，SQL 层直筛；作者/标签只存在于 snapshot JSON，带这两类条件时改为 SQL 预筛全量 → 解析快照过滤 → 内存排序分页（回收站量级小，TTL 清理兜底）。排序支持 delete_time/work_create_time 两列，默认 delete_time DESC；work_create_time 为 NULL 的条目（字段引入前删除的）不命中创建时间范围筛选、排序时 NULL 当最小值。
- **列表展示名组装**：siteName/authorNames 由 service 分页后批量查询组装（站点名/本地作者名优先、无本地关联回退站点作者名，顿号拼接），依赖 site/localAuthor/siteAuthor 的名字读取接口。
- **backupId 映射**：快照存 Backup 记录 ID（持久稳定），不存 persistent_store.id（复原后会变）。资源文件级信息由 Backup 记录承载。
- **复原冲突**：(site_id, site_work_id) 唯一索引——逻辑删除后该键释放，用户可能重新下载占用；复原时冲突由 overwrite 控制（放弃/覆盖，覆盖走 work.HardDeleteWork）。
- **引用校验**：复原重建关联时批量校验被引用实体（作者/标签/作品集）仍存在，已删除的关联跳过（部分复原）。
- **TTL 自动清理**：后台 goroutine 启动即清理一次 + 每 24h，按 RecycleBinSettings（autoCleanupEnabled/retentionDays）删除过期条目。

## 依赖关系

- 依赖：work（WorkRestorer：复原重建/冲突检查/覆盖删除）、backup（BackupReader：还原/清理 backup）、persistentStore（StoreImporter：还原文件）、settings（RecycleBinSettingsProvider：TTL 配置）、site/localAuthor/siteAuthor（名字批量读取：列表展示 siteName/authorNames）、database（Transactor）
- 被依赖：work（RecycleItemSaver：逻辑删除写入快照，由 recycleBin.Repository 实现）、前端回收站页面

## 关键设计

- **职责分离**：关联元数据快照归 recycleBin，资源文件备份归 backup，两层通过 backupId 衔接；不改动 backup 模块语义。
- **打破构造循环**：work ↔ recycleBin 双向能力依赖（work 写快照 / recycleBin 重建 work），RecycleItemSaver 由 recycleBin.Repository（数据层）实现、WorkRestorer 在 work 之后经构造注入，构造顺序线性。
- **跨存储一致性（best-effort）**：文件移动/还原在 DB 事务外（文件 IO 不可回滚），事务失败时文件已安全落在 backup 目录、DB 完整回滚，与 work.SoftDeleteWork 同类哲学。
