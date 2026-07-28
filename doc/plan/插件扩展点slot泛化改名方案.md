# 插件扩展点 slot 正名为「前端扩展」方案（后端 + SDK + 前端连带）

> **2026-07-28 修订**：推翻原「决策2 后端枚举二分」，改为「7 种类型平级 + 统称正名为 FrontendExtension（前端扩展）」。命名强调「前端扩展」而非笼统的 extension——因后端顶层早有 `model.Extension`/`ExtensionType` 笼统抽象（taskHandler/siteBrowser/slot 三类并列），`slot` 这一类实质是「面向前端 UI 的扩展」，正名为 FrontendExtension 才名副其实。前端 SlotRegistryStore/HandlerRegistryStore 二分（主动注入型 vs 被动响应型）合理保留，不进后端类型系统。

## 审查摘要

**关键声明**（抽查锚点）：
- 声明1：后端顶层早有笼统扩展抽象（指跨 taskHandler/siteBrowser/slot 三类的通用扩展容器 `model.Extension`，与本次正名的「前端扩展」不同层）——`model.ExtensionType`（三类并列）`backend/base/model/extension.go:4,8,10,12`、`model.ExtensionMetadata`（三类共用元数据）`:16`、`model.Extension[T]` 泛型容器 `:26`。**「笼统的 extension」已存在于此层**，本次不应在具体类型层重复造一个笼统 extension。
- 声明2：`ExtensionTypeSlot = "slot"`（extension.go:12）是顶层三大类之一；其下 `SlotType`（`backend/base/slot.go:10`）承载 7 个具体类型 embed/view/replaceView/menu/siteBrowserList/dialog/resourceViewer（slot.go:14-26），**resourceViewer 与其余 6 种平级**。
- 声明3：7 种类型走**单一统一管道**，后端从不区分 slot/handler——`SlotConfig` flat 14 字段（slot.go:44-59）、`parseSlotContent` switch 7 个 case（`app.go:472-546`，resourceViewer 是普通 case `:533`）、单一 `SlotRegistry` 管全部（`slot_registry.go:14,38`）、单一 `SlotPusher` 推全部（`wails_pusher.go:16`）、单一 `GetAllSlots` 查全部（`slot_handler.go:43`）。
- 声明4：**slot/handler 二分仅存在于前端消费层**——`SlotRegistryStore` 6 桶（`frontend/src/store/SlotRegistryStore.ts:54-59`）管主动注入型，`HandlerRegistryStore` 1 桶（`HandlerRegistryStore.ts:12`）管被动响应型 resourceViewer；分流发生在 `registerSlotByType` 按 `slot.type` 路由（`useSlotSyncListener.ts:446-465`，resourceViewer 走 `:448` 进 HandlerRegistry）。
- 声明5：resourceViewer 在后端**唯一**特殊点仅 `resourceType 非空`校验（app.go:539-541）；而 embed.position/replaceView.target 的非空校验反在前端（useSlotSyncListener.ts:58,102）——**前后端校验位置不一致**（与分类无关，附带修正）。
- 声明6：既有命名 bug——`ExtensionMetadata.ID` 的 json tag 为 `slotId`（extension.go:18），但它是三类共用的笼统元数据，taskHandler/siteBrowser 的 ID 也被序列化为 `slotId`。
- 声明7：SDK slot 符号集中 `library-squirrel-sdk/dto/plugin_types.go`（Slots 字段/SlotDeclaration + **5** 个具体 `XxxSlotContent`：Embed/Panel/View/Menu/SiteBrowserList）；其中 `PanelSlotContent` 主程序 `SlotType` 枚举与 `parseSlotContent` 均无对应 case（孤立死类型，本次删除，见决策7）；proto/gRPC 完全无 slot（slot 仅 plugin.json 解析、不走 gRPC），改名不涉及 proto 重生成；数据库无 slot 持久化（无 migration）。
- 声明8：错误变量已泛化（`ErrExtensionNotFound`/`ErrExtensionAlreadyExists`，`backend/plugin/extension/errors.go`），无需改。
- 声明9：plugin.json 的 JSON schema（`extensions.slots`/`slotType`/`slotId`）是对外契约，现有 pixiv/local/bilibili + 测试插件均使用。

