# site 模块说明

## 一句话职责

站点（远程来源）主数据的持久化与查询，供作品/任务/作品集/站点标签/站点作者按 `site_id` 挂靠。**站点表是 SDK `identity` 注册表的本库投影**：启动期 `SyncFromRegistry` 将注册表全量条目 insert-only 落库（缺失键按注册表权威值建行，既有行不动），站点行生命周期 = 注册表投影（只进不出，无删除入口）。站点身份 = `site_key`（注册表分配，唯一索引），`site_name` 为纯展示列（用户可改，投影不回写既有行、编辑持久）；站点行写入方封闭为「启动投影同步」单一。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `Update` | 更新站点展示字段（site_name/homepage，编辑在投影下持久） |
| `GetById` / `QueryPage` / `QuerySelectItemPage` | 单查 / 分页 / 选择项分页 |

> 站点表 = 注册表投影，非本库使用面（零数据站点同样在列）；「本库用过哪些站点」须经作品/标签数据查询。

## 核心概念

- **注册表投影（`SyncFromRegistry`）**：app.go 启动装配期调用，失败即启动失败（与 DB 初始化/AutoMigrate 同类 fail-fast——站点表缺行会让任务创建/导入报「站点未找到」等衍生错误掩盖根因）；insert-only 幂等保证重启重试安全。注册表只增不改（键一经发布不可变），投影无删除分支。
- **展示字段编辑**：`Update` 仅改展示字段；投影同步绝不更新既有行，用户编辑持久。
- **按键查（`GetByKey`，service 层）**：站点身份查询的规范入口，供 task 创建路由等模块内调用，不经 Handler 暴露。

## 依赖关系

- 依赖：database、SDK identity（注册表全量枚举 `All()`）
- 被依赖：前端站点管理页、siteTag / siteAuthor（站点查询接口）、task（创建路由经 `GetByKey` 回填 SiteID）、work / workSet / search 等（经各自定义的 SiteReader 窄接口）
