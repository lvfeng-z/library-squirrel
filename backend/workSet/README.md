# workSet 模块说明

## 一句话职责

作品集（聚合作品的编组容器，支持多父 DAG 层级与传递包含）：作品集 CRUD、成员管理（挂载/移除/排序/封面）、层级管理（父子关系+环路检测+物理纳入）、软删与复原（回收站模型）、站点侧批量 upsert（work 入库链消费）。

## 边界

- 与 **recycleBin**：本模块实现 `WorkSetRestorer` 接口（SoftDeleteWorkSet / GetDeletedWorkSet / GetBySiteAndSiteWorkSetID 冲突查询 / RestoreDeletedWorkSet / DeleteWorkSetAndAssociations 级联 / ListDeletedBefore TTL 圈定），复原与彻底删除的**编排**归 recycleBin（冲突裁决在彼处）；软删入口 `SoftDeleteWorkSet` 由本模块直接暴露给前端。
- 与 **work**：work 入库链经本模块 Repository 的 `BatchUpsert`/`Upsert`（站点侧作品集批量入库）与 `GetBySiteAndSiteWorkSetID`（原站序同步定位）；经 `FullWorkReader`/`WorkReader` 读作品（成员列表组装）。依赖经 app.go 的 `workSetWriterAdapter` 适配器注入（打破 work ↔ workSet 循环依赖）。
- 与 **reWorkWorkSet / reWorkSetWorkSet**：两关联表的仓储由本模块聚合消费（成员管理/层级管理/传递包含原语 `CollectDescendantWorkIDs`）。
- 与 **search**：作品集搜索链（`QueryWorkSetPageByConditions`）在 search 模块（原生 SQL），本模块的 Page/QueryPageWithCover 走 BaseRepository GORM 管线（软删过滤自动生效）。

## 对外接口（Handler）

作品集 CRUD 与成员/层级管理方法（Save/Update/GetById/QueryPage/QueryPageWithCover/GetWorksByWorkSetId/ListWorkSetsByWorkId/LinkBatch/RemoveBatch/AddChildWorkSet/RemoveChildWorkSet/ListChildWorkSets/MergeWorkSetInto/UpdateSortOrders/ApplySiteOrder/SetCover/UnsetCover/GetCoverWorkId/ListWorkSetWithWorkByIds/Upsert 系列），以及软删入口 `SoftDeleteWorkSet(id)`。

## 核心概念

- **软删模型**：`work_set.deleted_at`（毫秒时间戳，0=活）——Find/Count/Update 自动排除已删行、Delete 打时间戳；查询面经 GORM 管线自动过滤，`QueryWorkSetPageByConditions`（search 原生 SQL）带 `work_set.deleted_at = 0` 恒基线。
- **三列唯一索引** `idx_work_set_site_site_set_gen (site_id, site_work_set_id, deleted_at)`：活行（deleted_at=0）唯一占业务键、已删行按删除时刻互异释放键——删除后可重新下载同键作品集。取三列全量形态而非部分索引：SQLite 的 `ON CONFLICT (列)` 冲突目标只能匹配无 WHERE 的唯一索引，三列形态使 `BatchUpsert`/`Upsert` 的单语句原子 upsert 保留（冲突目标补 deleted_at 列——插入行 deleted_at=0 与表中活行冲突走更新、与死行（时间戳≠0）不冲突走新建不复活）。已知代价：同键两代死行同毫秒删除撞索引（显式报错，现实操作无产道）。
- **关联保留**：软删不动 re_work_work_set（成员）与 re_work_set_work_set（父子 DAG）两表行——复原零成本（清标志即全恢复，层级/成员/封面全在），彻底删除（`DeleteWorkSetAndAssociations`）才级联清理。消费面按**端点活性**过滤（非关联行自身活性——两关联表无软删行）：work 搜索的「不在作品集 X 中」条件 JOIN work_set 判活；传递包含 CTE 每步判活。
- **递归 CTE 的活性分途**：`CollectDescendantWorkSetIds`（传递包含，用户可见数据）递归每步 JOIN work_set 剪除已删子集（其活后代经其他活父集路径仍可达）；`CollectAncestorWorkSetIds`（环路检测，结构完整性）**保持全量不过滤**——过滤会让经已删节点闭合的环漏检，节点复原即成死环。
- **封面 = work_set.cover_work_id 集级引用**（可指向传递包含内任意作品——含子集作品；非传递包含内的作品拒绝）。封面是作品集自身属性而非成员关系属性：设置一条 UPDATE（单列天然单封面）、解析读列即可；指向的作品不在活行（软删期）时封面落空显示（作品复原后自愈），**无兜底转投成员**——兜底路径已随外键化退役（cover_work_id → work 外键下，作品彻底删除链首步清封面引用，悬空引用不复存在）。设置面校验归 service（`SetCoverWork` 前置传递包含校验）；列表批查经 `ListCoverWorkIdsByWorkSetIds`（search 的作品集页 CoverResolver 由本模块 Service 实现，直读列无兜底）。
- **传递包含原语** `CollectDescendantWorkIDs`：作品集自身作品在前（按 sort_order），其后逐后代作品集保序去重追加——`GetWorksByWorkSetId`/`ListWorkSetWithWorkByIds`/`MergeWorkSetInto` 共用。
- **物理纳入**（MergeWorkSetInto）：把源集及其后代的成员**复制**关联到目标集（静态快照，非转移，不可撤回），is_cover=false 维持目标自身封面。

## 依赖关系

- 依赖：reWorkWorkSet / reWorkSetWorkSet（关联仓储）、database（Transactor）、work（FullWorkReader / WorkReader 封面与成员组装）
- 被依赖：recycleBin（WorkSetRestorer）、work（入库链经 workSetWriterAdapter）、search（QueryRecycleWorkSetPage 读 work_set 已删行）、site（WorkSetSiteRefCounter：站点删除守卫的作品集引用计数，仓储 `CountBySiteId` 活行/软删行分别计数）、前端作品集页/回收站作品集 tab

## 关键设计

- **upsert 单语句原子**：站点侧入库（work 下载完成回传作品集信息）经 OnConflict 三列目标一条语句完成「活行更新元数据（NULL 覆盖）/无活行新建」，不复活已删行、不依赖事务包裹。
- **软删链极简**：作品集无自有资源文件（封面/内容均来自作品）、无任务关联——软删=校验活行+事务内一条 UPDATE；复原=冲突裁决（recycleBin 编排）+清标志。
- **既有 N+1 说明**：`QueryPageWithCover` 逐行查封面与 `ListWorkSetsByWorkId` 逐 ID 查集为存量形态（封面作品活性由 work 侧 ListByIds 软删过滤——指向软删作品时封面落空、已删集由 GetById 过滤跳过），重构留待按需立项。
