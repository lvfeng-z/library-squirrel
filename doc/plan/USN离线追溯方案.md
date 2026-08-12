# USN 离线追溯方案（fsmonitor 节点 C）

## 审查摘要

> 本方案为 task-graph `workdir-file-change-monitor` 谱系下节点 **C（USN 离线追溯）** 的设计 plan。目标：实现 Windows `OfflineChangeProvider`（USN Journal），在软件未运行期间产出**精确**文件变更（带移动配对），替代/增强首版的全量对账。下列关键声明均带代码锚点，未带锚点的"已就位/零改动"论断一律视为未核验。

### 关键声明（带锚点）

1. **离线编排入口已定位**：`backend/fsmonitor/service.go:89 runOfflineReconcile()`，当前硬走 `Scanner.Scan()`（`service.go:91`）全量对账；USN 须在此处插入「先 USN、后对账兜底」的分叉。启动触发点 `service.go:74-78 Start()`。
2. **关联层（Correlator）可复用但有缺口**：USN 产出 `[]FileChange`（`event.go:17`）本可复用 `Correlator.Process()`（`correlator.go:84`）；但 `correlator.go:91-99` 的 switch 仅处理 `ChangeCreate`/`ChangeRemove`，`ChangeMove`（`event.go:11`）走 default 返回 nil——USN 的精确 Move 配对无法透传，**必须补 `ChangeMove` 分支**。
3. **接口与注入点已预留**：`OfflineChangeProvider` 接口 `deps.go:16`、`Deps.OfflineProvider` 字段 `deps.go:37`、`NewPlatformDeps` 的 `case "windows"` 占位 `deps.go:54-57`——零接口改动，仅补实现 + 注入。
4. **白名单已固化为包级变量**：`scanner.go:14-19 scanDirs`（store/resource、store/thumbnail、store/avatar/{local,site}）；USN 产出的路径须用同一白名单过滤，避免 backup/ 等目录噪声。
5. **分平台构建模式可照搬**：`backend/window/titlebar_windows.go:1` 的 `//go:build windows` + `_windows.go` 后缀 + `syscall.NewLazyDLL` 模式，USN 实现走 `usn_windows.go` + 其他平台 stub。
6. **游标存储无现成主程序 KV**：`backend/settings/model.go:4 Settings` 是用户设置结构（工作目录/下载/外观等），塞卷级二进制 USN 游标语义不合；`plugin_storage` 仅插件可用。游标须新存储载体（见待决策 D1）。

### 待决策