**已定决策**（用户 2026-07-28 确认）：
- **决策0（核心·新）**：统称性 `slot` 正名为 **`FrontendExtension`（前端扩展）**——对应顶层 `ExtensionTypeSlot` 这一大类（与 taskHandler/siteBrowser 并列），因其承载的 7 种类型全是面向前端 UI 的扩展。**不在具体类型层造一个笼统 `extension`**（与顶层 `model.Extension` 重复）。
- **决策1（改）**：plugin.json JSON schema 改且字段名体现「前端扩展」——`extensions.slots`→`extensions.frontendExtensions`、`slotType`→`kind`、`slotId`（运行时 id）→`frontendExtensionId`。现有 4 插件 plugin.json 同步改。
- **决策2（推翻原方案）**：~~`SlotTypeResourceViewer` 剥离到 `HandlerKindResourceViewer`、后端枚举二分~~。改为：**7 种类型平级**，统一 `FrontendExtensionKind`（Embed/View/ReplaceView/Menu/SiteBrowserList/Dialog/ResourceViewer），resourceViewer 是其中之一，不剥离、不分组。slot/handler 二分是**前端消费契约**的分类（前端两个 store），**不进后端类型系统**。
- **决策3（保留·改名）**：`SlotConfig` 单一结构体泛化为 `FrontendExtensionConfig`（flat 平铺，不拆 Slot/Handler 两结构体）。共通字段 6（Metadata/Kind/Content/ContentType/Order/Props）+ 类型特定字段（Title/Icon/Position/Target/ViewId/ExtensionId/ResourceType/Children）全部 flat。
- **决策4（改）**：事件名 `slot-register`/`slot-unregister`/`slot-batch-register`→`frontend-extension-register`/`frontend-extension-unregister`/`frontend-extension-batch-unregister`（顺便修正 `batch-register` 名实不符——实为批量注销）。
- **决策5（改）**：文件改名 `backend/base/slot.go`→`frontend_extension.go`、`slot_registry.go`→`registry.go`、`slot_handler.go`→`handler.go`（`wails_pusher.go` 保留）。
- **决策6（新·附带）**：校验位置对齐——`parseFrontendExtensionContent` 统一校验各类定位键非空（embed.position/replaceView.target/menu.viewId/siteBrowserList.extensionId/resourceViewer.resourceType），消除「resourceViewer 在后端校验、embed/replaceView 在前端校验」的不一致（声明5）。该决策为校验**收紧**（新增非空校验，非纯改名），见风险8。
- **决策7（新）**：SDK 死代码清理——`PanelSlotContent`（主程序 `SlotType` 枚举与 `parseSlotContent` 均无 panel case，孤立死类型）删除（DEAD_CODE_CLEANUP）。
- **前端二分保留**（用户确认合理）：SlotRegistryStore（主动注入型 6 种）+ HandlerRegistryStore（被动响应型 resourceViewer）不动。前端「slot」词汇 = 主动注入型类别名（前端侧成立，因前端真分两 store）；后端不含 slot/handler 词汇。

**已定命名细节**（用户 2026-07-28 确认，均采纳推荐）：
- 命名1：JSON `extensions.slots` 字段名 → `frontendExtensions`（与 Go `PluginExtensions.FrontendExtensions` 一致）。
- 命名2：`FrontendExtensionResponse` 运行时 id `slotId` → `frontendExtensionId`（与类型名 FrontendExtension 三层一致）。**前端影响面大**：binding 重生成只改 TS 类型定义，前端代码中 ~20 处 `config.slotId`/`slot.slotId` 字段访问与对象字面量（`useSlotSyncListener.ts`/`usePluginRouter.ts`/`useBuiltinMenus.ts` 等）需手改。
- 命名3：事件名前缀 → `frontend-extension-*`（与统称一致）。
- 命名4：content struct 去类别后缀 → `EmbedContent`/`ViewContent`/`ReplaceViewContent`/`MenuContent`/`SiteBrowserListContent`/`DialogContent`/`ResourceViewerContent`（7 种平级，后端无 slot 类别概念，Slot 后缀成无根词）。
- 命名5：顶层 `ExtensionMetadata.ID` json tag `slotId`（`backend/base/model/extension.go:18`，三类共用元数据的名实不符 bug）→ 本次一并修正为 `extensionId`。审查实测：taskHandler/siteBrowser 经自有 IPC DTO 隔离，前端几乎不直接读 `ExtensionMetadata.slotId`（影响小）；核查重点是该 json tag 在后端三类扩展序列化路径的所有读写处。

