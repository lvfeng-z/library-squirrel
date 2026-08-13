# store 操作抑制（suppression）方案

## 审查摘要

> 本方案为 fsmonitor 可用化的关键一步：给 `storeRegistry` 装上**有状态的运行时操作抑制集**，让 fsmonitor 把"软件自身的 store/ 写入"与"外部文件操作"区分开，根治当前"内部写入被误报为外部变更（SemanticUntracked/SemanticMove/SemanticDelete）"的缺口。机制：写方在落盘前向 storeRegistry 登记路径 + 宽限期，读方（fsmonitor `handleFileChange`）在关联前查抑制集命中即丢弃。「storeRegistry 与指纹独立重构方案」已把 storeRegistry 作为载体就位（`backend/storeRegistry/registry.go`，当前无状态纯白名单），本方案为其加抑制状态。下列现状声明均带代码锚点。

### 关键声明（带锚点）

1. **误报根因在读方"不认自家人"**：fsmonitor 收到 fsnotify Create 事件后，`correlator.processCreate` 按**内容指纹**查 DB（`correlator.go:115` `GetByFingerprint(digest, excludePath=当前路径)`），**不检查"该路径是否为软件刚写入"**；故软件自身落盘的每个文件都被判为 `SemanticUntracked`（无同指纹他路径）或 `SemanticMove`（有同内容他路径）。事件经 `dispatchSemanticChange` → `Emit("fsmonitor:change")`（`service.go:227`）→ 前端 `MainIpcListener.ts:82` 弹窗。
2. **fsmonitor 无条件启动、无任何抑制**：`app.go` 创建 service 后直接 `Start()`（无总开关）；`FsmonitorSettings` 仅含 `UsnEnabled`（`settings/model.go:60`）；全包 grep `suppress` 零命中。
3. **storeRegistry 当前是无状态纯白名单**：`registry.go` 仅 `RegisteredDirs`/`ValidatePath`/`InScanDirs`/`RegisteredPaths`，无运行时状态——suppression 的载体已就位、灵魂未装入。
4. **写方高度收敛于 persistentStore 统一入口**（Explore 盘点）：taskManager 下载（`taskManager/model.go:937,1304,1385` StoreStream/Write/Complete）、merge（`resource/merge_service.go:220` StoreFromFile）、backup orchestrator（`store_backup_orchestrator.go:118 Delete` / `:178 StoreFromExternal`）、recycleBin（`recycleBin/service.go:203 StoreFromExternal`）**全部经 persistentStore**，无直接 os 写 store/。
5. **persistentStore 磁盘写点完整清单**（`persistentStore/service.go`）：`StoreStream`(350/356/361)、`ResumeStream`(432/436)、`Store`(470/476/481/487)、`StoreFromExternal`(709/719/725/728/731)、`Delete`(664/667/670)、`CleanupFile`(325)、`storeWriter.Complete`(102/104/108)、`storeWriter.Abort`(145)。`DeleteRecord`(685) 无磁盘。
6. **唯一绕过统一入口的活跃写方 = fsmonitor 自身**：`fsmonitor/repair.go:147`（`applyDirMove` ActionRestore `os.Rename`）、`repair.go:167`（`applyMove` ActionRestore `os.Rename`）——用户确认"复原"时把文件/目录移回 store/，**会触发 fsmonitor 自己的事件**（自反馈，不抑制则复原→再检测→重复报告）。ActionSync（UpdateFilePath/RenameDirectoryPrefix）、ActionAck（MarkInvalid）仅改 DB 无磁盘（经 storeRepairerAdapter）。
7. **潜在写方（当前死代码）**：`backup/service.go:258 RestoreFile`（268/273/276 直接 os.Remove/Rename/CopyFile），全仓 grep 无调用方（活跃还原走 persistentStore.StoreFromExternal）；接入时须记得登记。
8. **确认无 store/ 写入的模块**：localAuthor/siteAuthor（纯 DB，头像目录 `store/avatar/{local,site}` 已注册但本仓无落盘代码，预计插件侧/未实现）；backup.CreateBackup 写 `backup/`（白名单外）；assetserver/store_handler 只读；plugin 插件目录。
9. **读方抑制点唯一且干净**：`fsmonitor/service.go:276 handleFileChange` 是所有运行时 fsnotify 事件进 correlator 的唯一入口；离线对账（`runOfflineReconcile` :103）不经此函数（启动时无内部写入，不该抑制）——抑制只插在 handleFileChange，天然不污染离线对账。

### 待决策

