# 站点手动建站退役与 bilibili 插件键补迁移方案

## 审查摘要

**关键声明**
- 声明1　bilibili 站点缺行根因：插件仓 `activate.go:55` 的 AddSite 不带 SiteKey（新 SDK 契约下零值 `""`），宿主 `backend/plugin/extension/plugin_context.go:181` 的 `identity.Lookup` 拒绝空键（`log/server.log` 2026-09-02 09:13:19「站点键 "" 未注册」报错实录）。
- 声明2　bilibili 当前半激活：`activate.go` 在 AddSite 失败后即 `return`，跳过后续 `RegisterTaskHandler`/`RegisterSiteBrowser`（源码顺序已核验）——任务创建与站点浏览器同坏，不止缺站点行。
- 声明3　SDK 注册表已含 bilibili 键：`library-squirrel-sdk/identity/registry.go:46`（`Bilibili{Key: "bilibili"}`）——修复零 SDK 改动。
- 声明4　迁移形态对照（local 已迁，作为模板）：AddSite 改 `{SiteKey: identity.Local.Key}`；Create 应答 `SiteName` 常量改 `SiteKey: identity.Local.Key`（plugin-local 两文件 diff 已核验）。
- 声明5　站点行生产写入方全仓仅三处：插件 AddSite（`backend/plugin/extension/plugin_context.go:194`）、导入/分享回灌 find-or-create（`backend/import/ingest.go:653`）、前端手动 Save（`backend/site/handler.go:25`→`service.go:109`）；grep `entity.NewSite()` 生产调用方仅上述两写入点与 `site_dto.go:31`（ToSiteEntity——Save/Update 路径的 DTO→实体转换器，即第三写入方的组成部分），其余为测试。
- 声明6　`Service.Create` 必须保留：除手动 Save 外，插件 AddSite 经 `SiteSaveProvider.Create` 消费它（`plugin_context.go:33-35` 接口定义、`:200` 调用）——只删 Handler 端点，不删服务方法。
- 声明7　前端 `siteSave` 调用方唯一：`SiteDialog.vue:55`（NEW 分支）；新增按钮 `SiteManage.vue:206-211` + `handleSiteCreateButtonClicked`（:129）。
- 声明8　插件卸载链不清理站点行（`backend/plugin` 卸载链 grep 无站点行操作，只处理插件行标记/备份消费/扩展点注销）→ 站点 Delete 的 GC 职责须保留，不退役。
- 声明9　`site_description` 在插件/导入路径零写入：`plugin_context.go:194-199`、`ingest.go:652-663` 建行只设 Key/Name/Homepage——退役手动建站后该列无任何生产者。
- 声明10　站点管理页无键列、搜索仅按名：`SiteManage.vue:57`（thead 仅 siteName/siteDescription/时间列）、`:212-218`（siteName 搜索）；`backend/site/query.go` 的 SiteQueryDTO 无 siteKey 属性。
- 声明11　bilibili 仓 11 处 `SiteName: bilibiliName`（常量 `bilibili_task_handler.go:20`）分三类，处置各异：**3 处 `TaskCreateResponse`**（Create 应答结构，`:155/:223/:279`——该 proto 消息有 SiteKey 字段，SDK `gen/plugin.pb.go:1187`）→ 替换为键；**2 处 `TaskCreateChildResponse`**（`:121/:204`——该 proto 消息**无** SiteKey 字段，`gen/plugin.pb.go:1084-1175` 区间）→ 删站点字段（local 模板同款：`plugin-local/task_handler.go:133-140` 子应答直接无站点字段，子任务站点身份由父应答携带）；**6 处 PluginData JSON 构造**（`:145` WorkSetEntry、`:182/:251/:294/:304` SiteAuthorEntry、`:317` SiteTagEntry——插件自管格式的 `siteName` 字段，`internal/model/task_plugin_data.go:49-73`）→ 原样保留，常量 `:20` 因此**须存活**（被 6 处构造引用）。
- 声明12　build:plugins 管线覆盖 bilibili（`build/plugins.ps1:45`）；buildId 取 `git describe --tags --always --dirty`——插件仓带未提交改动时构建会产出 `-dirty` buildId 并被官方指纹名单收录，故**插件仓提交必须先于 build:plugins**。
- 声明13　行内跳转主页可行：DataTable 数据单元格统一经 CommonInput 渲染，`type: 'custom'` + config 的 `render: (data, extraData) => VNode`（`frontend/src/model/util/CommonInputConfig.ts:26`）为既定扩展点，custom 类型只读态亦走渲染器（`frontend/src/components/common/CommentInput/CommonInput.vue:158/:161` 的 v-if 分支专门放行 custom）；唤起系统浏览器用 `@wailsio/runtime` 的 `Browser.OpenURL`（`frontend/node_modules/@wailsio/runtime/dist/browser.js:18` 已确认导出——WebView 内直接 window.open 对外部 URL 不可靠）；`type: 'custom'` 先例 4 处（BackupManage/PluginManage/RecycleBin/ShareManage）；local 虚拟站点 homepage 为 null（注册表无主页 + 声明14 实测），render 须空值兜底。
- 声明14　当前库站点表零悬空键行：2026-09-02 SELECT 实测仅 pixiv/local 两行（均注册键，bilibili 行因插件未迁移缺席）——决策3 的清理据此落为启动断言而非存量删改。