**自曝风险**：
- 风险1：跨仓库（主程序 + SDK + 前端 + 4 插件）编译断链，需原子发布（任一未跟上则编译失败）。
- 风险2：事件名改破坏前后端运行时通信，必须同次发版前后端同步（主程序 Emit + 前端 Events.On）。
- 风险3：改动量大（统称改名 ~40 符号 + 顶层 ExtensionTypeSlot 正名 + 校验对齐 + 9 .go + SDK + 前端连带 + 4 插件 plugin.json），机械改名易遗漏——需 grep 兜底 + 三端编译验证。
- 风险4：JSON schema 不向后兼容（决策1），4 插件 plugin.json 必须同步改 `slots`→`frontendExtensions`/`slotType`→`kind`，否则后端解析失败（无 `frontendExtensions` 段 → 插件无扩展点）。
- 风险5：命名2（`slotId`→`frontendExtensionId`）是真实范围蔓延点——前端 ~20 处 `slotId` 字段访问需手改（见命名2）；命名5（`ExtensionMetadata.ID` json）对 taskHandler/siteBrowser 前端影响实测较小（经自有 DTO 隔离），核查重点是后端序列化读写处。
- 风险6：SDK 的 DTO 与主程序不一致——缺 `ReplaceViewSlotContent`/`DialogSlotContent`/`ResourceViewerSlotContent`（主程序有），多 `PanelSlotContent`（主程序无，死类型，决策7 删除）。缺的 3 个是否顺便补齐？（独立决策，不阻塞改名）
- 风险8（决策6 引入）：校验对齐为校验**收紧**——新增后端非空校验后，现有插件若声明了缺 position/target/viewId/extensionId 的脏数据，改名后会在后端 parse 阶段报错（原本仅前端 convert 抛错）。执行前 grep 4 插件 plugin.json 确认无脏声明。
- 风险7：历史 plan 文档（`doc/plan/插件slotType重构方案.md`/`slot-config-refactor-plan.md`）用旧名，改名后描述过时——加废弃标注，不重写。

---

## 一、背景与目标

### 1.1 命名论证（本次修订的核心依据）

后端扩展点实际是**三层结构**（本次调研确认）：

| 层次 | 符号 | 范围 | 命名现状 |
|---|---|---|---|
| **顶层（笼统扩展）** | `model.ExtensionType`/`ExtensionMetadata`/`Extension[T]` | taskHandler / siteBrowser / **slot** 三类并列 | 已用笼统 `Extension`（准确） |
| **中间层（一类扩展）** | `ExtensionTypeSlot`（= "slot"） | 该类承载 7 种具体类型 | 用 `slot`（名实不符：含 resourceViewer，且非「插槽」语义） |
| **具体层（具体类型）** | `SlotType`（embed/view/.../resourceViewer） | 7 种 | 用 `slot`（resourceViewer 并非插槽） |

**论证结论**（回答「两种扩展要素是否相同 / 现有模式能否统一承载」）：
1. **要素几乎完全相同**：声明字段、生命周期（注册/注销/批量注销）、存储 key、事件推送、IPC 查询——7 种类型 100% 同构（声明3）。唯一差异是前端「主动拉取 vs 按 resourceType 被动查找」的消费契约，而这种差异在 slot 内部 6 种之间本就存在（view→路由、menu→侧栏、embed→按 position 查）。
2. **现有「基于类型分发不同元数据」模式已无差别统一承载两者**：`SlotConfig` flat 14 字段 + `parseSlotContent` switch 7 case 一直运行良好（声明3）。resourceViewer 只是第 14 个字段 resourceType（slot.go:56），是普通 case（app.go:533）。
3. **故后端无需 slot/handler 二分**：二分是前端消费契约的分类（声明4），把它反映到后端类型系统（原决策2）是把消费层概念泄漏到数据层——这正是「重命名未达预期」的根因。

### 1.2 目标

- **中间层正名**：`ExtensionTypeSlot` 这一类（7 种前端 UI 扩展）正名为 **FrontendExtension（前端扩展）**，与顶层 taskHandler/siteBrowser 并列时语义清晰（任务扩展 / 浏览器扩展 / 前端扩展）。
- **具体层平级**：7 种类型平级为 `FrontendExtensionKind`，不二分。
- **前端二分保留**：SlotRegistryStore（主动注入型）+ HandlerRegistryStore（被动响应型）不动——合理的消费契约分层。
- **连带同步**：plugin.json JSON schema、事件名、前端 binding、4 插件 plugin.json、SDK、文档。

