# 作品集 Merge 功能方案

## 审查摘要

### 关键声明（抽查项）
- 声明1：作品集=`WorkSet`（多个作品的合集实体）、作品=`Work`（单个作品实体）是两个实体；WorkSet↔Work 是 N:M 关联表 `re_work_work_set`，联合唯一索引 `(work_id, work_set_id)` —— `backend/base/model/entity/re_work_work_set.go:10-16`、`backend/base/model/entity/work_set.go:9-21`
- 声明2：`WorkSet` 当前完全扁平，无任何父子/分组/自指字段 —— `backend/base/model/entity/work_set.go:10-21`
- 声明3：`workSet` service 具备批量关联 API，**但 `LinkBatchToWorkSet`（`backend/workSet/service.go:190-204`）内部的 `SaveBatch`（`backend/reWorkWorkSet/repository.go:246-261`）无 `OnConflict`**（见风险6），物理纳入复制不能直接复用
- 声明4：`localTag` 已用递归 CTE（SQL 的 `WITH RECURSIVE` 子句，用于遍历树/图）查后代与祖先 —— `backend/localTag/repository.go:72-118`
- 声明5：`ListWorkSetWithWorkByIds` 是「作品集→作品」聚合主战场，现状为循环内逐个 `ListByWorkSetId` —— `backend/workSet/service.go:281-320`
- 声明6：`WorkSet` **无软删/回收站**——`Delete` 为物理删除；`recycleBin` 仅回收 `Work`，`WorkSetSnapshot` 只是 Work 被回收时携带的关联快照 —— `backend/workSet/service.go:132-138`、`backend/recycleBin/snapshot.go:39-41`

### 待决策（需用户拍板）
- 无（全部已决，见下「已决策」）。

> 原决策3「WorkSet 软删机制」经审查证伪——见声明6，WorkSet 无回收站，已从阻塞项移除。

### 已决策
- 决策1（sort_order 续排）：A 原有作品保持原序在前，被复制作品追加在后，并保留其在来源作品集（B 及后代）中的相对顺序。
- 决策2（is_cover）：复制时一律置 false，A 维持自身封面语义不变。
- 决策4（前端层级管理 UI）：参照现有「向作品集添加作品」的交互模式（执行时定位现有组件复用）。
- 决策5（迁移回滚）：确认——GORM AutoMigrate 加表，无回滚需求。
- 决策6（解除清理）：删除父子关系 `(A,B)` 时，若存在缓存层须同步更新；当前无缓存层，预留接口。
- 决策7（并发去重）：用数据库唯一索引 + `OnConflict DoNothing` 解决，**逐冲突跳过——单条重复不得拒绝整批**。
- 物理纳入**不记录来源 B**：与"静态快照、脱钩"语义一致；A 下被复制的关联无来源标记、无一键撤回（见风险7）。若未来需撤回，须新增 `source_work_set_id` 列记录来源。

### 自曝风险
- 风险1：`localTag` 递归 CTE 是单父树，多父 DAG（有向无环图：一个作品集可有多个父集但禁止成环）下 `UNION ALL` 会重复展开菱形节点，必须加 distinct（`SELECT DISTINCT` 去重），不能直接照搬
- 风险2：环路检测须在事务内执行，否则两个并发建关系请求各自合法却合并成环
- 风险3：传递包含冲击所有按作品集聚合的查询点，遗漏任一处会导致展示/搜索/统计不一致
- 风险4：`work/service.go:1344-1346` 存在「全量替换关联会误删用户手动关联」的已知 TODO，merge 流程须回避
- 风险5：新 repository 方法须用 `dbFromCtx(ctx)`（绑定当前事务上下文的 DB 句柄）而非 `GORM()`（全局单句柄），否则事务内死锁（MaxOpenConns=1）
- 风险6：`SaveBatch` 无 `OnConflict` **已核验**（`reWorkWorkSet/repository.go:246-261` 直接 `Create()`，相对 merge 去重写入期望是缺陷），物理纳入复制必改用 `clause.OnConflict{DoNothing: true}` 批量写入（决策7：逐冲突跳过，单条重复不拒整批）
- 风险7：物理纳入**不可撤回**——A 下被复制关联无来源记录，误纳只能手动删；属数据层不可逆
- 风险8：**查询放大**——`CollectDescendantWorkIDs` 每次打开 A 都跑递归 CTE 无缓存，深 DAG + 大作品集下性能隐患，必要时加缓存/物化
- 风险9：**回滚不止撤表**——新功能还改动 §4.2 多处查询点与 §5 写入路径，上线后若传递包含行为异常，代码层回滚须配套还原这些改造，非仅撤 `re_work_set_work_set` 表

