# AI Assistant Documentation for LibrarySquirrel

## 文档概述

本目录包含为AI助手（如Claude Code）准备的LibrarySquirrel项目文档，旨在帮助AI快速理解项目架构、业务逻辑和开发模式，从而更高效地进行代码分析、问题诊断和开发任务。

## 文档结构

### 1. [business-logic.md](./business-logic.md) - 完整业务逻辑文档

- **用途**：全面理解项目的业务模型、数据流和架构
- **适合场景**：需要深度理解项目整体架构时
- **包含内容**：
  - 项目概述和核心价值
  - 详细业务概念解释
  - 数据模型关系图
  - 完整业务流程
  - 架构组件详细说明
  - 典型业务用例

### 2. [architecture-quick-reference.md](./architecture-quick-reference.md) - 架构速查

- **用途**：快速查找核心概念和技术要点
- **适合场景**：开发任务中需要快速参考时
- **包含内容**：
  - 核心业务概念速查表
  - 关键业务逻辑总结
  - 技术架构要点
  - 开发模式和常见场景

### 3. [glossary.md](./glossary.md) - 术语表

- **用途**：统一理解项目中的领域特定术语
- **适合场景**：遇到不熟悉的术语时参考
- **包含内容**：
  - 核心实体术语定义
  - 作者/标签系统术语
  - 插件系统术语
  - 架构和开发约定术语


### 4. [plugin-development.md](./plugin-development.md) - 插件系统开发指南

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

### 5. [code-rules.md](./code-rules.md) - 代码规则与约定

- **用途**：查看项目的代码编写规范、命名约定和开发规范
- **适合场景**：编写新代码、重构或评审代码时参考
- **包含内容**：
  - 文件命名规范和目录结构约定
  - TypeScript、Vue组件和命名约定
  - IPC通信、数据库操作和插件开发规范
  - 代码质量工具和日期处理规则
  - 新增功能开发流程和常见注意事项

### 7. [module-migration-guide.md](./module-migration-guide.md) - 模块迁移修复指南

- **用途**：模块从 Electron 架构修复到 Wails 架构的统一模式参考
- **适合场景**：修复尚未完成迁移的业务模块时参考
- **包含内容**：
  - 后端修复模式（N+1 查询消除、依赖注入、nullable 参数）
  - 前端修复模式（QueryAttribute 适配、类型统一、错误处理）
  - 已完成模块记录与待修复模块清单
  - 修复验证要点

## 如何使用这些文档

### 对于新任务分析

1. **首先阅读**：`architecture-quick-reference.md` - 获取快速概览
2. **深入理解**：`business-logic.md` - 理解完整业务模型
3. **术语澄清**：`glossary.md` - 统一术语理解

### 对于具体问题诊断

1. **定位相关概念**：使用`glossary.md`确定涉及的术语
2. **理解业务逻辑**：参考`business-logic.md`相关章节
3. **检查常见陷阱**：参考`common-pitfalls.md`避免重复已知错误

### 对于新功能开发

1. **检查架构约束**：`architecture-quick-reference.md`中的技术要点
2. **遵循代码规范**：`code-rules.md`中的编码规则和约定

## 关键架构要点（快速记忆）

### 双架构统一检索

- **本地作者** ↔ **站点作者** = 跨站点作者统一检索
- **本地标签** ↔ **站点标签** = 跨站点标签统一检索
- **业务价值**：一次检索，全站结果

### Go 主进程

> **项目状态**：已完成从 Node.js 到 Go 的重构

主进程位于 `internal/` 目录，使用 Go 实现。

**核心规范**：
- Repository 模式：数据访问通过接口隔离
- 消除循环依赖：使用依赖倒置原则
- 包内聚合：model, repository, service 在同一模块内
- 详细规范见 [go-repository.md](./go-repository.md)

### Vue Router 前端路由

- 使用 `createWebHashHistory()` 实现 hash 路由
- 路由配置在 `src/renderer/src/router/` 目录
- 路由定义在 `routes.ts`，实例在 `index.ts`
- App.vue 简化为只包含 `<router-view>`

### 插件化架构

- 插件在`plugin/package/`目录
- 每个插件是独立包
- 预置：本地导入 + pixiv插件
- BasePlugin 接口简化为只包含 `pluginId: number`

### Wails IPC通信模式

- Go 主进程：使用 Wails 的 `Bind` 机制自动生成前端 API
- 前端调用：`window.api.serviceNameMethodName(args)` (由 Wails 绑定自动生成)
- 响应格式：Go 端直接返回数据或 error

### 数据库设计

- Database 类 + BaseDao 基类（继承自 CoreDao）
- SAVEPOINT 事务（支持嵌套）
- 表结构在 YAML 配置中定义

### 共享代码架构

- **src/shared/** 目录包含主进程和渲染进程共用的代码
- **src/shared/model/** - 实体类、DTO、枚举、常量（所有进程共用）
- **src/shared/util/** - 工具函数（StringUtil, TreeUtil, AssertUtil等）
- **路径别名**: `@shared/*` → `src/shared/*`

## 项目文件定位指南

| 任务类型     | 主要文件位置                      |
| ------------ | --------------------------------- |
| 业务逻辑     | `internal/{module}/service.go`   |
| 共用实体/DTO | `frontend/src/model/`             |
| 数据库操作   | `internal/{module}/repository.go` |
| 插件开发     | `plugin/package/`                 |
| 前端路由     | `frontend/src/router/`            |
| 前端组件     | `frontend/src/components/`        |
| 前端视图     | `frontend/src/views/`              |
| 状态管理     | `frontend/src/store/`              |
| Wails 绑定   | `internal/{module}/handler.go`    |
| 共用工具函数 | `frontend/src/utils/`             |

## 常见开发任务参考

### 添加新 Handler

1. `internal/{module}/handler.go` 创建 handler（包含 Wails Bind 方法）
2. `internal/{module}/service.go` 实现业务逻辑
3. `internal/{module}/repository.go` 实现数据访问
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

```typescript
const response = await window.api.someMethod(args)
if (ApiUtil.check(response)) {
  const data = ApiUtil.data(response)
  // 处理成功
} else {
  // 处理错误 - Go 端返回 BusinessError
}
```

## 更新和维护

当项目架构或业务逻辑发生变化时，应相应更新这些文档以保持同步。特别是：

- 添加新的核心业务概念时更新`glossary.md`
- 架构重大变更时更新`architecture-quick-reference.md`
- 代码规范变更时更新`code-rules.md`
- 发现新的常见错误时更新文档

## 相关项目文档

- [../CLAUDE.md](../CLAUDE.md) - 项目级开发指南
- [../README.md](../README.md) - 项目基本说明
- 代码中的注释和类型定义