## 二、范围

**本次做**：后端（主程序）+ SDK 的 slot→FrontendExtension 正名 + 7 种类型平级（推翻二分）+ 顶层 ExtensionTypeSlot 正名 + 校验对齐（决策6）+ SDK 死代码清理（决策7，PanelSlotContent）+ plugin.json JSON schema 改 + 前端连带必须跟改点 + 4 插件 plugin.json 同步 + 文档同步。

**不做**：
- 前端二分 store 改名/合并（SlotRegistryStore/HandlerRegistryStore 合理保留）。
- 前端自身符号改名（EmbedSlotRenderer/model/slot/ 等名实相符，不动；前端「slot」= 主动注入型类别名，前端侧成立）。
- 历史 plan 文档重写（加废弃标注——风险7）。
- SDK DTO 补齐（独立决策——风险6）。

## 三、命名原则

| 类别 | 含义 | 处理 |
|---|---|---|
| **统称性 slot（后端）** | 指该类全部 7 种前端扩展（含 resourceViewer），如 SlotRegistry/SlotConfig/SlotType/SlotResponse | **正名 FrontendExtension**（非笼统 extension——顶层已有） |
| **顶层 ExtensionTypeSlot** | 与 taskHandler/siteBrowser 并列的大类 | **正名 ExtensionTypeFrontendExtension** |
| **具体类型**（7 种） | embed/view/replaceView/menu/siteBrowserList/dialog/resourceViewer | **平级**，`FrontendExtensionKind*`，无 slot/handler 前缀 |
| **前端 slot 词汇** | 主动注入型类别名（SlotRegistryStore 等） | **保留**（前端二分合理，前端侧成立） |
| **前端 handler 词汇** | 被动响应型类别名（HandlerRegistryStore） | **保留** |

## 四、改名映射表

> 以下命名细节均已确认（命名1-5，见审查摘要）。

### 4.1 顶层抽象（`backend/base/model/extension.go`）

| 当前 | 改为 | 说明 |
|---|---|---|
| `ExtensionTypeSlot = "slot"` :12 | `ExtensionTypeFrontendExtension = "frontendExtension"` | 三大类之一正名 |
| `ExtensionMetadata.ID` json `slotId` :18 | json `extensionId` | 三类共用元数据，名实不符修正（命名5）；执行前核查 taskHandler/siteBrowser 前端消费点 |

> 顶层 `ExtensionType`/`ExtensionMetadata`/`Extension[T]` 类型名**不动**（已是笼统抽象，准确）。

### 4.2 后端类型/常量（`backend/base/slot.go` → `frontend_extension.go`）

| 当前 | 改为 | 类别 |
|---|---|---|
| `SlotType`（type string）:10 | `FrontendExtensionKind` | 统称（该类） |
| `SlotConfig`（struct）:44 | `FrontendExtensionConfig`（单一 flat，决策3） | 统称 |
| `SlotConfig.SlotType` :46 | `FrontendExtensionConfig.Kind` | 跟随 |
| `NewSlotConfig()` :62 | `NewFrontendExtensionConfig()`（内 `Metadata.Type` 设 `ExtensionTypeFrontendExtension`） | 统称 |
| `SlotTypeEmbed`="embed" :14 | `FrontendExtensionKindEmbed` | 平级 |
| `SlotTypeView`/`ReplaceView`/`Menu`/`SiteBrowserList`/`Dialog` :16-24 | `FrontendExtensionKindView`/`ReplaceView`/`Menu`/`SiteBrowserList`/`Dialog` | 平级 |
| `SlotTypeResourceViewer`="resourceViewer" :26 | `FrontendExtensionKindResourceViewer`（**不剥离**，平级） | 平级 |

> `FrontendExtensionKind` 枚举值域 7 个**平级**，无 Slot/Handler 分组（推翻原决策2）。

### 4.3 后端扩展点包（`backend/plugin/extension/`）