- **D1 API 形态**：①显式路径登记集（`Suppress(relPath)`/`Release(relPath)`/`IsSuppressed(relPath)`，推荐——能覆盖 storeWriter 跨方法生命周期）vs ②回调式（`WithSuppression(ctx, func())`，简洁但无法表达 StoreStream→Complete 跨方法的登记）。推荐①。
- **D2 宽限期时长**：Release 后保留登记的时长，覆盖 fsnotify 异步延迟。推荐 **3 秒**（fsnotify Windows 通常亚秒级，3s 留足余量；过长会延长真外部操作被误抑制的窗口）。
- **D3 泄漏兜底超时**：Suppress 时设置的最长存活时间，防止写方崩溃/调用方忘记 Release 导致永久抑制。推荐 **30 秒**（覆盖最大单文件写入+Complete 时长；超时强制过期）。
- **D4 登记粒度——局部登记，不跨方法**：fsnotify 只对 Create/Remove 发事件（`source.go:129` default 不处理 Write），文件内容的写入/截断/关闭不产生事件。故每个产生 Create/Remove 的磁盘操作点**局部** `Suppress + defer Release` 即可，storeWriter 不持有任何抑制状态。映射：`StoreStream`(Create+Remove旧) / `Store` / `StoreFromExternal` / `Delete` / `CleanupFile` 入口 Suppress+defer Release；`storeWriter.Abort`(Remove) 局部 Suppress+defer Release；`storeWriter.Complete`(无事件) / `ResumeStream`(仅 Write，被丢弃) **不抑制**。
- **D5 前缀匹配 + 相对路径键**：`IsSuppressed` 须支持"路径自身或其祖先目录在抑制集"，按 `/` 逐级向上查祖先（O(深度)，抑制集小可接受）。**键用 workDir 相对正斜杠路径**（非绝对路径）：读方 `FileChange.Path` 天然相对（`source.go:209` toRel），相对键零转换；与 DB file_path / storeRegistry 白名单同基准；规避绝对路径的盘符+反斜杠归一、workDir 变更失效、跨平台不一致。写方登记用现成相对路径（persistentStore relPath / repair SemanticChange 字段）。
- **D6 repair 自反馈登记范围**：移动产生两端事件（From 的 Remove + To 的 Create）。推荐 Suppress(FromPath) + Suppress(ToPath) 双登记，宽限释放。
- **D7 紧急回滚**：settings 加 `fsmonitor.suppressEnabled`（默认 true）。suppression 逻辑 bug 致持续误抑制时，用户可关抑制止损（退回"无抑制=内部写入误报"原状态，由对账兜底，不致数据损坏）。推荐采纳——同时给 fsmonitor 自身留一个总闸。

### 自曝风险

- **R1 时序竞态（核心风险）**：三处窗口——①Suppress 调用与首个磁盘写之间（极窄，先 Suppress 再写即可）；②fsnotify 事件可能**早于** Suppress 到达（理论上，实测罕见，因写操作触发事件需文件系统已变更，而变更在 Suppress 之后）——本方案不专门处理（概率极低且后果仅一次误报）；③Release 与 fsnotify 延迟事件到达之间——**宽限期（D2）专门覆盖**。宽限期是本方案对时序竞态的主要防线。
- **R2 路径基准一致性**：写方登记键与读方查询键同基准（workDir 相对正斜杠），三处来源已核实：写方 persistentStore relPath 入库前 `filepath.ToSlash`（`service.go:391/517/743`）；repair 的 `SemanticChange.FromPath/ToPath` 与读方 `FileChange.Path` 同基准；读方 fsnotify source `toRel` 返回 `filepath.ToSlash`（`source.go:209`）。storeRegistry 抑制 API 内部对入参再 `filepath.ToSlash` 归一作双保险，杜绝反斜杠键漏匹配。
- **R3 宽限期内真外部操作漏报**：宽限期内若该路径恰有真外部操作，会被误抑制。概率低（宽限期短 3s + 该路径刚被内部写过、短时间内再被外部改的概率小），且后果可接受（下次对账兜底发现），不追求强一致。
- **R4 前缀匹配性能**：每次 fsnotify 事件查抑制集需逐级查祖先。高频事件 + 大抑制集时理论 O(深度×事件数)。实际抑制集只含"进行中操作"（个位数），深度有限（store/resource/作者/文件 ≈ 4 级），可接受；若未来写入并发极高再考虑前缀树优化。
- **R5 storeWriter 泄漏导致持续抑制**：调用方拿到 storeWriter 后不调 Complete/Abort（异常退出等）→ 登记不释放 → 该路径被持续抑制。防线：D3 泄漏兜底超时（30s 强制过期）+ 惰性清理（查询/定期清过期项）。
- **R6 不影响离线对账 / USN 路径无关**：抑制只插在 `handleFileChange`，而该函数**仅被 fsnotify LiveSource 事件循环调用**（`startLive` :243 → :263）；USN provider 的 `ChangesSince` 在 `runOfflineReconcile`（:118）调用、产物经 `correlator.Process`（:126）**不进 handleFileChange**——故 USN 产出路径基准与 suppression 完全无关，Windows USN 模式下抑制照常生效。离线对账启动时无内部写入，本就不需抑制。
- **R7 并发安全**：写方（多 goroutine 登记释放）与读方（fsmonitor 事件 goroutine 查询）并发访问抑制集。`sync.RWMutex` 保护 map（写少读多，RWMutex 合适）。

