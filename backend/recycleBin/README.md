# recycleBin 模块说明

## 一句话职责

作品回收站：软删除模型下管理 work 已删行的列表查询、复原（文件还原 + 清标志）、彻底删除（物理级联 + 清 backup）与 TTL 自动清理。

## 边界

- 与 **work**：work 实现 `WorkRestorer` 接口（软删/复原清标志/冲突查询/物理级联删除），recycleBin 负责复原与彻底删除的**编排**（业务键冲突裁决、文件还原顺序、backup 清理）。软删除入口 `work.SoftDeleteWork` 由 work 模块直接暴露给前端，不经本模块。
- 与 **search**：回收站列表查询转发 search 的 `QueryRecycleWorkPage`（条件体系与作品搜索同构，基线 `deleted_at > 0`）——作者/标签/站点/时间范围筛选与排序全部复用作品搜索链。
- 与 **backup**：软删除时资源文件移入 backup/（backup.work_id 归属关联），复原时经 `RestoreFile` 还原回 store/ 原路径并删备份记录，彻底删除时清备份文件与记录。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Page(page, query)` | 分页查询回收站列表（query.conditions 为 SearchCondition 条件体系 + sortBy/sortOrder） |
| `Restore(workId, overwrite)` | 复原已软删作品（overwrite 控制冲突时占位作品转入回收站） |
| `Purge(workId)` | 彻底删除已软删作品（不可恢复） |

> 逻辑删除入口 `work.SoftDelete`（handler）由 work 模块暴露；TTL 自动清理由后台 goroutine 内部调用 Purge，不经 Handler。快照时代的 recycle_bin 表与仓储已随快照体系整体移除（软件未发布，无兼容保留）。

## 核心概念

- **回收站条目 = work 已删行**：work.deleted_at（毫秒时间戳）> 0 即回收站条目；删除时间、TTL 过期判定、删除时间排序均由该列承担。从属行（resource/resource_store/re_work_*）与 persistent_store 记录在软删期间原地保留，复原无需重建。
- **backup 归属关联**：软删除链逐 store 建 backup 记录时写入 work_id；复原/彻底删除按 `ListByWorkId` 聚合取文件级明细。
- **复原冲突**：删除后重新下载同作品会占用业务键（部分唯一索引 `idx_work_site_site_work_active` 仅约束活行，已删行释放键）。复原时检测到活占位行 → 放弃（报 ErrRestoreConflict）或覆盖（占位作品转入回收站，文件移 backup 让出 store/ 路径，反悔可再复原）。
- **文件还原的操作抑制**：还原目标在 store/ 监控白名单内，逐文件 storeRegistry.Suppress/Release 登记避免被 fsmonitor 误报为外部变更。

## 依赖关系

- 依赖：work（WorkRestorer）、backup（BackupReader：ListByWorkId/RestoreFile/GetBackupPath/Delete）、search（RecycleWorkQuerier：QueryRecycleWorkPage）、settings（TTL 配置 + workDir）
- 被依赖：前端回收站页面

## 关键设计

- **编排归发起方**：复原/彻底删除的流程编排（校验→冲突裁决→文件→DB）在本模块，原子能力经接口注入（work/backup 提供）。
- **查询完全复用**：列表不自带查询实现，转发 search（EXISTS 条件体系 + 精简投影：site LEFT JOIN、作者名 GROUP_CONCAT 聚合）。
- **best-effort 文件链**：文件还原在清标志前、失败警告后继续（部分复原语义）；彻底删除的级联链内 store 原路径文件删除对已移 backup 的文件扑空无害。