| 当前 | 改为 | 文件名 |
|---|---|---|
| `SlotRegistry` :14 | `FrontendExtensionRegistry` | `slot_registry.go`→`registry.go` |
| `SlotHandler` :33 | `FrontendExtensionHandler` | `slot_handler.go`→`handler.go` |
| `SlotResponse` :11 | `FrontendExtensionResponse` | 同 `handler.go` |
| `SlotResponse.SlotID`（json `slotId`）:12 | `FrontendExtensionResponse.ID`（json `frontendExtensionId`，命名2） | |
| `SlotConfigToResponse` :53 | `FrontendExtensionConfigToResponse` | |
| `GetAllSlots` :43 | `GetAllFrontendExtensions` | IPC 方法（前端 binding 跟随） |
| `SlotPusher` :16 / `WailsSlotPusher` :29 | `FrontendExtensionPusher` / `WailsFrontendExtensionPusher` | `wails_pusher.go` 保留文件名 |
| `SlotEventType` :4 | `FrontendExtensionEventType` | |
| `SlotEventRegister`="slot-register" :8 | `FrontendExtensionEventRegister`="frontend-extension-register"（决策4） | |
| `SlotEventUnregister` :10 | `FrontendExtensionEventUnregister`="frontend-extension-unregister" | |
| `SlotEventBatchRegister`（名实不符）:12 | `FrontendExtensionEventBatchUnregister`="frontend-extension-batch-unregister"（决策4 修正） | |
| `SlotEventData` :71 / `SlotUnregisterItem` :81 | `FrontendExtensionEventData` / `FrontendExtensionUnregisterItem` | 字段 SlotID→ID、SlotType→Kind、Slots→Items 同步 |

### 4.4 后端 DTO（`backend/base/model/dto/plugin_types.go`）

| 当前 | 改为 |
|---|---|
| `SlotDeclaration`（json `slotType`）:129 | `FrontendExtensionDeclaration`（json `kind`，决策1） |
| `PluginExtensions.Slots []SlotDeclaration`（json `slots`）:88 | `PluginExtensions.FrontendExtensions []FrontendExtensionDeclaration`（json `frontendExtensions`，命名1） |
| `EmbedSlotContent`/`ViewSlotContent`/`ReplaceViewSlotContent`/`MenuSlotContent`/`SiteBrowserListSlotContent`/`DialogSlotContent`/`ResourceViewerSlotContent` | `EmbedContent`/`ViewContent`/`ReplaceViewContent`/`MenuContent`/`SiteBrowserListContent`/`DialogContent`/`ResourceViewerContent`（命名4，去类别后缀，7 种平级） |

> `ResourceViewerSlotContent` → `ResourceViewerContent`（**不再**叫 `ResourceViewerHandlerContent`——推翻决策2，无 handler 剥离）。

### 4.5 后端 status / service / app / main

| 当前 | 改为 | 位置 |
|---|---|---|
| `SlotInfo`（json `slotType`）/ `PluginStatus.Slots`（json `slots`） | `FrontendExtensionInfo`（json `kind`）/ `PluginStatus.FrontendExtensions`（json `frontendExtensions`） | `backend/plugin/status.go:27,13` |
| `SlotMeta` / `GetSlotsByPlugin` | `FrontendExtensionMeta` / `GetFrontendExtensionsByPlugin` | `backend/plugin/service.go:139,123` |
| `parseSlotContent` / `convertSlotChildren` | `parseFrontendExtensionContent`（**含决策6 校验对齐**）/ `convertFrontendExtensionChildren` | `app.go:467,552` |
| `app.SlotRegistry` / `app.SlotHandler`（字段） | `app.FrontendExtensionRegistry` / `app.FrontendExtensionHandler` | `app.go:97,141` |
| `extensionListProviderAdapter.slotRegistry` / `GetSlotsByPlugin` | `frontendExtensionRegistry` / `GetFrontendExtensionsByPlugin` | `app.go:1131,1152` |
| `main.go` service 注册 `app.SlotHandler` | `app.FrontendExtensionHandler` | `main.go:82` |

### 4.6 SDK（`library-squirrel-sdk/dto/plugin_types.go`）

| 当前 | 改为 |
|---|---|
| `SlotDeclaration`（json `slotType`）:101 | `FrontendExtensionDeclaration`（json `kind`） |
| `PluginExtensions.Slots []SlotDeclaration`（json `slots`）:82 | `PluginExtensions.FrontendExtensions []FrontendExtensionDeclaration`（json `frontendExtensions`） |
| `EmbedSlotContent`/`ViewSlotContent`/`MenuSlotContent`/`SiteBrowserListSlotContent` | `EmbedContent`/`ViewContent`/`MenuContent`/`SiteBrowserListContent`（命名4，与主程序对齐） |
| `PanelSlotContent`（主程序无 panel，死类型） | **删除**（决策7，DEAD_CODE_CLEANUP） |