---

## 一、背景与目标

### 1.1 解决的缺口

fsmonitor 功能闭环完整（监控→指纹→关联→对账→修复→前端弹窗），但**无 suppression 导致生产不可用**：软件自身的每一次 store/ 写入（下载落盘、合并、备份还原、回收站还原）都被 fsnotify 捕获，经 `correlator.processCreate`（按指纹、不认当前路径）误判为外部变更，前端 `ChangeConfirmDialog` 刷屏、`RepairManager` 队列堆积。用户无法区分弹窗里哪些是真正的外部操作。

详见前序诊断（对话记录）与「storeRegistry 与指纹独立重构方案」第六节预告——suppression 是 fsmonitor 可用的**硬前提**，非可选优化。

### 1.2 目标

1. storeRegistry 从无状态白名单升级为**有状态操作抑制中介**：写方登记进行中路径，读方查询命中即丢弃。
2. 覆盖**所有**内部写方：persistentStore 统一入口（收敛 taskManager/merge/backup/recycleBin）+ fsmonitor/repair 自反馈。
3. **时序安全**：宽限期覆盖 fsnotify 异步延迟；泄漏兜底防永久抑制。
4. **不破坏现有架构**：suppression 是 storeRegistry 内部状态，persistentStore 与 fsmonitor 仍只依赖 storeRegistry，不直接互认（保持 storeRegistry 与指纹独立重构的单向依赖）。

### 1.3 非目标

- 不改离线对账行为（启动时无内部写入，不抑制）。
- 不改 fsnotify 事件源、不改 correlator 关联逻辑（抑制在关联之前，命中直接丢弃）。
- 不处理"fsnotify 事件早于 Suppress 到达"的极罕见竞态（R1②，后果仅一次误报，对账兜底）。

---

## 二、机制设计

### 2.1 抑制集数据结构（storeRegistry 内）

```go
// suppressEntry 抑制登记项
type suppressEntry struct {
	expiry int64 // 过期毫秒时间戳（UnixMilli）；到达即失效
}

var (
	suppressMu    sync.RWMutex
	suppressSet   = make(map[string]suppressEntry) // 键：workDir 相对正斜杠路径
)
```

- 键：workDir 相对**正斜杠**路径（与 fsmonitor `FileChange.Path` 同基准）。
- 值：仅过期时间戳（不需要引用计数——同路径并发写罕见，且过期机制兜底）。

### 2.2 API

```go
// Suppress 登记路径为"软件自身操作中"，fsmonitor 命中即丢弃事件。
// expiry = now + writeTimeout（D3 泄漏兜底）。内部 ToSlash 归一。
func Suppress(relPath string)

// Release 宽限释放：把 expiry 刷新到 now + gracePeriod（D2），
// 覆盖 fsnotify 异步延迟事件，而非立即删除。
func Release(relPath string)

// IsSuppressed 查询路径是否被抑制：精确命中或其任一祖先目录在集（D5 前缀匹配），且未过期。
// 查询时顺带惰性清理过期项。
func IsSuppressed(relPath string) bool
```

> 常量：`suppressWriteTimeout = 30 * time.Second`（D3）、`suppressGracePeriod = 3 * time.Second`（D2）。

### 2.3 前缀匹配（D5）

`IsSuppressed("store/resource/作者/x.jpg")` 需命中登记的目录 `"store/resource/作者"`。实现：按 `/` 拆分路径，从完整路径逐级向上剥离末段查询（`store/resource/作者/x.jpg` → `store/resource/作者` → `store/resource` → `store`），任一命中且未过期即 true。O(深度)。

