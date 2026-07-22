---
description: "数据库架构与规则，适用于修改数据库模型、Repository 层、迁移等代码时加载"
globs:
  - "backend/base/repository/**"
  - "backend/migration/**"
  - "backend/base/model/entity/**"
  - "backend/persistentStore/**"
  - "backend/taskManager/**"
  - "backend/assetserver/**"
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

- **所有相对路径必须基于适当的根目录**，根据业务场景选择根目录：
  - **workDir（资源库根目录）**：用户配置的资源存储目录，用于资源文件、缩略图、备份等用户数据路径
  - **程序根目录**：应用运行目录，用于插件资源、配置文件等应用自身数据路径
- 相对路径禁止包含 `../`、`./` 或绝对路径
- 禁止相对于根目录的子目录存储（如相对于 `workDir/store/` 而非 `workDir`），避免路径层级混乱
- 例外：用户自定义资源目录（workdir 字段）、临时文件目录、外部关联文件路径可使用绝对路径，字段命名中需明确标识性质

### 各表路径字段基准

| 表 | 字段 | 根目录 | 说明 | 存储示例 |
|---|---|---|---|---|
| `persistent_store` | `file_path` | workDir | 资源文件存储 | `store/resource/作者/文件.mp4`、`store/thumbnail/作者/文件_thumbnail.jpg` |
| `backup` | `file_path` | workDir | 备份文件路径 | `backup/2026/06/08/文件.mp4` |
| `backup` | `original_file_path` | workDir | 同 `persistent_store.file_path`，用于还原时确定目标位置 | `store/resource/作者/文件.mp4` |

> 注：`persistent_store` 另有 `width`/`height` 字段（`int`，图像像素宽高，非图片资源为 0），由落盘时 `image.DecodeConfig` 提取，供前端瀑布流预计算卡片高度；属图像元数据，非路径字段。

### 路径解析约定

- **绝对路径解析**：`filepath.Join(rootDir, relativePath)`，禁止额外拼接中间目录（如 `"store"`）
- **路径校验**：`persistent_store.file_path` 必须以 `dir.go` 中已注册的子目录开头（如 `store/resource`、`store/thumbnail`、`store/avatar/local`、`store/avatar/site`）
- **URL 映射**：前端 `buildStoreUrl(filePath)` 将 workDir 相对路径编码为 `/store/{encoded}` URL，后端 `StoreFileHandler` 剥离 `/store/` 前缀后直接 `filepath.Join(workDir, path)` 解析

## 数据库相关编码规则

- **BASE_REPOSITORY_REUSE** (P0): 复用 `BaseRepository` 方法。仅当 `BaseRepository` 无法表达时才编写自定义 repository 逻辑。
- **ELIMINATE_N_PLUS_1_QUERY** (P0): 收集 ID → 批量查询 → 构建 map → 组装 DTO。禁止在循环中查询。
- **BATCH_UPDATE_OPTIMIZATION** (P1): 批量更新使用单条 SQL，禁止循环逐条 UPDATE。
- **ENTITY_USE_NEW_FACTORY** (P1): 使用 `entity.NewXxx()` 工厂方法，禁止 `&entity.Xxx{}`。
- **NULLABLE_PARAM_USE_POINTER** (P1): 可空参数使用 `*int64`/`*string`（null = 清除关联）。
- **RESOURCE_TYPE_STRICT** (P0): `resource.resource_type` NOT NULL；写入路径严格识别（空/未知 ResourceType 与 store_type 抛错，`entity.ValidateResourceType`/`ValidateStoreType`），不迁移历史数据（用户上线前手动处理）。规约见 `doc/resource-type-spec.md`。
- Service 层禁止直接导入 `backend/database`，仅 Repository 层可导入。