### 4.7 前端连带（必须跟改）

| 当前 | 改为 | 位置 |
|---|---|---|
| `Events.On('slot-register')` | `Events.On('frontend-extension-register')` | `useSlotSyncListener.ts:505` |
| `Events.On('slot-unregister')` | `Events.On('frontend-extension-unregister')` | `:513` |
| `Events.On('slot-batch-register')` | `Events.On('frontend-extension-batch-unregister')` | `:521`（含 `data.slots`→`data.items` 字段名） |
| `SlotResponse` binding 引用 | `FrontendExtensionResponse`（binding 重生成） | `useSlotSyncListener.ts:10` |
| `SlotHandler.GetAllSlots()` 调用 | `FrontendExtensionHandler.GetAllFrontendExtensions()`（binding 重生成） | `:14,497` |
| binding `slothandler.ts` | `frontendextensionhandler.ts`（自动生成） | 整文件 |
| binding `models.ts` 的 `SlotResponse`/`slotId` | `FrontendExtensionResponse`/`frontendExtensionId`（自动生成） | |
| `initSlotSyncListener`/`registerSlotByType`/`unregisterSlotByType` | `initFrontendExtensionSyncListener`/`registerFrontendExtensionByKind`/`unregisterFrontendExtensionByKind`（接收统称 FrontendExtensionResponse，内部分流 slot store/handler store） | `useSlotSyncListener.ts:493,446,470` |
| 前端 ~20 处 `slotId` 字段访问/字面量（`config.slotId`/`slot.slotId`/`{slotId:...}`） | 手改 `frontendExtensionId`（命名2；binding 重生成只改 TS 类型定义，字段访问与字面量需手改） | `useSlotSyncListener.ts`/`usePluginRouter.ts`/`useBuiltinMenus.ts`（grep `slotId` 兜底） |

### 4.8 前端保留（名实相符，不改）

`SlotRegistryStore`、`HandlerRegistryStore`、`EmbedSlotRenderer`、`DialogSlotRenderer`、`DynamicSideMenu`、`model/slot/*`（EmbedSlot/DialogSlot/ViewSlot/ReplaceViewSlot）、`SlotTypes.ts`、`SlotConfigs.ts`、`convertToXxxSlot`、`useBuiltinMenus`。

> 前端「slot」= 主动注入型类别名（SlotRegistryStore 管 6 种主动型），「handler」= 被动响应型类别名（HandlerRegistryStore 管 resourceViewer）——前端二分合理（用户确认），词汇保留。`convertToXxxSlot`/`registerXxxSlot` 等动作函数名保留（它们操作的就是前端 slot 桶）。

### 4.9 现有插件 plugin.json 同步（决策1 不兼容）

4 个插件的 plugin.json 改 `slots`→`frontendExtensions`、`slotType`→`kind`：
- `library-squirrel-plugin-pixiv/plugin.json`（5 slot 声明）
- `library-squirrel-plugin-local/plugin.json`（1 dialog slot）
- `library-squirrel-plugin-bilibili`（plugin.json）
- `library-squirrel-plugin-test/plugin.json`（1 resourceViewer point）

## 五、详细设计要点

### 5.1 JSON schema 改（决策1，不向后兼容）

| 旧 | 新 | 出现位置 |
|---|---|---|
| `extensions.slots` | `extensions.frontendExtensions`（命名1） | 4 插件 plugin.json |
| `slotType`（每声明项） | `kind` | 4 插件 plugin.json |
| `slotId`（FrontendExtensionResponse 运行时 id） | `frontendExtensionId`（命名2） | 后端 Response + 前端 binding 消费 |

理由：开发阶段无外部插件依赖；`frontendExtensions` 与顶层 `extensions.taskHandlers`/`extensions.siteBrowsers` 并列，语义明确（前端扩展子段）；`kind` 与 Go 类型 `FrontendExtensionKind`/字段 `Kind` 三层一致。

### 5.2 枚举平级（推翻原决策2）

`FrontendExtensionKind` 枚举值域 7 个**平级**：