- **D1 游标持久化载体**：①新建 `fsmonitor_cursor` 表（推荐）vs ②扩 `settings.json` 加游标段 vs ③独立游标文件。推荐①——随 DB 走、事务安全、随备份走，且游标绑定「卷 UsnJournalID + workDir」，结构化存储天然适配。
- **D2 USN 与对账的关系**：①USN 精确补账 + 对账兜底校验（推荐，互补）vs ②USN 成功即跳过对账（互斥）。推荐①——USN 游标可能因 journal 截断/卷格式化而过期，对账是唯一能捕获「游标失效」的兜底，不可省。
- **D3 `ChangeMove` 处理落点**：①Correlator 增 `ChangeMove` 分支（推荐，运行时/离线统一关联）vs ②USN 层直接产 `SemanticChange` 绕过 Correlator。推荐①——保持单一关联出口，避免两套 Move 语义。
- **D4 FRN 路径解析是否独立基建节点**：父 FRN 逐级回溯 + 缓存是 USN 复杂度核心（见风险 R1），可能值得 `derive` 独立子任务先做。倾向先 PoC 评估体量再定。
- **D5 首次启动无游标的起点语义**：`ChangesSince(nil)` 由实现定起点。①从 `NextUsn`（journal 当前末尾，不报历史，下次起开始追）（推荐）vs ②从 `FirstUsn`（报全部历史变更）。推荐①——首次报全部历史会刷屏且多为陈旧变更，与 USN「增量」定位相悖；首版的离线历史检测交给全量对账。
- **D6 USN 读取中途失败的游标更新位置（不可逆决策）**：分批读取中某批失败时，游标更新到①已成功解析并 dispatch 的位置（推荐）vs ②不更新（整次重来）。推荐①——已处理部分不重报（避免重复 dispatch），未读部分下次续读；须保证「游标更新与 dispatch 同事务」，否则错位致漏报/重报。
- **D7 USN 批次与实时 fsnotify 并发去重**：`Start()`（`service.go:74`）中 `runOfflineReconcile` 与 `startLive` 并发，启动瞬间 USN 离线事件与实时 fsnotify 事件可能重叠同一变更。须在 dispatch 层按 `(Kind, FromPath, ToPath, StoreID)` 去重，USN 批次与实时事件共用同一去重集合，窗口覆盖离线对账运行期。
- **D8 USN 设置开关与权限降级（2026-08-12 用户决策，由 R2 实测触发）**：`settings.fsmonitor.usnEnabled`（默认 false）。`NewPlatformDeps` 注入 USN provider 前检测：① 开关开启 ② 当前进程已提权（Windows token elevated，`windows.OpenCurrentProcessToken` + `IsEleved`）③ `runtime.GOOS=="windows"` 三者皆满足才注入 USN；否则 `OfflineProvider=nil` 降级对账。开关开启但非管理员时记中文日志提示（"USN 已开启，需以管理员运行，本次降级全量对账"），不阻断、不弹窗。

### 自曝风险

- **R1 FRN→全路径解析是 USN 的真正难点**（非 syscall 本身）：FRN（File Reference Number，NTFS 主文件表 MFT 中条目的文件引用号，文件在卷内的稳定标识）；`USN_RECORD` 只给文件名 + 父目录的 `ParentFileReferenceNumber`（父 FRN），**不含全路径**；须从父 FRN 逐级向上解析目录链拼出相对 workDir 的路径。大目录/深嵌套下解析开销与缓存一致性是非平凡问题，可能撑大 C 的实现体量，触发 D4 拆分。
- **R2 USN 读取需管理员权限（2026-08-12 C-0a PoC 实测确认）**：非管理员 GENERIC_READ 打开 `\\.\X:` 卷句柄返回 `ERROR_ACCESS_DENIED(5)`（FILE_READ_ATTRIBUTES 句柄能开但不足以发 FSCTL_QUERY_USN_JOURNAL，返回 `INVALID_FUNCTION(1)`）；以管理员运行同一 PoC，卷句柄打开成功、`FSCTL_QUERY_USN_JOURNAL`（function 61=0x000900F4）成功，返回有效 USN_JOURNAL_DATA（UsnJournalID/NextUsn/MaximumSize）。即**卷级 USN 离线追溯必须管理员权限**。对照实验 `FSCTL_READ_FILE_USN_DATA` 经文件句柄非管理员可成功，佐证机制/解析正确，障碍仅卷句柄权限。PoC 见 `usn-poc/main_windows.go`。
- **R3 journal 截断致游标过期**：NTFS journal 有最大尺寸，高 IO 卷会循环覆盖旧记录；离线期过长时 `StartUSN` 可能已被覆盖，`ChangesSince` 拿不到完整历史。必须靠 D2 的对账兜底捕获此情况（检测到 `StartUSN < NextUsn` 的「游标落后于 journal 起始」即判失效）。
- **R4 USN 记录版本差异**：`USN_RECORD` 有 V2/V3/V4 三版本，结构布局与对齐不同。MFT 文件引用号（FRN）恒为 64 位，三版本差异**不在 FRN 位宽**——V3 相对 V2 主要是 64 位对齐与字段扩展，V4 额外引入 OwnerId/SchemaVersion 等扩展字段（面向 ReFS 等长标识场景）。解析须按版本分支读结构布局，首版按 V2/V3 处理，V4 留兼容占位。
- **R5 路径过滤基准**：USN 是**卷级**事件流（整个 NTFS 卷所有文件变更），非 workDir 级；须按 workDir 路径前缀过滤，且 workDir 跨卷/移卷时 `UsnJournalID` 变化致游标失效——需显式失效重建逻辑。
- **R6 FRN 复用致缓存错映射（潜在数据损坏）**：NTFS 会回收已删文件的 FRN 分配给新建文件；若 FRN→路径缓存未及时清条目，新文件会被错映射到旧路径，关联层据此产错误 Move/修复，可能改错 DB 记录。4.2「靠 journal 序号窗口容忍」须显式落实为：缓存条目绑定（FRN + 最近见到的 USN 序号），删除记录到达时即时清条目，复用窗口靠 journal 单调序保证。C-0 PoC 须实测 FRN 复用频率以校准窗口策略。
- **R7 USN 记录乱序与 rename 配对不连续**：USN_RECORD 不保证严格按提交序对调用方可见，且 rename 的 OLD_NAME/NEW_NAME 两条记录可能不连续、中间夹其他记录；4.1 的「USN 层预先按 FRN 合并配对」须容忍乱序（按 FRN 在整批读出的记录里配对，非相邻也能配），本批未配上的 OLD 按 `ChangeRemove`、NEW 按 `ChangeCreate` 兜底。

