# tagNamespace 模块说明

## 一句话职责

tag namespace（标签关联级分类维度）的**候选值清单表**：登记「见过的 ns 值」（内置常量集启动投影 + 用户自设 / 插件声明 find-or-create），供前端 ns 选择器分组展示候选。清单是展示候选而非约束——`re_work_tag.namespace` 为开放字符串，未知值允许写入不依赖本表。总体设计见 `../library-squirrel-docs/plan/关联级维度体系重构方案.md`（私有文档库，路径相对主仓根）。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `List` | 全量清单行（value/label/origin/lastUse 全字段，value 升序）；分组（内置/用户/插件）与 last_use 排序归前端 |

无 Handler 消费方的服务方法：`SyncBuiltins`（app.go 启动装配期调用，失败即启动失败）、`EnsureUsedBatch`（关联写入链同事务调用，见下）。

## 核心概念

- `origin` 三态（`backend/base/constant/dimension.go`）：`PLUGIN=0 / USER=1 / BUILTIN=2`，**数值即优先序**——同值多来源时取 MAX（升级不降级）；内置行（label/origin 为权威值）不被 user/plugin 写入改写。
- `last_use`：使用统计（毫秒时间戳），任何来源使用都无条件刷新；仅作前端排序。
- 值归一化：`NormalizeDimensionValue`（去首尾空白 + 小写折叠）为清单唯一键与关联写入的统一规则；**空串=无 ns，不产生清单行**。
- 清单行**不删除**（与 site 注册表投影同语义：删后下次使用即复活）。

## 依赖关系

- 依赖：仅 `database.BaseRepository`（无跨模块依赖）。
- 被依赖：`work`（插件入库链 ns 登记，经其自定义 `TagNamespaceInventoryWriter` 接口注入）、`reWorkTag`（用户手动挂联链 ns 登记，经其自定义 `NamespaceInventoryWriter` 接口注入）、前端 ns 选择器（阶段 4）。

## 关键设计

- **清单登记与关联写入同事务**（D-21）：`EnsureUsedBatch` 经 ctx 携带事务连接（`dbFromCtx` 纪律），调用链为 `work.saveWorkInfoInTx`（SaveWorkInfo 事务内）与 `reWorkTag.LinkBatchToWork`（自包事务）——失败整体回滚，不留孤儿候选行。
- **启动投影 insert-only**：`SyncBuiltins` 只补缺失键，既有行（含用户改过的 label、低优先级来源建的行）一律不动；幂等，重启重试安全。
- **find-or-create + touch 一体**：`EnsureUsedBatch` 批内去重后一次 `ListByValues` 探测，缺失批量建、既有批量 `TouchBatch`（单条 SQL `MAX(origin, ?)` + last_use），无逐值查询。
