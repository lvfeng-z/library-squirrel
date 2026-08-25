# VERIFY-插件管理页来源标识强化

任务来源：`doc/plan/插件管理页来源标识强化方案.md` 验证锚定节。
阶段1（状态键登记）+ 阶段2（消费点接线）已由子代理完成，`yarn build:dev` 两阶段通过（构建级锚定 ✅，输出结尾 `✓ built in 2.06s/1.92s`）。

## 基建配方

- 测试二进制：`bin/library-squirrel-test.exe`（2026-08-25 09:18 构建；本任务 Go 零改动，直接复用）
- vite：`cd frontend && npx vite --port 9245 --strictPort`（后台）
- 应用：`FRONTEND_DEVSERVER_URL=http://localhost:9245 LS_CDP_PORT=9222 ./bin/library-squirrel-test.exe`（后台）
- 断言工具：`.claude/testing/cdp-eval.mjs` / `cdp-shot.mjs`；截图存 `.claude/testing/shots/`

## 测试清单

库内现状素材（只读 SQL 已核）：id1 pixivSuite(bundled,1)、id2 localImport(local,1)、id3 bilibiliSuite(local,1)、id4 测试插件(bundled,1)。bundled/local 双来源齐备；缺 trusted=false 行 → 由项0 经真实链路造。

| # | 目的 | 前置 | 操作 | 断言 | 状态 |
|---|---|---|---|---|---|
| 0 | 造数：trusted=false 第三方插件 | ASCII 最小纯 UI 插件 zip（id=com.lvfeng.test.sourcemark） | AI：`pluginInstallFromPath(zip, false)` | 返回 source=local/trusted=false；SQL 行存在 | ✅ |
| 1 | 表格来源列：绿/灰二分 | 项0；导航插件管理页 | AI：DOM 断言 + 截图 | bundled 行「官方」绿 #67c23a；local 行「本地」灰 #909399；无裸枚举文本 | ✅ |
| 2 | 信任列：true 绿 / 非 true 橙 | 同上 | AI：DOM 断言 | trusted=1 行「已信任」绿；fixture 行「未信任」橙 #e6a23c；全部 `.status-tag` 类 | ✅ |
| 3 | 免责提示：存在第三方时显示 | 同上 | AI：DOM 断言 | 提示行可见，文案含「第三方插件由其作者独立维护」 | ✅（修复后过，见失败记录） |
| 4 | 来源筛选：文案对齐 + 命中正确 | 同上 | AI：UI 点选 + 搜索按钮（真实链路） | 选项 官方/本地/网络/市场；官方过滤恰 2 行 bundled、本地过滤恰 3 行 local | ✅ |
| 5 | 状态抽屉：来源/信任 StatusTag | 键盘链路（Enter）开行下拉→状态 | AI：DOM 断言 | pixivSuite 抽屉：官方绿/已信任绿 StatusTag；fixture 抽屉：本地灰/未信任橙 StatusTag | ✅ |
| 6 | 查看对话框：标题 + 来源/信任行 | 行下拉→查看 | AI：DOM 断言 | 标题「插件」；表单含「来源」「信任」行（StatusTag 官方绿/已信任绿） | ✅ |
| 7 | StatusPalette：plugin 类目 6 键 | 导航 `#/statusPalette` | AI：DOM 断言 + 截图 | plugin-bundled/local/url/marketplace/unverified/trusted 6 键齐 | ✅ |
| 8 | 兼容级：非 true（含 NULL）→ 未信任 | 项0 行（false） | AI：DOM + 类型核对 | false 行显「未信任」✅；`PluginDTO.Trusted *bool`（dto/plugin_dto.go:26）——NULL→`null`/false→`false` 均标量，`=== true` 严格判断两形态同落未信任，无 `{Valid,Bool}` 对象序列化风险 | ✅ |
| 9 | 清理：fixture 真实链路回收 | 项0-8 完成 | AI：`pluginUnInstall` | 卸载成功、插件目录无残留、UI 列表复原为原 4 行；DB 留软卸载行（uninstalled=1）——**与真实卸载语义一致**（清单原措辞「无行」过严，卸载本为标记式，禁直写库故以软卸载为终态） | ✅ |

