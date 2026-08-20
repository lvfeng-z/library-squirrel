# backup 能力包纯化改造（方案二：纯文件仓库 + 发起方内嵌引用）

## 审查摘要

**关键声明（锚点）**：
- 错配爆发点①复原链：`backend/recycleBin/service.go:179-201` —— `ListByOriginalPaths(paths)` 按路径 IN 命中**全部代次**备份（同键作品各代 `file_path` 相同），循环 `RestoreFile` + `Delete(backup.GetID())` 把他代备份一并移回并删记录
- 错配爆发点②彻底删除链：`backend/recycleBin/service.go:218-232` —— 同款按路径反查，`purgeBackup` 物理删除他代备份
- 错配爆发点③/store/ 兜底：`backend/backup/repository.go:73-86`（`GetLatestByOriginalPath`，ORDER BY id DESC 启发式）——同路径多代时取「id 最大」，与请求代次无关联
- 同路径多代实证（2026-08-20 查 database.db）：`source_type=3` 存在大量同 `original_file_path` 多行（如 id 131/135/137/139 四代）；`source_type=3 AND source_id IS NULL` 为 0 行（无历史断层）；同 `source_id` 多行为 0 行
- **plugin 域已是内嵌形态**：`plugin.BackupID`（`backend/plugin/service.go:534` 写入、`:567` 读守卫）行内持有 backup 引用；但 `Reinstall` 取备份仍走 `GetPluginBackup(plugin.ID)` source 反查（`:572`）——**双通路并存即本次裁决要消灭的混乱形态**
- 资源类来源（`SourceTypeResource`=2）全链死代码：`MoveBackupForResource`（service.go:237）、`GetResourceBackup`（:213）、`GetResourceBackups`（:320）均无调用方（grep 证实）
- `GetBySourceTypeAndSourceIds`（repository.go:41-54）注释「每 sourceId 取最新」与实现（无去重）名实不符——随 source 通路整体退役，不再修正
- `/store/` 路由现状：`ResolveFileState`（`backend/persistentStore/service.go:904-916`）返回 `(completed, deleted) bool`；已删分支经 `BackupPathResolver.ResolveBackupPathByOriginal` 反查

**已裁决（2026-08-20，原待决策两项均已定）**：
1. **引用键全链统一为 backup 清单行 ID**（persistent_store 加 `backup_id` 列、plugin 沿用既有 `BackupID`）——用户否决「store 存路径省一次查表」的混合形态：「业务间模块间界线的清晰度远大于省一次查询的收益」。/store/ 路由已删分支按 ID 查保管清单取路径（多一次查表，代价已接受）
2. **存量无主备份（source 反查搬不动的，即业务行已物理删的清单行）迁移时直接清理**——测试库

**自曝风险**：
1. `backup_id` 写入与 `deleted_at` 打标之间无事务原子性（文件 IO 在事务外，与现链同构）——中断窗口可产生「行已删但列空」的不可复原行（文件在 backup/ 有清单行，治理反向对账可发现并补救：按清单行回填列）
2. 治理对账的引用方枚举是**开放集合**（本次 persistent_store + plugin 两处；未来新引用方须同步登记进对账 SQL，漏登记 = 该方备份被误判无主清理）——属结构性风险，规则同步处已注明登记义务
3. DTO 契约变化：`BackupDTO` 退役 sourceType/sourceId 字段 → bindings 再生成 → 前端若有消费需同步（实施时 grep `bindings` 前端引用面）
4. 2026-08-20 复原误消耗的存量脏数据（他代备份文件被移回覆盖、记录被删）不修复——测试库已实际损失

## 一、背景与裁决

2026-08-20 dev 验证爆发错配（复原链跨代误耗全部备份，见审查摘要①②），根因是 `original_file_path` 路径锚点无代次表达力。深挖后确认 backup 模块**两种职责形态并存**：来源三元组（source_type/source_id/original_*）反查 vs 发起方行内引用（plugin.BackupID）——每条链各选锚点，多代歧义此起彼伏。

**用户裁决（2026-08-20，定死边界）**：采用**方案二——backup 纯文件仓库 + 发起方内嵌引用**，**加强无主备份治理**，**引用键全链统一为 backup 清单行 ID**。边界与登记义务固化至 `.claude/rules/backend.md` MODULE_BOUNDARY_PURITY。

## 二、设计

### 2.1 边界定死（规则已固化）

- backup 表 = **纯保管清单**：只记自身领域的账（文件位置 `file_name`/`file_path`/`workdir` + 时间戳），不记「谁托付的、从哪来」
- 来源关联 = 发起方业务行的内嵌列，**引用键统一为 backup 清单行 ID**：persistent_store.`backup_id`（新增）、plugin.`BackupID`（既有）——全链单一键型，无路径/ID 双轨
- 退役 backup 全部 source 通路：`source_type`/`source_id`/`original_file_path`/`original_file_name`/`original_filename_extension` 五列、original_file_path 索引（85fb913 随列退役）、`GetBySourceTypeAndSourceId(s)`/`ListByOriginalPaths`/`GetLatestByOriginalPath`/`MoveBackupForResource`/`GetResourceBackup(s)` 与 `CreatePluginBackup` 的 source 参数全链
- 治理对账的引用方登记义务：新增「业务行引用 backup」时必须同步登记进对账（见 2.6），规则已注明

### 2.2 persistent_store 加列 `backup_id`

`int64`（0=无备份），与 `deleted_at` 的组合语义：

| deleted_at | backup_id | 语义 |
|---|---|---|
| 0 | 0 | 活行正常 |
| >0 | >0 | 软删+文件在备份（可复原） |
| >0 | 0 | 外部删除失效（fsmonitor MarkInvalid，无可复原文件） |
| 0 | >0 | **非法态**（同生共死不变量禁止；对账可查） |

