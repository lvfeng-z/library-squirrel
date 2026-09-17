# authorRole 模块说明

## 一句话职责

author role（作者关联级角色分工维度，如原画/脚本/声优）的**候选值清单表**：登记「见过的 role 值」（用户自设 / 插件声明 find-or-create），供前端 role 选择器分组展示候选。清单是展示候选而非约束——`re_work_author.role_name` 为开放字符串，未知值允许写入不依赖本表。与 `tagNamespace` 模块**结构同构、各自独立**（领域语义不同、未来演进可能分叉，不共用实现）。总体设计见 `../library-squirrel-docs/plan/关联级维度体系重构方案.md`（私有文档库，路径相对主仓根）。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `List` | 全量清单行（value/label/origin/lastUse 全字段，value 升序）；分组（内置/用户/插件）与 last_use 排序归前端 |

无 Handler 消费方的服务方法：`SyncBuiltins`（app.go 启动装配期调用——role 内置集当前为空，投影机制保留，将来加项自然补入）、`EnsureUsedBatch`（关联写入链同事务调用，见下）。

## 核心概念

- `origin` 三态（`backend/base/constant/dimension.go`）：`PLUGIN=0 / USER=1 / BUILTIN=2`，**数值即优先序**——同值多来源时取 MAX（升级不降级）；内置行（label/origin 为权威值）不被 user/plugin 写入改写。
- `last_use`：使用统计（毫秒时间戳），任何来源使用都无条件刷新；仅作前端排序。
- 值归一化：`NormalizeDimensionValue`（去首尾空白 + 小写折叠）为清单唯一键与关联写入的统一规则；**空串=无 role，不产生清单行**。
- 清单行**不删除**（删后下次使用即复活）；role 清单靠用户使用与插件声明自然生长（内置集空，无权威来源）。

## 依赖关系

- 依赖：仅 `database.BaseRepository`（无跨模块依赖）。
- 被依赖：`work`（插件入库链 role 登记，经其自定义 `AuthorRoleInventoryWriter` 接口注入）、`reWorkAuthor`（用户手动挂联链 role 登记，经其自定义 `RoleInventoryWriter` 接口注入）、前端 role 选择器（阶段 4）。

## 关键设计

- **清单登记与关联写入同事务**（D-21）：`EnsureUsedBatch` 经 ctx 携带事务连接（`dbFromCtx` 纪律），调用链为 `work.saveWorkInfoInTx`（SaveWorkInfo 事务内）与 `reWorkAuthor.LinkBatchToWork`（自包事务）——失败整体回滚，不留孤儿候选行。
- **启动投影 insert-only**：`SyncBuiltins` 只补缺失键，既有行一律不动；幂等，重启重试安全。
- **find-or-create + touch 一体**：`EnsureUsedBatch` 批内去重后一次 `ListByValues` 探测，缺失批量建、既有批量 `TouchBatch`（单条 SQL `MAX(origin, ?)` + last_use），无逐值查询。
