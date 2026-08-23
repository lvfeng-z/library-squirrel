# fsmonitor 覆盖 backup/ 目录方案（任务 H）

# 审查摘要

**关键声明（抽查项）**：

- 声明1：fsnotify 事件源对整个 workDir 递归加 watch，backup/ 的事件**已在捕获**，运行时接入零源改动——被 `service.go` 的白名单过滤丢弃（`backend/fsmonitor/service.go:281`，`InScanDirs` 为 store-only，`backend/storeRegistry/registry.go:50-61`）。
- 声明2：backup/ 现在监控白名单外，外部删除备份文件无感知；backupGovernance 明确「文件存在性感知属 fsmonitor 域」（`backend/backupGovernance/service.go:319` 注释），保持纯 DB 对账（行存在性为限）。
- 声明3：backup 清单行无指纹列（`backend/base/model/entity/backup.go:11-17`，仅 FileName/FilePath/Workdir），运行时 fsnotify 无 rename 配对（无 USN cookie）→ backup 域运行时**只做 Missing 检出，不做指纹配对**；USN 离线段 rename 由 journal 自带配对（`backend/fsmonitor/usn_provider_windows.go:237-254` 的 ChangeMove），按路径查行即可支持 Move。
- 声明4：backup 模块自操作 backup/ 文件的全部落点仅两处产生 Remove 事件须抑制——`RestoreFile`（`backend/backup/service.go:215-239`，调用方：recycleBin `restoreWorkFiles` `backend/recycleBin/service.go:322` 与 `RestoreStore` `backend/recycleBin/service.go:459`、taskManager 回滚 `backend/taskManager/model.go:1312`）与 `DeleteBackup`（`backend/backup/service.go:196-210`，调用方：recycleBin 三处清理、backupGovernance 正向清理 `backend/backupGovernance/service.go:307`、plugin 卸载/换版直清 `backend/plugin/service.go:543,731`）；`storeFile` 目的端是 Create 事件（`backend/backup/service.go:111` rename 落入 / `:106` copy 落入），backup 域不报 Create → 无需登记。plugin 无其他 backup 文件写点（`backend/plugin/service.go` 全量 grep 仅 CreateBackup/DeleteBackup 消费）。
- 声明5：USN 离线段不需要抑制——自操作完成后 DB 行态已同步（RestoreFile 后调用方删行、DeleteBackup 自删行），下轮 USN Remove 事件查行落空即丢弃，与 store 域同构（store 域 DeleteWithBackup 后行软删、GORM scope 排除，`backend/fsmonitor/correlator.go:142-149` 查无记录不报告）。
- 声明6：ack 删行后引用清理已有既有兜底——backupGovernance 反向对账清悬空引用（`backend/backupGovernance/service.go:188-228`），启动+24h 节奏（`backend/backupGovernance/service.go:116-128`）；回收站条目 `CanRestore = HasBackup(backup_id>0) 且挂载链活作品`（`backend/search/repository.go:674,735`），引用未清期间会呈现「可复原」但复原走容忍跳过（`backend/recycleBin/service.go:313-316` 清单行缺失告警跳过、`:309-311` backup_id=0 静默跳过）。
- 声明7：确认流先例——RepairManager 内存待修复队列 + sync/restore/ack 三动作（`backend/fsmonitor/repair.go:14-24,97-116`），前端 ChangeConfirmDialog 模态框逐条+全部接受（`frontend/src/components/dialogs/ChangeConfirmDialog.vue:64-106`），事件 `fsmonitor:change` 经 MainIpcListener 入 store（`frontend/src/MainIpcListener.ts:85-95`）。
- 声明8：backup.FilePath 为 workDir 相对正斜杠路径（`backend/backup/service.go:125`），行内另存创建时 workDir（`Workdir` 列，`:126`）——workDir 迁移后的旧行不在当前监控树，须跳过不报。

**待决策（需用户拍板）**：

