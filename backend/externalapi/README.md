# externalapi 模块说明

## 一句话职责

**外部 AI 操作接口**：本机回环 HTTP JSON 操作面（只绑 `127.0.0.1`），供部署在同一台机器上的外部智能体运行时以程序化方式操作本软件——人用 GUI、机用接口，两入口汇入同一服务层。模块自持监听器、Bearer token 鉴权、操作注册表与能力清单、发现文件契约与外部操作记录问责账；操作的目标方法是各业务模块既有 handler（本模块不复制业务逻辑、不直访仓储）。

面向用户的接入说明见 `doc/external-api-guide.md`；方案全貌见私有环境仓 `library-squirrel-docs/plan/外部AI操作接口方案.md`。

## 边界

- 与 **journal**（领域侧记账）：两领域分离。journal 记领域内状态迁移，本模块只**消费**其游标读面（`/api/journal`、`journal.since`），依赖方向单向——journal 永不反向依赖本模块。
- 与 **settings**：设置项（开关/端口/token）由 settings 模块持有与持久化；本模块只提供运行时联动实现（`Lifecycle` 满足 settings 侧 `ExternalApiRuntime` 接口），不做设置读写。
- 与 **assetserver / Wails 资产管线**：无交集——接口是独立 goroutine + `net/http`，不参与前端静态资源服务。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `ListOpRecords(pageNumber, pageSize)` | 分页列出外部操作记录（最新在前），设置页「外部操作记录」只读列表用 |

记录的**写入不经此处**：统一在操作分发层落账（`oprecord.go` 的 `recordInvocation`），本面只做问责回看。

## 核心概念

- **Operation / Registry**（`registry.go`）：每个操作 = 标识 + 参数解码 + 目标方法调用 + 结果编码，**显式注册**（类型安全、可枚举），不经反射泛调；注册表之外不存在任何动态路由。
- **能力清单 / manifest**（`manifest.go`）：操作目录由注册表枚举（注册即入清单，**不存在第二份操作表**），每操作附参数摘要（名称/类型/是否必填/说明）；固定端点（探测、清单、记账与记录查询）单列一节。清单是机器面接入的单一来源。
- **Server**（`server.go`）：回环监听 + `Start`/`Stop` 幂等 + Bearer 鉴权中间件 + 分发层并发信号量；状态码语义为鉴权失败 401、操作未注册 404、请求体超限 413、并发超限 503，**操作已调用的成败一律 200、由信封表达**。信封与前端面同构（`model.ApiResponse`：失败=纯失败、msg 用户可读）。
- **Lifecycle**（`lifecycle.go`）：设置目标态到运行时的差量同步单一入口，三项联动——开关→监听起停与发现文件写删、固定端口变更→换绑、token 变更→换钥（旧 token 立即失效）与发现文件重写；重复进入同一目标态为空操作。
- **发现文件 `api-access.json`**：写于用户数据目录（`{enabled, port, token, version}`），文件即产品契约；接口未启用时文件不存在而非写 `false`。
- **外部操作记录**（`oprecord.go` + `entity.ExternalApiOpRecord`）：接口私有的问责账，行只追加不更新、自增 `id` 即消费游标；参数摘要**写入即脱敏**（URL 类值仅落协议+主机+路径，查询串只记存在与字符数）；按 TTL 365 天 + 容量 20 万行裁最旧（**写账时顺带触发轮转**，轮转失败仅日志不阻断写账；`create_time` 索引由迁移后置段建出，保证轮转 TTL 轨不走全表扫描）。**被动记录，无事前把关职能**——把关时刻是任务状态机的非阻塞确认态。
- **双游标读面**：`/api/journal`（记账）与 `/api/oplog`（操作记录）返回同构游标契约（`Entries`/`NextCursor`/`HasMore`/`CursorExpired`，Go 字段名直出），消费者按同一套字段消费。
- **操作面两族**：只读（`ops_query.go`：作品/任务/插件/存储位置/资源/记账）与写面（`ops_task.go`：URL 建任务含消歧显选键透传、任务树启停/重试/强停、覆盖确认单个与批量）；确认与消歧全程复用任务模块既有「持久状态 + 后续调用解除」状态机，不建阻塞通道。

## 依赖关系

- 依赖：`base/model`（信封）、`base/model/entity`（记录实体）、`base/constant`（应用版本）、`base/logger`、`database`（记录仓储基类）、`util`（时间戳）。
- 经接口注入的依赖面（装配期由 `app.go` 注入，缺项即注册报错）：`QueryDeps`（search/work/task/taskManager/plugin/storageLocation/resource/journal 各 handler）、`TaskDeps`（task/taskManager handler）、`OpRecordStore`（记录服务）。
- 被依赖：settings 模块（`ExternalApiRuntime` 由 `Lifecycle` 实现，开关与 token 控制方法经它同步运行时）；前端设置页（经 bindings 调 `ListOpRecords`，开关/token 经 settings 的绑定方法）。

## 关键设计

- **token 防线定位**：防误触、防跨用户、提供归属标识，**不是恶意进程防线**——同机同用户进程可读发现文件；`X-Client-Label` 为调用方自报归属、非鉴别。
- **鉴权与并发兜底**：token 为内存单点比对（重置即换钥，无授权对象清单、无操作级权限）；HTTP 面首次引入可无上限并发的入口，v1 以分发层信号量限并发兜底，各入口共用同一上限。
- **落账口径**：固定端点（health/manifest/oplog）不落账，经操作分发的调用（含 `/api/journal`）落账；落账尽力而为——写账失败仅日志，不改变操作响应语义。