---

## 一、背景与目标

### 1.1 C 要解决什么

首版 fsmonitor（task-graph `workdir-file-change-monitor` 谱系根节点 A，已完成）的离线检测三平台统一走 **全量对账**（`ReconciliationScanner`）：比对 workDir 实际文件与 `persistent_store` 记录，产出 Missing/Untracked 差异集，再用指纹配对推断 Move。

对账的根本局限：**它只看「当前状态」，无法区分「本次离线期变的」与「历史遗留不一致」**。例如某文件三个月前就丢了但当时没启动软件，每次启动都重复报告。

USN Journal（Windows NTFS 卷级持久变更日志）记录卷上所有文件增删改/重命名，带时间序与游标，可从上次停止位置续读——能精确还原「软件未运行期间到底发生了什么」，且直接给出 rename 的旧名→新名配对（不依赖指纹）。

### 1.2 目标

> **定位（2026-08-12 用户决策）**：USN 作为**可选设置项**，默认关闭（离线走全量对账，普通用户无感）；用户主动开启时启用精确追溯。但卷级 USN 需管理员权限（R2 实测），故开关开启且当前进程非提权时降级回对账 + 日志提示，不阻断、不要求应用全局提权。

- Windows 平台实现 `OfflineChangeProvider`，基于 USN Journal 续读离线期精确变更。
- 接入 `runOfflineReconcile`，与全量对账互补（D2）。
- 游标跨重启持久化（D1），workDir/卷变化时失效重建。
- 其他平台保持 nil（降级对账），架构零扭曲。

---

## 二、现状（零改动核验）

| 要素 | 位置 | 现状 |
|---|---|---|
| `OfflineChangeProvider` 接口 | `deps.go:16` | 已定义，签名 `ChangesSince(ctx, cursor) ([]FileChange, OfflineCursor, error)` |
| `OfflineCursor` 类型 | `event.go:25` | `[]byte`，不透明 |
| `Deps.OfflineProvider` 字段 | `deps.go:37` | 存在，nil = 降级 |
| `NewPlatformDeps` windows 分支 | `deps.go:54-57` | 占位空实现，待注入 |
| 离线编排入口 | `service.go:89` | `runOfflineReconcile` 硬走对账 |
| 关联层 Move 缺口 | `correlator.go:91-99` | `ChangeMove` 未处理 |

> 接口层零改动；改动集中在「新增 USN 实现 + 游标存储 + 编排分叉 + Correlator 补 Move 分支」。

---

## 三、技术选型

### 3.1 USN 访问方式：syscall 直读（非库）