- 决策1（阻塞）：检出后联动形态——甲：进 fsmonitor 现有确认流（推荐）；乙：仅通知+备份管理面板治理；丙：自动删行无确认。
- 决策2（阻塞）：真相对齐形态——不加缺失标记列、ack=删清单行（推荐）；或 backup 表加缺失态列（触发能力包数据模型决策关口）。
- 决策3（阻塞）：引用清理时机——ack 时立即清（推荐）；或等既有反向对账（启动+24h）。
- 决策4（非阻塞）：USN rename 配对（Move 的 sync/restore）是否随 V1 支持——推荐支持（按路径查行，成本极低）；不支持则 USN 用户 rename 也降级为 Delete 报告。
- 决策5（非阻塞）：不增设监控开关（backup/ 随 fsmonitor 整体监控，D7 `suppressEnabled` 全局既有开关继续覆盖）。

**自曝风险**：

- 风险1：运行时（fsnotify 无配对）外部**改名** backup 文件 → 报 Delete，用户 ack 后行删除、新路径文件成无主磁盘孤儿（无行、治理不可见）。USN 开启者经 ChangeMove 获得 sync 避免误删。
- 风险2：整 backup/ 目录被删 → 目录 Remove 前缀展开 N 条目，模态确认框 N 条洪泛（与 store 域整删同病，「全部接受现状」可一键）。
- 风险3：D7 抑制开关关闭时主程序自写 backup/（还原/清理）会误报为外部删除，用户若 ack 会误删行——与 store 域 D7 退化行为同构，接受（开关文档已声明退回误报原状态）。
- 风险4：外部进程短暂移走又移回（杀毒隔离等）→ Remove 已入队，移回后 ack 会误删；窗口极窄+人工确认兜底。
- 风险5：待修复队列为内存态，未确认条目重启即失，靠下次离线对账复报（既有 store 域同构）。

---

## 1 背景与目标

任务 H（谱系 `work-lineage-soft-delete`）：用户经文件管理器直删 `backup/` 下备份文件时，该目录在 fsmonitor 监控白名单外无感知，backup 清单行指向已不存在的文件，复原链消费侧只能告警容忍（声明6）。C（backup 无主备份治理）裁决时把该感知职责移出：**backupGovernance 保持纯 DB 对账（行存在性为限），文件缺失感知归 fsmonitor 域**（声明2）。

本任务 = 把 `backup/` 纳入 fsmonitor 监控（运行时 + 离线两时机）+ 检出后与 backup 清单行的联动。

**目标态不变量**：`backup 清单行存在 ⇔ 其文件在位`（对受监控的当前 workDir 行成立）。行指向不存在文件即失真（RECORD_STATE_TRUTHFUL），感知属 fsmonitor 域，对齐手段见决策2。

## 2 现状勘察

### 2.1 fsmonitor 主链（监控/关联/修复三层）

- **事件源**：fsnotify 递归 watch 整个 workDir（`backend/fsmonitor/source.go:62`），backup/ 子目录全在 watch 树内；事件在 `handleFileChange` 的 `InScanDirs` 白名单过滤处被丢弃（声明1）。动态补 watch（新建目录）已有（`source.go:120-128`）。
- **关联层**（correlator）：store 域专用——Create 按指纹配对 Move / Remove 按路径查行产 Delete / 目录 Create 采样配对 DirMove。backup 域不复用指纹配对（无指纹列，声明3）。
- **修复层**（RepairManager）：内存 pending 队列，Confirm(id, action) 分派 sync/restore/ack（声明7）。
- **离线两段**：USN 精确追溯（Windows 管理员+开关）+ 全量对账兜底（scanner：walk 白名单目录 × persistent_store 比对，`backend/fsmonitor/scanner.go:25-57`），段间按 dedupKey 去重（`backend/fsmonitor/service.go:90-99`）。USN 段的 emit 过滤同 InScanDirs 口径（`backend/fsmonitor/usn_provider_windows.go:241-254`）。
- **操作抑制**：写方登记、读方 `handleFileChange` 查询丢弃；键为 workDir 相对正斜杠路径；登记点归文件操作属主（先例：`backup.storeFile` 源端自登记 `backend/backup/service.go:64-72`）。

### 2.2 backup 能力包与消费方

backup 为纯保管清单（五列：id/create_time/update_time/file_name/file_path/workdir），无来源信息；来源关联内嵌发起方业务行（persistent_store.backup_id、plugin.BackupID）。文件操作三个落点：`storeFile`（移入/复制入）、`RestoreFile`（移出还原）、`DeleteBackup`（删除）——抑制面核对见声明4。

