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
- **写方法语义（命名与 GORM finisher 对齐）**：
  - `Create` / `CreateBatch` → GORM `Create`（INSERT，新建）
  - `Save` → GORM `Save`（UPSERT 全字段含零值，完整替换已存在记录或清空字段时用）
  - `Updates` → GORM `Updates`（部分更新，仅写非零字段，编辑表单/更新状态时用）
  - ⚠️ `Updates` 跳过零值字段——若字段的 Go 零值（int 0/bool false/string ""）是合法业务取值，须用 `sql.Null*` 类型（靠 `Valid` 区分"未设置"与"零值"），否则零值无法落盘
- **事务**：`database.WithTransaction(db, func(tx *gorm.DB) error { ... })`
  - **连接池 MaxOpenConns=1**（`backend/database/db.go`）：SQLite 单写者，整个应用所有 DB 操作共享 1 个连接，Go `sql.DB` 连接池排队。
  - **事务内 repository 方法必须用 `dbFromCtx(ctx)`**（= `database.DBFromContext(ctx, r.GORM())`，从 ctx 取事务 tx），禁止 `r.GORM()`——后者会向连接池再取连接，而唯一连接正被事务占用 → Go 连接池永久等待 → **死锁**（`busy_timeout` 无效，卡在 Go 连接池层而非 SQLite 层）。自定义 repository 方法默认走 `dbFromCtx(ctx)` 模式。
- 实体通过 GORM 自动迁移，入口在 `backend/migration/migrate.go`
- 所有实体嵌入 `BaseEntity`（ID、CreateTime、UpdateTime）
- **时间字段**：统一使用 Unix 时间戳（毫秒），`INTEGER` 类型存储

## 路径存储规范

- **路径分隔符纪律见 `rules/backend.md` 的 PATH_SEPARATOR_DISCIPLINE（两域模型）**：本节以下所有相对路径（relPath 域）一律正斜杠——构造用 `path.Join`，禁止 `filepath.Join`/`Clean` 的结果进入存储/比较/传递；absPath 域仅存在于 os.* 调用点。
- **所有相对路径必须基于适当的根目录**，根据业务场景选择根目录：
  - **workDir（资源库根目录）**：用户配置的资源存储目录，用于资源文件、缩略图、备份等用户数据路径
  - **程序根目录**：应用运行目录，用于插件资源、配置文件等应用自身数据路径
- 相对路径禁止包含 `../`、`./` 或绝对路径
- 禁止相对于根目录的子目录存储（如相对于 `workDir/store/` 而非 `workDir`），避免路径层级混乱
- 例外：用户自定义资源目录（workdir 字段）、临时文件目录、外部关联文件路径可使用绝对路径，字段命名中需明确标识性质

### 各表路径字段基准

| 表 | 字段 | 根目录 | 说明 | 存储示例 |
|---|---|---|---|---|
| `persistent_store` | `file_path` | workDir | 资源文件存储 | `store/resource/作者/文件.mp4`、`store/resource/作者/文件_thumbnail_000.jpg` |
| `backup` | `file_path` | workDir | 备份文件路径 | `backup/2026/06/08/文件.mp4` |

> 命名规约:所有插件 store(含 thumbnail)统一进 `store/resource/`,文件名按 bas 基准 + 资源级多 store 判定(`<bas>_<role>_<seq>[_<描述>].<ext>`),详见 `doc/store-naming-convention.md`。历史独立目录 `store/thumbnail/` 白名单条目已退役(零写入方、库内零存量行)。

> 注：`persistent_store` 另有 `width`/`height` 字段（`sql.NullInt64`，图像像素宽高，非图片资源 Valid=false），由落盘时 `image.DecodeConfig` 提取，供前端瀑布流预计算卡片高度；属图像元数据，非路径字段。`completed_at`（落盘完成时刻毫秒时间戳，0=未完成）是合法零值——GORM Updates 跳零值，「续传重置回未完成」须经 `ResetCompleted` 显式列更新（service 层已封装）。

