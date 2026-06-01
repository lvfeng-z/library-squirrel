# 任务执行链路数据库操作优化 — 进度记录

> 计划文件：`C:\Users\Adminstrator\.claude\plans\cheerful-yawning-pixel.md`

---

## 问题背景

大量任务并发执行时前端请求响应极慢（即使与任务无关的请求）。根因是 `SaveWorkInfo`（`backend/work/service.go`）单次调用产生约 27 次 DB 往返，含 N+1 写入和冗余重复查询。在 SQLite 单写者模型下，并发任务的写操作排队串行执行，阻塞其他 IPC handler 的 DB 请求。

优化目标：单次 `SaveWorkInfo` DB 往返从 ~27 降至 ~16，且全部包裹在单个事务中（一次写锁获取）。

---

## 核心设计模式

### 事务感知机制

Service 层禁止导入 `backend/database`（架构规则），因此通过接口隔离实现事务：

```
work 包定义 Transactor 接口
    ↓ 实现
app.go 的 dbTransactorAdapter（调用 database.WithTransactionContext）
    ↓ 事务启动时
将 *gorm.DB 存入 context（key = database.TxKey）
    ↓ Repository 层
所有 DB 操作通过 DBFromContext(ctx, defaultDB) 获取 DB 实例
```

**已创建的工具**（`backend/database/tx_context.go`）：
```go
var TxKey = txKeyType{}
func DBFromContext(ctx context.Context, defaultDB *gorm.DB) *gorm.DB {
    if tx, ok := ctx.Value(TxKey).(*gorm.DB); ok && tx != nil {
        return tx  // 事务内：使用事务 DB
    }
    return defaultDB  // 非事务：使用默认 DB
}
```

**Repository 改造模式**：
- `BaseRepository`：新增 `db(ctx)` 私有方法，所有 `r.db.WithContext(ctx)` → `r.db(ctx).WithContext(ctx)`
- 非 BaseRepository（如 `siteAuthor`）：新增 `dbFromCtx(ctx)` 方法，同样替换

---

## 事务路径涉及的完整 Repository 清单

`SaveWorkInfo` 事务内会调用以下 Repository 方法，**全部需要事务感知改造**：

| 模块 | Repository | 需改造的方法 | 当前状态 |
|------|-----------|-------------|---------|
| `database` | `BaseRepository[T]` | `Save`、`SaveBatch`、`Update`、`UpdateBatch`、`Delete`、`DeleteBatch`、`GetById`、`List`、`Count`、`Page`、`Get`、`ExecRawSQL`、`Updates`、`FindAll`、`FindOne`、`Transaction` | ✅ 已完成 |
| `siteAuthor` | `SiteAuthorRepository` | `Upsert`、`Save`、`SaveBatch`、`Update`、`GetById`、`List`、`Count`、`Delete`、`Get`、`ListByWorkId`、`ListBySiteAuthorIds`、`ListRankedSiteAuthorWithWorkIdByWorkIds`、`UpdateBindLocalAuthor`、`UpdateLastUseByIds` | ✅ 已完成 |
| `siteTag` | `SiteTagRepository`（嵌入 BaseRepository） | 自定义方法：`Upsert`、`GetBySiteAndSiteTagID`、其他使用 `r.GORM()` 的方法 | ✅ 已完成 |
| `workSet` | `WorkSetRepository`（嵌入 BaseRepository） | 自定义方法：`Upsert`、`GetBySiteAndSiteWorkSetID`、其他使用 `r.GORM()` 的方法 | ✅ 已完成 |
| `work` | `WorkRepository`（嵌入 BaseRepository） | 无自定义方法，BaseRepository 改造已覆盖 | ✅ 已覆盖 |
| `reWorkAuthor` | 关联表 Repository | `DeleteByWorkId`、`SaveBatch` 等 | ✅ 已完成 |
| `reWorkTag` | 关联表 Repository | `DeleteByWorkId`、`DeleteByWorkAndTag`、`SaveBatch` 等 | ✅ 已完成 |
| `reWorkWorkSet` | 关联表 Repository | `DeleteByWorkId`、`SaveBatch` 等 | ✅ 已完成 |
| `localAuthor` | `LocalAuthorRepository`（嵌入 BaseRepository） | 自定义方法中的 `r.GORM()` 调用 | ✅ 已完成 |
| `localTag` | `LocalTagRepository`（嵌入 BaseRepository） | 自定义方法：`GetByName`、`GetByNames` 等 `r.GORM()` 调用 | ✅ 已完成 |
| `resource` | `ResourceRepository`（嵌入 BaseRepository） | `Save` — 不在事务内但也被 BaseRepository 改造覆盖 | ✅ 已覆盖 |

