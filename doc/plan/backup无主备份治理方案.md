# backup 无主备份治理方案（双向对账 + 发起方直清）

## 审查摘要

**关键声明（抽查项）**：
- 声明1：替换/板块重执行是膨胀主源且无清理路径——`BackupStores`（按 store_type 板块备份作品旧文件的编排方法）经 `HardDelete(id, backup=true)` 移文件入 backup/ 并**物理删 store 行**（backend/backup/store_backup_orchestrator.go:131 → backend/persistentStore/service.go:677-694），备份清单行从此零业务引用，仅存 taskManager 内存清单（backend/taskManager/model.go:380）；失败路径还原后自清（backend/taskManager/model.go:773-787 → backend/backup/store_backup_orchestrator.go:191 逐条删备份），**成功终态无任何清理调用**（grep backend/taskManager 无 DeleteBackup 消费点）
- 声明2：plugin 升级覆盖引用不清旧备份——install 每次建新备份（复制安装包）并覆盖行内 BackupID（backend/plugin/service.go:529-537），旧清单行零引用；卸载仅置 Uninstalled 标记、行与引用保留（backend/plugin/service.go:693-696，不产生无主）
- 声明3：作品软删/复原/彻底删除三链已闭环——复原按行内 backup_id 定位、还原后删备份（backend/recycleBin/service.go:175-203），彻底删除级联前收集行内 backup_id 逐个删（backend/recycleBin/service.go:206-236）
- 声明4：persistent_store **软删行是合法引用者**——backup_id 与 deleted_at 同生共死/复原双清均 Unscoped 单条 UPDATE（backend/persistentStore/repository.go:64-86）；引用集查询必须**含已删行**（GORM 默认软删 scope 会静默排除已删行）
- 声明5：调度先例——recycleBin TTL 清理循环「启动即跑一次 + 24h ticker + stopCh 退出」（backend/recycleBin/service.go:239-266）
- 声明6：外部失效行 backup_id 保持 0——MarkInvalid 经 repo.Delete 纯软删、不动 backup_id（backend/persistentStore/service.go:246-250）
- 声明7：backup 仓储现有面仅 Create/GetById/Delete（backend/backup/service.go:23-30），治理所需按时间扫描/全量 ID 查询需新增
- 声明8：迁移期已有一次性无主清理先例（raw SQL `NOT EXISTS` 双引用方子查询，backend/migration/migrate.go:194-212）——运行期不复用其跨模块 raw SQL 形态，改经接口分区查询
- 声明9：**合并 overwrite 亦为零清理路径的备份产生点**（2026-08-21 调用方全量审计补充）——MergeStrategy=Overwrite 时源视频/音频轨经 `HardDelete(id, true)` 备份并物理删行（backend/resource/merge_service.go:245-258），备份行零引用、零内存清单（纯留档），成功后无人清理——治理落地前与替换场景同为永久累积源，落地后由正向无主清理覆盖

**已裁决（2026-08-21 用户拍板）**：
- 决策1＝**仅治理兜底**：发起方直清补齐不纳入本任务，另立延后分支（树 G 节点）——膨胀由保留期延迟消除，实践中不可接受时再回到该分支
- 决策2＝**保留期可配置**：settings 新增备份治理段（retentionDays，默认 7 天），经既有 SettingsHandler 通道读写，前端设置页落项（见 F 方案 2.4）
- 决策3＝**治理常开**：不加 enabled 开关
- 关联新增：备份管理 GUI（树 F 节点，独立方案 doc/plan/备份管理界面方案.md）——复用本方案的 BackupReferencer 枚举提供「有主/无主」引用状态数据面
- **监视哨采纳（2026-08-21）**：反向对账顺带计算按引用方分组的引用年龄统计（数量/占用/最老天数），超观察阈值（90 天常量）记 Warn 日志、F 面板高亮——有主侧生命周期机制失效（无终态调用方）年龄曲线单调上升，必然可见
- **规则固化采纳（2026-08-21）**：实施步骤 7 在 backend.md 登记义务旁补「新增引用方登记时须自带终态清理路径（消费式复原/物理删联动/手工入口），登记时检查」——与「漏登记=误清」对称的「登记了但无终态=永不清」防线

