# 插件 URL 监听缺陷修复方案

> 状态：方案已确认（保留独立 API + 修复缺陷），待执行
> 日期：2026-06-25
> 涉及仓库：`library-squirrel-sdk`、`library-squirrel`（主程序）
> 取代：`插件URL监听整合到TaskHandler方案.md`（已废弃，整合会丧失动态监听能力）

## 一、背景与决策

`RegisterUrlListener(contributionId, patterns)` 的 `contributionId` 在语义上等于 TaskHandler id，曾考虑整合进 `RegisterTaskHandler`。但**整合会让 patterns 在注册时固定，丧失运行时动态增删 URL 监听的能力**——而动态监听（用户配置站点、运行时调整监听范围、多 TaskHandler 各自管理）是真实需求，必须保留独立的、可重复调用的 `RegisterUrlListener`。

**决策**：**不整合**。`RegisterUrlListener`/`UnregisterUrlListener` 保留独立 API；只修复当前独立实现的真实缺陷，并适度改善 API 以支持未来的动态/多 TaskHandler 场景。

## 二、要修复的缺陷

### 2.1 卸载/崩溃时 UrlListener 未清理（必修）

插件卸载或子进程崩溃时，URL 监听未被清理，残留 patterns 导致路由命中后 `GetTaskHandler` 失败的脏数据（`task/service.go:545` 仅 warn 后 continue，不致命但是隐患）。

遗漏点：
- `loader.go:217` `UnloadPlugin`：只清 `taskHandlerRegistry.UnregisterAll` + `siteBrowserRegistry.UnregisterAll`。
- `loader.go:305` `handlePluginCrash`：同上。
- `app.go:817` `onUnload`：清了 UnloadPlugin + StaticResource + Slot，漏 UrlListener。

**修复**：在卸载（`onUnload`）与崩溃（`handlePluginCrash`）两条路径都补 UrlListener 清理。

### 2.2 UnregisterUrlListener 无法按 contributionId 精细注销（改善，可选）

当前 `UnregisterUrlListener()` 无参数，`Manager.Unregister` 按 `pluginPublicId` 全量清理——不支持「多 TaskHandler 各自独立注销监听」的动态场景。

**改善**：`UnregisterUrlListener(contributionId string)` 加参数，`Manager.Unregister(pluginPublicId, contributionId)` 支持精细（contributionId 非空）与全量（contributionId 空）两种语义。

### 2.3 contributionId 与 TaskHandler id 对齐（改善，可选）

`RegisterUrlListener` 的 contributionId 必须等于某已注册 TaskHandler id，否则路由失败。当前靠开发者自觉。

**改善**：注册时校验 contributionId 是否为已注册 TaskHandler id，不符则 warn（不阻断，兼容未来可能的非 TaskHandler 监听场景）。

## 三、改动细节

### 3.1 SDK（`library-squirrel-sdk`）

- `dto/context.go`：`UnregisterUrlListener(contributionId string) error`（加参数；破坏性变更，但当前无插件调用它，迁移零成本）。`RegisterUrlListener`、`RegisterTaskHandler` **不变**。
- `proto/plugin.proto`：`UnregisterUrlListener` 的请求从 `Empty` 改为 `UnregisterRequest`（已有 `contributionId` 字段，`UnregisterSiteBrowser` 共用）。
- `gen/*`：重新生成。
- `transport/client.go`：`UnregisterUrlListener` 实现传 contributionId。

### 3.2 主程序（`library-squirrel`）

- **Manager 支持 contributionId 精细清理**（`backend/pluginTaskUrlListener/manager.go`）：
  - `Unregister(pluginPublicId, contributionId string)`：contributionId 非空 → 只清该 contributionId 的 patterns；空 → 全量清该插件（替换当前 `Unregister(pluginPublicId)`）。
