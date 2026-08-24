# site 模块说明

## 一句话职责

站点（远程来源）主数据的持久化与查询，供作品/任务/作品集/站点标签/站点作者按 `site_id` 挂靠；站点删除采用**前置守卫**（引用非空即拒，不做级联清理）。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Save` / `Update` | 创建 / 更新站点 |
| `Delete` | 删除站点（前置守卫：五类引用任一存在即拒绝并返回聚合计数与清理指引） |
| `GetById` / `GetByName` / `QueryPage` / `QuerySelectItemPage` | 单查 / 按名查 / 分页 / 选择项分页 |

## 核心概念

- **删除守卫（纯守卫形态）**：`Delete` 在单事务内聚合查询该站点的五类引用计数——作品（活行与软删行分别计）、任务、作品集（活行与软删行分别计）、站点标签、站点作者；任一计数 > 0 返回 `ErrSiteHasReferences`（`errors.Is` 判别，包装后的消息含各项计数与对应清理路径，前端 ElMessage 直接展示），全部为零才删行。软删作品/作品集行同样拒绝——外键拦截不分行态，且软删行须经回收站彻底删除，清理路径与活行不同。
- **计数提供方经仓储接线**：五类计数接口（`WorkSiteRefCounter` 等）定义在本模块，由各提供方**仓储**实现（`CountBySiteId`）——work/task/workSet 的服务在装配序列上晚于 site，经仓储接线打破装配环（与 localTag/localAuthor 删除编排同款）。

## 依赖关系

- 依赖：database（Transactor，守卫与删除同事务）、work / task / workSet / siteTag / siteAuthor（各自仓储实现按站点计数接口）
- 被依赖：前端站点管理页、siteTag / siteAuthor（站点查询接口）、task（创建路由经 `GetByName` 回填 SiteID）、work / workSet / search 等（经各自定义的 SiteReader 窄接口）
