# AI Assistant Documentation for LibrarySquirrel

## 文档概述

本目录包含为AI助手（如Claude Code）准备的LibrarySquirrel项目文档，旨在帮助AI快速理解项目架构、业务逻辑和开发模式，从而更高效地进行代码分析、问题诊断和开发任务。

## 文档结构

### 1. [business-logic.md](business-logic.md) - 完整业务逻辑文档

- **用途**：全面理解项目的业务模型、数据流和架构
- **适合场景**：需要深度理解项目整体架构时
- **包含内容**：
  - 项目概述和核心价值
  - 详细业务概念解释
  - 数据模型关系图
  - 完整业务流程
  - 架构组件详细说明
  - 典型业务用例

### 2. [architecture-quick-reference.md](architecture-quick-reference.md) - 架构速查

- **用途**：快速查找核心概念和技术要点
- **适合场景**：开发任务中需要快速参考时
- **包含内容**：
  - 核心业务概念速查表
  - 关键业务逻辑总结
  - 技术架构要点
  - 开发模式和常见场景

### 3. [glossary.md](glossary.md) - 术语表

- **用途**：统一理解项目中的领域特定术语
- **适合场景**：遇到不熟悉的术语时参考
- **包含内容**：
  - 核心实体术语定义
  - 作者/标签系统术语
  - 插件系统术语
  - 架构和开发约定术语


### 4. [plugin-development.md](plugin-development.md) - 插件系统开发指南

- **用途**：理解插件系统的架构和开发方法
- **适合场景**：开发或扩展插件功能时
- **包含内容**：
  - 插件目录结构和核心概念
  - 插件清单配置和激活类型
  - 贡献点（TaskHandler、SiteBrowser）
  - 插件上下文 API
  - 完整插件开发示例
  - 主程序端管理器
  - IPC 通信和扩展方法

### 5. [task-execution-flow.md](task-execution-flow.md) - 任务执行流程

- **用途**：理解任务从创建到完成的完整生命周期
- **适合场景**：修改任务执行逻辑、排查任务问题、理解事务和补偿机制时
- **包含内容**：
  - 任务状态模型（10 个状态、转换图、稳定/瞬态分类）
  - 完整任务生命周期（创建、启动、执行、暂停/恢复、停止、重试、清理）
  - 事务边界（SaveWorkInfo、StoreStream+Resource、任务创建）
  - 补偿机制（备份还原、事务失败文件清理）
  - 崩溃恢复（PendingResourceID 机制）
  - 插件交互（TaskExecutor 接口、StoreWriter 生命周期）

### 6. [module-migration-guide.md](module-migration-guide.md) - 模块迁移修复指南

- **用途**：模块从 Electron 架构修复到 Wails 架构的统一模式参考
- **适合场景**：修复尚未完成迁移的业务模块时参考
- **包含内容**：
  - 后端修复模式（N+1 查询消除、依赖注入、nullable 参数）
  - 前端修复模式（QueryAttribute 适配、类型统一、错误处理）
  - 已完成模块记录与待修复模块清单
  - 修复验证要点

### 7. [tour-feature.md](tour-feature.md) - 向导功能规格

- **用途**：理解向导（Tour）功能的架构、数据模型、运行时序和扩展规范
- **适合场景**：新增向导、修改向导引擎、排查跨页面引导或元素高亮问题时
- **包含内容**：
  - 声明式向导框架设计（声明式定义 + 集中渲染 + 就绪协议）
  - 数据模型（TourDefinition / TourStep / TourStepData / TourContext）
  - 跨页面引擎时序（resolveStep、目标元素等待、就绪信号）
  - el-tour 适配要点与踩坑记录（footer 全局隐藏、description 渲染）
  - 持久化（Settings.tour.completed）与页面接入规范
  - 新增向导的扩展流程

## 如何使用这些文档

### 对于新任务分析

1. **首先阅读**：`architecture-quick-reference.md` - 获取快速概览
2. **深入理解**：`business-logic.md` - 理解完整业务模型
3. **术语澄清**：`glossary.md` - 统一术语理解

### 对于具体问题诊断

1. **定位相关概念**：使用`glossary.md`确定涉及的术语
2. **理解业务逻辑**：参考`business-logic.md`相关章节
3. **查阅模块说明**：参考核心复杂模块目录下的 `README.md`

### 对于新功能开发

1. **检查架构约束**：`architecture-quick-reference.md`中的技术要点
2. **遵循代码规范**：`.claude/rules/` 中按领域拆分的编码规则（backend/frontend/database/plugin）

## 关键架构要点（快速记忆）

### 双架构统一检索

- **本地作者** ↔ **站点作者** = 跨站点作者统一检索
- **本地标签** ↔ **站点标签** = 跨站点标签统一检索
- **业务价值**：一次检索，全站结果