### 2.4 过期清理

惰性：`IsSuppressed` 查询路径时，若该键存在但已过期则删除并返回 false。辅以 `Start` 时启动的后台 goroutine 每 10s 全量清过期项（防永不查询的键堆积）。

---

## 三、写方登记点

### 3.1 persistentStore 统一入口（覆盖 taskManager/merge/backup/recycleBin）

| 方法 | 登记动作 | 释放动作 |
|---|---|---|
| `Store`(453) / `StoreFromFile`(539) | 入口 `Suppress(relPath)` | `defer Release(relPath)` |
| `StoreFromExternal`(694) | 入口 Suppress(relPath) | defer Release(relPath) |
| `Delete`(635) / `DeleteByFilePath`(760) | 入口 Suppress(record.FilePath) | defer Release |
| `CleanupFile`(325) | 入口 Suppress(relPath) | defer Release |
| `StoreStream`(335) | 入口 Suppress(relPath)（覆盖 Remove旧 + Create） | defer Release（方法返回=创建完成） |
| `storeWriter.Abort`(135) | 局部 Suppress(filePath)（覆盖 Remove） | defer Release |
| `storeWriter.Complete`(97) | — | —（无 Create/Remove 事件，不抑制） |
| `ResumeStream`(411) | — | —（仅 Write 事件被 source 丢弃，不抑制） |

**全部局部登记**（D4）：每个产生 Create/Remove 的点 `Suppress + defer Release`，不跨方法、storeWriter 不持有抑制状态。依据 fsnotify 事件模型——只 Create/Remove 发事件，下载期间的写入无事件，故 Suppress 只需覆盖"创建/删除那一瞬 + fsnotify 延迟"，不需覆盖整个写入过程。

> 写方登记用 relPath（workDir 相对）；Delete/CleanupFile/Abort 用 record.FilePath / storeWriter.filePath（已正斜杠入库值）。storeRegistry API 内部 ToSlash 双保险。

### 3.2 fsmonitor/repair 自反馈（D6，绕过统一入口的唯一活跃写方）

`repair.go:147 applyDirMove`(ActionRestore) / `:167 applyMove`(ActionRestore) 的 `os.Rename(from, to)`：
- 登记两端：`storeRegistry.Suppress(FromPath)` + `storeRegistry.Suppress(ToPath)`
- 执行 os.Rename
- 宽限释放两端：`Release(FromPath)` + `Release(ToPath)`

> 移动产生 From 的 Remove 事件 + To 的 Create 事件，两端都须抑制，否则复原后立刻又报一次"移动"。

### 3.3 死代码标注（非本次实现）

`backup/service.go:258 RestoreFile`：当前无调用方。在其函数注释加 TODO 标注"接入时须 storeRegistry.Suppress(targetPath)"，防未来接入漏登记。

---

## 四、读方改造（fsmonitor handleFileChange）

`fsmonitor/service.go:276 handleFileChange` 入口、`correlator.Process` 之前加抑制查询：

```go
func (s *Service) handleFileChange(ctx context.Context, ev FileChange) {
	// 内部操作抑制：软件自身的 store/ 写入登记的路径，命中即丢弃，不进关联层
	if storeRegistry.IsSuppressed(ev.Path) {
		return
	}
	if ev.ToPath != "" && storeRegistry.IsSuppressed(ev.ToPath) {
		return // Move 事件两端任一命中都抑制
	}
	// 原有关联逻辑
	if s.correlator == nil { ... }
	sc := s.correlator.Process(ctx, ev)
	...
}
```

- 抑制在关联**之前**：命中直接 return，省指纹计算。
- 仅作用于 fsnotify LiveSource 的**运行时实时事件**（`handleFileChange` 的唯一调用方）；离线对账与 USN 追溯不经此函数（见 R6）。

---

## 五、时序竞态分析（R1 详解）

正常路径（以 StoreStream 下载为例）：

```
t0  Suppress(path)            登记 expiry=now+30s
t1  os.Create(path)           磁盘变更
t2  storeWriter.Write(...)    多次写
t3  Complete()                Release → expiry=now+3s（宽限期起点）
t4  fsnotify Create 事件到达  handleFileChange 查 IsSuppressed → 命中（t4 < t3+3s）→ 丢弃 ✓
t5  宽限期到期                清理，恢复正常监控
```

竞态窗口与对策：

