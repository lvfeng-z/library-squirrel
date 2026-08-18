# store 文件备份操作收口 persistentStore 方案

## 审查摘要

**关键声明（抽查项）**：
- 声明1：work.Service 的 backupMover 依赖注入的是原始 backup.Service（`app.go:893`），其辅助函数 backupStore（`backend/work/service.go:790`）直调 FileMover.MoveToBackup（文件移动备份原语接口），绕过 persistentStore。
- 声明2：backupStore 与 persistentStore.Delete(id, backup=true)（store 删除+可选备份方法）的前奏逐行重复：查记录 → filepath.Join 拼绝对路径 → 抽文件名/扩展名（`backend/work/service.go:790-815` vs `backend/persistentStore/service.go:648-696`）。
- 声明3：work.Service 的 storeReader 依赖仅 backupStore 一个使用点（`backend/work/service.go:791`，grep `s.storeReader.` 全包仅此一处）。
- 声明4：逻辑删除的"事务内删记录"半边已有 persistentStore 对口能力 DeleteRecord（`backend/persistentStore/service.go:698-702`，注释明言"用于逻辑删除场景：文件已由调用方移入 backup"）——文件移动半边缺对应能力，即本方案要补的缺口。
- 声明5：替换流程 StoreBackupOrchestrator 走 persistentStore.Delete(id, true)（`backend/backup/store_backup_orchestrator.go:118`），零改动。
- 声明6：fsmonitor 误报修复已在 MoveBackup 汇点登记操作抑制（`backend/backup/service.go:126-133`），本方案不动它；重构后它继续兜底所有调用方。
- 声明7：两条路径存在两处未文档化的行为分歧——Status 门控（`backend/persistentStore/service.go:670` 仅 Complete 才备份；backupStore 不看状态）与失败处理（`backend/persistentStore/service.go:681-683` 失败降级 os.Remove；`backend/work/service.go:811-813` 失败中断整个删除）。
- 声明8：work 包测试用 `&Service{...}` 字面量构造、不经 NewService（`backend/work/service_workset_relation_test.go:107`），改构造函数签名不破坏既有测试。
- 声明9：persistentStore 已定义 FileMover 接口（`backend/persistentStore/service.go:186-196`，由 backup.Service 实现）、Service.fileMover 字段持有（`:201`），app.go 构造时已注入（`app.go:816` 传 app.BackupService）；既有 Delete(id,true) 即经 `s.fileMover.MoveToBackup` 消费（`backend/persistentStore/service.go:679`）——DeleteWithBackup 复用该字段，**零新增接口与装配**。

**待决策（需用户拍板）**：
- 决策1（已拍板 2026-08-18：保留）：新能力不看 Status、文件存在即备份（回收站复原保真；替换流程的"未完成即丢弃"语义留在 Delete 原地）。
- 决策2（已拍板 2026-08-18：fail-fast）：新能力失败时中断删除（维持 SoftDeleteWork 现状）。
- 决策3（已拍板 2026-08-18：采纳修复）：源文件已被外部删除时 Warn + 返回 0 跳过（快照侧 BackupID=0 已有跳过语义），删除继续；现行为"删除作品整体失败"确认为疣子非设计。
- 决策4（已确认 2026-08-18：新能力与 Delete 平行共存、各自文档化，见正文『2.3 与 Delete 的关系』）。**用户追加：新能力更名 `DeleteWithBackup`**（原名 BackupFile），并以扩展 work 既有 `StoreDeleter` 接口的方式消费（不再新建独立接口）——实施已按此落地。

**自曝风险**：
- 风险1：半迁移状态（多 store 循环中前面已移入 backup、后面失败）为既有问题，SoftDeleteWork 事务外逐个移动的现状未解决，本方案仅留档不处理。
- 风险2：重构后操作抑制登记三层并存（MoveBackup 汇点 + Delete + 新能力），时间戳制嵌套无害（`backend/storeRegistry/suppression.go:29` 注释），但需在代码注释说明防维护者困惑。
- 风险3：决策3 若采纳会改变"删除文件已外部缺失的作品会失败"的现行为——该现行为未见文档记载为有意设计，但采纳前需用户确认。