> 注：`site_tag.namespace` / `re_work_tag.namespace`（`sql.NullString`，tag 关联级 namespace 维度，非路径字段）：站点有 namespace 时存值（如 e-hentai 的 character），无 namespace 站点（pixiv）落 NULL。落库守卫 `Valid: namespace != ""`——插件不声明（空串）→ `Valid:false` = NULL，对无 namespace 站点插件无感。`re_work_tag.namespace` 为所指 `site_tag.namespace` 镜像（site 关联）或用户自设（local 关联）。**`site_tag.namespace` 仅作站点元数据/镜像源，不直接参与关联/搜索维度——namespace 维度统一在 `re_work_tag.namespace`（搜索过滤 `rwt.namespace`，不读 site_tag.namespace）**。设计见 `doc/plan/tag体系演化方案.md`。

### 路径解析约定

- **绝对路径解析**：`filepath.Join(rootDir, relativePath)`，禁止额外拼接中间目录（如 `"store"`）
- **路径校验**：`persistent_store.file_path` 必须以 `storeRegistry` 中已注册的子目录开头（如 `store/resource`、`store/avatar/local`、`store/avatar/site`）
- **URL 映射**：前端 `buildStoreUrl(filePath)` 将 workDir 相对路径编码为 `/store/{encoded}` URL，后端 `StoreFileHandler` 剥离 `/store/` 前缀后直接 `filepath.Join(workDir, path)` 解析

## 软删除（GORM soft_delete）

- **当前 work 与 persistent_store 两表启用**（persistent_store 见其模块 README 的记录-文件不变量）：`DeletedAt soft_delete.DeletedAt` + tag `softDelete:milli`（毫秒时间戳，0=活）。Find/Count/Get/Update 自动排除已删行、Delete 自动改写为打时间戳（仅作用活行）——**经 GORM 管线的查询全受保护**（含各模块自定义仓储的链式调用）；**原生 SQL（Raw/Exec）不受保护**，须手工补 `deleted_at = 0` 基线（参照 search 模块 `buildWhereClauseWithBaseline`）。
- **含已删/仅已删查询**：`QueryOption.IncludeDeleted=true`（Unscoped）；「仅已删」由调用方自组 `deleted_at > 0` 条件；按 ID 查含已删行用 `GetByIdUnscoped`。
- **对已删行的写操作必须 Unscoped 变体**（`DeleteUnscoped` 或专用方法）：普通 Update/Delete 被软删 scope 静默挡住（无效果且不报错）。对**活行**的既有物理删调用点同样须 `DeleteUnscoped`——否则被静默改写为软删（语义反转事故）。
- **业务键唯一性**：启用软删的表用部分唯一索引 `CREATE UNIQUE INDEX ... WHERE deleted_at = 0`（AutoMigrate 不管部分索引，drop 旧 + 建新全手写，以新索引名做幂等标记）；**加列迁移必带存量回填** `UPDATE ... SET deleted_at = 0 WHERE deleted_at IS NULL`——AutoMigrate 加列无默认值，存量行 NULL × `deleted_at = 0` 过滤不命中，全部存量行从查询中消失（实机踩中）。
- **fsmonitor 联动**：store 行软删后经 GORM 自动 scope 排除在对账/关联查询外（曾用消费侧 JOIN work 排除条件 `notDeletedWorkCond`，属 persistentStore 越界感知业务实体，已随 persistent_store 软删落地删除）。

## 外键强制执行（schema 级引用完整性）

