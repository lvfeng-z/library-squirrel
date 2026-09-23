# plugin/extension 模块说明

## 一句话职责

运行时插件与主程序之间的**桥接层**：以 go-plugin 子进程方式加载运行时插件，把 SDK 契约的 gRPC 服务接线上主程序各域——插件侧能力入口 `PluginContext` 的实现、主程序侧 HostService/LibraryQuery 服务的桥接、扩展点注册表与 WorkFetcher 调用代理。插件系统整体架构（协议、时序、信任模型）见私有环境仓 rules/plugin.md，生命周期管理入口见上级 `backend/plugin/README.md`。

## 边界

- 与 **backend/plugin**：上级模块管插件记录与生命周期编排（安装/激活/停用/升级）；本包管进程级桥接与扩展点运行时。
- 与 **search 模块**：插件库查询**不走** search（前端搜索页专用、原生 SQL 自管软删基线）；统一走各域 repository 的 GORM 管线。

## 提供什么（组件）

| 组件 | 职责 |
| --- | --- |
| `loader.go` | 插件进程加载与 HostDeps 装配：契约版本校验（current 引用 SDK `transport.ContractVersion`，minSupported=12——扩展点正名为作品拉取 workFetch 的破坏性分界）、声明面门控查询（清单原文校验 `ValidateManifestDeclarations`、能力集合派生 `deriveCapabilities`、拉取条目声明 `SiteAuthorFetchEntries`、条目级 `HasWorkFetchOption`）、激活期按清单声明派生注册作品拉取/站点浏览器代理（元数据 name 取清单条目，停用/崩溃清理走 `UnloadPlugin` 的 `UnregisterAll`）、HostService RPC 桥接适配器（含库查询方法组 → `PluginContext` 的适配 `hostLibraryQueryProvider`） |
| `library_query_provider.go` | **库查询核心（Tier 1 只读）**：实现 SDK `LibraryQuery` 契约 21 端点，逐端点桥接 14 个域只读接口（`LibraryQueryDeps`）并映射为契约消息；分页钳制（page≥1、缺省 20、上限 200）；Get* 未命中 `NotFound`、GetWorkDir 未配置 `FailedPrecondition`；零自拼 SQL——查询全走各域 repository GORM 管线（软删 scope 自动排除、resource_store 关联活行过滤） |
| `plugin_context.go` | 插件侧 `PluginContext` 实现：自存 KV、任务触发、前端通信转发、库查询方法组收口（每调用记诊断级日志——调用方插件 + 端点 + 关键参数）。**注册相关方法为契约 v11 前遗留**（运行时注册/注销 RPC 已从 SDK 接口删除，宿主侧实现零调用方，待清死代码） |
| `task_executor.go` / `work_fetch_proxy*.go` | WorkFetcher 执行与 gRPC 代理（实现 download 模块定义的 `PluginExecutor` 接口；proxy registry 按插件身份取代理） |
| `registry.go` / `site_browser_registry.go` | 前端扩展注册中心（7 种 kind 统一管道）与 SiteBrowser/WorkFetcher 复合键注册表（激活期按清单声明派生填充；URL 监听派生索引已迁 `backend/pluginTaskUrlListener`） |
| `handler.go` | 前端扩展 IPC 响应 DTO（`FrontendExtensionResponse`） |
| `static_resource_service.go` | 插件静态资源路径映射与文件服务 |
| `wails_pusher.go` | 前端扩展事件推送器（插件→前端事件经 Wails Emit 转发） |
| `workset_order_fetcher.go` / `workset_relation_fetcher.go` | **条目级**声明驱动的可选能力获取器（实现 work 模块定义的 `WorkSetOrderFetcher`/`WorkSetRelationFetcher`；该（插件, workFetch 条目）未在 `extensions.workFetch[].options` 声明对应方法组则跳过不盲调 gRPC——`workset_order_fetcher.go:27,42-47`） |
| `site_author_fetcher.go` | 站点作者信息拉取能力桥（实现 authorInfo 模块定义的 `SiteAuthorFetcher`：拉取与候选枚举两面）。候选 = 声明 `extensions.siteAuthorFetch` 条目、且该条目 `sites` 含本次请求站点键、插件有可用服务客户端的 (插件, 条目) 对（候选粒度=条目；发现侧即按站点归属收窄，**候选集恒等于归属集**，插件不自判归属——`site_author_fetcher.go` 的 `fetchCandidates`），经 `backend/route` 基座按候选全键（插件公开 ID 与条目 id 的 NUL 拼接）字典序逐个调用流（拉取请求携带候选条目 id，插件侧按条目分派），首个成功者即终点、任一候选失败即终止并点名候选（单极，无不适配态）；零候选单态收口报「无插件覆盖该站点」。候选枚举面按全键字典序返回、首位即默认选中项，展示名 =「插件名 · 条目名」两段，供交互面判冲突与校验显选键（显选两键经排序键置首） |
| `convert.go` | 任务实体 → SDK `TaskDTO` 跨进程序列化组装（字段集不随表拆分变化） |
| `process_group_windows.go` / `process_group_other.go` | 子进程 Job Object 归组（主进程异常退出时终止插件子进程，Windows） |

## 依赖关系

- 依赖：插件 SDK（transport/dto/gen 契约与握手）；多候选路由基座（`backend/route`，站点作者拉取广播经其按序尝试）；库查询核心经 `LibraryQueryDeps`（装配处 app.go 构造一次、全体插件共享）注入 14 个域只读接口——work/site/resource（含 resource_store）/persistentStore/localAuthor/siteAuthor/localTag/siteTag/reWorkTag/workSet/reWorkWorkSet/reWorkSetWorkSet 各域 repository 与 settings.Service（工作目录读取）
- 被依赖：app.go 装配（进程加载、`NewLibraryQueryProvider`）；download/taskManager（TaskExecutor 任务执行）；work 模块（作品集原站序/父集关系获取器）；authorInfo 模块（站点作者信息拉取能力桥）；前端（前端扩展声明与静态资源）

## 关键设计

- **查询实现分层**：插件查询不走 search、provider 不自拼 SQL——同一过滤语义的单一落点 = 各域 repository（缺口过滤在对应域 repository 补方法，前端将来亦可复用）。
- **库查询核心无插件态**：`libraryQueryProvider` 不感知调用方插件；诊断日志与调用方归因在 `pluginContext` 调用点记录。
- **声明面门控方向性**：声明面（`extensions` 各条目）门控**主程序→插件**方向的可选能力调用（能力包在场 / 条目 `options` 声明驱动）；插件→宿主方向的库查询 RPC 无声明直接调用。
- **激活插件清单载体**：`ActivePluginLister.ListActivePlugins()` 返回 `[]ActivePlugin{PublicID, Name}`（非裸 ID 清单）——同一清单既作广播路由的候选集、又作交互面的候选展示名来源（插件未设置名时回落公开 ID）；清单按字典序返回（`loader.go:273-274,286`），「命中一个即止」的候选序据此可复现。