---

## 一、背景与问题

删除作品（SoftDeleteWork，逻辑删除进回收站）的事务约束：文件 IO 不可回滚 → 移文件必须在事务外、删记录必须在事务内（`backend/work/service.go:623-632` 注释）。persistentStore.Delete(id, backup=true) 把"移文件 + 删记录"耦合在一个非事务签名里不合用，于是 work 对两半分别下探——但不对称：事务内半边拿到了 persistentStore 级能力 DeleteRecord，事务外半边却下探到文件级原语 FileMover.MoveToBackup（backup.Service 实现，`app.go:893` 注入原始服务）。

代价：
1. **领域知识重复**："store 记录 → 绝对路径 → 备份参数"的转换逻辑在 backupStore 与 Delete 各编码一份（声明2），已产生两处行为分歧（声明7）。
2. **操作登记旁路**：fsmonitor suppression 的 6 个登记点全在 persistentStore，绕行即漏登记——2026-08-18 删除作品误报 bug 的结构性根因（已在 MoveBackup 汇点修补，声明6）。
3. **接口粒度错位**：work 消费的是文件机械学（4 个路径/文件名位置参数），实际需要的是 store 语义（storeId 进、backupId 出）。

## 二、目标设计

### 2.1 persistentStore 新能力

```go
// DeleteWithBackup 移动 store 文件到 backup 目录并创建备份记录，保留 persistent_store 记录
// （记录由调用方在事务内经 DeleteRecord 删除，两者构成逻辑删除的两半）。
// 返回备份记录 ID；0 = 无需备份（记录不存在/脏数据容忍跳过/路径无效，不阻断删除）。
func (s *Service) DeleteWithBackup(ctx context.Context, id int64) (int64, error)
```

实现要点：
- 文件移动经既有依赖注入：复用 `s.fileMover`（FileMover 接口，persistentStore 定义、backup.Service 实现、app.go:816 已注入），**不新增接口与装配**；fileMover 为 nil 时（"可选依赖"语义，实际 app.go 恒注入）Warn + 返回 0 跳过备份，删除继续。
- 查记录失败（含 ErrRecordNotFound 脏数据）→ Warn + 返回 0（沿袭 backupStore 容忍语义，`backend/work/service.go:791-797`）。
- 不做 Status 门控（决策1）：文件存在即备份。
- 文件操作前 storeRegistry.Suppress(relPath) + defer Release（与模块内其他落盘点同款模式；MoveBackup 汇点已兜底，此为同层第二道）。
- MoveToBackup 失败 → 返回包装错误（决策2 fail-fast）。
- 源文件缺失 → Warn + 返回 0（决策3 已采纳：删除继续，快照 BackupID=0 跳过）。
- 不删记录。

### 2.2 work 侧收缩

- 删 `BackupMover` 接口（原 `backend/work/service.go:293-297`）与 `StoreReader` 接口（`:281-285`，声明3 仅一个使用点）。
- 消费方式（按用户追加决策，实施形态）：扩展既有 `StoreDeleter` 接口加 `DeleteWithBackup` 方法，不新建独立接口——work 对 persistentStore 的窄接口由三个（StoreDeleter/StoreReader/StoreRecordDeleter）收敛为两个：

```go
// StoreDeleter PersistentStore 删除接口
type StoreDeleter interface {
    // Delete 删除记录及对应文件
    // backup: 是否对已完成文件进行移动备份
    Delete(ctx context.Context, id int64, backup bool) (int64, error)
    // DeleteWithBackup 删除 store 文件（移入 backup 并建备份记录），保留 persistent_store 记录
    // （记录由调用方在事务内经 DeleteRecord 删除，配合构成逻辑删除两半）
    DeleteWithBackup(ctx context.Context, id int64) (int64, error)
}
```

