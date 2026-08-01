# workSet 模块说明

## 一句话职责
作品集（`WorkSet`，多个作品的合集实体）的领域服务——作品集 CRUD、作品归属管理、以及作品集间 merge（逻辑关联多父 DAG + 物理纳入复制快照）。

## 边界
- 与 `reWorkWorkSet`：管**作品↔作品集**归属（`re_work_work_set`），本模块通过 `ReWorkWorkSetRepository` 接口调用。
- 与 `reWorkSetWorkSet`：管**作品集↔作品集**父子层级（`re_work_set_work_set`），本模块通过 `ReWorkSetWorkSetRepository` 接口调用。merge 功能组合这两张表。
- `WorkSet` 无回收站——`Delete` 为物理删除（事务内清理 `re_work_work_set` + `re_work_set_work_set`[父/子] + 实体）。

## 对外接口（Handler）
### 作品集 CRUD / 查询
| 方法 | 作用 |
| --- | --- |
| `Save` / `Update` / `Delete` | 作品集增删改 |
| `GetById` / `QueryPage` / `QueryPageWithCover` | 单查 / 分页 / 带封面分页 |
| `GetBySiteWorkSetIdAndSiteName` | 按站点作品集 ID + 站点名查（插件去重用） |

### 作品归属管理（`re_work_work_set`）
| 方法 | 作用 |
| --- | --- |
| `LinkWorkToWorkSet` / `UnlinkWorkFromWorkSet` | 单个关联/解除 |
| `LinkBatchToWorkSet` / `RemoveBatchFromWorkSet` | 批量关联/解除 |
| `GetWorksByWorkSetId` | 取作品集下作品（**传递包含**：含全部后代作品集作品） |
| `ListWorkSetsByWorkId` | 取作品关联的作品集列表 |
| `SetCover` / `UnsetCover` / `GetCoverWorkId` | 封面管理 |
| `UpdateSortOrders` | 批量更新作品在集内排序 |
| `ListWorkSetWithWorkByIds` | 作品集+作品完整信息聚合（传递包含，批量化） |

### 作品集 merge（逻辑关联 / 物理纳入）
| 方法 | 作用 |
| --- | --- |
| `AddChildWorkSet` | 建立 parent→child 父子关系（事务内防环路） |
| `RemoveChildWorkSet` | 解除父子关系 |
| `ListChildWorkSets` | 取直接子作品集（层级管理 UI 用） |
| `MergeWorkSetInto` | 物理纳入：把源作品集及其后代的 work 复制到目标（静态快照） |

## 核心概念
- **传递包含原语 `CollectDescendantWorkIDs`**：返回作品集自身及全部后代作品集所含 work（去重、保序）。`GetWorksByWorkSetId`、`ListWorkSetWithWorkByIds`、`MergeWorkSetInto` 均经此聚合——打开 A 看到的作品含其全部子作品集的作品。
- **逻辑关联（多父 DAG）**：A 将 B 纳为子集 = 写一行 `re_work_set_work_set (A, B)`。B 不消失，B 后续变化实时跟随。事务内环路检测（`CollectAncestorWorkSetIds`）。
- **物理纳入（复制快照）**：把 B 及其后代的 work 关联**复制**一份给 A（`re_work_work_set` 加行），B 不变、不记录来源、不可撤回。
- **`sort_order` 两列职责**：`re_work_work_set.sort_order` 管作品在集内顺序，`re_work_set_work_set.sort_order` 管子集在父集下顺序。

## 依赖关系
- 依赖：`Repository`（作品集实体）、`ReWorkWorkSetRepository`（作品归属）、`ReWorkSetWorkSetRepository`（父子层级）、`Transactor`（事务执行器）、`FullWorkReader` / `WorkReader`（作品读取，接口隔离避免跨模块直接引用）
- 被依赖：`search`（作品集分页/筛选经本 service）、`work`（作品回收时清理归属）、前端作品集管理页

## 关键设计
- **传递包含的保序**：`CollectDescendantWorkIDs` 落实 §4.4——自身作品在前（按集内 `sort_order`），其后逐个后代作品集展开，每集内按集内 `sort_order`；同一 work 仅出现一次，首次来源为准。
- **物理纳入去重写入用 `SaveBatchOnConflict`**（`OnConflict DoNothing`）：`reWorkWorkSet.SaveBatch` 无 OnConflict，遇联合唯一索引会抛 UNIQUE 冲突，故物理纳入**不可复用** `LinkBatchToWorkSet`/`SaveBatch`，必须走 `SaveBatchOnConflict`（逐冲突跳过，单条重复不拒整批）。
- **环路检测须事务内**：建父子关系前查祖先，与写入同事务，否则两个并发合法请求合并成环。
- **`ListWorkSetWithWorkByIds` 批量化**：收集所有后代 ID → 批量查 → 组装，消除原循环内逐集查询的 N+1。
