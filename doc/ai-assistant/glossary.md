# LibrarySquirrel 术语表

## 核心实体术语

### 站点 (Site)

- **英文**：Site
- **定义**：远程作品源，如bilibili、pixiv等外部内容平台
- **领域角色**：插件、作品、作者、标签的容器和上下文
- **相关文件**：`backend/site/`（站点模块）

### 作品 (Work)

- **英文**：Work
- **定义**：图片、视频、音频、文本等资源与其相关信息的集合
- **领域角色**：系统的核心数据实体，所有功能的中心
- **关键属性**：标题、描述、来源站点、创建时间等
- **相关文件**：`backend/work/`（作品模块）

### 资源 (Resource)

- **英文**：Resource
- **定义**：作品的具体文件内容（图片、视频文件等）
- **领域角色**：作品的物理文件表示
- **存储方式**：本地文件系统，通过 `/store/{encoded}` HTTP 路由访问（StoreFileHandler）

### 文件持久存储 (PersistentStore)

- **英文**：PersistentStore
- **定义**：拥有独立数据库表的文件存取模块，负责所有需要长期存储的文件的记录管理与磁盘文件管理
- **领域角色**：纯粹的文件存取基础设施，保证数据库记录与磁盘文件的严格一致性
- **核心职责**：存入文件 → 返回记录 ID；按 ID/路径获取记录；删除记录时同步删除磁盘文件
- **与 Resource 的区别**：Resource 依赖 Work（`work_id`），PersistentStore 不依赖任何业务模块；其他模块通过存储 `persistent_store_id` 引用文件
- **存储根目录**：`{workDir}/store/`
- **访问协议**：`/store/` HTTP 路由（前端使用 `buildStoreUrl()` 构建请求 URL）
- **子目录管理**：所有子目录必须显式声明（如 `resource`、`thumbnail`、`avatar/local`、`avatar/site`），`Store` 时校验路径前缀
- **管理模块**：`backend/persistentStore/`
- **HTTP 服务**：`backend/assetserver/store_handler.go`

### 存储子目录 (Store Directory)

- **英文**：Store Directory (StoreDir)
- **定义**：PersistentStore 中已注册的存储子目录，路径相对于 `{workDir}/store/`
- **领域角色**：约束文件存储路径，避免目录混乱
- **已注册子目录**：`resource`（作品资源迁移过渡用）、`thumbnail`（视频缩略图）、`avatar/local`（本地作者头像）、`avatar/site`（站点作者头像）
- **多级支持**：子目录可以是多级的（如 `avatar/local`），调用方可在此基础上创建动态子目录
- **校验规则**：路径必须以某个已注册子目录为前缀（精确匹配或后接 `/`）
- **相关文件**：`backend/storeRegistry/registry.go`

### 任务 (Task)

- **英文**：Task
- **定义**：作品创建的完整执行流程
- **领域角色**：协调插件执行和作品保存的工作单元
- **状态**：等待、执行中、完成、失败等
- **相关文件**：`backend/task/`（任务模块）、`backend/taskManager/`（执行引擎）

## 作者相关术语

### 站点作者 (Site Author)

- **英文**：Site Author
- **定义**：来自远程站点的原始作者信息
- **领域角色**：直接对应站点上的真实作者账号
- **属性**：站点作者ID、作者名、介绍等
- **相关文件**：`backend/siteAuthor/`（站点作者模块）

### 本地作者 (Local Author)

- **英文**：Local Author
- **定义**：本地创建的作者，用于统一作者在不同站点的身份
- **领域角色**：跨站点作者统一检索的桥梁
- **业务价值**：实现"一次搜索，全站结果"
- **相关文件**：`backend/localAuthor/`（本地作者模块）

### 作者关联 (Author Association)

- **英文**：Author Association
- **定义**：本地作者与站点作者的关联关系
- **领域角色**：实现统一检索的关键映射
- **实现表**：`re_work_author`（作品作者关联表）

## 标签相关术语

### 站点标签 (Site Tag)

- **英文**：Site Tag
- **定义**：来自站点的原始标签
- **领域角色**：作品的原始分类标记
- **属性**：标签名、使用频率等
- **相关文件**：`backend/siteTag/`（站点标签模块）

### 本地标签 (Local Tag)

