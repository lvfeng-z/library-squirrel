# VERIFY-工作目录外部操作防护

实施计划：`doc/plan/工作目录外部操作防护方案.md` 三阶段 + 集成接线完成后，对以下实机行为做验收级验证。

## 基建配方

- vite: `cd frontend && npx vite --port 9245 --strictPort`（后台）
- 二进制: `go build -o bin/library-squirrel-test.exe .`（CGO，sqlite）
- 应用: `FRONTEND_DEVSERVER_URL=http://localhost:9245 LS_CDP_PORT=9222 ./bin/library-squirrel-test.exe`（后台）
- CDP: `http://127.0.0.1:9222`（Edge/151）
- 数据库: `database/database.db`（WAL），workdir=`D:\LS`
- 进度: 7/7（V-6 跳过，其余全过）

## 测试清单

### V-1 目录保护卡片渲染
- **目的**：设置页工作目录区块的「目录保护」卡片正确渲染（机制/受支持/探测结果/Guide/重新检测按钮）。
- **前置**：应用运行，workdir=`D:\LS` 可写。
- **操作**：AI——导航 `#/settings`，DOM 读取 + 截图。
- **断言**：卡片存在；含「受控文件夹访问」机制文本 + 受支持 tag；探测结果成功态；Guide 文案含「Windows 安全中心」；「重新检测」按钮存在。

### V-2 自动修复开关（关闭态）与策略下拉隐藏
- **目的**：fsmonitor 区块自动修复开关存在；关闭时策略下拉不渲染。
- **前置**：`autoRepairEnabled=false`（默认）。
- **操作**：AI——读取设置页 fsmonitor 区块 DOM。
- **断言**：自动修复开关存在且未开启；策略下拉（store:Move 等）不渲染。

### V-3 开关开启 + 策略下拉渲染 + 持久化
- **目的**：开启自动修复后策略下拉出现（可选项来自 schema，前端不写死）；保存后持久化。
- **前置**：V-2。
- **操作**：AI——wrapper 开启 `autoRepairEnabled` 保存 → 重读 settings 确认；DOM 断言下拉出现。
- **断言**：`settingsGetSettings` 返回 `autoRepairEnabled=true`；下拉渲染 store:Move/store:DirMove/backup:Move 三项；选项 {sync,restore} 与 schema 一致。

### V-4 目录保护探测（wrapper 真实链路）
- **目的**：`GetWorkDirGuardInfo` 后端链路返回正确 Info + 探测结果。
- **前置**：应用运行。
- **操作**：AI——CDP 调 `workdirGuardGetInfo(D:\LS)`。
- **断言**：`probeOk=true`、`platform=windows`、`mechanism=受控文件夹访问`、`supported=true`、guide 非空。

### V-5 live 自动修复——外部移动 store 文件（核心端到端）
- **目的**：autoRepair 开启时，live 外部移动（用户整理移动语义）被自动 sync 处理——DB 记录跟随新路径、前端弹聚合提示、不弹确认框。
- **前置**：`autoRepairEnabled=true`（V-3 已开）；素材 work 176（store 512，`store/resource/test01/[test01]_[...]_IMG_20251028_192432.jpg`）文件在磁盘。
- **操作**：AI——`backup-file-op.mjs` 外部移动该文件到同目录新名 → 观察前端聚合提示 + 日志 → sqlite 断言 → 截图。
- **断言**：① persistent_store 记录 `file_path` 自动更新为新路径（auto-sync 生效）；② 前端出现聚合 ElMessage「已自动处理 N 条外部变更，详见日志」（防抖聚合）；③ 不弹 ChangeConfirmDialog（无待确认项）；④ 日志含自动处理 Infof 行。
- **恢复**：反向外部移动回原路径 → auto-sync 反向恢复记录（终态与初始一致）。