- **规则固化采纳（2026-08-21）**：实施步骤 7 在 backend.md 登记义务旁补「新增引用方登记时须自带终态清理路径（消费式复原/物理删联动/手工入口），登记时检查」——与「漏登记=误清」对称的「登记了但无终态=永不清」防线
- **决策4＝不采纳（2026-08-21）**：反向对账**不做**清单行文件存在性校验——外部文件变更感知属 fsmonitor 域，backupGovernance 保持纯 DB 对账（行存在性为限）；backup/ 目录外部变更（用户经文件管理器直删文件）暂无感知，复原链消费侧告警容忍（`RestoreFile` 报「备份文件不存在」跳过），未来如需覆盖属 fsmonitor 域任务（树 H 分支）

**自曝风险**：
- 风险1：暂停超过保留期的替换任务，其还原点被正向清理（之后恢复执行且失败时旧文件已不可还原）——现状进程重启本就丢内存清单（同源缺口，见『非目标』持久化分支），风险面未实质扩大；直清纳入后仅剩「长暂停 + 恰逢治理到期」窄窗口
- 风险2：引用集查询若误走 GORM 默认软删 scope，已删行引用不可见 → 活备份被误判无主清删 → 回收站复原不可用——灾难性失败模式，须测试锚定（声明4）
- 风险3：引用方枚举是开放集合，未来新增「业务行引用 backup」的列而未登记进对账 → 该方备份被误清（结构性风险；规则已注明登记义务，本方案把登记落点收敛到 app.go 装配处单一位置）
- 风险4：正向清理删文件不可逆（对齐 DeleteBackup 现状，无隔离区/软删缓冲）
- 风险5：plugin 直清时序——必须「建新成功 → 改引用 → 删旧」；建新失败不删旧（否则重装能力丢失）（随决策1 移出本任务，归树 G 分支实施时适用）

## 一、背景

任务树 C 节点（谱系 work-lineage-soft-delete）：期三存量清空实测暴露 backup/ 目录持续膨胀，清理路径缺失（声明1、2 及审计补充的声明9）+ 崩溃/中断窗口遗留。D 裁决（backup 能力包纯化）已把治理模型骨架定死于其方案 2.6 节：正向=清单行无业务列引用且超保留期→清理；反向=业务列悬空引用→清列。本方案落地该模型；发起方直清经决策1 裁决移出（树 G 延后分支）。

## 二、现状盘点：备份清单行的完整生命周期

| 链路 | 备份写入 | 业务引用 | 清理现状 | 遗留 |
|---|---|---|---|---|
| 作品软删链（`DeleteWithBackup`） | 移文件+行内写 backup_id（软删行持有） | persistent_store.backup_id | 复原/Purge/回收站 TTL 三链闭环（声明3） | 无 |
| 替换/板块重执行（`BackupStores`） | `HardDelete(backup=true)` 移文件+**物理删行** | **无**（仅 taskManager 内存清单） | 失败还原自清；**成功无清理** | **主源**（声明1） |
| 合并 overwrite（`MergeService`） | `HardDelete(backup=true)` 移文件+**物理删行** | **无**（纯留档，连内存清单都没有） | **无** | 同主源（声明9，审计补充） |
| plugin 安装/升级（install） | `CreateBackup` 复制安装包+写 BackupID | plugin.BackupID | 升级覆盖引用、删行后旧清单零引用，无清理 | **次源**（声明2） |
| 崩溃/中断窗口 | 任意链中断 | 不定 | 无 | 兜底对象 |

关键结构事实：备份引用分**持久引用**（业务行内嵌列）与**内存引用**（替换场景的还原点清单）两形。替换场景在途期间备份**合法地零业务引用**——正向判定必须以保留期垫住该在途窗口，**保留期是正确性参数而非卫生参数**。

## 三、设计

### 3.1 模块与接口