- **UrlListenerRegistry 接口**（`backend/plugin/extension/plugin_context.go`）：`UnregisterUrlListener(pluginPublicId, contributionId string)` 加 contributionId。
- **plugin_context.go 实现**：`UnregisterUrlListener(contributionId)` 透传 contributionId。
- **卸载清理（修复 2.1，必修）**：
  - `app.go:817` `onUnload`：补 `app.PluginTaskUrlListenerSvc.UnregisterByPlugin(pluginPublicId)`（全量清插件）。
  - `loader.go:305` `handlePluginCrash`：补 UrlListener 清理——loader 当前不持有 UrlListener Service，需通过依赖注入（如 loader 增加 `urlListenerCleaner` 回调）或让崩溃也走 onUnload 路径。**实施时定具体机制**。
- **contributionId 校验（修复 2.3，可选）**：`urlListenerAdapter.RegisterUrlListener`（`app.go:595`）注册前查 TaskHandlerRegistry 是否有该 contributionId，无则 warn。

### 3.3 插件（pixiv / local）

**无需改动**——`RegisterUrlListener` 签名不变，现有 activate.go 继续工作。`UnregisterUrlListener` 无插件调用，签名变更不影响。

### 3.4 文档

- `doc/plugin-dev-guide.md`：PluginContext API 表的 `UnregisterUrlListener` 更新签名（加 contributionId）；第八节前端通信无关；无需大改。
- `.claude/rules/plugin.md`：「插件 SDK 能力边界」表同步。

## 四、改动清单（按仓库）

### library-squirrel-sdk
- `dto/context.go`：`UnregisterUrlListener` 加 contributionId 参数
- `proto/plugin.proto`：`UnregisterUrlListener` 请求改为 `UnregisterRequest`
- `gen/*`：重新生成
- `transport/client.go`：实现适配

### library-squirrel（主程序）
- `backend/pluginTaskUrlListener/manager.go`：`Unregister(pluginPublicId, contributionId)` 精细/全量
- `backend/plugin/extension/plugin_context.go`：`UnregisterUrlListener(contributionId)` + Registry 接口
- `backend/plugin/extension/loader.go`：`handlePluginCrash` 补 UrlListener 清理
- `app.go`：`onUnload` 补 UrlListener 清理；（可选）`urlListenerAdapter` 加 contributionId 校验

### 文档
- `doc/plugin-dev-guide.md`、`.claude/rules/plugin.md`

## 五、待确认的实施细节

1. **崩溃清理机制**：`handlePluginCrash` 在 loader，不持有 UrlListener Service。方案：①loader 增加清理回调注入；②崩溃也走 onUnload。倾向①（loader 已注入多个 registry，再加一个 cleaner 一致）。
2. **Manager.Unregister 语义**：`Unregister(pluginPublicId, contributionId)` 用 contributionId 空表示全量（一个方法），还是分 `Unregister(pluginPublicId, contributionId)` + `UnregisterByPlugin(pluginPublicId)` 两个方法？倾向前者（参数控制，简单）。
3. **contributionId 校验**：做（warn）还是不做？倾向做（减少开发者对齐错误，成本极低）。

## 六、实施顺序

1. SDK：proto + gen + context.go + client.go（UnregisterUrlListener 加参数）
2. 主程序：Manager + plugin_context + loader 崩溃清理 + app.go onUnload 清理（+ 可选校验）
3. 编译验证主程序 + SDK + pixiv + local（pixiv/local 无需改但需验证兼容）
4. 验证：卸载/崩溃后 URL 不再命中（脏数据消除）
5. 文档同步
6. 提交

## 七、验证

- **卸载清理**：安装 pixiv → 卸载 → 提交 pixiv URL → 不再命中（UrlListener 已清，无 GetTaskHandler 失败 warn）。
- **崩溃清理**：模拟 pixiv 子进程崩溃 → 提交 pixiv URL → 不再命中。
- **精细注销（改善后）**：插件调 `UnregisterUrlListener("main")` → 只清 contributionId=main 的 patterns（多 TaskHandler 场景其他监听不受影响）。
- **兼容**：pixiv/local 现有 `RegisterUrlListener("main", patterns)` 无需改动，正常工作。
