---
description: "数据库架构与规则，适用于修改数据库模型、Repository 层、迁移等代码时加载"
globs:
  - "backend/base/repository/**"
  - "backend/migration/**"
  - "backend/base/model/entity/**"
  - "database/**"
---

# 数据库架构与规则

## 数据库
- SQLite via GORM，数据库文件位于 `database/database.db`
- `BaseRepository[T]` 提供泛型 CRUD + 分页，通过 `QueryOption`/`PageOption`
- **事务**：`database.WithTransaction(db, func(tx *gorm.DB) error { ... })`
- 实体通过 GORM 自动迁移，入口在 `backend/migration/migrate.go`
- 所有实体嵌入 `BaseEntity`（ID、CreateTime、UpdateTime）
- **时间字段**：统一使用 Unix 时间戳（毫秒），`INTEGER` 类型存储

## 路径存储规范

- 数据库中存储的相对路径必须相对于资源库根目录
- 禁止包含 `../`、`./` 或绝对路径
- 例外：用户自定义资源目录（workdir）、临时文件目录、外部关联文件路径可使用绝对路径，字段命名中需明确标识性质

## 数据库相关编码规则

- **BASE_REPOSITORY_REUSE** (P0): 复用 `BaseRepository` 方法。仅当 `BaseRepository` 无法表达时才编写自定义 repository 逻辑。
- **ELIMINATE_N_PLUS_1_QUERY** (P0): 收集 ID → 批量查询 → 构建 map → 组装 DTO。禁止在循环中查询。
- **BATCH_UPDATE_OPTIMIZATION** (P1): 批量更新使用单条 SQL，禁止循环逐条 UPDATE。
- **ENTITY_USE_NEW_FACTORY** (P1): 使用 `entity.NewXxx()` 工厂方法，禁止 `&entity.Xxx{}`。
- **NULLABLE_PARAM_USE_POINTER** (P1): 可空参数使用 `*int64`/`*string`（null = 清除关联）。
- Service 层禁止直接导入 `backend/database`，仅 Repository 层可导入。
