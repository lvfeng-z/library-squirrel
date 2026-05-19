---
description: "Go 后端架构与编码规则，适用于修改 backend/ 目录下的代码时加载"
globs:
  - "backend/**"
---

# 后端架构与规则

## 模块模式（Repository-Service-Handler）

每个业务模块位于 `backend/{module}/`，遵循以下结构：
```
handler.go          — Wails Bind 方法（通过 IPC 暴露给前端）
service.go          — 业务逻辑
repository.go       — 数据访问接口 + 实现
query.go            — 查询 DTO
```

共享模型位于 `backend/base/model/entity/`（18 个实体）和 `backend/base/model/dto/`。

## 文件与命名规范

| 元素        | 规则                       | 示例                              |
| ----------- | -------------------------- | --------------------------------- |
| Go 源文件   | snake_case + `.go`         | `handler.go`、`work_service.go`   |
| 目录        | 单元命名，全小写           | `backend/author/`                 |
| 包名        | 与目录同名，简洁           | `package author`                  |
| 结构体/接口 | PascalCase                 | `WorkService`、`Repository`       |
| 变量/函数   | camelCase                  | `getWorkById`                     |
| 常量        | UPPER_SNAKE_CASE           | `MAX_RETRY_COUNT`                 |
| 错误变量    | `Err` 前缀                 | `ErrNameEmpty`                    |
| 接口命名    | `er` 后缀或名词            | `Repository`、`Provider`          |

## 代码组织

文件内声明顺序：
1. 包声明
2. 导入（标准库在前，按长度排序）
3. 错误定义（`var ErrXxx = errors.New(...)`）
4. 领域实体
5. 接口定义
6. Service/Repository 实现

函数/方法顺序：构造函数（`NewXxx`）→ 接口实现方法 → 业务方法 → 私有辅助方法。

## 核心业务概念

- **Site（站点）**: 远程来源（pixiv 等）
- **Work（作品）**: 核心实体 — 资源集合 + 元数据
- **本地标签/作者 ↔ 站点标签/作者**: 通过关联实现跨站点统一搜索
- **Task（任务）**: 作品创建流程（URL → 插件 → 保存）

## Go 后端编码规则

- **DTO_COMPOSITION_OVER_EMBEDDING** (P0): DTO 禁止嵌入实体，使用其他 DTO 的命名字段。DTO 之间禁止匿名嵌入（Wails 会展平 JSON tag）。
- **ELIMINATE_N_PLUS_1_QUERY** (P0): 收集 ID → 批量查询 → 构建 map → 组装 DTO。禁止在循环中查询。
- **SERVICE_DEPENDENCY_VIA_INTERFACE** (P0): Service 依赖由**调用方定义**的接口，由**提供方实现**。禁止持有具体 `*OtherService`。通过构造函数注入。
- **BASE_REPOSITORY_REUSE** (P0): 复用 `BaseRepository` 方法。仅当 `BaseRepository` 无法表达时才编写自定义 repository 逻辑。
- **ENTITY_USE_NEW_FACTORY** (P1): 使用 `entity.NewXxx()` 工厂方法，禁止 `&entity.Xxx{}`。
- **DTO_USE_TO_ENTITY** (P1): 使用 `ToXxxEntity()` 转换函数，禁止手动逐字段映射。
- **BATCH_UPDATE_OPTIMIZATION** (P1): 批量更新使用单条 SQL，禁止循环逐条 UPDATE。
- **NULLABLE_PARAM_USE_POINTER** (P1): 可空参数使用 `*int64`/`*string`（null = 清除关联）。
- **REMOVE_REDUNDANT_QUERY_FIELDS** (P2): QueryDTO 中禁止为同一列定义多个语义重复的字段（如精确+模糊），保留一个字段通过 `QueryAttribute.operator` 控制匹配方式。
- **DEAD_CODE_CLEANUP** (P2): 重构后确认无调用方的旧方法直接删除，禁止保留"以防万一"的代码。
- **错误处理**：`var ErrXxx = errors.New(...)`，使用 `errors.Is()` 判断。
- **所有公开方法以** `context.Context` 作为第一个参数，禁止在 `context.WithValue` 中存储业务数据。
- **Service 层禁止直接导入** `backend/database`，仅 Repository 层可导入。

## 禁止的做法

| 禁止                                   | 正确做法               |
| -------------------------------------- | ---------------------- |
| 在 Service 层 import `backend/database` | 在 Repository 层 import |
| 返回 `*gorm.DB` 或 `sql.Rows`          | 返回领域实体或 DTO     |
| 跨模块直接引用其他 Service             | 使用接口隔离           |
| 在 `util` 包中包含有状态逻辑           | 使用纯函数             |