### 2.3 backupGovernance 边界

双向对账：正向无主清理（超保留期+零引用）、反向悬空清列（引用的行已不存在）。**不感知文件**（`backupFileBytes` 文件缺失计 0，注释明示感知归 fsmonitor，声明2）。引用方枚举（persistentStore/plugin）的登记唯一落点在 app.go 装配处。

### 2.4 复原链容忍点（H 落地后的升级对象）

| 消费点 | 现行为 | H 后 |
|---|---|---|
| `recycleBin.restoreWorkFiles`（`backend/recycleBin/service.go:309-316`） | backup_id=0 静默跳过；清单行缺失 Warn 跳过 | 行已删+引用已清 → 前置态准确（HasBackup=false 不可复原），不再走到容忍分支 |
| `recycleBin.RestoreStore`（`:451-453`） | 查行失败报错中止 | 同上（CanRestore=false 前端不可点） |
| assetserver `/store/` 已删记录 | `ResolveBackupPathById` 返回不存在路径 → 文件层 404 | 行删除后返回空串（同现状 404，不恶化） |

## 3 设计

### 3.0 域模型：fsmonitor 从「store 域」扩展为「store + backup 双域」

fsmonitor 的既有定位（README）是「感知外部文件操作 + 编排修复」，backup 域是该定位的自然扩展，非新机制。落地为**按路径域路由**：

```
FileChange 事件
  ├─ 命中 store 白名单（InScanDirs，现状） → Correlator（指纹/路径关联） → store 域 SemanticChange
  ├─ 命中 backup 子树（InBackupDir，新增） → backupWatcher（路径/前缀关联，只认 Remove 与 ChangeMove） → backup 域 SemanticChange
  └─ 其余 → 丢弃（现状）
```

### 3.1 监控接入形态

- **storeRegistry 增 backup 根单一源**：新增 `BackupDirPath = "backup"` 常量与 `InBackupDir(rel)` 谓词（与 `InScanDirs` 同构：前缀或相等判定）；`backup.BackupRootDirName` 改为引用该常量（backup 已 import storeRegistry，无环）。`RegisteredDirs` 保持 store-only——`ValidatePath`（persistentStore 落盘校验）语义不变，仍拒绝 backup 路径。
- **运行时**：源零改动（声明1）；`handleFileChange` 过滤改域路由（backup 域仅消费 Remove 与 ChangeMove；Create 不报——无指纹配对，外部文件落入 backup/ 不产生清单行也不影响行真性）。
- **USN 离线段**：emit 过滤扩展同口径（backup 子树的 Remove/ChangeMove 放行）；自操作免疫靠 DB 行态已对齐（声明5），不需要抑制。
- **离线对账**：scanner 增 backup 段——walk `backup/` 收集磁盘文件集 × 清单行（限 `workdir = 当前 workDir` 且 file_path 有效的行）比对：行无文件 = BackupMissing；磁盘无行（孤儿文件）不报。`DiffSet` 增 `BackupMissing` 段。USN 段与对账段的去重键（dedupKey）增 domain 维度。

### 3.2 backup 域语义模型

`SemanticChange` 增 `Domain`（0=store 默认/1=backup）与 `BackupID`；backup 域仅两种形态：

| 形态 | 产生路径 | 含义 |
|---|---|---|
| Delete | 运行时 Remove（文件：按 file_path 精确查行；目录：按前缀圈行后逐行 stat 复核——目录 Remove 可能是 backup 子树内改名，行确实已失真但须排除「文件还在新位置而前缀误伤」之外的误报） | 清单行文件缺失 |
| Move | 仅 USN ChangeMove（From 落在 backup 子树）：按 From 路径精确查行（决策4） | 备份文件被改名/移动 |

**目录 Remove 的展开**：查 `file_path` 前缀命中的行 → 逐行 stat 复核（文件确实不在才报）→ 每行一条 Delete 条目。逐条形态与 store 域整删一致（风险2）。

