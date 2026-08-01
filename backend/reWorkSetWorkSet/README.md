# reWorkSetWorkSet 模块说明

## 一句话职责
作品集间父子关联（多父 DAG）的持久化层——CRUD 关系行、递归遍历后代/祖先作品集，供 `workSet` service 实现逻辑关联（A 将 B 纳为子集）与传递包含。

## 边界
- 与 `reWorkWorkSet`：本表管**作品集↔作品集**（`re_work_set_work_set`，父子层级），`reWorkWorkSet` 管**作品↔作品集**（`re_work_work_set`，作品归属）。两者独立，`workSet` service 在 `CollectDescendantWorkIDs` 中组合二者。
- 本模块无 Handler，仅 repository，被 `workSet` service 通过 `ReWorkSetWorkSetRepository` 接口注入调用。

## 对外接口（被 workSet service 调用）
| 方法 | 作用 |
| --- | --- |
| `SaveRelation` | 建立父子关系（OnConflict DoNothing，幂等） |
| `DeleteByParentAndChild` | 解除指定 `(parent, child)` 关系 |
| `DeleteByParentWorkSetId` | 删除某父集的全部子集关系（父作品集删除时清理） |
| `DeleteByChildWorkSetId` | 删除某子集的全部父集关系（子作品集删除时清理） |
| `ListChildWorkSetIds` | 父集的直接子集 ID（按 `sort_order` 升序） |
| `CollectDescendantWorkSetIds` | 递归 CTE 向下取全部后代作品集 ID（不含自身） |
| `CollectAncestorWorkSetIds` | 递归 CTE 向上取全部祖先作品集 ID（不含自身，环路检测用） |

## 核心概念
- **多父 DAG**：一个子作品集可有多个父集，但禁止成环。本表只存关系，环路由 service 层在事务内用 `CollectAncestorWorkSetIds` 检测。
- **`sort_order`** 属 `(parent_work_set_id, child_work_set_id)` 元组——同一子集在不同父集下顺序可不同，故 order 与关系行绑定，联合唯一索引 `(parent, child)`。

## 依赖关系
- 依赖：`database`（`BaseRepository` + `dbFromCtx` 事务感知）
- 被依赖：`workSet` service（接口注入）

## 关键设计
- **递归 CTE 用 `UNION`（非 `UNION ALL`）**：多父 DAG 下菱形依赖（A→B、A→C、B→D、C→D）会让同一后代会经多条路径到达，`UNION` 自动去重，否则重复展开。
- **深度上限 `maxTraversalDepth=100`**：无环 DAG 自然终止；此上限是异常数据（误成环）下的防御性兜底。
- **全部方法用 `dbFromCtx(ctx)`**：事务内调用必须感知事务 DB 句柄（MaxOpenConns=1，误用全局句柄会死锁）。环路检测与写入同事务，故遍历方法亦须事务感知。