---

## 1. 背景与概念对齐

- **作品集 = `WorkSet`**：多个作品的合集实体（如 pixiv 系列）。
- **作品 = `Work`**：单个作品实体（一张图/一话），其下挂 `Resource`（资源单元）。
- 一个 `Work` 本就可属于多个 `WorkSet`（`re_work_work_set` 是 N:M），这是本方案的天然基础。

本功能让作品集 A 以两种**并存**方式合并作品集 B（不是二选一的路线）：

| 方式 | 性质 | 数据层 | B 的状态 | B 后续变化对 A |
|---|---|---|---|---|
| 逻辑关联 | 动态引用 | 新表 `re_work_set_work_set`（多父 DAG） | 独立保留 | 实时跟随 |
| 物理纳入 | 静态快照 | 往 `re_work_work_set` 给 A 复制行 | 独立保留 | 不跟随 |

**动机边界**：本方案只解决「本地已下载作品集的归类组织与聚合展示」，**不**处理跨站点统一同一作品集（不引入 `LocalWorkSet` 双轨）。

> 下文约定：A 一律指**目标作品集**（并入方/父集），B 一律指**源作品集**（被并入方/子集）。

## 2. 核心技术收敛

两种方式喂同一个查询原语：

```
CollectDescendantWorkIDs(A) = distinct( A 直接挂载的 work  ∪  所有后代作品集的 work )
```

- 逻辑关联 → 扩展「后代作品集集合」。
- 物理纳入 → 往「A 直接挂载的 work」里加行。
- 两者结果 `union + distinct`，天然不重复计数。

## 3. 数据模型

### 3.1 新增关联表 `re_work_set_work_set`（作品集间父子关系，多父 DAG）

DAG 即有向无环图——本方案中指「一个作品集可有多个父集、但禁止成环」的父子关系网。结构仿 `re_work_work_set`（声明1）：

| 字段 | 说明 |
|---|---|
| `parent_work_set_id` | 父集 |
| `child_work_set_id` | 子集（B 作为 A 的子集 = 一行 `(A, B)`） |
| `sort_order` | 子集在**该父集**下的排序（多父下同一 B 在不同父集顺序不同，故 order 属 `(parent,child)` 元组） |
| 联合唯一索引 | `(parent_work_set_id, child_work_set_id)` |

实体放 `backend/base/model/entity/re_work_set_work_set.go`，用 `NewReWorkSetWorkSet()` 工厂；在 `backend/migration/migrate.go` 注册自动迁移（迁移回滚见决策5）。

### 3.2 环路检测（DAG 保持无环）

建立「A 包含 B」（写 `(A,B)`）前，校验 **A 是否已是 B 的后代**（从 B 沿 child 边能否到达 A）。能到达则禁止——否则 `A→B` 闭合环路。

- 校验实现：复用声明4 的 `SelectParentNode` 式递归 CTE 反向查可达性，或从 A 向下查后代集合判断是否含 B。
- 须在**事务内**校验（风险2），与写入同事务。
- 自指（A==B）直接拒绝；成环时前端拒绝并提示。

## 4. 逻辑关联（多父 DAG + 传递包含）

### 4.1 传递包含原语 `CollectDescendantWorkIDs(workSetID) → []int64`

