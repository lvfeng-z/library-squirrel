# VERIFY 插件官方身份判定与来源维度拆分·阶段4 实机验证

方案：`doc/plan/插件官方身份判定与来源维度拆分方案.md`（第 9 节阶段4 / 第 10 节验证锚定）
被测代码：工作树未提交改动（阶段1-3 全部就位）
日期：2026-08-25

## 基建配方

- vite dev server：`cd frontend && npx vite --port 9245 --strictPort`（后台）
- 测试二进制：`go build -o bin/library-squirrel-test.exe .`（CGO 开）
- 应用：`FRONTEND_DEVSERVER_URL=http://localhost:9245 LS_CDP_PORT=9222 ./bin/library-squirrel-test.exe`
- 连通自检：`curl -s http://127.0.0.1:9222/json/version`、`node .claude/testing/cdp-eval.mjs 'location.href'`

## 被测名单（locked_config.yaml 编译内嵌）

bilibiliSuite `2acb0ff` / localImport `30c1d20` / pixivSuite `e1a0f00` / test-plugin `477b994`

## 开发库初始态（测试前快照，2026-08-25）

| id | publicId | source | build_id | uninstalled | 说明 |
|---|---|---|---|---|---|
| 1 | com.lvfeng.pixivSuite | bundled | 394e66f-dirty | 0 | 与 zip 名单 e1a0f00 不等 → 启动记 available 待办 |
| 2 | com.lvfeng.localImport | local | e894386-dirty | 0 | T3 观察对象 |
| 3 | com.lvfeng.bilibiliSuite | local | 1d7bd5e-dirty | 0 | T3 观察对象 |
| 4 | com.lvfeng.test-plugin | bundled | test-build-005 | 0 | 与 zip 名单 477b994 不等 → 启动记 available 待办 |
| 5 | com.lvfeng.test.sourcemark | local | NULL | 1 | 历史测试残留 |

注：`official` 列测试前不存在（AutoMigrate 在本测试首次启动加列，存量行 NULL）。

## 测试清单

### T1 手装官方包判官方
- 目的：手动安装捆绑 zip 副本（重新下载形态）经知情同意后，判官方并全 UI 链呈现。
- 前置（AI）：启动应用（自动加列+启动扫描）；`pluginApplyPendingUpgrade('com.lvfeng.test-plugin')` 建 bundled 官方参考态；`pluginUnInstall` 卸载；外部文件操作拷贝 `resources/bundled-plugins/test-plugin.zip` → `.claude/testing/tmp/test-plugin-copy.zip`。
- 操作（AI）：优先拦截 `Call.ByID(645505400)`（文件选择对话框）后点真实「安装」按钮走完整 UI 链（含知情同意弹窗 DOM 断言+点确认）；不可行时降级为页面上下文调 `pluginInstallFromPath(path, true)`（=同意弹窗确认按钮的同一 wrapper 调用）。
- 断言：
  - [ ] a. DB：行 id=4 source=local、source_detail=副本路径、build_id=477b994、official=1、trusted=1
  - [ ] b. 列表行来源列 StatusTag 文本=「官方」且 computed color 为 done tone 绿
  - [ ] c. 状态抽屉：「官方身份」行 plugin-official tag、「安装渠道」行「本地」tag
  - [ ] d. 查看对话框：同 c 两行
  - [ ] e. 官方身份与捆绑行一致（升级后捆绑态 official=1 → 卸载 → 手装后仍 official=1，同 zip 同判定）
  - [ ] f. 免责提示不因它显示：该行 official===true 不满足 `hasThirdPartyPlugin` 条件（页面数据逻辑断言；DOM 层因 id=1/2/3 存在非官方行仍显示——如实记录）