- **英文**：Local Tag
- **定义**：本地创建的标签，用于统一具有相同含义的站点标签
- **领域角色**：跨站点标签统一检索的桥梁
- **业务价值**：语义统一的标签管理
- **相关文件**：`backend/localTag/`（本地标签模块）

### 标签关联 (Tag Association)

- **英文**：Tag Association
- **定义**：作品与标签的关联关系（work↔tag；tag 来源为本地标签或站点标签，按 tag_type 区分）
- **领域角色**：作品按标签检索与组织的映射
- **实现表**：`re_work_tag`（作品标签关联表）

### 本地↔站点标签桥接 (Local↔Site Tag Bridge)

- **定义**：站点标签与本地标签的统一映射（哪个 local_tag 对应哪个 site_tag）
- **实现列**：`site_tag.local_tag_id`（站点标签记录指向本地标签的外键）
- **领域角色**：跨站点标签统一检索的桥梁

## 插件相关术语

### 插件 (Plugin)

- **英文**：Plugin
- **定义**：扩展对不同站点的作品下载支持的 Go 共享库模块（`.dll`/`.so`）
- **领域角色**：系统扩展性的核心组件
- **结构**：独立包，包含作者、名称、版本元数据，导出 `Activate(PluginContext)` 函数
- **相关文件**：`backend/plugin/`，`backend/plugin/extension/`

### 插件上下文 (PluginContext)

- **英文**：PluginContext
- **定义**：主程序提供给插件的完整 API，每个插件拥有独立实例。接口定义在 SDK 库中
- **领域角色**：插件访问主程序能力的唯一入口
- **能力**：扩展点注册/注销、数据持久化、加密存储、业务查询、任务管理、日志
- **接口来源**：`github.com/lvfeng-z/library-squirrel-sdk`（SDK）
- **实现**：`backend/plugin/extension/plugin_context.go`

### 注册器 (Registrar)

- **英文**：Registrar
- **定义**：PluginContext 内嵌的扩展点注册接口
- **领域角色**：插件通过 Registrar 注册 TaskHandler、SiteBrowser 扩展点（Slot 已改为声明式注册）
- **方法**：`RegisterTaskHandler`、`RegisterSiteBrowser`（`RegisterSlot` 已移除）
- **相关文件**：`backend/plugin/extension/registrar.go`

### Provider 接口 (Provider Interface)

- **英文**：Provider Interface
- **定义**：PluginContext 的服务依赖接口，由 `extension` 包定义，各 `backend` 服务实现
- **领域角色**：实现依赖倒置，隔离 PluginContext 与内部服务
- **接口列表**：`PluginStorageService`、`WorkSetQueryProvider`、`SiteSaveProvider`、`TaskCreateProvider`、`UrlListenerRegistry`
- **相关文件**：`backend/plugin/extension/plugin_context.go`

### 扩展点 (Extension Point)

- **英文**：Extension Point
- **定义**：插件可贡献的三类功能：TaskHandler、SiteBrowser、Slot
- **领域角色**：插件与主程序的功能契约
- **注册中心**：`TaskHandlerRegistry`、`SiteBrowserRegistry`、`SlotRegistry`
- **相关文件**：`backend/plugin/extension/`、`github.com/lvfeng-z/library-squirrel-sdk`

### 插件 SDK (Plugin SDK)

- **英文**：Plugin SDK
- **定义**：独立第三方 Go 库，定义主程序与插件之间的共享接口和类型
- **模块路径**：`github.com/lvfeng-z/library-squirrel-sdk`
- **包含内容**：PluginContext、TaskHandler、SiteBrowser 接口；SlotType/ContentType 枚举；Task/Work/WorkSet/Site 等效实体类型（使用指针替代 sql.Null*）；各类 DTO
- **领域角色**：主程序和插件共同依赖的接口契约，实现开发环境隔离

### 声明式 Slot 注册 (Declarative Slot Registration)

- **英文**：Declarative Slot Registration
- **定义**：通过 `plugin.json` 的 `extensions.slots` 配置声明 UI 扩展，主程序在启动时自动读取和注册
- **领域角色**：替代运行时 `PluginContext.RegisterSlot()`，简化插件开发，支持纯 UI 插件
- **流程**：主程序读取 `plugin.json` → 构建 `SlotConfig` → `SlotRegistry.Register()`
- **优势**：无需 DLL 加载、无需 Go 编译、配置即注册