**裁决记录（原待决策，2026-09-02 全部落定）**
- 决策1 → **B 全链删除**；且管理页**原描述列位置替换为首页列**（只读链接，行内点击经 `Browser.OpenURL` 唤起系统浏览器——可行性见声明13，设计见『设计』第二节）。
- 决策2 → **跑 `/live-test`**（阶段5 内执行）。
- 决策3 → **清理**：按声明14 当前库零悬空行，落为阶段5 启动断言（站点表全量 site_key ∈ 注册表；如出现违者行 SQL 清除并记录，当前预期零命中）。

**自曝风险**
- 风险1　声明5/7 的「无其他消费方」基于 grep `siteApi.` 与 `entity.NewSite()`，可能漏间接路径（影响低：未发布期）。
- 风险2　声明11 的三类处置依赖逐处上下文判定（应答结构 vs PluginData 构造）——已按 proto 字段有无给出判别依据（`TaskCreateResponse` 有 SiteKey、`TaskCreateChildResponse` 无、Entry 族为 PluginData），实施时逐处核对行号。
- 风险3　决策1=B 牵动 entity/DTO/manifest/bindings/前端多处，漏改点由编译兜底——阶段3(B) 已列全仓 build + export/import/share 包测试；前端按改动文件 0 错误口径（全仓 vue-tsc 基线破损不承诺全仓通过）。
- 风险4　退役后不存在任何手动产生站点行的入口——未来若出现「无插件的手动作品录入」需求，须由该功能自带按键创建，不恢复旧机制（方向性约束）。

## 背景

「统一站点与作品标识规范」（谱系 site-identity-spec，方案 `doc/plan/统一站点与作品标识规范.md`，2026-09-02 复审改判语义键）实施完毕后的收尾聚合。改判落地后暴露两处遗留：

1. **bilibiliSuite 插件未迁移**（本次任务范围只覆盖 local/pixiv 两插件）：AddSite 空 键被拒 + 半激活（声明1/2）。
2. **手动建站口子与新模型冲突**：站点身份已被 SDK identity 注册表中心锁定（编译期内置、未注册键拒绝），站点行的合法生产者只剩「插件激活声明」与「导入/分享回灌」两个；前端手动建站（`Handler.Save`）成为唯一无注册表校验的写入方，产出的行是悬空行（任务路由按 siteKey 匹配插件应答——无插件能产出内容；manifest 校验拒绝未注册键——无外部数据能挂上）。旧模型（按名匹配）下手动建站的「预登记来源」职能已被注册表 + AddSite 整体接管，存在理由消失。**退役范围精确化为创建单项**：编辑（名=展示可改，设计内语义）、删除（声明8 的 GC 职责）、查询（六处选择器纯消费）全部保留。

## 设计

### 一、bilibili 插件键补迁移（零 SDK 改动）

照 plugin-local 的迁移形态（声明4/声明11，三类处置）：

- `activate.go`：`AddSite([]*sdkdto.SiteDTO{{SiteKey: identity.Bilibili.Key}})`，删自报的 `bilibiliSiteName`/`bilibiliHomepage` 局部变量与 SiteDescription/Homepage 字段（注册表权威值由宿主侧落库）；import 增加 identity 包。
- `bilibili_task_handler.go`：
  - **3 处替换**：`TaskCreateResponse` 字面量（`:155/:223/:279`）`SiteName: bilibiliName` → `SiteKey: identity.Bilibili.Key`。
  - **2 处删字段**：`TaskCreateChildResponse` 字面量（`:121/:204`）整行删 `SiteName: bilibiliName`（proto 消息无 SiteKey 字段，子任务站点身份由父应答携带——local 模板同款）。
  - **6 处保留**：PluginData JSON 构造（WorkSetEntry/SiteAuthorEntry/SiteTagEntry）原样不动；常量 `bilibiliName`（`:20`）保留（被 6 处构造引用）。

### 二、手动建站退役（主仓）

**后端**（只删端点，`Service.Create` 保留——声明6）：

