# CLAUDE.md

本文件为 Claude Code (claude.ai/code) 在此仓库中工作提供指导。

## 项目概述

**LibrarySquirrel** 是一个基于 Wails 3 的桌面应用，用于创建和维护基于标签的个人资源库。它从远程站点（如 pixiv）下载资源到本地库，并提供基于标签的搜索。后端：Go，前端：Vue 3 + TypeScript，通信：Wails IPC bindings。

## 项目背景
本项目由[LibrarySquirrel](https://gitee.com/lv__feng/library-squirrel.git)重构而来，本项目依赖[SDK](https://github.com/lvfeng-z/library-squirrel-plugin-sdk)，此外存在两个插件[localImport](https://github.com/lvfeng-z/library-squirrel-plugin-local.git)和[pixivSuite](https://github.com/lvfeng-z/library-squirrel-plugin-pixiv-go.git)，以上仓库的开发环境都位于本项目的同级目录下。

## 构建与开发命令

```bash
# 开发（热重载）
task dev                           # 或: wails3 dev -config ./build/config.yml

# 构建
task build                         # 或: wails3 build

# 运行编译产物
task run

# 仅前端
cd frontend && yarn build          # 生产构建
cd frontend && yarn build:dev      # 开发构建（不压缩）

# 修改 Go handler 后生成 TypeScript bindings
wails3 generate bindings -ts

# Go 测试
go test ./...
go test ./backend/util/...         # 单个包

# 服务端模式（无 GUI）
task build:server && task run:server
```

## 架构

### 技术栈
- **框架**: Wails v3（Go 后端 + Web 前端原生窗口）
- **后端**: Go 1.26、GORM（SQLite WAL 模式）、Viper（配置）、Zap（日志）
- **前端**: Vue 3 + Composition API (`<script setup lang="ts">`)、Element Plus、Pinia、Vue Router（hash 模式）、Vite 8
- **IPC**: Wails Bind — Go handler 方法自动暴露给前端，TypeScript bindings 自动生成

## 编码规则（全局）

### 通用

- **注释**: 使用中文注释，出现专有名词时使用对应语言，仅描述目的和约束，禁止使用变更描述类词语（"改为"、"重构"、"优化"）。
- **日志**: 输出中文日志，出现专有名词时使用对应语言。

### 领域架构与规则
各板块的架构说明和编码规则已拆分到独立文件，按 globs 自动加载：
- **后端**: `.claude/rules/backend.md` — 模块模式、业务概念、Go 编码规则
- **前端**: `.claude/rules/frontend.md` — 目录结构、TypeScript 编码规则
- **数据库**: `.claude/rules/database.md` — SQLite/GORM 架构、Repository 规则
- **插件**: `.claude/rules/plugin.md` — 插件系统架构

### Git 提交
- 中文，格式：`类型(范围): 描述`
- 类型：`feat`、`fix`、`docs`、`style`、`refactor`、`test`、`chore`、`build`

## 添加新功能

1. 创建 `backend/{module}/handler.go`、`service.go`、`repository.go`
2. 在 `app.go` 的 App 结构体中注册 handler 并初始化 service
3. 运行 `wails3 generate bindings -ts` 生成前端 bindings
4. 在 `frontend/src/apis/http/wrappers/` 中创建 wrapper
5. 按需创建页面/对话框组件

## 配置

- `config.yaml` — 应用配置（服务器、数据库、站点、插件）
- `config/settings.json` — 运行时用户设置
- `build/config.yml` — Wails 开发模式配置

## 交互规则
- 当我意图制定计划或方案时，直接用 Write 工具将计划文件写到 `doc/plan/` 目录下，然后停下来询问我要执行计划还是要检查计划，不要直接开始执行。
- 当本次对项目的修改达到某个里程碑或完成时，请判断当前的修改是否导致项目的实际逻辑与你所理解的项目上下文（比如rule中的规则）出现了差异，如果有差异，则向我询问是否需要对文档进行同步。