- **执行开关**：DSN（`backend/database/db.go`）带 `_foreign_keys=on`——SQLite 外键强制按连接生效、默认关闭，MaxOpenConns=1 单连接一处全覆盖；不能在事务内切换，DSN 级设置天然规避。
- **形态仅 NO ACTION**：外键只做「被引用行删除/改键时若有子引用即拒绝」的报错式防线。**禁用 CASCADE/SET NULL**——级联清理是业务编排，归发起方模块（删除链手工显式子→父顺序，如 `DeleteWorkAndSurroundingData`）；外键不覆盖软删行态、「关联行缺失」类孤儿、跨表行态不变量与文件面一致性。
- **声明登记面**：`backend/migration/foreign_keys.go` 的 `fkBatches`（全库 25 对）。SQLite 无 `ALTER TABLE ADD CONSTRAINT`，存量表挂 FK 经**表重建舞步**（以 `sqlite_master` 现表 DDL 为源文本注入 FK 子句→建新表→拷数据→删旧表→改名→复原索引；**禁用实体重建 DDL**——实体字段序与表列序漂移会静默错位）。幂等标记：`pragma_foreign_key_list` 的（引用列, 引用表）对（SQLite FK 无独立命名）。
- **迁移时序与存量**：存量悬空引用先清（`cleanDanglingAssociations`——关联行 DELETE、业务行引用列置 NULL）；0 哨兵引用列一律 NULL 化（FK 对 NULL 豁免、对 0 不豁免）。悬空引用的存量遗留形态在 FK 强制下不可经正常写入产生，测试须关 PRAGMA 种植（对照 `plantDanglingPluginRef` 先例）。**自引用外键（task.pid→task.id、local_tag.base_local_tag_id）的悬空清理子查询必须以别名引用父行**（`NOT EXISTS (SELECT 1 FROM task parent_tk WHERE parent_tk.id = task.pid)`）——内层 `FROM task` 同名会遮蔽外层表引用，条件退化为内层行自身比较恒空、NOT EXISTS 恒真，每次启动把全表引用列清成 NULL（存量事故：子任务 pid 全灭致任务页树状塌平，回归测试 `clean_dangling_self_ref_test.go` 锚定）。
- **AutoMigrate 交互**：GORM AutoMigrate 不感知 FK；对 FK 表的列型变更走其 SQLite 重建可能掉 FK 子句——**FK 表结构变更此后只走命名迁移**，在其中保留 FK 定义。
- **测试协同**：测试库一律 `migration.OpenTestDB()`（`:memory:?_foreign_keys=on` + 完整迁移），杜绝裸 `gorm.Open`——裸开测试库无 FK 强制、断言失效且与生产行为分叉。

## 数据库相关编码规则

- **BASE_REPOSITORY_REUSE** (P0): 复用 `BaseRepository` 方法。仅当 `BaseRepository` 无法表达时才编写自定义 repository 逻辑。
- **ELIMINATE_N_PLUS_1_QUERY** (P0): 收集 ID → 批量查询 → 构建 map → 组装 DTO。禁止在循环中查询。
- **BATCH_UPDATE_OPTIMIZATION** (P1): 批量更新使用单条 SQL，禁止循环逐条 UPDATE。
- **ENTITY_USE_NEW_FACTORY** (P1): 使用 `entity.NewXxx()` 工厂方法，禁止 `&entity.Xxx{}`。
- **NULLABLE_PARAM_USE_POINTER** (P1): 可空参数使用 `*int64`/`*string`（null = 清除关联）。
- **RESOURCE_TYPE_STRICT** (P0): `resource.resource_type` NOT NULL；写入路径严格识别（空/未注册 ResourceType 与 store_type 抛错，`entity.ValidateResourceType`/`ValidateStoreType`——Registry 来源为内置 6 类 + 插件自定义，未注册即抛错），不迁移历史数据（用户上线前手动处理）。规约见 `doc/resource-type-spec.md`。
- **TRANSACTION_INTERNAL_USE_DBFROMCTX** (P0): 事务（`ExecInTransaction`/`WithTransaction`）内调用的 repository 方法必须用 `dbFromCtx(ctx)` 取事务连接，禁止 `r.GORM()`——MaxOpenConns=1 下后者会死锁（详见上「事务」）。自定义 repository 方法默认走 `dbFromCtx(ctx)` 模式；排查 `grep "GORM()\." backend/*/repository.go` 逐一确认是否被事务调用链触发。
- Service 层禁止直接导入 `backend/database`，仅 Repository 层可导入。