### Go 主进程

> **项目状态**：已完成从 Node.js 到 Go 的重构

主进程位于 `backend/` 目录，使用 Go 实现。

**核心规范**：
- Repository 模式：数据访问通过接口隔离
- 消除循环依赖：使用依赖倒置原则
- 包内聚合：handler、service、repository 在同一模块内（实体集中在 `backend/base/model/entity`）
- 详细规范见 [.claude/rules/backend.md](../../.claude/rules/backend.md)

### Vue Router 前端路由

- 使用 `createWebHashHistory()` 实现 hash 路由
- 路由配置在 `frontend/src/router/` 目录
- 路由定义在 `routes.ts`，实例在 `index.ts`
- `App.vue` 挂载 `<router-view>`，`MainLayout.vue` 为根路由布局

### 插件化架构

- 插件运行时位于 `plugin/package/` 目录（`{publicId}/{version}/`）
- 运行时插件为 Go 子进程，导出 `Activate(PluginContext)`；纯 UI 插件仅 `plugin.json` 声明 Slot
- 预置：本地导入 + pixiv 插件（`resources/bundled-plugins/` 首启自动安装）
- PluginContext 接口定义在 SDK 库 `github.com/lvfeng-z/library-squirrel-sdk`

### Wails IPC通信模式

- Go 主进程：使用 Wails 的 `Bind` 机制自动生成前端 API
- 前端调用：`window.api.serviceNameMethodName(args)` (由 Wails 绑定自动生成)
- 响应格式：Go 端直接返回数据或 error

### 数据库设计

- SQLite（WAL 模式），经 GORM 操作，文件位于 `{RootPath}/database/database.db`
- 泛型 `BaseRepository[T]` 提供通用 CRUD + 分页
- 表结构经 GORM 自动迁移（`backend/migration/migrate.go`）
- 事务经 `database.WithTransaction()` 支持，基于 context 注入可嵌套

### 类型与工具代码

- **后端共享模型**：`backend/base/model/`（entity/、dto/、query/、constant/）
- **前端类型**：`frontend/src/model/`（dto/interface/slot/tour/util/constant），与后端 DTO 对应的类型逐步迁移至 `frontend/bindings/`
- **前端工具函数**：`frontend/src/utils/`（ApiUtil、UrlUtil、CommonUtil 等）
- **路径别名**：`@renderer/*` → `frontend/src/*`、`@bindings/*` → `frontend/bindings/*`、`@apis/*` → `frontend/src/apis/*`

## 项目文件定位指南

| 任务类型     | 主要文件位置                      |
| ------------ | --------------------------------- |
| 业务逻辑     | `backend/{module}/service.go`    |
| 共用实体/DTO | `frontend/src/model/`             |
| 数据库操作   | `backend/{module}/repository.go`  |
| 插件开发     | `plugin/package/`                 |
| 前端路由     | `frontend/src/router/`            |
| 前端组件     | `frontend/src/components/`        |
| 前端视图     | `frontend/src/views/`              |
| 状态管理     | `frontend/src/store/`              |
| Wails 绑定   | `backend/{module}/handler.go`     |
| 共用工具函数 | `frontend/src/utils/`             |

## 常见开发任务参考

### 添加新 Handler

1. `backend/{module}/handler.go` 创建 handler（包含 Wails Bind 方法）
2. `backend/{module}/service.go` 实现业务逻辑
3. `backend/{module}/repository.go` 实现数据访问
4. 前端通过 `window.api.handlerMethod(args)` 调用

### 数据库事务

```go
// Go 端使用 gorm.Transaction 支持嵌套事务
err := database.WithTransaction(db, func(tx *gorm.DB) error {
    // 多个操作
    return nil
})
```

### 响应处理

Go Handler 返回 `*ApiResponse[T]{ success, msg, data }`（`model.Success` / `model.Error`）。前端 Wrapper 经 `requireResponse` 校验并转为 `data` 非空的 `ApiResult<T>`：

```typescript
const data = (await someApi.someMethod(args)).data  // requireResponse 已在 Wrapper 内完成 null/错误校验
// 失败时 requireResponse 抛 Error，调用方 try/catch + ElMessage.error 提示
```

## 更新和维护

当项目架构或业务逻辑发生变化时，应相应更新这些文档以保持同步。特别是：

- 添加新的核心业务概念时更新`glossary.md`
- 架构重大变更时更新`architecture-quick-reference.md`
- 发现新的常见错误时更新文档

## 相关项目文档

- [../../CLAUDE.md](../../CLAUDE.md) - 项目级开发指南
- [../../README.md](../../README.md) - 项目基本说明
- 代码中的注释和类型定义