### T2 伪造包反向断言（核心）
- 目的：照抄 buildId 但内容不同的包不可冒充官方（判定不可被包自声明冒充）。
- 前置（AI）：卸载 T1 手装行；解压 test-plugin.zip → plugin.json 原样（id/buildId/version 不动）→ 新增 `forged-marker.txt` → 重打包（bsdtar，条目路径正斜杠）→ `.claude/testing/tmp/test-plugin-forged.zip`。
- 操作（AI）：同 T1 安装链路装伪造包。
- 断言：
  - [ ] a. DB：official=0（false）、source=local、build_id=477b994（照抄成功但内容不等）
  - [ ] b. 列表行来源列灰「本地」（idle tone）
  - [ ] c. 免责提示行显示（DOM 存在）
  - [ ] d. 补充（重启后）：启动扫描 verifyExistingOfficial 对该伪造行 buildId 命中但目录摘要不等 → 不证实，official 维持 0

### T3 存量行启动扫描不纠正（风险5 预期）
- 目的：存量 local 行 buildId 与名单不等 → 官方身份维持未证实（NULL），不被误纠。
- 前置（AI）：首次启动后、任何 T1 改动前抓取。
- 断言：
  - [ ] a. DB id=2/3：official 仍 NULL、source=local、build_id 不变（e894386-dirty/1d7bd5e-dirty）
  - [ ] b. 日志无对这两个 publicId 的「存量插件经内容摘要证实为官方发布产物」
  - [ ] c. 结论说明符合方案风险5（dirty 构建不入名单 → 保守不纠正）

### T4 受限模式行为不变（按 source 渠道判定）
- 目的：restrictedMode 开启时非 bundled 插件（含手装官方包）不激活——官方身份不接安全语义。
- 前置（AI）：T1 完成态（存在 source=local 且 official=1 的行）。
- 操作（AI）：`settingsSaveSettings([{path:'pluginSettings.restrictedMode', value:true}])`（真实链路）→ 重启测试应用 → 采集 → 恢复 false → 重启确认复原。
- 断言：
  - [ ] a. 日志「受限模式启用，跳过非 bundled 插件」覆盖全部非 bundled publicId（含手装官方 test-plugin——official=1 不放行）
  - [ ] b. 「插件加载完成」行「受限模式跳过 N 个」计数与预期一致
  - [ ] c. 恢复 false 重启后无跳过日志；settings.json 终值 false

## 进度

- [x] 清单生成
- [x] 基建启动（vite 9245 + 测试二进制 + CDP 9222）
- [x] T1 / [x] T2 / [x] T3 / [x] T4
- [x] 清理与终态核对
- [x] 结论

## 执行记录

### 基建

- vite `npx vite --port 9245 --strictPort` 后台、`go build -o bin/library-squirrel-test.exe .`、`FRONTEND_DEVSERVER_URL=http://localhost:9245 LS_CDP_PORT=9222` 直启二进制。CDP `/json/version` = Edg/151.0.4129.101，`cdp-eval` 页面上下文可用。
- 首次启动即完成 AutoMigrate 加列（`official` 全行 NULL）与启动扫描：config.yaml 仅声明 test-plugin.zip → 仅 test-plugin 判变（installed `test-build-005` ≠ zip `477b994`）记 available 待办，红点与升级按钮如期出现。

### T1 手装官方包判官方 —— 全部通过