- `backend/site/handler.go`：删 `Save` 方法（Wails Bind 端点随方法消失）。
- `backend/site/query.go`：SiteQueryDTO 增加 `SiteKey query.QueryAttribute[string]`（`query:"site_key"`，精确匹配）——支撑键搜索。
- 运行 `wails3 generate bindings -ts` 再生成 bindings（site handler 的 Save 消失、SiteQueryDTO 增字段）。
- 检查 `backend/site` 测试对 Save 端点的引用并同步调整；`ToSiteEntity` 保留（Update 仍用，`handler.go:41`）。

**前端**：

- `frontend/src/views/SiteManage.vue`：删「新增」按钮（:206-211）与 `handleSiteCreateButtonClicked`（:129-133）及 `DialogMode.NEW`（对话框模式枚举的「新增」值）相关分支；thead 增加 `siteKey` 只读列（`defaultDisabled: true` 且不开 `dblclickToEdit`——键不可编辑，行内编辑仅 siteName 保留）；**原描述列位置替换为首页列**——`type: 'custom'` + `render` 返回 `el-link`，点击调 `Browser.OpenURL(url)`（`@wailsio/runtime` browser 服务，声明13）；homepage 空值（local 虚拟站点）render 返回 `-` 不出链接；render 内校验 `http(s)://` 前缀再放行跳转；该列不参与行内编辑（homepage 编辑能力保留在 SiteDialog EDIT 模式，与「名=展示可改」一致）；列序：名称 / 站点键 / 首页 / 修改时间 / 创建时间。工具栏增加键搜索输入（绑定 `siteSearchParams.siteKey.value`，精确）。
- `frontend/src/components/dialogs/SiteDialog.vue`：删 NEW 分支（siteSave 调用 + 键必填校验 `validateForm`）；VIEW/EDIT 模式保留，键字段保留显示（禁改——注册身份可见）。
- `frontend/src/apis/http/wrappers/site.ts`：删 `siteSave`。
- 决策1=B 时同步删描述全链：主仓 entity `SiteDescription` 列、`site_dto.go` ToSiteEntity/DTO 字段、`export/manifest.go` SiteRecord 字段、`export/collector.go` 采集点、导入侧消费点、SiteDialog 描述表单项、SiteManage 描述列；**SDK `proto/plugin.proto` 的 SiteDTO siteDescription 字段（字段3）+ pb 再生成**（主仓 IPC DTO 复用该 proto 类型）。

### 三、文档同步

- `backend/site/README.md`：接口表删 Save 行；「一句话职责」补「站点行创建封闭为插件 AddSite 与导入/分享回灌两径，手动建站已退役」。
- `doc/site-identity-spec.md` 第四节：拒绝语义表备注补「前端手动建站入口已退役——站点行写入方封闭为插件 + 导入两径，注册表校验全覆盖」。
- `.claude/rules/backend.md` Site 概念行：补「手动建站已退役」半句。
- `doc/plugin-dev-guide.md` AddSite 段：确认无「宿主亦提供手动建站」类残留表述（有则删，无则零改动）。

### 四、提交序列（buildId 时序，声明12）

依赖序 + 脏树约束的合并结果：

1. **SDK** 提交（identity 子包 + proto/transport 接线）。
2. **plugin-local / plugin-pixiv / plugin-bilibili** 三插件仓提交（local 补 `.gitignore` 排除 `*.exe` 构建残留；三仓序不限）。
3. `task build:plugins`——此时三插件仓 working tree 全 clean，buildId 无 `-dirty` 后缀，官方指纹名单（`backend/config/locked_config.yaml`）收录干净构建。
4. **主仓** 提交（全部退役改动 + 新 zip + 重写的 locked_config）。主仓根 `config/` 目录为无关残留，不入提交。
5. dev 启动验证：log 无「注册站点失败」、站点表出现 bilibili 行、站点管理页无新增按钮。

## 阶段清单

### 阶段1　bilibili 插件键补迁移

- **目标**：插件仓编译通过；AddSite 携带 `identity.Bilibili.Key`；三类处置全部落位——3 处父应答带键、2 处子应答删站点字段、6 处 PluginData 构造原样。
- **涉及文件**：`E:\code\lvfeng\library-squirrel-plugin-bilibili\activate.go`、`bilibili_task_handler.go`
- **验证命令**：`go build ./... && go vet ./...`（插件仓根；PostToolUse 钩子的 build/ 过滤不适用于插件仓，直接全量）
- **退出标准**：build/vet 零错误；`:155/:223/:279` 三处 `TaskCreateResponse` 携带 `SiteKey: identity.Bilibili.Key`；`:121/:204` 两处子应答无任何站点字段；6 处 PluginData 构造与常量 `bilibiliName`（`:20`）原样。
- **依赖**：无（独立可先行）。
- **模型建议**：主模型（三类处置需逐处上下文判定，判别依据见声明11）。
- **交接摘要**：产出「改动两文件清单 + 三类处置逐处位置确认 + 编译结果」。