**不变量**（新增测试锚定）：`backup_id` 写入 ⇔ 软删打标（同生共死，均在 `DeleteWithBackup` 内、文件移动与清单行建立成功后）；复原清列 ⇔ 清标。行内嵌后**多代备份概念消失**——同路径多代行各行指各的清单行，代次隔离结构性保证。

### 2.3 四链改造

1. **软删链**（`DeleteWithBackup`，persistentStore/service.go:717）：`MoveToBackup` 移文件+建清单行并返回其 ID → 行写入 `backup_id` + 软删（同生共死不变量落地；失败不写不删）
2. **复原链**（`restoreWorkFiles`，recycleBin/service.go:177）：按 work→resource→resource_store 收集 store 行（含删）→ 按各行 `backup_id` 取清单行路径 → 文件还原回 `file_path` → 清列+复活行+删清单行。**零路径反查**
3. **彻底删除链**（`Purge`，recycleBin/service.go:207）：级联**前**收集行（含 `backup_id`）→ 按 ID 删保管文件+清单行 → 物理级联
4. **/store/ 路由**（assetserver/store_handler.go）：`ResolveFileState` 返回扩展为 `(completed, deleted bool, backupId int64)`（一次行查询全得）；已删分支按 `backupId` 查保管清单（`GetById`）取 `file_path` 服务，`BackupPathResolver` 接口整个退役

### 2.4 plugin 域：删冗余通路

`Reinstall` 的 `GetPluginBackup(plugin.ID)` source 反查改为按行内 `BackupID` 直查（`backup.GetById`——按主键查保管清单，合法）；安装链备份改 `MoveToBackup`/`CreateBackup` 纯路径形态 + 写回 `BackupID`。升级成功后的备份清理（现状缺失）归 plugin 自治，与治理机制（2.6）衔接。

### 2.5 backup 接口纯化

- `MoveToBackup(absFilePath string) (backupId int64, error)`——移文件+建清单行，返回清单行 ID（全链统一 ID 键，无路径变体）
- `RestoreFile(backupRelPath, targetAbsPath)`、`DeleteBackup(backupId)`（删文件+清单行，按 ID）——保管域内部用路径定位文件、对外以 ID 为键
- `CreateBackup`（复制式）同型纯化，供 plugin 安装包备份
- `GetById`（既有）成为对业务行的唯一查询面

### 2.6 无主备份治理（用户点名加强；模型本次定死，巡检实现归树 C 节点）

**双向对账**（启动 + 定期，fsmonitor 对账先例）：
- **正向（无主检测）**：backup 清单行 ID 不被任何业务列引用 且 `create_time` 超过保留期（防写入窗口误判）→ 清理文件+行。SQL 形态（单一键型，统一 `NOT IN` 两处）：`id NOT IN (SELECT backup_id FROM persistent_store WHERE backup_id > 0) AND id NOT IN (SELECT backup_id FROM plugin WHERE backup_id > 0) AND create_time < :now - 保留期`
- **反向（悬空引用）**：业务列引用的 ID 无清单行/清单行文件缺失 → 清列/回填（补救中断窗口，见自曝风险1）
- 引用方枚举：`persistent_store.backup_id`、`plugin.BackupID`（开放集合，新增引用方须登记——见自曝风险2）

**迁移即首次治理**：存量 227 条 source 记录，store 类按 `source_id` 搬得动的写 `backup_id`；搬不动的（行已物理删）即无主存量，**迁移时直接清理**（裁决2）。

### 2.7 迁移与契约

1. persistent_store 加列（AutoMigrate，int64 默认 0，无 NULL 回填问题）
2. 存量 backup 记录搬迁：`UPDATE persistent_store SET backup_id = (SELECT id FROM backup WHERE source_type=3 AND source_id=persistent_store.id)`；行已物理删的清单行直接删除（文件+行）
3. drop backup 五列 + original_file_path 索引（幂等命名迁移）
4. `BackupDTO` 退役 sourceType/sourceId → `wails3 generate bindings -ts` → 前端消费面检查

### 2.8 测试

- 不变量：同生共死（移动/建行失败不写不删、成功即写即删）、非法态不出现
- 多代隔离回归：两代已删作品复原 A → 仅 A 的行清列+清单行删除+文件回位，B 原封不动；Purge 同理
- 治理对账纯函数测试：无主判定/悬空判定（含保留期窗口、ID 键型）
- 既有测试全量绿（`go test ./backend/...`）

## 三、实施步骤

1. backup：接口纯化（MoveToBackup 新签名/RestoreFile/DeleteBackup/GetById 为查询面）+ source 通路退役（列暂不 drop，链路先切）
2. persistentStore：加列 + `DeleteWithBackup` 写列 + `ResolveFileState` 扩展返回
3. recycleBin：复原/Purge 换行内读写
4. assetserver：`BackupPathResolver` 退役，已删分支按 ID 查清单
5. plugin：`Reinstall` 按 BackupID 直查 + 安装链接口适配
6. 迁移：加列 + 存量搬迁 + 无主存量直接清理（裁决2）+ drop 列/索引
7. DTO/bindings/前端检查
8. 测试补齐 + 全量

## 四、非目标

- 巡检任务的调度实现与清理策略 UI（树 C 节点，按 2.6 模型）
- 2026-08-20 复原误耗的存量脏数据修复（自曝风险4）
- `backup.file_path` 存量反斜杠行的分隔符规范化迁移缺口（独立待办；drop 列不受影响，`file_path` 列保留故仍需另行修正）