- 新模块 `backend/backupGovernance/`：横切维护域（定位对齐 fsmonitor——经接口编排他方能力，不直接读写他模块表）；无自有表，Handler 面向 F 的 GUI（见 F 方案 2.2）。
- 接口由调用方（治理方）定义、提供方实现（SERVICE_DEPENDENCY_VIA_INTERFACE）：
  - `BackupCatalog`（backup.Service 实现）：`ListCreatedBefore(ctx, beforeMs)`（正向候选）、`ListAllIDs(ctx)`（反向现存集）、`DeleteBackup(ctx, id)`（既有，backend/backup/service.go:172）。
  - `BackupReferencer`（persistentStore.Service 与 plugin.Service 各实现一份）：`Name()`（引用方展示名，监视哨与 F 统计面板用）；`ListReferencedBackupIDs(ctx)`（本方当前引用的全部清单行 ID）；`ClearBackupRefsByBackupIDs(ctx, ids)`（按引用目标清列——治理方算出悬空 ID 后调用，引用方无需感知 backup 全量）。
- persistentStore 额外方法 `ClearIllegalAliveBackupRefs(ctx)`：活行（deleted_at=0）携带 backup_id>0 的防御清列——构造上不可达（同生共死单条 UPDATE，声明4），实体注释已锚定该非法态（backend/base/model/entity/persistent_store.go:33）。
- 装配：app.go 注入 catalog + `[]BackupReferencer{persistentStore, plugin}`——**引用方枚举登记的唯一落点**，旁注释登记义务（漏登记=该方备份被误判无主清理，风险3）。
- 依赖方向（排查机制不得住进 backup 包）：backup 是纯叶子，只实现他方接口；枚举/清列接口的消费方是 backupGovernance（第三方编排者，ORCHESTRATION_BY_CALLER）。若由 backup.Service 反持 `[]BackupReferencer` 回调调用方，即能力包编排业务——PURITY 违规 + 依赖倒转（调用方依赖 backup 用能力、backup 反依赖调用方要接口，架构意义的环）。

### 3.2 双向对账算法

```text
run(ctx):
  referenced = ⋃ 各 referencer.ListReferencedBackupIDs()
  反向（悬空清列，防御性先行）:
    existing  = catalog.ListAllIDs()
    dangling  = referenced − existing
    各 referencer.ClearBackupRefsByBackupIDs(dangling)
      — persistent_store: Unscoped UPDATE backup_id=0（须含已删行，声明4）
      — plugin: UPDATE backup_id=NULL（NullInt64 语义）
    persistentStore.ClearIllegalAliveBackupRefs()
  监视哨（有主侧可观测性）:
    按 referencer.Name() 分组统计其引用备份的 数量/占用/最老引用年龄
    最老年龄 > 观察阈值（90 天常量，回收站默认 30 天的 3 倍）→ Warn 日志（不弹窗——已卸载插件重装包等
    合法长寿命引用不该被骚扰）；统计供 F 面板展示与高亮
    （已知行为：合法长寿命引用会周期性命中 Warn 日志——仅日志级噪音，接受为监视哨的代价）
  正向（无主清理）:
    candidates = catalog.ListCreatedBefore(now − 保留期)
    orphans    = candidates 中 id ∉ referenced
    逐个 catalog.DeleteBackup(id)（文件缺失容忍已内建，backend/backup/service.go:180-185）
```

- 两向无数据依赖，反向先行属防御性排序。
- 与迁移期单语句 `NOT IN` SQL（声明8）语义等价；运行期改为「接口分区查询 + 内存集合差」，避免业务模块跨模块 raw SQL（迁移层可用、运行期不可）。
- persistent_store 引用查询必须 Unscoped/原生投影（`SELECT DISTINCT backup_id WHERE backup_id > 0`）——GORM 默认软删 scope 排除已删行即风险2。
- plugin 引用集自然含已卸载行（表无软删，全量即含；卸载行持有重装能力引用，合法有主）。

### 3.3 调度

对齐 recycleBin TTL 清理循环先例（声明5）：`NewService` 后由 app 启动序列 `Start()`（goroutine：启动即跑一次 + 24h ticker + stopCh 退出），应用退出 `Stop()`。

### 3.4 保留期