```go
FrontendExtensionKindEmbed          FrontendExtensionKind = "embed"
FrontendExtensionKindView           FrontendExtensionKind = "view"
FrontendExtensionKindReplaceView    FrontendExtensionKind = "replaceView"
FrontendExtensionKindMenu           FrontendExtensionKind = "menu"
FrontendExtensionKindSiteBrowserList FrontendExtensionKind = "siteBrowserList"
FrontendExtensionKindDialog         FrontendExtensionKind = "dialog"
FrontendExtensionKindResourceViewer FrontendExtensionKind = "resourceViewer"  // 与前 6 个平级，不分组
```

`parseFrontendExtensionContent` 的 switch 按 7 个 kind 平级分发到各 `XxxContent`，resourceViewer 是普通 case（无 handler 分组逻辑）。slot/handler 二分仅在前端 `registerFrontendExtensionByKind` 内部按 kind 分流到 SlotRegistryStore/HandlerRegistryStore。

### 5.3 单一 FrontendExtensionConfig（决策3）

`FrontendExtensionConfig` flat 平铺全部字段（共通 6 + 类型特定），字段注释标归属（如 `Position // embed`、`ResourceType // resourceViewer`）。不拆 Slot/Handler 两结构体（handler 独有字段仅 ResourceType，拆分代价不值；YAGNI）。

### 5.4 事件名修正（决策4）

`slot-register`→`frontend-extension-register`、`slot-unregister`→`frontend-extension-unregister`、`slot-batch-register`→`frontend-extension-batch-unregister`（顺便修正名实不符——原 batch-register 实为批量注销）。前端 `Events.On` 同步。

### 5.5 校验位置对齐（决策6·新）

`parseFrontendExtensionContent` 各 case 统一加定位键非空校验，消除「resourceViewer 在后端校验（app.go:539）、embed/replaceView 在前端校验（useSlotSyncListener.ts:58,102）」的不一致：

| kind | 定位键 | 校验位置（现状） | 改后 |
|---|---|---|---|
| embed | position | 前端 | 后端 parseFrontendExtensionContent |
| replaceView | target | 前端 | 后端 |
| menu | viewId | 无 | 后端（叶子项必填） |
| siteBrowserList | extensionId | 无 | 后端 |
| resourceViewer | resourceType | 后端 | 后端（保持） |

前端 `convertToXxxSlot` 的非空校验保留作双保险（不删，前端边界仍抛错给用户清晰提示）。

### 5.6 文件改名（决策5）

`backend/base/slot.go`→`frontend_extension.go`、`slot_registry.go`→`registry.go`、`slot_handler.go`→`handler.go`。`wails_pusher.go` 保留（无 slot 前缀，内容改符号即可）。

## 六、实施顺序

1. **后端类型层**：`slot.go`→`frontend_extension.go`（FrontendExtensionKind/FrontendExtensionConfig + 7 平级常量 + NewFrontendExtensionConfig 设 ExtensionTypeFrontendExtension）。
2. **顶层抽象**：`model/extension.go`（ExtensionTypeSlot→ExtensionTypeFrontendExtension；**先 grep 核查 `slotId` 在 taskHandler/siteBrowser 路径的读写处**，确认后 ExtensionMetadata.ID json `slotId`→`extensionId`，命名5）。
3. **后端扩展点包**：`slot_registry.go`→`registry.go`、`slot_handler.go`→`handler.go`、`wails_pusher.go`（事件名 + Pusher 改名 + SlotResponse→FrontendExtensionResponse + frontendExtensionId）。
4. **后端 DTO**：`dto/plugin_types.go`（SlotDeclaration→FrontendExtensionDeclaration + JSON kind、Slots→FrontendExtensions + JSON frontendExtensions、各 XxxSlotContent→XxxContent）。
5. **后端 status/service/app/main**：SlotInfo→FrontendExtensionInfo、SlotMeta→FrontendExtensionMeta、parseSlotContent→parseFrontendExtensionContent（含决策6 校验）、app 字段、main service 注册。
6. **SDK**：`library-squirrel-sdk/dto/plugin_types.go`（SlotDeclaration→FrontendExtensionDeclaration + JSON kind、Slots→FrontendExtensions + JSON frontendExtensions、XxxSlotContent→XxxContent；**删除 PanelSlotContent**，决策7）。
7. **现有插件 plugin.json**：4 插件改 slots→frontendExtensions、slotType→kind。
8. **前端连带**：事件名 3 处 + `initSlotSyncListener` 等函数名 + binding 重生成（`wails3 generate bindings -ts`）。
9. **文档同步**：`.claude/rules/plugin.md`（数据流 FrontendExtensionDeclaration→FrontendExtensionConfig→FrontendExtensionResponse + JSON schema frontendExtensions/kind + 前端二分框架说明）、`backend/plugin/README.md`、`doc/plugin-dev-guide.md`。
10. **编译 + 验证**：主程序 `go build ./...` + SDK build + 前端 `yarn build:dev` + 测试插件端到端（resourceViewer 仍生效 + slot 渲染器仍生效）。