### 纯 UI 插件 (Pure UI Plugin)

- **英文**：Pure UI Plugin
- **定义**：仅包含 Slot 扩展（无 TaskHandler/SiteBrowser）的插件，不需要 DLL 入口文件
- **领域角色**：降低插件开发门槛，轻量级 UI 扩展
- **判断条件**：`extensions.taskHandlers` 和 `extensions.siteBrowsers` 均为空

### 静态资源服务 (Static Resource Service)

- **英文**：Static Resource Service
- **定义**：线程安全的插件静态资源 HTTP 服务，提供 `http://wails.localhost:{port}/plugin/{author}/{id}/{version}/...` URL 访问
- **领域角色**：插件 Vue/JS/CSS/HTML/图片等文件的安全分发
- **安全机制**：路径遍历防护、目录白名单校验、ETag 缓存
- **相关文件**：`backend/plugin/extension/static_resource_service.go`

### 组合 Asset Handler (PluginAwareAssetHandler)

- **英文**：Plugin-Aware Asset Handler
- **定义**：组合前端嵌入式资源（embed.FS）与插件静态资源的 HTTP handler
- **领域角色**：Wails asset handler 的扩展，使插件资源通过同一 HTTP 路由（`/plugin/` 前缀）访问
- **路由规则**：`/plugin/` 前缀 → StaticResourceService，其余 → 前端 embed.FS
- **相关文件**：`backend/plugin/extension/asset_handler.go`

### 内容类型 (ContentType)

- **英文**：Content Type
- **定义**：Slot 内容的格式类型
- **可选值**：`vueSource`（Vue SFC）、`precompiled`（预编译 JS/CSS）、`code`（行内 JS）、`html`（HTML 文件）
- **已废弃**：`component`

### 类型适配器 (Type Adapter)

- **英文**：Type Adapter
- **定义**：主程序中 `convert.go` 的 `taskHandlerAdapter`，将 `pluginsdk.TaskHandler` 适配为内部 `dto.TaskHandler`
- **领域角色**：桥接 SDK 类型和主程序内部 entity/DTO 类型，使 task/service、taskManager 等模块无需修改

### 插件任务 (Plugin Task)

- **英文**：Plugin Task
- **定义**：由插件处理的具体作品下载任务
- **领域角色**：插件执行的工作单元
- **处理流程**：URL解析 → 数据获取 → 作品信息提取

### 插件响应DTO (PluginWorkResponseDTO)

- **英文**：Plugin Work Response DTO
- **定义**：插件返回的作品数据格式
- **领域角色**：插件与主系统间的数据交换格式
- **转换**：由`WorkService`转换为`WorkSaveDTO`

## 系统架构术语

### IPC通信 (IPC Communication)

- **英文**：Inter-Process Communication
- **定义**：后端（Go）与前端（Vue）之间的通信机制
- **模式**：Wails Bind —— Go Handler 方法自动暴露给前端，前端经自动生成的 bindings（`frontend/bindings/`）调用，Wrapper 层封装于 `frontend/src/apis/http/wrappers/`
- **相关文件**：`backend/{module}/handler.go`、`frontend/bindings/`、`frontend/src/apis/http/`

### 文件访问路由 (File Access Route)

- **英文**：File Access Route
- **定义**：Wails 架构下，资源文件经 HTTP 路由访问（非旧 Electron 的 `resource://` 协议，该协议已废弃）
- **作品文件**：`/store/{encoded}` 路由 → `StoreFileHandler`（`backend/assetserver/`）解析为 `{workDir}` 下的文件；前端用 `buildStoreUrl()` 构建 URL
- **插件静态资源**：`/plugin/{id}/{ver}/...` 路由 → 插件静态资源服务

### 任务队列 (Task Queue)

- **英文**：Task Queue
- **定义**：管理任务执行顺序和并发的系统
- **领域角色**：协调任务执行，防止资源冲突
- **相关文件**：`backend/taskManager/`（多根调度、信号量、批量查重、flush 批量持久化）

### Repository 模式 (Repository Pattern)

