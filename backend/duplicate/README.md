# duplicate 模块说明

## 一句话职责

作品查重判定能力：输入「站点键（site_key）+ 站点侧作品 ID + 期望板块角色集合」，输出未命中 / 命中无冲突 / 命中冲突三分类。纯查询判定，无副作用、无 handler。

## 边界

- 与 import：两条共享查询（`ListSitesByKeys`、`ListWorksBySiteAndWorkIDs`）自 import ingestor 迁出归本模块持有，import 的 find-or-create 查重（`mapExistingSites`/`partitionWorks`/`ensureSites`）复用本模块仓储实例；本模块不感知 import 的 manifest 结构与入库编排。
- 与 taskManager：taskManager 的两处内联查重判定（批量派发预检 + 单任务 fallback）改接本模块 `DuplicateChecker`，控制面动作（existingWorkId 赋值、WaitingForInput、事件推送、确认重入）仍归 taskManager。

## 对外接口

无 Handler。消费方调用：

| 方法 | 作用 |
| --- | --- |
| `Service.Check(ctx, items) ([]DuplicateCheckResult, error)` | 批量三分类判定，结果与输入按下标对齐 |

## 核心概念

- **输入键形态**：站点键（site_key，manifest/插件应答域）而非本库 siteID——share-receive/zip 导入的键源在 manifest 域，统一键入口；插件任务侧由调用方把 task.SiteID 反查站点键（一次查询）后传入。站点身份规范见 `doc/site-identity-spec.md`。
- **行级门槛**：期望板块角色与已有作品**活行** store 角色求交，交集非空才落「命中冲突」；空交集/零行落「命中无冲突」（保留已有作品 ID 供替换定位，不弹窗）；板块为空（插件自决全量）时已有任意活行即冲突，载荷取已有行角色全集。
- **保守弹窗**：行级角色集合查询失败（或角色集合提供方未装配）时命中作品一律落「命中冲突」且载荷不带交集角色（宁多弹不漏弹）；站点映射/作品定位失败则返回 error，由消费方降级。
- **输出语义**：`ConflictRoles` 为 nil 表示行级信息不可得（保守弹窗），非空为交集角色（保持期望角色原序；全量语义取字母序全集）。

## 依赖关系

- 依赖：`Repository`（本模块仓储：`ListSitesByKeys`/`ListWorksBySiteAndWorkIDs`，事务感知 dbFromCtx）；`StoreRoleSetProvider`（`resource.Service.ListStoreTypeSetsByWorkIds` 实现，活行角色集合）。
- 被依赖：taskManager（两处查重判定）、share（阶段5 收件查重接入）、import ingestor（共享查询复用）。

## 关键设计

- **三分类与两分支语义随迁**：判定规则（含「查询失败保守弹窗」「板块空时已有任意活行即冲突」）自 taskManager 内联逻辑迁入，插件任务改接后行为等价；`intersectRoles`/`sortedStoreRoles` 纯函数同步迁入。
- **查询失败分级**：站点映射与作品定位失败 → error（批量派发降级逐任务检查、逐任务按未命中处理）；行级角色集合失败 → 保守冲突（不升为 error，消费方无需区分失败位置即可还原既有降级语义）。