- 递归 CTE 沿 `child_work_set_id` 边向下取所有后代作品集 ID（仿声明4 的向下 CTE）。
- **关键差异（风险1）**：多父 DAG 必须对后代作品集 ID 做 distinct（菱形依赖 `A→B、A→C、B→D、C→D` 中 D 被两条路径到达，不去重会重复），再 join `re_work_work_set` 取 work ID，再对 work ID distinct（N:M 允许同一 work 挂多个作品集）。
- 保留 `level < ?` 深度限制作为超深 DAG 的防御性兜底。
- 性能：每次打开 A 都跑递归 CTE，深 DAG + 大作品集有查询放大风险（风险8），必要时对结果加缓存/物化视图。

### 4.2 受传递包含影响的查询点（风险3 须穷举）

| 查询点 | 现状锚点 | 改造 |
|---|---|---|
| 作品集→作品聚合 `ListWorkSetWithWorkByIds` | `workSet/service.go:281-320` | `ListByWorkSetId` 替换为 `CollectDescendantWorkIDs` |
| 作品集分页（含封面/数量）`QueryPageWithCover` | `workSet/service.go:328+` | 作品数 = 直接 + 后代 distinct；封面兜底见决策2 |
| 作品集基础取 work `ListByWorkSetId` | `reWorkWorkSet/repository.go` | 新增「取子树 work」原语，旧方法保留供内部用 |
| 按作品集筛选作品（搜索） | work 查询链路 | 见 4.3，传递包含后代 |
| 前端作品集卡片数量 | 前端消费 | 后端聚合后返回，前端纯消费 |

### 4.3 搜索/筛选传递（已决策：传递）

用户「按作品集 = A 筛选作品」时，结果含 A 所有后代的作品，与展示行为一致。

### 4.4 展示形态（已决策：扁平混排）

打开 A 时，A 直接作品 + 所有后代作品扁平混排于同一列表，仅以来源标记区分归属，前端不做分组。展示排序：A 直接作品在前（按其在 A 下的 `re_work_work_set.sort_order`），其后按后代作品集在 A 下的顺序（`re_work_set_work_set.sort_order`）逐集展开，每集内部按该集 `re_work_work_set.sort_order`——两列职责不同，前者管作品在集内顺序，后者管子集在父集下顺序。

## 5. 物理纳入（复制快照）

### 5.1 操作语义（已决策：复制，非转移；不记录来源）

把 B **及其所有后代**当前所含的全部 work 关联，复制一份挂到 A：

1. `descendantWorkIds = CollectDescendantWorkIDs(B)`（含 B 自身直接 work + B 后代 work，distinct）。
2. 对每个 workId，往 `re_work_work_set` 插入 `(workId, A)`——**必须用 `clause.OnConflict{DoNothing: true}` 批量写入**（风险6），**不能复用 `LinkBatchToWorkSet`**（声明3：其内部 `SaveBatch` 无 `OnConflict`，遇联合唯一索引必抛 UNIQUE 冲突）。语义要求（决策7）：逐冲突跳过——单条重复不得拒绝整批，批量 INSERT 在 SQLite 下走 `INSERT ... ON CONFLICT DO NOTHING`（冲突行忽略、非冲突行照常插入）。冲突目标须为 `(work_id, work_set_id)` 去重索引；决策2 的 `is_cover=false` 同时回避 `idx_re_work_work_set_set_cover`（每集一封面）约束，若未来改 is_cover 语义须重审冲突目标。
3. `sort_order`：A 原有作品保持原序在前，被复制作品追加在后并保留其在来源作品集（B 及后代）中的相对顺序（决策1）；`is_cover` 一律置 false，A 维持自身封面（决策2）。
4. B 及其后代、B 的父子关系**原样保留**——B 不消失，无悬空关系，无级联。

> upsert 指「存在则跳过/更新、不存在则插入」的写入语义，此处用其「冲突跳过」形态实现去重复制。

**不记录来源（已决策）**：复制的 `(workId, A)` 行不携带来源 B 标记。后果见风险7——A 下这些关联无法反向追溯来源、无一键撤回，误纳只能手动删除。若未来要支持撤回，需给 `re_work_work_set` 新增 `source_work_set_id` 列。

