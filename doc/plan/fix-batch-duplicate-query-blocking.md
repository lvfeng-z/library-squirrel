# 修复：批量查重巨型 OR 查询导致永久阻塞

## 问题现象

开始任务 1（含 1000 个子任务）时无响应。调试发现：
- `backend/work/repository.go:78` `ListBySiteAndSiteWorkIDs` 调用 `List`
- `backend/database/base_repository.go:211` `db.Find(&entities).Error` 永久阻塞
- `Conditions[0].Exprs` 长度为 1000

## 根因

`ListBySiteAndSiteWorkIDs` 用 `clause.OR` 把 1000 组 `(site_id=? AND site_work_id=?)` 拼成**一条 SQL**，产生 1000 个 OR 子句、2000 个绑定参数。SQLite 查询规划器对超大量 OR 子句的优化组合爆炸，`sqlite3_step` 长时间不返回。

叠加 `db.go:45` 的 `SetMaxOpenConns(1)`：这条卡住的 `Find` 独占唯一连接，后续所有 DB 操作在 Go 连接池排队（无超时），整个应用 DB 层瘫痪。

日志佐证：taskId=1002（2 个 OR）`505µs` 完成；taskId=1（1000 个 OR）从无对应 GORM 日志（gorm logger 仅在执行完成后记录），用户反复重试 10+ 次均卡死。

## 修复方案

聚焦最小改动，两层修复：

### 1. 拆分巨型 OR 查询（根因）

**文件**：`backend/work/repository.go` — `ListBySiteAndSiteWorkIDs`

把一次性构建 N 组 OR 条件，改为按固定批大小分批查询、合并结果。

- 提取批大小常量 `batchSizeOfSiteWorkQuery = 200`
- 每批构建至多 200 组 `(site_id=? AND site_work_id=?)` OR 条件（即每批 ≤ 400 个绑定参数，远低于 SQLite `SQLITE_MAX_VARIABLE_NUMBER` 上限，OR 子句数也处于 SQLite 规划器高效区间）
- 多批结果 `append` 合并
- 任一批次出错立即返回错误（上层 `batchCheckDuplicates` 已有 `err != nil` 降级到 `run()` 逐个查重的兜底）

1000 个子任务 → 5 批串行查询，每批毫秒级，总量可接受，且每批都不再触发规划器爆炸。

### 2. 查重调用加 context 超时（防御）

**文件**：`backend/taskManager/manager.go` — `batchCheckDuplicates`

对 `ListBySiteAndSiteWorkIDs` 调用包一层超时 context（`30s`），即使将来出现新的卡死场景，超时返回 error 后能触发已有的降级逻辑（`manager.go:323`），避免永久占用唯一连接拖垮全局。

- `time` 包已在 manager.go 导入，无需新增 import
- `defer cancel()` 确保 context 释放

## 不改动

- `WorkChecker` 接口签名不变（`ListBySiteAndSiteWorkIDs(ctx, siteIds, siteWorkIds)`）
- `db.go` 的 `MaxOpenConns(1)` 保持不变（这是 SQLite 单写者的正确配置，问题不在连接池本身）
- 数据库索引 `idx_work_site_site_work` 已存在，无需调整

## 影响面

- 仅两个文件、两个函数，无接口变更、无 bindings 重生成、无前端改动
- 行为对外完全一致：同样返回已存在的 Work 列表，只是查询方式从"一条巨型 SQL"变为"分批小 SQL 合并"

## 验证

1. `go build ./...` 编译通过
2. 重新启动任务 1（1000 个子任务），确认 `loadAndStartTaskTree: 查询到 1001 条任务记录` 后能正常进入派发流程，不再卡死
3. 日志中应出现 5 条 `[GORM] ... work WHERE (... OR ...)` 的分批查询记录，每条耗时毫秒级