### 阶段2　主仓后端：Save 端点退役 + 键查询属性

- **目标**：`Handler.Save` 删除、SiteQueryDTO 增 siteKey、bindings 再生成；`Service.Create` 保留。
- **涉及文件**：`backend/site/handler.go`、`backend/site/query.go`、`frontend/bindings/github.com/library-squirrel/backend/site/*`（再生产物）、`backend/site/service_test.go`（如有 Save 端点引用）
- **验证命令**：`go test ./backend/site/...`；bindings 生成后 `grep -c "Save" frontend/bindings/github.com/library-squirrel/backend/site/handler.ts` 确认 Save 消失。
- **退出标准**：site 包测试全绿；bindings 中无 Save。
- **依赖**：无（与阶段1 可并行）。
- **模型建议**：主模型。
- **交接摘要**：产出「端点删除位置 + query.go 字段 + bindings diff 概要 + 测试结果」。

### 阶段3　前端：建站入口退役 + 键列/首页链接列/键搜索 + 描述全链删除（决策1=B）

- **目标**：站点管理页无新增按钮、键列只读展示、首页列为可点击链接（`Browser.OpenURL`）、键搜索可用；SiteDialog 无 NEW 模式；描述按决策1=B 全链删除（含 SDK proto）。
- **涉及文件**：`frontend/src/views/SiteManage.vue`、`frontend/src/components/dialogs/SiteDialog.vue`、`frontend/src/apis/http/wrappers/site.ts`；（B）`backend/base/model/entity/site.go`、`backend/base/model/dto/site_dto.go`、`backend/export/manifest.go`（SiteRecord 删 SiteDescription 字段）、`backend/export/collector.go`（:682 采集点）、`backend/import/ingest.go` 及消费 SiteRecord 的读取点 + bindings 再生成；**SDK 仓** `proto/plugin.proto`（SiteDTO 删字段3）+ `gen/plugin.pb.go` 再生成
- **验证命令**：`npx vue-tsc --noEmit` 按改动文件 0 错误口径（全仓基线破损，不承诺全仓）；另跑 `go build $(go list ./... | grep -v /build/)`（主仓全仓编译兜底，排除 build/ 平台脚手架）+ `go test ./backend/site/... ./backend/export/... ./backend/import/... ./backend/share/...` + SDK 仓 `go build ./...`
- **退出标准**：改动文件类型零错误；grep `siteSave`/`DialogMode.NEW` 在站点相关文件零命中；grep `SiteDescription` 在主仓与 SDK 仓零命中（manifest 测试字面量同步更新后）；首页列 render 含空值兜底与 `http(s)://` 前缀校验、点击路径调 `Browser.OpenURL`。
- **依赖**：阶段2（bindings 先行）。
- **模型建议**：主模型。
- **交接摘要**：产出「前端改动清单 + 删除入口确认 + 类型检查结果」。

### 阶段4　文档同步

- **目标**：README/spec/rules 三处与退役后机制一致。
- **涉及文件**：`backend/site/README.md`、`doc/site-identity-spec.md`、`.claude/rules/backend.md`、`doc/plugin-dev-guide.md`（零改动则注明）
- **验证命令**：无（文档阶段）。
- **退出标准**：三处主文档含退役表述；plugin-dev-guide 核查结论记录在交接摘要。
- **依赖**：阶段2/3 完成后（表述以最终形态为准）。
- **模型建议**：便宜模型。
- **交接摘要**：产出「各文档改动行号清单」。

### 阶段5　构建、五仓提交、启动验证与实机测试

- **目标**：五仓全部提交完毕，捆绑包为干净构建，启动后 bilibili 站点入表、站点表零悬空键行，`/live-test` 报告落盘。
- **涉及文件**：`resources/bundled-plugins/*.zip`、`backend/config/locked_config.yaml`（build:plugins 产物）；五仓提交按第四节序列
- **验证命令**：`task build:plugins`；dev 启动后查 `log/server.log` 无「注册站点失败」；`SELECT site_key FROM site` 断言全量 ∈ 注册表（声明14 基线仅 pixiv/local，启动后应含 bilibili；违者行 SQL 清除并记录——决策3）
- **退出标准**：五仓工作树 clean（除主仓根 `config/` 残留）；启动验证通过；站点表断言通过；`/live-test` 报告落盘（决策2 已裁决执行，覆盖分享接收/导入键匹配链路、bilibili 站点入表、建站入口消失、首页链接跳转）。
- **依赖**：阶段1-4 全部完成。
- **模型建议**：主模型（走 `/commit` 技能多仓协同；live-test 走 `/live-test` 技能）。
- **交接摘要**：产出「五仓提交哈希清单 + 捆绑包 buildId + 启动验证日志摘录 + 站点表断言结果 + live-test 报告路径」。