### V-6 offline 不自动（跳过）
- **操作**：跳过。
- **原因**：offline 一律入队人工确认的判定逻辑已由单测锚定（阶段1 auto_repair_test.go）；实机验证需停机重启 + 制造批量变更，成本高且恰会触发「无感知批量失效」风险场景——正是不自动的原因，实机制造无意义。

### V-7 auto 关闭时外部变更仍入确认队列（既有流回归对照）
- **目的**：autoRepair 关闭时，live 外部变更仍走既有确认流（弹确认框，不入队失败）——证明 auto 分支只在其开启时生效，既有流未回归。
- **前置**：`autoRepairEnabled=false`（V-3 验证后关闭恢复）；素材 work 175（store 528，test01 目录）文件在磁盘。
- **操作**：AI——`backup-file-op.mjs` 外部移动 work 175 文件 → 检查确认队列 + DOM。
- **断言**：① persistent_store 记录**未变**（未自动处理）；② 前端出现确认提示/确认框（ChangeConfirmDialog 或待确认通知）；③ 待确认列表含该变更。
- **恢复**：经真实链路确认 `sync`（或反向移回）恢复素材，确认队列清空。

## 结论

### 通过面（6/6 可执行项全过；V-6 跳过）

| 项 | 结果 | 证据摘要 |
|---|---|---|
| V-1 目录保护卡片渲染 | ✅ | 设置页「工作目录」区块渲染：防护机制「受控文件夹访问」+「受支持」tag + 探测通过 + Guide 文案（Windows 安全中心→...→允许应用白名单）+「重新检测」按钮；截图 `shots/guard-settings.png` |
| V-2 开关关闭态 + 下拉隐藏 | ✅ | 刷新后 persisted=false，DOM「自动修复｜关」，策略下拉（资源文件/目录/备份移动）均不渲染 |
| V-3 开关开启 + 下拉渲染 + 持久化 | ✅ | wrapper 开启 → `settingsGetSettings` 返回 `autoRepairEnabled=true`；DOM 渲染 store:Move/store:DirMove/backup:Move 三下拉（值同步路径）；Delete 单选项组合未渲染；选项集与后端 schema 一致 |
| V-4 目录保护探测 | ✅ | `GetWorkDirGuardInfo("D:\LS")` → `probeOk=true`、`platform=windows`、`mechanism=受控文件夹访问`、`supported=true`、guide 非空 |
| V-5 live 自动修复核心端到端 | ✅ | 外部改名 store 512 → ① 记录自动 sync 为 `_auto_repair_moved.jpg`；② 日志「已自动处理：...（动作 sync）」；③ 前端聚合 ElMessage「已自动处理 1 条外部变更，详见日志」；④ 待确认队列空（不入队）。反向改名×2 亦自动跟随，记录+磁盘均复原初始态 |
| V-6 offline 不自动 | ⏭️ 跳过 | offline 一律入队人工确认已由单测锚定（阶段1 auto_repair_test.go）；实机验证需停机重启+制造批量变更，成本高且恰是「不自动」要规避的风险场景 |
| V-7 auto 关闭时既有确认流回归 | ✅ | 外部改名 store 528 → 记录**未变**、日志仅检测无自动、待确认队列含该变更（id=1, domain=0=store, kind=0=Move）；经真实链路 `restore` 确认后文件+记录复原、队列清空 |

### 残余风险

- ElMessage 为瞬时提示（约 3s），首次移动的提示因轮询连接时序未直接捕获；经 MutationObserver 在反向移动时捕获到精确文案，链路确认无虞。
- 设置页本地副本在 wrapper 直接改值后不自动刷新（SPA 组件已挂载）；真实用户流程（开关→保存）不受影响，仅测试路径需 reload。
- offline 不自动的实机端到端（重启后批量变更入队）未实测，依赖单测锚定；如需可另立 live-test 项。

### 素材复原核对

- work 176（store 512）/ work 175（store 528）记录与磁盘路径均复原为初始值；test01 目录无 `_auto_repair`/`_back_to`/`_manual_confirm` 残留；待确认队列空；autoRepairEnabled 复位为默认 false。
