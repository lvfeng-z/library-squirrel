# LibrarySquirrel 业务逻辑文档

## 项目概述

**LibrarySquirrel** 是一个用于在个人电脑中创建并维护基于标签检索的资源库的 Wails 3 桌面应用程序。主要功能包括从远程站点下载资源到本地资源库中，并提供标签式的检索服务。

**核心价值**：统一的跨站点资源管理和检索系统，支持离线内容管理和智能检索。

## 核心业务概念

### 1. 站点 (Site)

- **定义**：远程作品源，如 bilibili、pixiv 等内容平台
- **作用**：插件、站点标签、站点作者、作品的基础容器
- **管理模块**：`internal/site/`

### 2. 作品 (Work)

- **定义**：图片、视频、音频、文本等资源与其相关信息的集合
- **核心地位**：所有功能的中心数据实体
- **数据模型**：
  - 实体：`internal/work/model.go`
  - 服务：`internal/work/service.go`
  - Repository：`internal/work/repository.go`
- **关联关系**：多对多关联资源、作者、标签、任务

### 3. 任务 (Task)

- **定义**：作品创建的完整执行流程
- **业务流程**：
  1. 用户输入支持的 URL（本地文件路径或站点作品 URL）
  2. 创建任务
  3. 开始执行任务
  4. 插件处理
  5. 保存作品到资源库
- **管理模块**：`internal/task/`
- **执行引擎**：任务队列系统 (`internal/taskManager/`)

### 4. 作者系统 (双架构模式)

#### 4.1 站点作者 (Site Author)

- **来源**：来自远程站点的原始作者信息
- **获取方式**：插件下载作品时自动添加
- **管理模块**：`internal/siteAuthor/`
- **特点**：直接对应站点上的真实作者账号

#### 4.2 本地作者 (Local Author)

- **定义**：本地创建的作者，用于统一作者在不同站点的身份
- **业务价值**：实现跨站点作者统一检索
- **管理模块**：`internal/localAuthor/`
- **检索优势**：
  - 本地作者 LA 关联站点作者 SA
  - 作品 W 包含作者 SA
  - 搜索包含 LA 的作品时，作品 W 也会被搜索到

### 5. 标签系统 (双架构模式)

#### 5.1 站点标签 (Site Tag)

- **来源**：来自站点的原始标签
- **获取方式**：插件下载作品时自动添加
- **管理模块**：`internal/siteTag/`

#### 5.2 本地标签 (Local Tag)

- **定义**：本地创建的标签，用于统一具有相同含义的站点标签
- **业务价值**：实现跨站点标签统一检索
- **管理模块**：`internal/localTag/`
- **检索优势**：
  - 本地标签 LT 关联站点标签 ST
  - 作品 W 包含 ST 标签
  - 搜索包含 LT 的作品时，作品 W 也会被搜索到

### 6. 插件系统 (Plugin System)

- **目的**：扩展对不同站点的作品下载支持
- **架构位置**：`plugin/package/`
- **核心组件**：
  - `internal/plugin/loader.go` - 插件加载器
  - `internal/plugin/service.go` - 插件服务
  - `internal/pluginTaskUrlListener/` - 任务URL监听
- **预置插件**：
  - 本地导入作品插件
  - pixiv 作品下载插件
- **扩展性**：每个插件是独立包，包含作者、名称和版本元数据

## 数据模型关系

```
站点(Site) 1:n 作品(Work)
站点(Site) 1:n 站点作者(SiteAuthor)
站点(Site) 1:n 站点标签(SiteTag)

作品(Work) n:n 资源(Resource)
作品(Work) n:n 站点作者(SiteAuthor)
作品(Work) n:n 站点标签(SiteTag)
作品(Work) n:n 任务(Task)

本地作者(LocalAuthor) n:n 站点作者(SiteAuthor)
本地标签(LocalTag) n:n 站点标签(SiteTag)

插件(Plugin) 1:1 站点(Site)
```

## 业务流程详解

### 作品下载流程

```
1. 用户输入URL → 创建任务
2. 任务进入队列 → 插件执行
3. 插件获取作品信息 → 创建站点作者/标签
4. 保存作品 → 关联资源
5. 可选：关联本地作者/标签
```

