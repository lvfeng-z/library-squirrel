# 插件扩展点命名统一方案（contribution → extension）

> 状态：待确认范围后执行
> 日期：2026-06-25
> 涉及仓库：`library-squirrel-sdk`、`library-squirrel`（主程序）、`library-squirrel-plugin-pixiv`、`library-squirrel-plugin-local`、前端

## 一、背景

插件系统扩展点的命名残留了 Electron 版本的 `contribution`，应统一为 `extension`（项目已大量使用 extension：`ExtensionMetadata`、`ExtensionType`、`Extension[T]`、proto message `RegisterExtensionRequest`、目录 `backend/plugin/extension/`）。当前两套命名混用，`contribution` 是历史包袱。

排查结论：`contribution` 分布在 **7 层、约 70 处**（proto 字段 / gen / Go 类型 / Go 字段 / Go 方法参数 / 前端 TS / DB 列 / 文档）。

## 二、命名映射

| 当前 | 统一为 |
|---|---|
| `contributionId`（proto 字段 / JSON key / 参数） | `extensionId` |
| `ContributionID`（Go 字段） | `ExtensionID` |
| `ContributeKey`（Go 字段，值 `"taskHandler"`） | `ExtensionKey` |
| `PluginWithContribution`（Go 类型） | `PluginWithExtension` |
| `pluginContributionId`（JSON / proto Task 字段） | `pluginExtensionId` |
| `PluginContributionID`（Go 字段） | `PluginExtensionID` |
| `plugin_contribution_id`（DB 列） | `plugin_extension_id`（或保留，见决策） |

> `RegisterTaskHandler(id, name, ...)` 的参数名已是中性 `id`，无需改 API（语义统一即可）。

## 三、分步策略

### 第一步：内部统一（无破坏，可立即执行）

改主程序/SDK/前端的**内部**命名，不动对外协议与存储：
- **Go 内部**：局部变量、方法参数、`PluginWithContribution`→`PluginWithExtension` 类型、`ContributionID`/`ContributeKey` 字段、`task_executor.go`/`plugin_host.go`/`task_handler_proxy.go`/`app.go`/`loader.go`/`task service.go` 等。
- **前端手写 TS**：`SlotConfigs.ts`、`SlotRegistryStore.ts`、`useSlotSyncListener.ts`、wrappers、views 的 `contributionId`→`extensionId`。
- **文档**：`plugin.md`、`plugin-dev-guide.md`、ai-assistant。
- **bindings 重生成**（Go 类型改完跑生成器）。

**暂不动**：proto 字段名、`plugin.json` JSON key、DB 列名、SDK 对外 API 参数名（`RegisterUrlListener(contributionId)`）。

效果：内部代码全 extension，但对外（插件 SDK、plugin.json、DB）仍是 contribution——**兼容现有插件，零破坏**。

### 第二步：对外统一（破坏性，主版本升级时）

- **proto 字段** `contributionId` → `extensionId`（保持字段编号不变 → wire 兼容，旧插件二进制可工作，但源码需升级 SDK）。
- **`plugin.json` JSON key** `contributionId` → `extensionId`，slot 解析**过渡期兼容旧 key**（同时接受两者，旧插件 warning）。
- **SDK API 参数名** `RegisterUrlListener(contributionId)` → `(extensionId)`（源码不兼容，插件配合升级）。
- **DB 列名** `plugin_contribution_id` → `plugin_extension_id`（迁移脚本 `ALTER TABLE ... RENAME COLUMN`）。
- pixiv/local 的 plugin.json + activate.go 同步。

## 三、决策点

1. **执行范围**：
   - 仅第一步（内部统一，零破坏，立即可做）
   - 第一步 + 第二步（彻底统一，破坏性，需协调插件升级）
2. **DB 列名**（第二步时）：
   - 改列名 `plugin_extension_id`（一致，需 ALTER TABLE 迁移）
   - 保留列名 `plugin_contribution_id`，仅改 Go 字段名 + gorm tag column 不变（零迁移，但 DB 列名遗留）
3. **plugin.json JSON key**（第二步时）：
   - 直接改 + slot 解析兼容旧 key（过渡期，推荐）
   - 直接改不兼容（强制插件升级）

## 四、改动清单（按层）

### SDK（第二步含 proto/DTO）
- proto：10 处 `contributionId` 字段 + Task.`pluginContributionId`
- gen：3 份重新生成（含 2 份 vendored）
- dto：`context.go`/`providers.go` 参数、`plugin_types.go` slot 字段、`task_dto.go`/`site_browser_dto.go` 字段
- transport：`client.go`/`host.go`/`plugin_server.go`

### 主程序
- `backend/plugin/extension/`：`plugin_context.go`/`loader.go`/`plugin_host.go`/`task_handler_proxy.go`/`task_executor.go`/`task_handler_registry.go`/`slot_handler.go`/`convert.go`
- `backend/pluginTaskUrlListener/`：`PluginWithContribution`→`PluginWithExtension` + 字段（manager/service/handler）
- `backend/task/`：service.go/query.go
- `backend/base/model/entity/task.go`：DB 列（决策 2）
- `backend/base/model/dto/`：task_dto/plugin_types
- `backend/base/slot.go`：SlotConfig.ContributionId
- `app.go`：slot 配置 + urlListenerAdapter

### 前端
- bindings：全量重生成
- 手写：SlotConfigs.ts/SlotRegistryStore.ts/useSlotSyncListener.ts/wrappers(siteBrowser/pluginTaskUrlListener)/views(SiteBrowserManage/TaskManage)

### 插件（第二步）
- pixiv/local 的 plugin.json（`contributionId`→`extensionId`）

### 文档
- plugin.md / plugin-dev-guide.md / ai-assistant / 历史 plan

## 五、风险

- **第一步**：bindings 重生成后前端字段名变（`contributionId`→`extensionId`），手写 TS 需同步（否则前端编译错）。属内部一致性，可控。
- **第二步**：proto 字段名变（源码不兼容，全员升级 SDK）；plugin.json JSON key 变（第三方插件需改，过渡期兼容缓解）；DB 列迁移（数据风险，需备份 + 校验）。

## 六、推荐

**先执行第一步**（内部统一，零破坏，立即清掉大部分 contribution）。第二步作为主版本升级的破坏性变更，择期执行（含 DB 迁移 + 插件协调）。

第一步完成后，对外残留的 contribution 仅限：proto 字段名、plugin.json JSON key、DB 列名、SDK 对外 API 参数名——这些在第二步统一。