Go 生态**无成熟维护的 USN 库**（现有仅为 2014 年 WIP gist）。采用 `golang.org/x/sys/windows` 直接 syscall，与项目既有模式（`titlebar_windows.go:12 syscall.NewLazyDLL`）一致：

- `CreateFile` 打开卷根 `\\.\C:`（卷句柄）
- `DeviceIoControl(FSCTL_QUERY_USN_JOURNAL)` → 拿 `USN_JOURNAL_DATA`（含 `UsnJournalID`、`FirstUsn`、`NextUsn`）
- `DeviceIoControl(FSCTL_READ_USN_JOURNAL)` 带 `READ_USN_JOURNAL_DATA`（含 `StartUsn` 游标 + `ReasonMask`）→ 返回记录缓冲区
- 步进缓冲区解析 `USN_RECORD` 链（按 `RecordLength` 偏移）

### 3.2 游标持久化：新表 `fsmonitor_cursor`（D1 推荐）

```
fsmonitor_cursor
  id            (BaseEntity)
  journal_id    BIGINT    -- UsnJournalID，标识卷 journal 实例（卷格式化后变）
  start_usn     BIGINT    -- 下次续读起点
  work_dir      TEXT      -- 绑定的 workDir 绝对路径（游标与目录绑定）
  updated_at    BIGINT    -- 最近一次成功续读时间
```

- key 语义：`(journal_id, work_dir)` 唯一；workDir 切换或卷变化（journal_id 变）自动新建行，旧行自然失效。
- 续读成功后事务内更新 `start_usn`；与对账/修复共享 DB 连接，遵循 `dbFromCtx` 事务规约（database.md）。
- migration 注册到 `backend/migration/migrate.go`。

> 否决 settings.json：卷级二进制语义游标混入用户设置结构，且 settings 走 SettingsHandler 非事务路径，与「续读即落盘」的原子性要求不符。

---

## 四、USN 数据流与路径解析（核心难点 R1）

### 4.1 记录→FileChange 映射

`USN_RECORD.Reason` 掩码映射到 `FileChange.Kind`（`event.go:6`）：

| USN Reason | 映射处理 | 说明 |
|---|---|---|
| `USN_REASON_FILE_CREATE` | `ChangeCreate` | 新文件/目录 |
| `USN_REASON_FILE_DELETE` | `ChangeRemove` | 删除 |
| `RENAME_OLD_NAME` + `RENAME_NEW_NAME` | **USN 层预合并为单条 `ChangeMove`**（Path=旧名全路径, ToPath=新名全路径） | 见下方「rename 预合并」 |
| 数据修改类（`USN_REASON_DATA_*`） | 不产出 | 首版只追路径类变更（与主方案决策3「内容修改场景延后」一致；内容修改属 task-tree 节点 D） |

**rename 预合并**（解决审查 🟡 矛盾）：USN 一个 rename 产生 OLD_NAME + NEW_NAME 两条记录，二者按相同 FRN 关联。USN 层在产出 FileChange 前，**在整批读出的记录里按 FRN 配对** OLD/NEW，合并为单条 `ChangeMove`（旧路径→新路径已配对）。这样下游 Correlator 的 `ChangeMove` 分支拿到的就是配对好的事件，**不依赖 `correlator.go:55` 那个仅 5 秒的 `recentMoves` 去重窗口**（该窗口为运行时 fsnotify 设计，离线分批/跨重启读取时会断配对——OLD 先到误报 Delete、NEW 后到无法配对）。配对容忍乱序（R7）；本批未配上的 OLD 按 `ChangeRemove`、NEW 按 `ChangeCreate` 兜底。

路径过滤：解析出的全路径须命中 `scanDirs` 白名单（`scanner.go:14-19`），白名单外丢弃。

### 4.2 FRN→全路径解析（R1）

`USN_RECORD` 仅含 `FileName`（单级文件名）+ `ParentFileReferenceNumber`（父目录 FRN）。拼相对 workDir 全路径须逐级向上解析父目录链：