1. `pluginApplyPendingUpgrade('com.lvfeng.test-plugin')`（页面上下文 wrapper，真实升级链）→ DTO 返回 `official: true, source: bundled`；DB：id=4 `source=bundled, build_id=477b994, official=1`（捆绑官方参考态）。
2. `pluginUnInstall` 卸载 → 外部文件操作拷贝官方 zip 至临时区（sha256 与原件逐字节一致：`e36b1951…`）。
3. 手装链路：原生文件选择对话框无法自动驱动——尝试补丁 `@wailsio/runtime` 的 `Call.ByID`（拦截 645505400）失败（`Call` 对象冻结，赋值静默不生效，且触发了一次真实原生弹窗，杀进程重启清除）。**降级方案**：页面上下文调 `pluginInstallFromPath(copyPath, true)` ——与组件知情同意弹窗「确认安装」按钮执行的同一 wrapper 调用（`PluginManage.vue:496`→`installFromPath(packagePath, true)`→`pluginApi.pluginInstallFromPath`）。
4. 知情同意弹窗渲染面：以与组件完全同参（同文案/同 options）调用 `ElMessageBox.confirm` 于真实应用内渲染并 DOM 断言——标题「安装插件」、按钮 取消/确认安装、含中性化文案「此插件非随主程序捆绑分发…」、**无**「来自第三方」身份断言、含「不可逆」风险提示。
5. 断言结果：
   - a ✓ DB：`source=local, source_detail=副本路径, build_id=477b994, official=1, trusted=1`
   - b ✓ 列表行来源列 StatusTag 文本「官方」、computed color `rgb(103, 194, 58)`（done tone 绿）
   - c ✓ 状态抽屉：「官方身份」=官方（绿）、「安装渠道」=本地（灰 `rgb(144,147,153)`）、「信任状态」=已信任
   - d ✓ 查看对话框：官方身份=官方（绿）、安装渠道=本地（灰）、信任=已信任
   - e ✓ 一致性：同 zip 捆绑安装态 official=1 → 手装后 official=1（同内容同判定，渠道两分互不影响）
   - f ✓ 逻辑断言：`pluginQueryPage` 数据中手装行 `official===true` 不满足 `hasThirdPartyPlugin`（`official !== true`）条件；免责行 DOM 当时因 pixivSuite/localImport/bilibiliSuite（official=NULL）显示——归因于其他行，非手装官方行
- 截图：`p4-t1-list-official.png`、`p4-t1-status-drawer.png`、`p4-t1-view-dialog.png`

### T2 伪造包反向断言 —— 全部通过（含重启扫描层）

1. 构造：bsdtar 解压官方 zip → plugin.json 原样（id/buildId/version 不动）→ 新增 `forged-marker.txt` → `tar -a -c -f` 重打包（条目正斜杠、plugin.json 可解析、结构合法）。
2. 卸载 T1 手装行 → 同链路手装伪造包。
3. 断言结果：
   - a ✓ DB：`official=0（false）、source=local、build_id=477b994`（buildId 照抄成功但内容摘要不等，终裁拦截）
   - b ✓ 列表行来源列灰「本地」（idle tone）
   - c ✓ 免责提示行 DOM 显示
   - d ✓ 重启扫描层（R2）：伪造行进入 `verifyExistingOfficial`（official 非 true 且 buildId 命中名单 477b994）→ 对已装目录算摘要与名单不等 → 不证实：无「存量插件经内容摘要证实为官方发布产物」日志、DB official 维持 0
- 截图：`p4-t2-forged-list.png`

### T3 存量行启动扫描不纠正 —— 通过（含扫描路径强化）

- 基础断言（首次启动即采）：DB id=2/3 `official` NULL、`source=local`、build_id 不变（`e894386-dirty`/`1d7bd5e-dirty`）。
- 路径强化（R2）：开发库 config.yaml 原本仅声明 test-plugin.zip（id=1/2/3 根本不进启动扫描）——临时将 local-plugin.zip、bilibili-plugin.zip 追加进 config.yaml 声明重启一次，使存量行真正走过「已装跳过分支 + verifyExistingOfficial」：日志出现「捆绑插件已安装，跳过: com.lvfeng.localImport / com.lvfeng.bilibiliSuite」，无任何「存量插件经内容摘要证实」日志，DB official 维持 NULL。测试后 config.yaml 已还原为单条声明。
- 符合风险5 预期：dirty 构建 buildId 与名单（`30c1d20`/`2acb0ff`）不等 → buildId 门早退、保守不纠正。
- 正向控制说明（如实记录）：「启动扫描证实→置 official=true」的正向路径在实机不可经干净链路触达（唯一安装路径在装时即判 official，无法造出「内容官方+buildId 命中+official≠true」的起始行；直写 DB 属红线），由单测锚定：`TestVerifyExistingOfficialHit`（`backend/plugin/official_test.go:227`）；实机观察到的是其守卫分支行为。