**改造方法**：嵌入 `BaseRepository` 的 Repository 只需改造**自定义方法**中使用 `r.GORM()` 的地方（改为 `r.db(ctx).WithContext(ctx)` 或在自定义方法开头添加 `db := database.DBFromContext(ctx, r.GORM())` 后续用 `db` 替代 `r.GORM()`）。BaseRepository 继承的方法（`Save`、`SaveBatch`、`Update` 等）已自动支持事务。

---

## 已完成

### ✅ 前置：SQLite 连接配置优化
- `_busy_timeout=5000`、`MaxOpenConns=5`（`backend/database/db.go`）

### ✅ 步骤 1：删除 Phase 2 冗余查询
- `work/service.go` 中 `upsertSiteAuthors`/`upsertSiteTags`/`upsertWorkSets` 已返回 DB ID 列表
- 原 Phase 2 三行调用丢弃结果重新逐条查询，完全冗余
- 已删除 `SaveWorkInfo` 中 Phase 2 调用块和三个死代码方法 `querySiteAuthorDBIds`、`querySiteTagDBIds`、`queryWorkSetDBIds`
- **文件**：`backend/work/service.go`

### ✅ 步骤 2a：新建 context 事务工具
- **文件**：`backend/database/tx_context.go`（新建）

### ✅ 步骤 2b：BaseRepository 事务感知
- **文件**：`backend/database/base_repository.go`
- 新增 `db(ctx)` 私有方法，所有 17 个方法中的 `r.db.WithContext(ctx)` 替换为 `r.db(ctx).WithContext(ctx)`
- **已确认无遗留** `r.db.WithContext(ctx)` 调用

### 🔄 步骤 2c：siteAuthor.Repository 事务感知（进行中）
- **文件**：`backend/siteAuthor/repository.go`
- 已添加 `dbFromCtx(ctx)` 辅助方法
- 已替换全部 14 个方法中的 DB 调用
- **需确认无遗留**：最后检查 `r.GORM().WithContext(ctx)` 是否还有未替换的（理论上已全部改为 `r.dbFromCtx(ctx).WithContext(ctx)`）

### ✅ 步骤 2c（续）：其余 Repository 事务感知改造
- **siteTag**：添加 `dbFromCtx`，替换全部 `r.GORM().WithContext(ctx)` → `r.dbFromCtx(ctx).WithContext(ctx)`
- **workSet**：同上
- **reWorkAuthor**：添加 `dbFromCtx`，替换全部 `r.GORM().WithContext(ctx)`
- **reWorkTag**：添加 `dbFromCtx`，替换全部 `r.BaseRepository.GORM()` → `r.dbFromCtx(ctx)`
- **reWorkWorkSet**：同 reWorkTag
- **localAuthor**：添加 `dbFromCtx`，替换全部 `r.GORM().WithContext(ctx)`
- **localTag**：添加 `dbFromCtx`，替换全部 `r.GORM().WithContext(ctx)`（含跨行调用）
- **BaseRepository 命名冲突修复**：方法名 `db` 与字段 `db *gorm.DB` 冲突，重命名为 `getDb`

### ✅ 步骤 2d：Transactor 接口定义
- **文件**：`backend/work/service.go`（~line 162）
- 新增 `Transactor` 接口，定义 `ExecInTransaction(ctx, fn)` 方法

### ✅ 步骤 2e：dbTransactorAdapter 实现
- **文件**：`app.go`（~line 836）
- 新增 `dbTransactorAdapter`，内部调用 `database.WithTransactionContext` 并将 `*gorm.DB` 存入 context

### ✅ 步骤 2f：注入 Transactor
- **文件**：`backend/work/service.go`、`app.go`
- `Service` 结构体新增 `transactor Transactor` 字段
- `NewService` 新增第二个参数 `transactor Transactor`
- `app.go` 构造时传入 `&dbTransactorAdapter{db: app.db}`

### ✅ 步骤 2g：事务包裹 SaveWorkInfo
- **文件**：`backend/work/service.go`
- `SaveWorkInfo` 改为调用 `s.transactor.ExecInTransaction`，内部委托 `saveWorkInfoInTx`
- 原 `SaveWorkInfo` 方法体提取为 `saveWorkInfoInTx`

---

### ✅ 步骤 3：批量 Upsert 站点作者/标签/作品集

- **3a**：`SiteAuthorWriter`、`SiteTagWriter`、`WorkSetWriter` 接口各增加 `BatchUpsert` 和 `ListBySiteAnd*IDs` 方法
- **3b**：`siteAuthor/repository.go`、`siteTag/repository.go`、`workSet/repository.go` 各实现 `BatchUpsert`（OnConflict）和批量查询方法
- **3c**：`siteAuthor/service.go`、`siteTag/service.go` 各添加透传方法；`siteAuthor/service.go` 和 `siteTag/service.go` 的 Repository 接口同步更新
- **3d**：`app.go` 的 `workSetWriterAdapter` 实现新增的 `BatchUpsert` 和 `ListBySiteAndSiteWorkSetIDs`
- **3e**：`upsertSiteAuthors`/`upsertSiteTags`/`upsertWorkSets` 从逐条循环改为批量操作模式（DTO→实体→BatchUpsert→批量回查→map→按序输出）