```
方案B（推荐）：FRN→路径缓存 + 增量维护
  启动时遍历 workDir 子树一次，建立 FRN → 相对路径 映射（缓存）
  USN 事件到达时：ParentFRN 查缓存得父目录相对路径，拼 FileName
  缓存随 USN 事件增量更新（rename/delete 同步改缓存）
  缓存未命中（父目录不在 workDir 子树）→ 事件丢弃（workDir 外变更）
```

- 缓存载体：内存 map（FRN → 相对路径），进程内有效；不落盘（每次启动重建成本可接受，workDir 子树有限）。
- 一致性：USN rename 事件同步更新缓存条目；FRN 复用（NTFS 回收 FRN）靠 journal 序号窗口容忍。
- workDir 外的父目录无需解析（直接判定为外部变更丢弃），避开「解析到卷根」的全链回溯开销。

> 若 PoC 发现缓存一致性/重建开销超预期（大库 workDir 子树文件极多），按 D4 `derive` 独立基建节点。

---

## 五、与现有离线编排集成

### 5.1 编排分叉（`runOfflineReconcile` 改造）

```
runOfflineReconcile:
  1. 若 deps.OfflineProvider != nil:
     a. 读 fsmonitor_cursor (journal_id, work_dir) 得 StartUsn 游标
     b. OfflineProvider.ChangesSince(cursor) → []FileChange + next cursor
     c. 检测游标失效（StartUsn < journal.FirstUsn，见 R3）→ 标记 USN 不可用，转 2
     d. 每个 FileChange 经 Correlator.Process → SemanticChange → dispatchSemanticChange
     e. 续读成功 → 事务更新 fsmonitor_cursor.start_usn
  2. 始终跑 Scanner.Scan 对账（D2 兜底）：
     - USN 已报告的变更，对账结果去重（同路径不重复 dispatch）
     - USN 未覆盖的（游标失效/USN 降级/历史遗留）由对账补齐
```

去重键：`(SemanticChange.Kind, FromPath, ToPath, StoreID)`，USN 批次完成后构建已报告集合，对账阶段命中即跳过。

### 5.2 Correlator 补 `ChangeMove` 分支（D3）

`correlator.go:91-99` switch 增分支：

```go
case ChangeMove:
    // USN rename：旧路径→新路径已配对，查 DB 旧路径记录拿 StoreID，直接产 SemanticMove
    // （不依赖指纹，比 Create/Remove 指纹配对更精确）
    old, _ := c.storeReader.GetByFilePathComplete(ctx, ev.Path)  // ev.Path = 旧路径
    if old == nil { return nil }  // DB 无旧记录，非本库文件 rename，不报告
    return &SemanticChange{Kind: SemanticMove, FromPath: ev.Path, ToPath: ev.ToPath, StoreID: old.ID, DetectedAt: now}
```

> 运行时不走此分支（fsnotify Windows 对 rename 只发 Create 新名事件、不发旧名事件，见 `workdir-file-change-monitor/TREE.md`「决策与约束·Windows fsnotify rename 限制」）；此分支专为 USN 离线 Move 服务，放在 Correlator 保持单一关联出口。

### 5.3 产品约束：pending 为快照，不持续追踪（用户决策 2026-08-12）

dispatch 入队 `RepairManager` 的待修复项（pending）一旦生成即**快照**，软件不持续追踪该文件后续外部变更、不更新 pending 路径。若用户搁置修复期间文件被再次操作（如又移动），导致后续点修复时源/目标状态已变而修复失败，视为**合理可接受**——据此降低实现复杂度。