跳过项：构建级验证（阶段1/2 已锚定）；live-test 修复改动后补跑 `yarn build:dev` 通过（`✓ built in 1.81s`）。

## 执行记录

- 项1/2（一次采集）：5 行表格文本与 `.status-tag` 计算色全中——TestSourceMark(本地灰/未信任橙)、测试插件(官方绿/已信任绿)、bilibiliSuite(本地灰/已信任绿)、localImport(本地灰/已信任绿)、pixivSuite(官方绿/已信任绿)。
- 项4：UI 全链路（点选来源下拉→点搜索按钮→后端过滤）。「官方」→[测试插件,pixivSuite]；「本地」→[TestSourceMark,bilibiliSuite,localImport]；清空过滤→5 行复原。
- 项5：el-dropdown 合成 hover 不可触发，改键盘链路（focus 触发器 + Enter）稳定复现。抽屉内 PixivTaskHandler/PixivSiteBrowser 能力 tag 为蓝色 status-tag（能力展示，非本任务范围）；「在线」el-tag 为运行时状态，方案范围外。
- 项6：首次误点开「插件设置」对话框（PluginSettingDialog），关后精确点「查看」命中 FormDialog 标题插槽「插件」（PluginDialog.vue:72）。
- 项9 终态 SQL：原 4 行 source/trusted 不变；fixture 行 uninstalled=1；`plugin/com.lvfeng.test.sourcemark` 目录已删；临时 zip/plugin.json 已清理。
- 进程回收：9245/9222 无 LISTENING、无 library-squirrel 进程（仅 TIME_WAIT 尾巴）。

## 失败与修复记录

### 失败1（项3）：免责提示不渲染——hasThirdPartyPlugin 数据源恒空

- 现象：`.plugin-disclaimer` 与文案全文均不在 DOM（表格/信任列等其他改动均已生效，排除 HMR 未热更）。
- 根因：computed 读 `pluginPage.value.data`，但 SearchTable（`frontend/src/components/common/SearchTable.vue:133-137`）只把 `dataCount`/`pageCount` 回写 v-model:page，**行数据留在组件内部 `data` ref**——父级 `pluginPage.data` 永远为空数组。
- 修复（PluginManage.vue）：新增 `tableRows` ref，`queryPage` 每次查询写入 `response.data.data`（数据漏斗单点）；`hasThirdPartyPlugin` 改读 `tableRows`。修复后提示行渲染、文案全对。
- **连带发现存量缺陷并同修**：`declinedPlugins`（原 147-151 行，检查更新功能的「已跳过更新」告知数据源）同根因恒空——`hasPendingEntry` 消费它致「已跳过更新」子区块永不显示（forcedList/errorList 来自 store 不受影响）。同改为读 `tableRows`，与本次修复同一模式、同一文件、同两行改动。**此为存量 bug，非本方案引入**，随本修复一并落地（终审可否决）。

## 结论

**通过**。方案验证锚定全项命中：

- 构建级：三段 build:dev 全过（阶段1/阶段2/修复后补跑）。
- 实机级：四消费点（表格来源列、信任列、状态抽屉、查看对话框）渲染正确；来源筛选文案对齐且命中正确；StatusPalette plugin 类目 6 键齐全。证据截图：`shots/plugin-manage-source-mark.png`、`shots/status-drawer-pixiv.png`、`shots/status-palette-plugin.png`。
- 兼容级：`Trusted *bool` 标量序列化 + 严格 `=== true` 判断，false/NULL 同落「未信任」（false 已实机命中）。
- 残余风险：①「在线/离线」运行时 el-tag 仍用 EP type 色（方案范围外，STATUS_TOKEN_USAGE 违例存量）；②软卸载 fixture 行留库（uninstalled=1，与真实卸载语义一致）；③附带修复的「已跳过更新」告知路径未实机复验（其触发需捆绑插件存在已跳过 buildId 的真实数据；修复为数据源接线同模式改动，风险低）。
- 方案风险3（表格宽度）实机观察：来源列 90 + 信任列 90 后表格正常渲染无异常换行，横向滚动未恶化（截图可核）。
