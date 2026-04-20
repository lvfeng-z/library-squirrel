# LibrarySquirrel 术语表

## 核心实体术语

### 站点 (Site)

- **英文**：Site
- **定义**：远程作品源，如bilibili、pixiv等外部内容平台
- **领域角色**：插件、作品、作者、标签的容器和上下文
- **相关文件**：`Site.ts`（实体），`SiteService.ts`（服务）

### 作品 (Work)

- **英文**：Work
- **定义**：图片、视频、音频、文本等资源与其相关信息的集合
- **领域角色**：系统的核心数据实体，所有功能的中心
- **关键属性**：标题、描述、来源站点、创建时间等
- **相关文件**：`Work.ts`（实体），`WorkService.ts`（服务）

### 资源 (Resource)

- **英文**：Resource
- **定义**：作品的具体文件内容（图片、视频文件等）
- **领域角色**：作品的物理文件表示
- **存储方式**：本地文件系统，通过`resource://`协议访问

### 任务 (Task)

- **英文**：Task
- **定义**：作品创建的完整执行流程
- **领域角色**：协调插件执行和作品保存的工作单元
- **状态**：等待、执行中、完成、失败等
- **相关文件**：`TaskService.ts`，`taskQueue.ts`

## 作者相关术语

### 站点作者 (Site Author)

- **英文**：Site Author
- **定义**：来自远程站点的原始作者信息
- **领域角色**：直接对应站点上的真实作者账号
- **属性**：站点作者ID、作者名、介绍等
- **相关文件**：`SiteAuthor.ts`，`SiteAuthorService.ts`

### 本地作者 (Local Author)

- **英文**：Local Author
- **定义**：本地创建的作者，用于统一作者在不同站点的身份
- **领域角色**：跨站点作者统一检索的桥梁
- **业务价值**：实现"一次搜索，全站结果"
- **相关文件**：`LocalAuthor.ts`，`LocalAuthorService.ts`

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
- **相关文件**：`SiteTag.ts`，`SiteTagService.ts`

### 本地标签 (Local Tag)

- **英文**：Local Tag
- **定义**：本地创建的标签，用于统一具有相同含义的站点标签
- **领域角色**：跨站点标签统一检索的桥梁
- **业务价值**：语义统一的标签管理
- **相关文件**：`LocalTag.ts`，`LocalTagService.ts`

### 标签关联 (Tag Association)

- **英文**：Tag Association
- **定义**：本地标签与站点标签的关联关系
- **领域角色**：实现统一检索的关键映射
- **实现表**：`re_work_tag`（作品标签关联表）

## 插件相关术语

### 插件 (Plugin)

- **英文**：Plugin
- **定义**：扩展对不同站点的作品下载支持的模块
- **领域角色**：系统扩展性的核心组件
- **结构**：独立包，包含作者、名称、版本元数据
- **相关文件**：`PluginLoader.ts`，`PluginFactory.ts`

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
- **定义**：主进程与渲染进程间的通信机制
- **模式**：`ipcRenderer.invoke('service-method', args)`
- **相关文件**：`MainProcessApi.ts`，`preload/index.ts`

### 自定义协议 (Custom Protocol)

- **英文**：Custom Protocol
- **定义**：`resource://`协议，用于访问本地资源文件
- **领域角色**：安全的本地文件访问机制
- **实现**：在`src/main/index.ts`中注册和处理

### 任务队列 (Task Queue)

- **英文**：Task Queue
- **定义**：管理任务执行顺序和并发的系统
- **领域角色**：协调任务执行，防止资源冲突
- **相关文件**：`taskQueue.ts`

### DAO模式 (Data Access Object)

- **英文**：Data Access Object
- **定义**：数据访问对象模式，封装数据库操作
- **领域角色**：业务逻辑与数据存储的隔离层
- **基类**：`BaseDao.ts`提供通用CRUD操作

### 统一响应格式 (Unified Response Format)

- **英文**：Unified Response Format
- **定义**：所有IPC响应的标准格式
- **组成**：`{ success: boolean, data?: any, message?: string }`
- **工具**：`ApiUtil.response()` / `ApiUtil.error()`

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
- **定义**：数据传输对象，用于进程间或层间数据传递
- **示例**：`WorkSaveDTO`、`PluginWorkResponseDTO`
- **位置**：`src/main/model/dto/`

### QueryDTO (Query Data Transfer Object)

- **英文**：Query Data Transfer Object
- **定义**：查询参数数据传输对象
- **示例**：`WorkQueryDTO`、`SiteQueryDTO`
- **位置**：`src/main/model/queryDTO/`

### Entity (实体)

- **英文**：Entity
- **定义**：对应数据库表的领域实体
- **示例**：`Work`、`Site`、`Author`
- **位置**：`src/main/model/entity/`

### Domain Object (领域对象)

- **英文**：Domain Object
- **定义**：业务领域中的核心概念对象
- **示例**：`RankedSiteAuthor`、`WorkWithWorkSetId`
- **位置**：`src/main/model/domain/`

## 开发约定术语

### 服务方法命名约定 (Service Method Naming Convention)

- **英文**：Service Method Naming Convention
- **定义**：IPC方法命名的统一规则
- **主进程**：`'serviceName-methodName'`
- **预加载**：`serviceNameMethodName`
- **示例**：`'work-save'` ↔ `workSave()`

### 路径别名 (Path Alias)

- **英文**：Path Alias
- **定义**：TypeScript中定义的路径简写
- **渲染进程**：`@renderer/*` → `src/renderer/src/*`
- **配置**：`tsconfig.web.json`

### SAVEPOINT事务 (SAVEPOINT Transaction)

- **英文**：SAVEPOINT Transaction
- **定义**：使用SAVEPOINT而非BEGIN/COMMIT的事务机制
- **优势**：支持嵌套事务
- **实现**：`DatabaseClient.transaction()`

## Go 主进程重构术语

### Repository 模式 (Repository Pattern)

- **英文**：Repository Pattern
- **定义**：数据访问逻辑的抽象，将业务逻辑与数据存储分离
- **Go 实现**：
  - 接口定义在 `internal/{module}/repository.go`
  - 实现代码在 `internal/{module}/repository_impl.go`
  - Service 只依赖接口，不直接访问数据库
- **优势**：解耦、可测试、消除循环依赖

### 依赖倒置 (Dependency Inversion)

- **英文**：Dependency Inversion
- **定义**：高层模块不依赖低层模块，两者都依赖抽象
- **Go 实现**：接口由调用方定义，实现方隐式满足
- **示例**：`WorkService` 需要调用 `AuthorService` 时，定义 `AuthorProvider` 接口

### 包内聚合 (Package-Level Aggregation)

- **英文**：Package-Level Aggregation
- **定义**：将相关的接口、实现、实体放在同一包内
- **结构**：
  ```
  internal/author/
  ├── model.go             # 领域实体
  ├── repository.go        # 接口定义
  ├── repository_impl.go   # 实现
  └── service.go           # 业务逻辑
  ```

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
- **示例**：`relations` 模块被 `work` 依赖，但不依赖其他业务模块
- **优势**：不会形成循环依赖，可以安全地被多个模块引用