**不报 Create/Untracked/DirMove**：外部文件落入 backup/ 不影响行真性；目录改名（运行时）经目录 Remove 展开为逐行 Delete 覆盖；USN 段目录改名按现状发 ChangeCreate(IsDir)（`usn_provider_windows.go:235`），backup 域不消费，兜底由对账段接住。

### 3.3 修复动作（联动形态，决策1 甲）

RepairManager 按 Domain 路由；backup 域注入两个新能力（Deps 增 `BackupReader`/`BackupRepairer`/`BackupRefCleaner`，nil = backup 域整体降级丢弃）：

| 动作 | backup 域 Delete | backup 域 Move |
|---|---|---|
| ack / sync（接受现状） | `BackupRepairer.DeleteRow(id)`（= backup.DeleteBackup，文件缺失容忍）+ `BackupRefCleaner.ClearBackupRefs([id])` 立即清引用（决策3） | `BackupRepairer.UpdateFilePath(id, newPath)`（行路径同步到新位置） |
| restore（复原） | 不适用（文件已失，无从复原；前端不渲染该按钮） | os.Rename 移回旧行路径，两端登记抑制（镜像 store 域 applyMove `repair.go:161-183`） |

- ack 幂等：行已不存在（并发/重复条目确认）视为成功出队。
- `BackupRefCleaner` 由 backupGovernance.Service 实现新公开方法（内部遍历 referencers 调 `ClearBackupRefsByBackupIDs`——引用方枚举知识不外溢，登记义务单一源不破）；查询失败 Warn 降级，残 dangling 由既有反向对账兜底（声明6）。
- 编排归属：确认流（发起方 fsmonitor）经注入的两提供方能力串联删行+清引用，符合 ORCHESTRATION_BY_CALLER；backupGovernance 不感知 fsmonitor，backup 不感知治理。

### 3.4 主程序自写抑制登记（两处，均在 backup.Service 内部）

| 落点 | 事件 | 登记 |
|---|---|---|
| `RestoreFile`（`service.go:215`） | 源端 Remove（rename 移出 / 跨盘回退 copy 后 `os.Remove` `:236`） | 方法内计算 backupPath 相对 workDir 的 rel（Rel 逃逸/旧 workDir 不登记，镜像 `storeFile:68` 手法），`Suppress` → 操作 → `Release` |
| `DeleteBackup`（`service.go:196`） | 文件 Remove（`:205`，行删除在文件删除之后 → 竞态窗口） | 同上 |

单一登记点覆盖全部调用方（声明4 的 recycleBin×2 还原链、taskManager 回滚、backupGovernance 清理、plugin 卸载/换版）。`storeFile` 目的端 Create 不报不登记；其源端 Remove（MoveToBackup 移出 store/）既有登记保留。

### 3.5 真相对齐（决策2 推荐：不加列）

行存在 ⇔ 文件在位的不量由「ack 删行」维持：检测→用户确认→删行+清引用，行集与文件集重新对齐。backup 表**零结构变更**——不触发能力包数据模型关口；backupGovernance 边界不破（它继续只见行，缺失感知与对齐动作都在 fsmonitor 域编排）。

### 3.6 接口与契约变更清单

| 模块 | 变更 |
|---|---|
| storeRegistry | `BackupDirPath` 常量 + `InBackupDir(rel)`；README 同步 |
| backup | Repository/Service 新增：`GetByFilePath`、`ListByPathPrefix`、`ListAll`（workdir 过滤在查询参数）、`UpdateFilePath`；`RestoreFile`/`DeleteBackup` 抑制登记；`BackupRootDirName` 改引用 storeRegistry 常量；README 同步 |
| fsmonitor | `SemanticChange.Domain/BackupID`、`DiffSet.BackupMissing`、dedupKey 增域；新文件 backup_domain.go（backupWatcher + 三接口定义）；`handleFileChange` 域路由；`runOfflineReconcile` backup 段；scanner backup walk；repair 域分派；handler DTO 增 `domain`/`backupId`；README 同步 |
| backupGovernance | 新公开方法 `ClearBackupRefs(ctx, ids)`（遍历 referencers 清列）；README 同步 |
| app.go | 三个适配器装配（backup.Service×2 + backupGovernance.Service） |
| 前端 | bindings 再生成；MainIpcListener payload、UseChangeConfirmStore.ChangeInfo 增 domain/backupId；ChangeConfirmDialog 域感知（backup Delete 文案「备份文件缺失」仅 ack 按钮；backup Move 同步/复原两键） |