| 窗口 | 风险 | 对策 |
|---|---|---|
| t0 前（Suppress 前） | fsnotify 事件早于登记到达 | 不处理（R1②，极罕见，对账兜底） |
| t0–t3（写期间） | 事件到达 | 登记有效，命中丢弃 ✓ |
| t3–t5（宽限期） | fsnotify 延迟事件到达 | 宽限期（D2）覆盖 ✓ |
| t5 后 | 真外部操作 | 正常监控 ✓ |
| 写方崩溃（无 Release） | 永久抑制 | writeTimeout（D3）30s 强制过期 ✓ |

---

## 六、实施阶段

| 阶段 | 内容 | 依赖 |
|---|---|---|
| **S1** storeRegistry 抑制状态 | 加 `suppressSet`+`RWMutex`+`Suppress`/`Release`/`IsSuppressed`+常量+惰性清理+后台清理 goroutine；单测（登记/宽限释放/前缀匹配/过期/并发） | 无 |
| **S2** persistentStore 登记接入 | 各 Create/Remove/Rename 点局部 Suppress+defer Release；StoreStream 入口登记、storeWriter.Abort 局部登记；Complete/ResumeStream 不抑制（D4） | S1 |
| **S3** fsmonitor 读方 + repair 自反馈 | `handleFileChange` 入口查 IsSuppressed；`repair.applyMove/applyDirMove` 双端 Suppress/Release | S1 |
| **S4** 死代码标注 + 文档 | backup.RestoreFile TODO 注释；storeRegistry/README、fsmonitor/README、persistentStore/README 同步 suppression | S2,S3 |
| **S5** 端到端验证 | 运行验证：下载落盘不再误报、外部改名仍检测、复原不再自反馈 | S2,S3,S4 |

> 每阶段 `go build ./...` + 相关包测试。S2/S3 可并行（分别改 persistentStore/fsmonitor，都只依赖 S1）。

---

## 七、验证清单

- [ ] `go build ./...` 通过；storeRegistry import 仍仅标准库（R1 循环依赖）
- [ ] storeRegistry 单测：Suppress→IsSuppressed 命中、Release 后宽限期内仍命中、宽限期后失效、前缀匹配（目录登记命中下级文件）、过期惰性清理、并发登记释放无 race
- [ ] persistentStore：Store/StoreStream/StoreFromExternal/Delete 落盘期间 IsSuppressed(path)=true，Complete/Release 后宽限期内仍 true、之后 false
- [ ] fsmonitor handleFileChange：抑制路径事件被丢弃（不进 correlator、不 Emit）
- [ ] repair ActionRestore：复原后不再产生新的 fsmonitor:change 事件（自反馈消除）
- [ ] **端到端**：①任务下载落盘 → 前端无"外部新增"误报弹窗；②资源管理器改名 workDir 文件 → 仍正常检测+弹窗；③对变更点确认"复原" → 文件移回且不重复报告
- [ ] 离线对账行为不变：启动时外部操作仍被对账发现（抑制不污染对账路径）
- [ ] 文档同步：storeRegistry/README（加 suppression 能力）、fsmonitor/README、persistentStore/README

---

## 八、实施验证发现的根因（2026-08-14 实测定位）

运行时实测暴露两个方案未预见的问题，已修：

1. **`defaultSettings()` 与 `NewSettings()` 双源不一致 → suppressEnabled 默认 false**：`settings/service.go` 的 `defaultSettings()`（koanf 默认源）首版漏设 `FsmonitorSettings`，而 `NewSettings()` 设了 `SuppressEnabled:true`。koanf 加载用 defaultSettings 的零值 false；用户旧 settings.json 无该字段 → merge 后仍 false → `SetSuppressEnabled(false)` → suppression 全程关闭、下载全误报。**修复**：defaultSettings 补 `FsmonitorSettings{SuppressEnabled:true}`。教训见 memory `settings-default-dual-source-trap`。
2. **`IsSuppressed` 只查祖先不查后代 → 作者目录 Create 漏抑制**：`StoreStream` 的 `os.MkdirAll` 创建作者目录发目录 Create 事件，但 Suppress 登记的是文件路径（下级）；目录是文件的祖先，原 `IsSuppressed` 只查"查询路径的祖先"，查不到"登记项的后代" → 作者目录误报。**修复**：`IsSuppressed` 加后代匹配（登记文件→其所有祖先目录查询命中）。

实测验证（workDir `D:\LS`，bilibili 下载+合并+外部改名/目录改名+复原+离线对账+D7开关）：四项全过（下载/合并零误报、外部操作仍检测、复原不自反馈、离线对账不受污染、D7 开关可关）。