- **英文**：Repository Pattern
- **定义**：数据访问逻辑的抽象，封装数据库操作，将业务逻辑与数据存储分离
- **领域角色**：业务逻辑与数据存储的隔离层
- **Go 实现**：各模块 `backend/{module}/repository.go` 定义接口与实现；泛型 `BaseRepository[T]`（`backend/database/`）提供通用 CRUD + 分页，模块仅在无法表达时编写自定义逻辑
- **规范**：Service 只依赖接口，禁止直接导入 `backend/database`；事务经 `database.WithTransactionContext()` 参与，Repository 通过 `DBFromContext` 自动加入事务

### 统一响应格式 (Unified Response Format)

- **英文**：Unified Response Format
- **定义**：所有 IPC 响应的标准格式
- **组成**：`{ success: boolean, msg: string, data: T }`
- **后端**：`model.Success(data)` / `model.Error(msg)`（`backend/base/model/api_response.go`，泛型 `ApiResponse[T]`）
- **前端**：Wrapper 经 `requireResponse<T>()`（`frontend/src/apis/http/types.ts`）校验并转为 `data` 保证非空的 `ApiResult<T>`；调用方用 `ApiUtil.check/data/msg`（`frontend/src/utils/ApiUtil.ts`）读取

## 业务流程术语

### 作品下载流程 (Work Download Flow)

- **英文**：Work Download Flow
- **定义**：从URL输入到作品保存的完整过程
- **步骤**：URL输入 → 任务创建 → 插件执行 → 信息获取 → 作品保存

### 统一检索 (Unified Search)

- **英文**：Unified Search
- **定义**：通过本地实体关联实现跨站点检索
- **机制**：本地作者/标签关联多个站点作者/标签
- **优势**：一次搜索返回所有关联站点的结果

### 关联映射 (Association Mapping)

- **英文**：Association Mapping
- **定义**：本地实体与站点实体间的关联关系
- **类型**：作者关联映射、标签关联映射
- **表结构**：关联表存储映射关系

### 语义统一 (Semantic Unification)

- **英文**：Semantic Unification
- **定义**：将不同站点中含义相同或相似的标签/作者统一管理
- **实现**：通过本地标签/作者关联多个站点标签/作者

## 数据模型术语

### DTO (Data Transfer Object)

- **英文**：Data Transfer Object
- **定义**：数据传输对象，用于层间/进程间数据传递
- **示例**：`WorkSaveDTO`、`PluginWorkResponseDTO`
- **位置**：`backend/base/model/dto/`

### QueryDTO (Query Data Transfer Object)

- **英文**：Query Data Transfer Object
- **定义**：查询参数数据传输对象，封装分页、排序、筛选等查询条件
- **示例**：`WorkQueryDTO`、`SiteQueryDTO`、`LocalAuthorQueryDTO`
- **位置**：`backend/{module}/query.go`

### Entity (实体)

- **英文**：Entity
- **定义**：对应数据库表的领域实体，统一嵌入 `BaseEntity`（ID/CreateTime/UpdateTime）
- **示例**：`Work`、`Site`、`LocalAuthor`
- **位置**：`backend/base/model/entity/`

### Domain Object (领域对象)

- **英文**：Domain Object
- **定义**：业务领域中的核心概念对象，多为运行期组装的内存结构
- **示例**：`RankedSiteAuthor`、`WorkWithWorkSetId`
- **位置**：分散于各业务模块内部（如 `backend/reWorkAuthor/`、`backend/search/`），无独立 domain 目录

## 开发约定术语

### IPC 方法命名约定 (IPC Method Naming)

- **英文**：IPC Method Naming
- **定义**：Wails Bind 自动生成前端调用方法名的规则
- **规则**：`{ServiceName}{MethodName}`（驼峰拼接），前端经 bindings 调用，如 `WorkService.GetById` → `workServiceGetById`
- **生成**：修改 Go Handler 后执行 `wails3 generate bindings -ts` 重新生成 `frontend/bindings/`

### 路径别名 (Path Alias)

- **英文**：Path Alias
- **定义**：TypeScript 中定义的路径简写
- **配置**：`frontend/tsconfig.json` 与 `frontend/vite.config.js`
- **可用别名**：`@renderer/*` → `frontend/src/*`、`@bindings/*` → `frontend/bindings/*`、`@apis/*` → `frontend/src/apis/*`