事件 payload `fsmonitor:change` 增 `domain`（0/1）与 `backupId` 字段（store 域条目 backupId=0）。

## 4 测试锚定

1. **运行时检出**：Remove(backup 文件)×行存在 → pending 条目（domain/backupId/kind=Delete）；行不存在 → 无条目（外部无关文件不报）。
2. **目录展开**：Remove(backup/2026/08 目录) → 前缀圈行逐行 stat → 条目；子树内改名（Remove 旧目录+Create 新目录、文件在新位置）→ 旧行 stat 失败仍报（行失真是事实）。
3. **自操作抑制**：调用 `DeleteBackup`/`RestoreFile` 后宽限窗口内 `IsSuppressed(rel)` 为真（Release 宽限 3s > 测试时长，确定性断言）；抑制命中时 `handleFileChange` 丢弃 backup 事件。
4. **离线对账**：scanner backup 段——行无文件 → BackupMissing；磁盘孤儿文件 → 不报；旧行（workdir≠当前）→ 跳过。
5. **ack 联动**：确认后行删除 + `ClearBackupRefs([id])` 被调；行已不存在的重复 ack 幂等成功。
6. **USN Move**（决策4 采纳时）：ChangeMove(From=backup 路径) → Move 条目；sync → `UpdateFilePath` 生效；restore → 文件移回。
7. **跨段去重**：USN 段与对账段同报一行缺失 → 仅一次 dispatch（dedupKey 含 domain）。

全量 `go test ./...`（排除 build/，go-check 钩子）+ `yarn build` + dev 实机手测：文件管理器直删 backup 文件 → 检出 → 确认联动（行删除、回收站 HasBackup/CanRestore 即时翻false）；主程序自身删除（清理无主）/还原（复原作品）零误报。

## 5 实施步骤

1. storeRegistry：`BackupDirPath` + `InBackupDir` + backup 常量翻转引用。
2. backup：四个查询/更新方法 + 两点抑制登记。
3. fsmonitor：域模型扩展（SemanticChange/DiffSet/dedupKey）+ backup_domain.go（接口+backupWatcher）+ handleFileChange 路由 + scanner 对账段 + runOfflineReconcile backup 段 + repair 域分派 + handler DTO。
4. backupGovernance：`ClearBackupRefs` 公开面。
5. app.go 装配；bindings 再生成。
6. 前端：MainIpcListener / UseChangeConfirmStore / ChangeConfirmDialog 域感知。
7. 测试（第 4 节七组）+ 四模块 README 同步。
8. 全量验证 + dev 手测 + /commit 提交 + 回树填哈希。

## 6 决策详述（对摘要待决策项的展开）

- **决策1 联动形态**：推荐甲（进现有确认流）。丙（自动删行）在 D7 抑制开关关闭时把自操作误报放大为静默数据删除，不可接受；乙（仅通知+面板治理）需在备份管理面板再造缺失态呈现+清理动作面，且丢失运行时即时推送的消费闭环，机制二存。甲复用 RepairManager/确认框/事件三件套，backup 域只多一个 Domain 维度。
- **决策2 真相对齐**：推荐不加列。缺失态列使「行存在」不再蕴含「文件在位」，消费面（治理/引用/复原）全都要加过滤——引入第二真源；ack 删行一步对齐，行集即文件集。能力包表结构关口因此不触发。
- **决策3 引用清理时机**：推荐 ack 时立即清。等 24h 的窗口期内回收站条目 `CanRestore` 虚高（声明6），用户点复原走容忍跳过——正是 H 要消灭的体验；立即清由 backupGovernance 新公开方法承担，枚举知识不外溢，失败有既有反向对账兜底，代价一个小方法。
- **决策4 USN Move**：推荐随 V1 支持。processMove 形态按路径查行（`correlator.go:169-189` 同构），无指纹依赖；不支持则 USN 用户的 rename 降级为 Delete（风险1 扩大到 USN 群体）。
- **决策5 开关**：不增设。backup/ 监控无独立退场需求，D7 全局开关已覆盖紧急回退。