### T4 受限模式行为不变 —— 全部通过

- `settingsSaveSettings([{path:'pluginSettings.restrictedMode', value:true}])`（真实链路）→ 重启测试应用。
- a ✓ 日志三条「受限模式启用，跳过非 bundled 插件」：localImport、bilibiliSuite、**test-plugin（source=local 且 official=1 的手装官方行——官方身份不改变受限模式跳过）**。
- b ✓ 「插件加载完成: 1 个运行时, 0 个纯 UI, 共 5 个; 未信任待激活 0 个, 受限模式跳过 3 个」（仅 bundled pixivSuite 激活）。
- c ✓ 恢复 false（真实链路）→ 重启后四插件全部激活、受限模式跳过 0 个；`config/settings.json` 终值 `restrictedMode: false`。
- 受限模式期间 DB 行不受影响（id=4 手装官方行原样，仅激活被跳过）。

### 清理与终态

- 测试安装的伪造行已卸载，R3 重启后 InstallBundled 全新安装恢复：id=4 = `source=bundled, build_id=477b994, official=1, version=1.0.0`（健康捆绑官方态；终态列表行显示绿「官方」）。
- 留痕（不可逆/合理变更）：
  - id=4 行从测试前 `buildId=test-build-005 / v1.0.4 / official NULL` 变为 `477b994 / v1.0.0 / official=1`——等价于用户批准了启动扫描记出的捆绑升级（测试前每次启动本就记 available 待办）；原 test-build-005 zip 已被阶段2 管线重建取代，旧态不可恢复。
  - backup 表新增安装包备份行若干（installCore 每次安装建备份、换版直清旧备份，净增 1-2 行），引用链自洽。
  - id=1/2/3/5 与测试前逐字段一致（除新加 official 列 = NULL）；无新增 plugin 行（全程复用 id=4）。
- config.yaml 已还原；settings.json 已还原；临时区（副本 zip、伪造 zip、解压目录、清理脚本）已删除。
- 测试进程全杀（应用、vite 9245 监听者），端口 9245 已释放，tasklist 无 library-squirrel 残留。

## 失败与修复记录

- 进程清理脚本首版误杀 9245 监听者（vite 本体）→ 应用 FATAL「unable to connect to frontend server」→ 修脚本（node 清理改 `--kill-vite` 显式开关）重启恢复。属测试基建问题，不影响被测功能结论。
- `Call.ByID` 补丁失败（runtime `Call` 对象冻结）触发一次真实原生文件对话框 → 杀进程重启清除；安装链路按预案降级为 wrapper 直调（同 consent 确认按钮调用）。

## 结论

**四项全过（含伪造包不命中的反向断言）：**

| 项 | 结果 | 关键证据 |
|---|---|---|
| T1 手装官方包判官方 | 通过 | DB official=1 + 列表绿「官方」+ 抽屉/对话框 官方身份=官方·安装渠道=本地 |
| T2 伪造包反向断言 | 通过 | 照抄 buildId 内容不同 → official=0、灰「本地」、免责显示；重启扫描层同样不证实 |
| T3 存量行不纠正 | 通过 | id=2/3 official 维持 NULL（buildId 门），扫描路径经 config 强化实测覆盖 |
| T4 受限模式不变 | 通过 | official=1 的手装行仍被跳过（渠道判定），恢复后全量激活 |

方案验证锚定（第 10 节）实机级四项全部满足。残余说明：a) 启动扫描正向证实路径由单测锚定（实机不可达，见 T3 记录）；b) 知情同意弹窗文案为渲染面验证（同参调用）+ 源码核，未走原生对话框选择文件的完整 UI 点击链（原生对话框不可自动驱动，安装链路为 consent 确认按钮的同一 wrapper 调用）。