### 事务 (Transaction)

- **英文**：Transaction
- **定义**：基于 context 注入的事务机制，支持嵌套
- **优势**：Repository 经 `database.DBFromContext(ctx)` 自动获取事务 DB，无需感知事务存在
- **实现**：`database.WithTransactionContext()`（`backend/database/`）

## Go 主进程重构术语

### Repository 模式 (Repository Pattern)

- **英文**：Repository Pattern
- **定义**：数据访问逻辑的抽象，将业务逻辑与数据存储分离
- **Go 实现**：
  - 接口与实现均在 `backend/{module}/repository.go`（无单独 `repository_impl.go`）
  - Service 只依赖接口，不直接访问数据库
- **优势**：解耦、可测试、消除循环依赖

### 依赖倒置 (Dependency Inversion)

- **英文**：Dependency Inversion
- **定义**：高层模块不依赖低层模块，两者都依赖抽象
- **Go 实现**：接口由调用方定义，实现方隐式满足
- **示例**：`WorkService` 需要调用 `AuthorService` 时，定义 `AuthorProvider` 接口

### 包内聚合 (Package-Level Aggregation)

- **英文**：Package-Level Aggregation
- **定义**：将相关的接口、实现放在同一业务模块包内
- **结构**（以某业务模块为例）：
  ```
  backend/{module}/
  ├── handler.go           # Wails Bind 方法（暴露给前端）
  ├── service.go           # 业务逻辑
  ├── repository.go        # 数据访问接口 + 实现
  └── query.go             # 查询 DTO
  ```
- **实体位置**：领域实体集中在 `backend/base/model/entity/`（嵌入 `BaseEntity`），不在各模块内

### context.Context

- **英文**：Context
- **定义**：Go 中用于传递请求范围的值、取消信号和超时控制
- **规范**：所有 Repository 和 Service 方法第一个参数必须是 `context.Context`

### 错误 sentinel

- **英文**：Sentinel Error
- **定义**：使用预定义的错误变量进行错误判断
- **示例**：`var ErrNotFound = errors.New("not found")`
- **判断方式**：`errors.Is(err, ErrNotFound)`

### 叶子节点 (Leaf Node)

- **英文**：Leaf Node
- **定义**：在依赖图中不依赖其他业务模块的模块
- **示例**：底层基础模块（如 `site`、`localTag`）被上层模块依赖，自身不依赖其他业务模块
- **优势**：不会形成循环依赖，可以安全地被多个模块引用

---

## 更新记录

### 2026-06-25
- [重构] 系统架构/数据模型/开发约定术语：清除 Electron/Node 遗留（ipcRenderer、preload、BaseDao、src/main、src/renderer、src/shared、SAVEPOINT、ApiUtil.response 等），对齐 Wails/Go 现状（Wails Bind、BaseRepository、ApiResponse{msg}、@renderer/@bindings/@apis 别名、WithTransactionContext）
- [修改] 各实体术语的"相关文件"由虚构 `.ts` 改为后端模块目录；Repository 包内聚合/叶子节点示例修正

### 2026-06-04
- [新增] 文件持久存储 (PersistentStore)、存储子目录 (StoreDir) 术语

### 2026-05-06
- [新增] 声明式 Slot 注册、纯 UI 插件、静态资源服务、组合 Asset Handler、ContentType 术语
- [修改] 注册器 (Registrar) 说明：移除 RegisterSlot
- [新增] 插件 SDK (Plugin SDK) 术语
- [新增] 类型适配器 (Type Adapter) 术语
- [修改] PluginContext 说明更新为 SDK 定义

### 2026-05-04

- [修改] 插件术语更新为 Go/Wails 架构描述
- [新增] PluginContext、Registrar、Provider 接口、扩展点术语

### 2026-04-22

- [修改] QueryDTO 位置更新：`src/main/model/queryDTO/` → `backend/base/model/dto/query/`
- [修改] QueryDTO 示例补充完整模块列表
- [新增] 说明 QueryDTO 按模块分文件存储

### 2026-05-05
- [修改] 目录结构调整：`internal/` → `backend/`，`pkg/` → `backend/base/`