---

### ✅ 步骤 4：批量创建本地作者/标签

- **4a**：`LocalTagFindOrCreator` 和 `LocalAuthorFindOrCreator` 接口各增加 `SaveBatch` 方法
- **Service 层透传**：`localAuthor/service.go`、`localTag/service.go` 各添加 `SaveBatch` 透传方法（委托 `repo.SaveBatch`）；Repository 接口同步更新
- **4b**：`resolveLocalAuthors` 和 `resolveLocalTags` 中的逐条 `Save` 循环改为收集新建实体到切片 → 一次 `SaveBatch` → 从实体中提取 ID 填入 `existingMap`

---

### ✅ 步骤 5：pending_resource_id 纳入批量刷盘

- **5a**：`Manager` 结构体新增 `pendingResourceIDUpdates map[int64]sql.NullInt64`，`NewManager` 中初始化
- **5b**：`onResourceIDUpdate` 回调从即时 `repo.UpdatePendingResourceID` 改为缓冲到 `pendingResourceIDUpdates`，非阻塞通知 flushLoop
- **5c**：`task/repository.go` 新增 `BatchUpdatePendingResourceID` 方法，使用 `CASE WHEN` SQL 模式（参考 `BatchSetStatus`）
- **5d**：`doFlush` 扩展为同时处理 `pendingStatusUpdates` 和 `pendingResourceIDUpdates`，分两批写入
- **Manager Repository 接口**：移除 `UpdatePendingResourceID`（单条），新增 `BatchUpdatePendingResourceID`（批量）
- **app.go**：`taskRepo` 字段类型从 `task.Repository`（接口）改为 `*task.TaskRepository`（具体类型），以同时满足 `task.Repository` + `taskManager.Repository`

---

## 关键代码位置索引

| 位置 | 说明 |
|------|------|
| `backend/work/service.go:652` | `SaveWorkInfo` — 优化核心方法 |
| `backend/work/service.go:740-805` | `upsertSiteAuthors`/`upsertSiteTags`/`upsertWorkSets` — 步骤 3 改造目标 |
| `backend/work/service.go:840` | `resolveLocalAuthors` — 步骤 4 改造目标 |
| `backend/work/service.go:958` | `resolveLocalTags` — 步骤 4 改造目标 |
| `backend/work/service.go:138-168` | Writer 接口定义 — 步骤 2d/3a/4a 需扩展 |
| `backend/work/service.go:200-258` | Service 结构体和 NewService — 步骤 2f 需修改 |
| `app.go:711-735` | `work.NewService` 构造调用 — 步骤 2f 注入 transactor |
| `app.go:829-851` | `workSetWriterAdapter` — 步骤 3d 需扩展 |
| `backend/taskManager/manager.go:728-732` | `onResourceIDUpdate` 回调 — 步骤 5b 改造目标 |
| `backend/taskManager/manager.go:755-776` | `doFlush` — 步骤 5d 扩展目标 |
| `backend/taskManager/manager.go:48-89` | Manager 结构体 — 步骤 5a 增加字段 |
| `backend/task/repository.go:177-179` | `UpdatePendingResourceID` — 现有单条方法 |
| `backend/task/repository.go:183-214` | `BatchSetStatus` — 步骤 5c 的参考模板 |

---

## 注意事项

1. **cgo 环境问题**：当前环境 `go build` 因 cgo 编译器问题失败（`cgo.exe: exit status 2`），非代码错误。建议新会话中优先确认编译环境可用，每步完成后编译验证。

2. **依赖顺序**：步骤 3 和 4 依赖步骤 2 完成事务能力建设。步骤 5 独立可并行。

3. **嵌入 BaseRepository 的 Repository 改造要点**：这些 Repository 的继承方法（`Save`、`SaveBatch`、`GetById` 等）已自动支持事务。只需改造**自定义方法**中使用 `r.GORM()` 的地方。改造模式：在自定义方法开头获取 `db := database.DBFromContext(ctx, r.GORM())`，后续用 `db.WithContext(ctx)` 替代 `r.GORM().WithContext(ctx)`。

4. **workSetWriterAdapter**：这是 `app.go` 中为打破 `work ↔ workSet` 循环依赖而创建的适配器，直接持有 `workSet.WorkSetRepository`。新增的 `BatchUpsert` 和 `ListBySiteAndSiteWorkSetIDs` 方法可直接委托给 `repo` 的对应方法。
