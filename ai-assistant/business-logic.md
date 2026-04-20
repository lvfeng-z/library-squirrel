# LibrarySquirrel 业务逻辑文档

## 项目概述

**LibrarySquirrel** 是一个用于在个人电脑中创建并维护基于标签检索的资源库的 Electron 桌面应用程序。主要功能包括从远程站点下载资源到本地资源库中，并提供标签式的检索服务。

**核心价值**：统一的跨站点资源管理和检索系统，支持离线内容管理和智能检索。

## 核心业务概念

### 1. 站点 (Site)

- **定义**：远程作品源，如 bilibili、pixiv 等内容平台
- **作用**：插件、站点标签、站点作者、作品的基础容器
- **管理服务**：`SiteService.ts`

### 2. 作品 (Work)

- **定义**：图片、视频、音频、文本等资源与其相关信息的集合
- **核心地位**：所有功能的中心数据实体
- **数据模型**：
  - 实体：`src/main/model/entity/Work.ts`
  - 服务：`src/main/service/WorkService.ts`
  - DAO：`src/main/dao/WorkDao.ts`
- **关联关系**：多对多关联资源、作者、标签、任务

### 3. 任务 (Task)

- **定义**：作品创建的完整执行流程
- **业务流程**：
  1. 用户输入支持的 URL（本地文件路径或站点作品 URL）
  2. 创建任务
  3. 开始执行任务
  4. 插件处理
  5. 保存作品到资源库
- **管理服务**：`TaskService.ts`
- **执行引擎**：任务队列系统 (`src/main/core/taskQueue.ts`)

### 4. 作者系统 (双架构模式)

#### 4.1 站点作者 (Site Author)

- **来源**：来自远程站点的原始作者信息
- **获取方式**：插件下载作品时自动添加
- **管理服务**：`SiteAuthorService.ts`
- **特点**：直接对应站点上的真实作者账号

#### 4.2 本地作者 (Local Author)

- **定义**：本地创建的作者，用于统一作者在不同站点的身份
- **业务价值**：实现跨站点作者统一检索
- **管理服务**：`LocalAuthorService.ts`
- **检索优势**：
  - 本地作者 LA 关联站点作者 SA
  - 作品 W 包含作者 SA
  - 搜索包含 LA 的作品时，作品 W 也会被搜索到

### 5. 标签系统 (双架构模式)

#### 5.1 站点标签 (Site Tag)

- **来源**：来自站点的原始标签
- **获取方式**：插件下载作品时自动添加
- **管理服务**：`SiteTagService.ts`

#### 5.2 本地标签 (Local Tag)

- **定义**：本地创建的标签，用于统一具有相同含义的站点标签
- **业务价值**：实现跨站点标签统一检索
- **管理服务**：`LocalTagService.ts`
- **检索优势**：
  - 本地标签 LT 关联站点标签 ST
  - 作品 W 包含 ST 标签
  - 搜索包含 LT 的作品时，作品 W 也会被搜索到

### 6. 插件系统 (Plugin System)

- **目的**：扩展对不同站点的作品下载支持
- **架构位置**：`src/main/plugin/`
- **核心组件**：
  - `PluginLoader.ts` - 插件加载器
  - `PluginFactory.ts` - 插件工厂
  - `TaskHandler.ts` - 任务处理器
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

### 前端架构 (Renderer Process)

- **框架**：Vue 3 + Composition API
- **组件**：`Guide.vue`（用户指南页面）
- **状态管理**：Pinia stores
  - `UseTourStatesStore.ts` - 导览状态管理
  - `UsePageStatesStore.ts` - 页面状态管理
  - `UseNotificationStore.ts` - 通知管理
- **UI框架**：Element Plus

### 后端架构 (Main Process)

- **IPC通信**：
  - `MainProcessApi.ts` - 主进程API注册中心
  - `preload/index.ts` - 预加载脚本桥接
  - 模式：`ipcRenderer.invoke('service-method', args)`
- **数据库**：
  - Client：`DatabaseClient` 连接池
  - 模式：DAO层 + BaseDao基类
  - 事务：SAVEPOINT支持嵌套事务
- **自定义协议**：`resource://` 协议处理本地文件访问

### 核心服务

- `WorkService.ts` - 作品业务逻辑
- `TaskService.ts` - 任务管理
- `SiteService.ts` - 站点管理
- `LocalAuthorService.ts` - 本地作者管理
- `SiteAuthorService.ts` - 站点作者管理
- `LocalTagService.ts` - 本地标签管理
- `SiteTagService.ts` - 站点标签管理
- `PluginService.ts` - 插件管理

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

## 扩展点和新功能开发

### 添加新站点支持

1. 创建新插件包
2. 实现站点特定解析逻辑
3. 注册到插件系统
4. 自动支持所有现有功能（作者/标签统一、检索等）

### 添加新功能模块

1. 创建对应Service类
2. 在MainProcessApi注册IPC方法
3. 在preload脚本添加API包装
4. 创建前端组件和Store

### 数据库扩展

1. 在`createDataTables.yml`定义新表
2. 创建Entity、DTO、QueryDTO模型
3. 创建DAO类继承BaseDao
4. 创建Service类继承BaseService

## 开发注意事项

1. **IPC命名约定**：`serviceName-methodName`（主进程）对应`serviceNameMethodName`（预加载）
2. **响应格式**：统一使用`ApiUtil.response(data)`/`ApiUtil.error(message)`
3. **事务处理**：使用SAVEPOINT而非BEGIN/COMMIT
4. **路径别名**：渲染进程使用`@renderer/*`路径别名
5. **日期处理**：所有datetime字段使用Unix时间戳（毫秒）

## 项目文件位置速查

### Node.js 主进程 (当前活跃)

| 组件 | 路径 |
|------|------|
| 业务逻辑 | `src/main-old/service/` |
| 数据模型 | `src/main-old/model/` |
| 数据库操作 | `src/main-old/dao/` |
| 插件系统 | `src/main-old/plugin/` |
| IPC注册 | `src/main-old/core/MainProcessApi.ts` |
| 任务队列 | `src/main-old/core/taskQueue.ts` |
| 数据库配置 | `src/main-old/resources/database/createDataTables.yml` |

### Go 主进程 (重构中)

| 组件 | 路径 |
|------|------|
| 程序入口 | `my-ipc-service/cmd/server/main.go` |
| 业务模块 | `my-ipc-service/internal/{module}/` |
| 数据库基础设施 | `my-ipc-service/internal/database/` |
| 程序配置 | `my-ipc-service/internal/config/` |
| 共享DTO | `my-ipc-service/pkg/model/` |

> **注意**：Go 重构正在进行中，原 Node.js 代码位于 `src/main-old/`。重构完成后将替换为 `my-ipc-service/`。

### 前端 (Renderer)

| 组件 | 路径 |
|------|------|
| 前端组件 | `src/renderer/src/components/` |
| 状态管理 | `src/renderer/src/store/` |
| API包装 | `src/renderer/src/apis/` |
| 路由配置 | `src/renderer/src/router/` |