settings 备份治理段 retentionDays（决策2，默认 7 天），每轮巡检时经 settings 读取（对齐回收站 TTL 的 settingsReader 读取形态，backend/recycleBin/service.go:269-273）。⚠️ settings 默认值双源陷阱：默认层两处定义（默认值构造与设置结构初始化）须同步写入 7，漏一处即零值生效（bool 零值无法区分未设置/显式关的历史事故同源）。常开无开关（决策3）。数值论证：7 天大于替换任务合理在途时长（含暂停数日的手动恢复节奏）；覆盖任意链崩溃/中断窗口；是 D 方案「防写入窗口误判」原意的超集。并发安全论证：新备份行的 create_time 恒为创建时刻，任何「先建行、后写引用」的写入窗口（毫秒级～任务在途）内 create_time 距今 << 保留期，不进正向候选；引用写入完成后 id ∈ referenced 不命中；DeleteBackup 幂等（行不存在返回 nil），与 GUI 手动清理（F 方案）/未来直清分支并发安全。

### 3.5 发起方直清（决策1 裁决移出）

替换成功终态清理与 plugin 升级/删行清旧备份另立延后分支（树 G 节点）。本任务落地后膨胀由保留期延迟消除；若实践中延迟不可接受（如高频替换场景磁盘压力）再回到该分支，实现要点已在本方案观察记录中留档（taskManager 失败还原 defer 的兄弟分支、plugin install 覆盖 BackupID 的删旧时序）。

## 四、实施步骤

1. backup：Repository + Service 增 `ListCreatedBefore` / `ListAllIDs`（backup 表无软删，普通查询）
2. settings：备份治理段（retentionDays 默认 7，默认值双源同步，3.4）
3. backupGovernance 新模块：接口定义 + 对账实现 + 调度（3.1–3.3）
4. persistentStore：`ListReferencedBackupIDs`（Unscoped DISTINCT）/ `ClearBackupRefsByBackupIDs`（Unscoped UPDATE backup_id=0）/ `ClearIllegalAliveBackupRefs`；**附带迁移：`UPDATE persistent_store SET backup_id = 0 WHERE backup_id IS NULL` 存量回填**（2026-08-21 查库发现 d6c7240 加列未回填、老行 NULL——`>0` 语义下与 0 等价无行为洞，规范化对齐「加列迁移必带回填」纪律）
5. plugin：同款两方法（BackupID 置 NULL）
6. app.go 装配（登记注释）+ 启动/退出挂钩
7. 文档同步：backend.md MODULE_BOUNDARY_PURITY 登记义务指向 BackupReferencer 装配点，并补「新增引用方登记时须自带终态清理路径，登记时检查」条款（已裁决采纳）；backup/persistentStore/plugin 各 README 增量
8. 测试补齐 + 全量 `go test`（排除 build/ 脚手架，既有钩子口径）

## 五、测试

- 正向：无主超期→清文件+行；**软删行引用不清**（核心回归，锚定声明4/风险2）；plugin 已卸载行引用不清；未超期不清；文件缺失容忍。
- 反向：persistent_store 悬空清列（含已删行——Unscoped 断言）；plugin 悬空置 NULL；非法态活行防御清列。
- 监视哨：按引用方分组的年龄统计正确；超阈值 Warn 触发、合法长寿命引用不误报路径不被阻塞。
- 保留期配置：settings 读取生效、默认值双源一致。
- 端到端（内存库）：作品软删 → 治理巡检运行 → 复原全链仍可用（误清防护的总验证）。

## 六、非目标

- 发起方直清补齐（决策1 裁决移出，树 G 延后分支）
- 清单行文件存在性校验（决策4 裁决移出，属 fsmonitor 域，树 H 延后分支）
- 备份管理 GUI 与设置页落项（树 F 节点，doc/plan/备份管理界面方案.md——本方案只保证 BackupReferencer 枚举可复用为其数据面）
- 替换还原点持久化（跨重启失败还原——已 reveal 为任务树延后分支 E，独立任务）
- backup.file_path 存量反斜杠迁移缺口（A 节点遗留独立待办，与 D 方案非目标一致）
- 2026-08-20 复原误耗存量脏数据修复（D 方案已裁定不修）
