---
name: git-commit-writer
description: 根据项目提交规范生成规范的 Git 提交消息
version: 1.0.0
source: user-habit-analysis
---

# Git Commit Writer

根据用户提交习惯编写的规范化提交消息。

## 提交消息格式

### 基本格式
```
type: 简短描述
```
或
```
type(范围): 简短描述
```

### 多项变更格式
当有多个相关变更时，使用：
```
type: 主要修改概述

1. 第一项修改
   - 子项说明
2. 第二项修改
   - 子项说明
```

## Type 类型定义

| Type | 用途 | 使用场景 |
|------|------|----------|
| `fix` | Bug 修复 | 修复功能错误、接口问题、逻辑错误 |
| `feat` | 新功能 | 添加新接口、新组件、新模块 |
| `refactor` | 重构 | 优化代码结构、迁移代码、移除功能 |
| `perf` | 性能优化 | 提升执行效率、减少资源消耗 |
| `docs` | 文档更新 | 更新 README、注释、文档 |
| `build` | 构建相关 | 依赖更新、构建配置修改 |

## 范围标识

常用范围（可选）：
- `主进程` - Go 后端代码
- `渲染进程` - Vue 渲染进程代码
- `插件` - 插件系统相关

## 用户提交习惯

### 1. 多项变更组织方式
```
fix: 修复 XX 问题

1. 模块A: 修改内容
   - 详细说明
2. 模块B: 修改内容
   - 详细说明
```

### 2. 问题修复类提交
```
fix: 修复 XX 接口问题

- 详细描述问题
- 修复方案
- 与旧实现的对比
```

### 3. 迁移类提交
```
refactor(主进程): XX 迁移到 YY

- 移除旧的 XX 实现
- 新增 YY 实现
- 迁移范围
```

### 4. 接口修复类提交
```
fix: 修复 search/conditionPage 接口 SQL 语法错误和 types 参数解析问题

1. SQL 语法错误修复：
   - 移除每个 UNION 子查询的多余括号
   - 修复 count 查询格式
2. types 参数解析修复：
   - 从顶层 types 参数改为从 query JSON 对象中解析
```

## 生成规则

1. **第一行**是简短总结，使用中文，控制在 72 字符以内
2. **正文**使用中文，描述修改的"为什么"而非"是什么"
3. **多项变更**用数字编号，子项用 `-` 前缀
4. **保持一致性**：相同类型的提交使用相同的格式

## 输出格式

生成完成后，使用以下格式输出：
```
[提交消息草稿]

确认无误请回复"确认提交"或"confirm"
如需修改请指出具体位置
```

## 示例

### 示例 1：单一修复
```
fix: 修复 siteTag updateBind 接口字符串数组解析问题
```

### 示例 2：多模块修复
```
fix: 修复 Go 迁移接口不一致问题

1. reWorkWorkSet: 添加缺失的路由
   - updateSortOrders
   - unsetCover
   - getCoverWorkId
2. appLauncher: 修复请求体格式
3. search: workPage 改为 POST
```

### 示例 3：迁移类
```
refactor(前端): 迁移 window.api 调用到 HTTP wrappers

- 将 window.api.* 替换为 HTTP wrappers
- 新增 appLauncher.ts, siteBrowser.ts wrappers
- 更新 SiteBrowserManage.vue 等组件
```