### 检索流程

```
1. 用户输入检索条件（本地作者/标签）
2. 系统查找关联的站点作者/标签
3. 查询包含相关站点作者/标签的作品
4. 返回统一检索结果
```

### 插件执行流程

```
1. 任务调用对应站点插件
2. 插件访问远程站点API
3. 解析作品信息
4. 返回 PluginWorkResponseDTO
5. 系统转换为 WorkSaveDTO 并保存
```

## 架构组件关联

### 前端架构

- **框架**：Vue 3 + Composition API
- **组件**：`MainLayout.vue`（主布局）
- **状态管理**：Pinia stores
- **UI框架**：Element Plus

### 后端架构 (Go)

- **IPC通信**：Wails Bind（`window.api.method(args)`）
- **Handler注册**：`internal/{module}/handler.go` 中的 Handler 方法
- **数据库**：
  - ORM：GORM
  - 模式：Repository层
  - 事务：`database.WithTransaction()` 支持嵌套事务
- **响应格式**：`model.Success(data)` / `model.Error(msg)`

### 核心模块

- `internal/work/` - 作品业务逻辑
- `internal/task/` - 任务管理
- `internal/site/` - 站点管理
- `internal/localAuthor/` - 本地作者管理
- `internal/siteAuthor/` - 站点作者管理
- `internal/localTag/` - 本地标签管理
- `internal/siteTag/` - 站点标签管理
- `internal/plugin/` - 插件管理

## 关键数据流转

### 作品创建数据流

```
用户界面 → IPC调用 → 任务服务 → 插件执行 → 作品服务 → 数据库
                 ↓                ↓                ↓
             任务队列         作者/标签服务     资源保存
```

### 检索数据流

```
用户检索 → 本地作者/标签查询 → 关联站点作者/标签 → 作品查询 → 结果聚合
```

### 资源访问流

```
前端请求 → resource://协议 → 文件系统访问 → 图片处理(sharp) → 返回资源
```

## 典型业务用例

### 用例1：从pixiv下载作品

1. 用户在任务页面输入pixiv作品URL
2. 系统创建pixiv下载任务
3. pixiv插件执行，获取作品信息
4. 创建pixiv站点作者和标签
5. 保存作品和资源文件
6. 用户可关联本地作者/标签实现统一检索

### 用例2：跨站点作者检索

1. 用户创建本地作者"插画师A"
2. 关联pixiv作者"artist_a"和bilibili作者"artist_a_official"
3. 搜索包含"插画师A"的作品
4. 系统返回两个站点的所有相关作品

### 用例3：标签统一管理

1. 用户创建本地标签"风景"
2. 关联pixiv标签"scenery"和bilibili标签"风景摄影"
3. 搜索包含"风景"标签的作品
4. 系统返回两个站点的所有风景相关作品

## 开发注意事项

1. **IPC通信**：Wails Bind 自动生成前端 API，执行 `wails3 generate bindings -ts` 更新
2. **响应格式**：Go 端使用 `model.Success(data)` / `model.Error(msg)`
3. **事务处理**：使用 `database.WithTransaction()` 而非手动 BEGIN/COMMIT
4. **路径别名**：前端使用 `@renderer/*` 和 `@shared/*` 路径别名
5. **日期处理**：所有 datetime 字段使用 Unix 时间戳（毫秒）

## 项目文件位置速查

### 后端 (Go)

| 组件 | 路径                                |
|------|-----------------------------------|
| 程序入口 | `main.go`                         |
| 业务模块 | `internal/{module}/`              |
| 数据库基础设施 | `internal/database/`              |
| 程序配置 | `internal/config/`                |
| 共享DTO | `pkg/model/`                      |
| QueryDTO | `internal/{module}/query.go`      |
| Handler | `internal/{module}/handler.go`    |
| Service | `internal/{module}/service.go`    |
| Repository | `internal/{module}/repository.go` |

### 前端 (Vue 3)

| 组件 | 路径 |
|------|------|
| 前端组件 | `frontend/src/components/` |
| 状态管理 | `frontend/src/store/` |
| API包装 | `frontend/src/apis/` |
| 路由配置 | `frontend/src/router/` |
| 前端DTO | `frontend/src/model/` |