落实点：
- **修复层**（`RepairManager.Confirm`）：复原/同步时若文件状态已变（源不在/目标被占），返回失败错误并中文提示，不静默。
- **对账去重**（全量对账无记忆，须显式处理）：`scanner.go:33 Scan` 只比对当前状态、不感知 pending 队列，同一 store 被二次操作后下次对账会按当前状态重新报告（旧 pending X→Y 之外又出 X→Z）。`RepairManager` 须按 StoreID 作废旧 pending / 合并为最新一条，避免用户看到自相矛盾的多条修复项。USN 增量流侧天然好处理（增量事件可过滤 pending store 的新变更）。

---

## 六、降级路径（守住 A 的「功能降级非架构缺陷」约束）

```
OfflineProvider 构造失败（卷句柄拿不到/非 NTFS/权限不足 R2） → 不注入(nil)，离线纯对账
游标失效（journal 截断 R3 / 卷格式化 / workDir 移卷 R5）     → 本次跳过 USN，纯对账，重建游标
FRN 路径解析失败（缓存未命中且父目录在 workDir 外）           → 丢弃该事件，对账兜底
USN 读取中途错误                                                → 已读部分 dispatch，剩余转对账
非 Windows 平台                                                 → OfflineProvider 恒 nil
```

每次降级打 `logger.Log.Warnf` 中文日志，便于诊断。

---

## 七、实施阶段（C 的 WBS，不入 TREE）

| 阶段 | 内容 | 依赖 |
|---|---|---|
| **C-0a PoC**（✅ 完成 2026-08-12） | QUERY 卷句柄权限验证（`usn-poc/`）。**结论：function 61=0x000900F4 正确、USN_RECORD 解析正确、R2 确认需管理员** → 触发 D8（设置开关+权限降级），C 重新定位为可选增强 | 无 |
| **C-0.5** | USN 设置开关 + 权限检测降级基建：settings 加 `fsmonitor.usnEnabled`（默认 false）；`NewPlatformDeps` 注入前检测开关+管理员提权；非管理员/开关关 → 不注入（对账）+ 中文日志 | D8 |
| **C-0b PoC** | 管理员环境：FSCTL_READ_USN_JOURNAL 续读一批记录 + 解析，验证 R1 路径解析体量 + R6 FRN 复用频率 → 定 D4 | C-0a |
| **C-1** | USN 记录解析 + Reason→FileChange 映射 + 白名单过滤（`usn_windows.go`） | C-0 |
| **C-2** | FRN→路径缓存（方案B）+ 增量维护 | C-1（若 D4 拆分则独立节点） |
| **C-3** | `fsmonitor_cursor` 表 + migration + 游标读写 repository | D1 |
| **C-4** | `usnProvider` 实现 `OfflineChangeProvider`，`NewPlatformDeps` windows 注入 | C-1/2/3 |
| **C-5** | Correlator 补 `ChangeMove` 分支（D3）+ 单测 | 无 |
| **C-6** | `runOfflineReconcile` 编排分叉 + USN/对账去重 + 游标失效检测 | C-4/5 |
| **C-7** | 端到端验证：离线期改名/移动/删除三场景，USN 精确报告 + 对账兜底 | 全部 |

> C-0a 已完成：R2 确认需管理员（→ D8 设置开关降级）、function 61/解析验证通过。C-0.5（开关+权限基建）可在普通环境先做；C-0b（READ 记录）须在管理员环境做。

---

## 八、验证清单

- [x] C-0 PoC（R2）：**否**——非管理员 ACCESS_DENIED，需管理员（详见自曝风险 R2）
- [ ] C-0 PoC：FRN→路径解析在 workDir 子树的体量可接受（R1/D4 判据）
- [ ] C-0 PoC：实测 FRN 复用频率，校准缓存窗口策略（R6）
- [ ] 游标跨重启续读：停机前 StartUsn → 重启后续读无遗漏无重复
- [ ] 游标失效场景：journal 截断 / 卷格式化 / workDir 移卷 → 自动降级对账 + 重建游标
- [ ] USN/对账去重：同一变更不重复 dispatch
- [ ] 非 Windows 构建：`usn_windows.go` 外有 stub，编译通过，OfflineProvider 恒 nil