### 5.2 与逻辑关联的边界

- 物理纳入后 A 直接拥有这些 work 的快照；B 之后新增/删除作品，A 不跟随。
- 若 A 同时对 B 有逻辑关联，`CollectDescendantWorkIDs` 的 distinct 保证不重复计数，无需特殊处理。

## 6. 前端

- 作品集详情/列表：消费后端聚合结果，扁平混排 + 来源标记。
- 层级管理 UI：参照现有「向作品集添加作品」的交互模式（决策4，执行时定位现有组件复用），用于建立/解除父子关系、调整层级；提交前由后端做环路检测，成环则提示。
- 物理纳入入口：选择源 B 与目标 A，二次确认（静态快照、不可自动跟随、**误纳不可撤回**的提示）。
- 新增 Wails bindings（`wails3 generate bindings -ts`）+ frontend wrapper。

## 7. 任务分解（执行阶段建议启用 `task-graph` 维护派生图）

> **进度**：第 1-4、7 块完成 ✓；第 6 块前端完成（层级管理 UI + 物理纳入入口 + 扁平混排展示自动生效）✓，6e 来源标记延后；6b DTO 增强延后；第 5 块搜索传递未落地（延后）。三个测试期阻塞 bug 已修复：作品集添加作品崩溃（`LinkBatchToWorkSet` 字面量构造漏 `BaseEntity` 初始化 → 改工厂）、勾选失效/名称"?"（`fetchWorkPageForAdd` 返回 WorkFullDTO 未包装 WorkCardItem）、搜索条件不生效（`buildSearchConditions` 误判 operator 为 undefined 跳过选中标签）。逻辑关联防环、物理纳入、传递包含已实机验证通过。

1. **数据模型层**：`re_work_set_work_set` 实体 + 迁移注册 + repository（CRUD、递归 CTE 后代查询、祖先/可达性查询，均用 `dbFromCtx`）。✓
2. **查询原语层**：`CollectDescendantWorkIDs`，含 DAG distinct 改造（风险1）。✓
3. **逻辑关联 service/handler**：建/删父子关系 + 事务内环路检测（风险2）；传递包含接入 4.2 各查询点（风险3）。✓
4. **物理纳入 service/handler**：复制流程（5.1），**新建 `OnConflict DoNothing` 写入路径**（风险6），不得复用 `LinkBatchToWorkSet`（声明3）。✓
5. **搜索传递**：work 查询链路按作品集筛选处接入子树（4.3）。⏸ 延后
6. **前端**：层级管理 UI（决策4）+ 扁平混排展示 + 物理纳入入口（含不可撤回提示）+ bindings/wrapper。✓（6e/6b 延后）
7. **WorkSet 删除/关联解除**：依声明6，WorkSet 无回收站——删除按物理删除其 `re_work_set_work_set`（作为 parent 或 child 的行）+ `re_work_work_set` 行 + 实体；解除 `(A,B)` 即删 `re_work_set_work_set` 那一行。解除时若存在缓存层须同步失效（决策6）。✓
8. **回避已知坑**：全量替换关联模式（风险4）。✓

## 8. 已知坑与回避

- 风险4：`work/service.go:1344-1346` 全量替换关联会误删用户手动关联——merge 流程不得走该模式，改用增量 upsert。
- 风险5：新 repository 方法用 `dbFromCtx(ctx)`，禁止 `GORM()`（MaxOpenConns=1 事务死锁）。
- 声明5 现状 `ListWorkSetWithWorkByIds` 为循环内查询，改造时一并消除 N+1（收集所有后代 ID → 批量查 → 组装）。
- 风险6：`SaveBatch` 无 `OnConflict`（`reWorkWorkSet/repository.go:246-261`），物理纳入复制禁用，须新建 `OnConflict DoNothing` 路径。
- `workSet/query.go:9` 存在 `SiteWorkSetID` 类型不一致（`QueryAttribute[int64]` vs 实体 `sql.NullString`），复用查询 DTO 时注意。