## 七、验证

- **编译**：主程序 + SDK + 前端三端全绿。
- **slot 扩展点回归**：pixiv test-view/test-embed/test-menu/test-replaceView 等 slot 声明仍正常加载渲染（前端 SlotRegistryStore 桶不变）。
- **handler 扩展点回归**：测试插件的 resourceViewer(article) 仍覆盖内置 ArticleRenderer（前端 HandlerRegistryStore 桶不变）。
- **事件同步**：插件注册/注销仍触发前端 store 更新（事件名改后前端正确监听 frontend-extension-register/unregister/batch-unregister）。
- **插件状态**：`PluginStatusPanel` 正确显示扩展点清单（status.frontendExtensions 字段消费）。
- **校验对齐**（决策6）：embed 缺 position / replaceView 缺 target 的插件声明在后端 parse 阶段即报错（不再拖到前端 convert）。

## 八、风险与回滚

- **跨仓库原子发布**（风险1）：主程序 + SDK + 前端 + 4 插件必须同次发布。建议主程序 + SDK 改完先编译验证，前端 binding 重生成后联调，最后改 4 插件 plugin.json。
- **JSON 不兼容**（风险4）：4 插件 plugin.json 必须同步改，否则后端解析失败（`frontendExtensions` 段缺失 → 插件无扩展点）。改动清单见 4.9。
- **命名2 前端字段访问手改**（风险5）：`slotId`→`frontendExtensionId` 牵动前端 ~20 处字段访问（`useSlotSyncListener`/`usePluginRouter`/`useBuiltinMenus`），binding 重生成只改 TS 类型定义，字段访问需手改；grep `slotId` 兜底。命名5（`ExtensionMetadata.ID` json）对 task/siteBrowser 前端影响小，核查后端序列化读写处即可。
- **机械改名遗漏**（风险3）：每步 grep 兜底（`grep -r "Slot" backend/` 清零除前端保留项），三端编译验证。
- **校验对齐暴露历史脏声明**（风险8）：决策6 新增后端非空校验为行为收紧——现有 4 插件 plugin.json 若有缺 position/target/viewId/extensionId 的脏声明，改名后会在后端 parse 阶段报错（原本仅前端 convert 抛错）。执行前 grep 核查 4 插件 plugin.json。
- **回滚**：改名重构 + 校验收紧 + 死代码清理（无 DB migration；决策6 为校验新增、决策7 为死代码删除，非纯改名），回滚 = revert commit。无数据风险。

## 九、与既有计划/文档的关系

- `doc/plan/作品详情弹窗重构与资源渲染器Handler扩展点方案.md`：本方案是其「决策2 命名泛化暂仅前端，后端泛化留后」的**后端落地**；并修正其隐含的「后端也二分」倾向——本次论证确认后端不二分。
- `doc/plan/插件扩展点命名统一方案-contribution转extension.md`：contribution→extension 内部统一（已执行）。本方案是 slot→FrontendExtension 的正名，方向一致——历史统称 contribution + slot 最终在「前端扩展」这一准确语义上归位（顶层笼统 extension 保留给 model.Extension 抽象）。
- `doc/plan/插件slotType重构方案.md`：定义当前 6 种 slotType 体系（+ resourceViewer 共 7 种）。本方案不改体系本身，只正名承载它的类型/字段/函数（SlotType→FrontendExtensionKind 等）+ JSON schema（slotType→kind）。该文档加废弃标注（类型名已正名，体系内容不变）。
- `.claude/rules/plugin.md`：已采用二分框架描述（前端 SlotRegistryStore/HandlerRegistryStore），但数据流仍用旧类型名（SlotDeclaration→SlotConfig→SlotResponse）+ 旧 JSON（slots/slotType）——本方案实施后同步为 FrontendExtensionDeclaration→FrontendExtensionConfig→FrontendExtensionResponse + frontendExtensions/kind，并补充「后端 7 种平级、前端二分消费」的论证说明。