- backupStore 辅助函数（原 `backend/work/service.go:790-815`）整体删除，SoftDeleteWork 循环内改为 `s.storeDeleter.DeleteWithBackup(ctx, rsRow.StoreID)`。
- NewService 签名：移除 backupMover、storeReader、workDirGetter 三参数（workDirGetter 原仅服务于 backupStore 的路径拼接，随收口一并撤除）。
- `app.go` 装配：删 storeReader/backupMover/workDirGetter 三个实参（原 `:891/:893/:896`）。

### 2.3 与 Delete 的关系（为何不复用 Delete，决策4）

Delete(id, backup=true) 是"删除（附带可选备份）"，DeleteWithBackup 是"保全"，两者正交：

- **事务原子性（根本障碍）**：SoftDeleteWork 需要"移文件在事务外、删记录在事务内"（`backend/work/service.go:661-774`，快照的 StoreBackupRef{BackupID} 也要在事务前收集）。Delete 把移文件+删记录焊死在非事务签名里——若复用，记录在事务外即刻删除，事务一旦失败则 work/resource 尚存而 store 记录已没、backup 文件无任何回收站入口指向（复原/清空均由 recycle_item 快照驱动），作品损坏且复原路径丢失。这正是当初 work 绕开 Delete 下探 FileMover 的成因。
- **五维语义分化**（目标相反所致：Delete 优化删除可靠性、备份尽力而为；DeleteWithBackup 优化保全完整性、回收站复原保真）：Status 门控（Complete-only vs 不看状态，决策1）、MoveToBackup 失败（降级 os.Remove vs fail-fast，决策2）、源文件缺失（降级路径顺带容忍 vs 显式 Warn+返回 0，决策3）、查询失败容忍（仅 ErrRecordNotFound vs 任意错误 Warn+跳过，继承 backupStore 现状）、fileMover 为 nil（os.Remove 销毁 vs 返回 0 留原地）——硬塞进 Delete 要么加模式参数使签名撒谎，要么统一语义必改替换流程或回收站一边的行为。
- **共享面很小**：仅约 15 行前奏（查记录→拼路径→抽文件名）同形，且查询失败容忍策略不同（Delete 仅容忍 ErrRecordNotFound，DeleteWithBackup 沿袭 backupStore 任意错误容忍）；抽公共 helper 需参数化错误策略，得不偿失，保持平行实现各自文档化。

### 2.4 不改的部分

- 替换流程 StoreBackupOrchestrator → Delete(id, true)（声明5）零改动。
- recycleBin 复原/清空（走 StoreFromExternal / backup 侧 purge）零改动。
- MoveBackup 汇点抑制登记（声明6）零改动，继续兜底。
- Delete(id, true) 本体不动（决策4）。

## 三、实施步骤

1. persistentStore/service.go 增 DeleteWithBackup（含容忍查询、抑制登记、失败语义按决策2/3）。
2. work/service.go：接口替换（删 BackupMover/StoreReader，StoreDeleter 扩展 DeleteWithBackup）、SoftDeleteWork 调用点改写、删 backupStore、NewService 签名调整（含撤 workDirGetter）。
3. app.go 装配调整（删 storeReader/backupMover/workDirGetter 三实参，原 `:891/:893/:896`）。
4. `go vet ./backend/...` + `go build ./backend/...` + `go test ./backend/...`。
5. 运行时验证（与 I 节点验收合并做）：删除作品 → fsmonitor 零误报 + 回收站可复原 + 替换流程不受影响。

## 四、影响面

| 文件 | 改动 |
|---|---|
| backend/persistentStore/service.go | +DeleteWithBackup 方法 |
| backend/work/service.go | 接口替换（StoreDeleter 扩展）、调用点改写、删辅助函数、构造签名（含撤 workDirGetter） |
| app.go | 三处装配实参 |

行为变化：决策3 已采纳——删除文件已外部缺失的作品从"失败"变"跳过该文件继续删"；其余为等价重构。
