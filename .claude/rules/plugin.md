---
description: "插件系统架构与规则，适用于修改 plugin/ 目录或插件相关代码时加载"
globs:
  - "plugin/**"
  - "**/plugin.json"
  - "app.go"
  - "main.go"
  - "backend/plugin/**"
  - "frontend/src/composables/useSlotSyncListener.ts"
---

# 插件系统架构与规则

## 插件系统概述
- 插件位于 `plugin/`，由 `app.go` 的 `loadInstalledPlugins()` 加载
- **两种类型**：运行时插件（Go 子进程）和纯 UI 插件（仅 `plugin.json`）
- **扩展点**：TaskHandler、SiteBrowser（运行时注册）；view/replaceView/embed/dialog/menu/siteBrowserList（通过 `plugin.json` 声明式 Slot）
- **插件 SDK**：`github.com/lvfeng-z/library-squirrel-sdk`（本地 replace 指令）
- **静态资源服务地址**：`http://wails.localhost:{backend-port}/plugin/{id}/{ver}/...`

## 初始化时序

主程序初始化必须按以下顺序执行，否则插件事件通道不可用：

1. `NewApp()` — 创建 App（**不加载插件**）
2. `SetEventEmitter(emitter, onEvent)` — 设置 Wails 事件发射器和前端事件监听函数
3. `LoadPlugins()` — 加载并激活插件（此时事件通道已就绪）

> `LoadPlugins()` 须在主窗口 native handle 就绪后调用（当前在 `WindowRuntimeReady` 回调内执行）。激活插件时需随 `Activate` 传递主窗口 HWND（`mainHWND`），供插件 `OpenWindow` 作为 owner 置顶显示；而窗口 native handle 在 `application.Run()` 事件循环启动后才创建，故 `LoadPlugins` 延迟到窗口就绪后。`InstallBundledPlugins()`（仅写 DB）不受此约束，仍在 Run 前。

`wailsFrontendEventProvider` 使用闭包（`emitterFunc`/`onEventFunc`）延迟读取，避免初始化顺序问题。禁止在 `SetEventEmitter` 之前调用 `LoadPlugins()`。

## plugin.json 结构

### 顶层字段
```json
{
  "id": "com.example.plugin_uuid",
  "name": "插件名称",
  "version": "1.0.0",
  "entryFile": "plugin.exe",
  "activation": {"type": 1},
  "extensions": { ... }
}
```

### extensions.slots[] 声明

每个 slot 声明包含通用字段和按 slotType 区分的 `content` 配置：

**通用字段：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 插槽唯一标识（插件内唯一） |
| `name` | string | 是 | 显示名称 |
| `description` | string | 否 | 描述 |
| `slotType` | string | 是 | `embed` \| `view` \| `replaceView` \| `dialog` \| `menu` \| `siteBrowserList` |
| `order` | number | 否 | 排序权重 |
| `content` | object | 是 | 按 slotType 区分的专属配置（见下方） |

### content 按 slotType 的格式

**embed：** 嵌入组件到主程序的具名插槽位

| 字段 | 类型 | 说明 |
|------|------|------|
| `contentType` | string | `precompiled` \| `vueSource` \| `code` \| `html` |
| `source` | object/string | 组件源（格式见 source 格式表） |
| `position` | string | 主程序定义的具名插槽位标识（如 `work.toolbar`） |
| `props` | object | 传递给组件的额外属性（可选） |

```json
{"contentType": "precompiled", "source": {"js": "views/btn.js", "css": "views/btn.css"}, "position": "work.toolbar"}
```

> 主程序在 Vue 模板中用 `<EmbedSlotRenderer position="work.toolbar">默认内容</EmbedSlotRenderer>` 暴露插槽位。有插件声明该位则渲染插件组件，无则渲染默认内容。

**view：** 新增独立路由页面

| 字段 | 类型 | 说明 |
|------|------|------|
| `contentType` | string | 同 embed |
| `source` | object/string | 同 embed |
| `title` | string | 页面标题（可选） |
| `props` | object | 传递给组件的额外属性（可选） |

**replaceView：** 替换主程序已有页面（覆盖路由 component）

| 字段 | 类型 | 说明 |
|------|------|------|
| `contentType` | string | 同 embed |
| `source` | object/string | 同 embed |
| `target` | string | 主程序路由 name（覆盖目标，见路由清单） |
| `props` | object | 传递给组件的额外属性（可选） |

```json
{"contentType": "precompiled", "source": {"js": "views/work.js", "css": "views/work.css"}, "target": "work-manage"}
```

**dialog：** 弹窗（模态层）

| 字段 | 类型 | 说明 |
|------|------|------|
| `contentType` | string | 同 embed |
| `source` | object/string | 同 embed |
| `props` | object | 传递给组件的额外属性（可选） |

```json
{"contentType": "precompiled", "source": {"js": "views/browser.js", "css": "views/style.css"}, "title": "浏览器"}
```

**menu：** 侧边栏菜单项，点击跳转到关联的 view

| 字段 | 类型 | 说明 |
|------|------|------|
| `icon` | string | 图标相对路径（可选，自动解析为完整 URL） |
| `viewId` | string | 点击后跳转到的 view slot ID |
| `children` | array | 子菜单项（可选，递归 slot 声明） |

```json
{"icon": "assets/icon.png", "viewId": "browser-view"}
```

**siteBrowserList：** 站点浏览器入口卡片

| 字段 | 类型 | 说明 |
|------|------|------|
| `icon` | string | 图标相对路径（自动解析为完整 URL） |
| `extensionId` | string | 关联的 `siteBrowsers` 扩展点 ID |

```json
{"icon": "assets/icon.png", "extensionId": "main"}
```

### source 格式（contentType 对应）

| contentType | source 格式 |
|-------------|------------|
| `precompiled` | `{"js": "path/to/file.js", "css": "path/to/style.css"}` |
| `vueSource` | `{"vue": "path/to/Component.vue", "js": "path/to/file.js", "css": "path/to/style.css"}`（`js`/`css` 为可选预编译缓存，存在则跳过运行时编译） |
| `html` | `{"html": "path/to/file.html"}` |
| `code` | JavaScript 代码字符串（行内 JS，通过 `new Function` 执行，不注入 Vue/WailsRuntime 依赖） |

source 中的相对路径会自动解析为 `/plugin/{publicId}/{version}/...` 形式的完整 URL。

### extensions.settings[] 声明

插件可通过 `extensions.settings` 声明用户可配置项，主程序据此在插件管理页渲染设置表单；用户编辑后存入 `plugin_storage`，插件用 `GetValue(key)` 读取。

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `key` | string | 是 | 设置项键（插件内唯一） |
| `type` | string | 是 | `string` \| `integer` \| `boolean` \| `select` |
| `title` | string | 是 | 显示标题 |
| `description` | string | 否 | 描述 |
| `default` | string | 否 | 默认值（值统一以 string 存储） |
| `encrypted` | bool | 否 | 是否加密存储（敏感项，渲染为密码框，保存走 `SetValueEncrypted`） |
| `group` | string | 否 | 分组名（同组用分隔标题展示） |
| `order` | number | 否 | 组内排序 |
| `options` | array | 否 | `select` 的选项：`[{label, value}]` |
| `min` / `max` | number | 否 | `integer` 的范围 |

```json
"settings": [
  {"key": "downloadQuality", "type": "select", "title": "下载质量", "group": "下载", "default": "original",
   "options": [{"label":"原图","value":"original"},{"label":"压缩","value":"compressed"}]}
]
```

## 预编译组件模式

### 构建流程

插件前端组件通过 Vite 构建为**工厂函数**格式：

1. Vite 配置中使用 `componentFactoryPlugin()` 后处理输出
2. 该插件替换 `import { ... } from 'vue'` 为 `const { ... } = __VUE__;`
3. 替换 `import { ... } from '@wailsio/runtime'` 为 `const { ... } = __WAILS_RUNTIME__;`
4. **import `as` 语法**必须转换为解构冒号语法（如 `import { X as Y }` → `const { X: Y }`），否则运行时报 SyntaxError
5. 将 `export default` 替换为 `return`，整体包裹为 `export default function(__VUE__, __WAILS_RUNTIME__) { ... }`

### 加载流程

主程序 `useSlotSyncListener.ts` 的 `loadCompiledComponent()` 负责加载：

```typescript
const module = await import(jsUrl)
const component = module.default(Vue, WailsRuntime)  // 调用工厂函数注入依赖
return defineComponent(component)
```

CSS 通过 `<link>` 标签注入 DOM。工厂函数确保插件组件使用主程序的 Vue 实例，避免多实例问题。

### 构建脚本规范

插件 `build.ps1` 必须按以下顺序执行：
1. `yarn install && yarn build`（前端编译）
2. `go build`（Go 编译）
3. 打包到 `dist/` 目录

## 插件↔前端事件通信

### 事件通道

- **插件→前端**：`ctx.PublishToFrontend(topic, data)` → 主程序通过 Wails `Emit` 转发 → 前端 `Events.On(topic)` 接收
- **前端→插件**：前端 `Events.Emit(topic, data)` → 主程序通过 Wails `Event.On` 拦截 → 调用 `pushCh([]byte)` → 插件 `ctx.SubscribeFrontend(topic)` 返回的 channel 接收

### 协议约定

- 事件 topic 格式：`plugin:{plugin-name}:{feature}:{action}`（如 `plugin:local-import:classify:request`）
- 数据格式：JSON 序列化的 `[]byte`
- 插件端使用阻塞 channel 等待响应（如 `pendingCh`），需设置超时防止永久阻塞

## Slot 数据流

```
plugin.json → SlotDeclaration(解析 DTO) → SlotConfig(领域模型) → SlotResponse(IPC DTO) → TypeScript 接口(前端)
```

- `SlotDeclaration`：`backend/base/model/dto/plugin_types.go` — plugin.json 直接映射
- `SlotConfig`：`backend/base/slot.go` — 运行时模型，SlotType/ContentType 为枚举常量
- `SlotResponse`：`backend/plugin/extension/slot_handler.go` — IPC 响应 DTO
- 前端接口：`frontend/src/model/interface/SlotConfigs.ts` — 按类型做可辨识联合
- 前端 Slot 类型：`frontend/src/model/slot/` — ViewSlot/EmbedSlot/DialogSlot/ReplaceViewSlot

## 插件 SDK 能力边界

插件通过 `PluginContext`（gRPC `HostService`）访问宿主能力，不限于扩展点注册。所有可用能力：

| 类别 | SDK 方法 | 说明 |
|------|----------|------|
| 扩展点注册 | `RegisterTaskHandler`、`RegisterSiteBrowser`、`UnregisterSiteBrowser` | 注册运行时扩展点 |
| 数据写入 | `AddSite` | 向主库插入站点记录 |
| 数据查询 | `GetWorkSetBySiteWorkSetId` | 按站点作品集 ID 查询是否已存在 |
| 插件自存信息 | `GetValue` / `SetValue` / `SetValueEncrypted` / `DeleteValue` / `GetAllValues` | 统一 KV 持久化（`plugin_storage` 单表）；明文项直接读写，加密项 `SetValueEncrypted` 存密文、读取自动解密。取代旧的 `GetPluginData/SetPluginData` 与加密存储 |
| 任务触发 | `CreateTask` | 向主程序提交 URL 创建任务（路由到匹配的插件） |
| URL 监听 | `RegisterUrlListener` / `UnregisterUrlListener(extensionId)` | 注册 URL 匹配模式，匹配时路由到本插件的 TaskHandler；`UnregisterUrlListener` 按 extensionId 精细注销（空则清该插件全部，用于卸载） |
| 前端通信 | `PublishToFrontend` / `SubscribeFrontend` / `UnsubscribeFrontend` | 与前端双向 pub/sub |
| 原生窗口 | `window.OpenWindow`（仅 Windows） | 创建 WebView2 弹窗，支持 JS 执行和导航拦截 |
| 文件路径 | `GetPluginRoot` | 获取插件目录路径（相对或绝对） |
| 窗口句柄 | `GetMainWindowHandle` | 获取主窗口 Win32 HWND |
| 日志 | `Infof` / `Debugf` / `Warnf` / `Errorf` | 写入主程序日志系统 |

宿主端通过 `HostDeps`（`backend/plugin/extension/loader.go`）注入各 Provider 适配器，将插件 RPC 调用桥接到对应的 Service/Registry。

## 插件开发规范

- **Slot 注册**：通过 `plugin.json` 的 `extensions.slots` 声明式注册，调用 `RegisterSlot()` 的这种方式已不再被支持
- **静态资源**：在 `extensions.staticResources.directories` 声明可访问目录
- **入口函数**：运行时插件导出 `func Activate(ctx pluginsdk.PluginContext)`
- **PRECOMPILED_OVER_VUESOURCE** (P0): 新组件优先使用 `precompiled` contentType，`vueSource` 需要运行时 SFC 编译开销更大
- **FACTORY_IMPORT_AS** (P0): 预编译组件中禁止使用 `import { X as Y }` 的 `as` 语法（Vite 工厂插件不兼容），需要别名时直接修改变量名
- **LAZY_EMITTER_CLOSURE** (P1): 主程序中引用 `emitter` 必须通过闭包延迟读取，禁止在初始化阶段直接持有 emitter 引用
- **THEME_TOKEN_CONFORMANCE** (P1): 插件 UI 样式遵循主题令牌契约（`doc/plugin-theme-tokens.md`），使用主程序的 `var(--app-*)` 令牌，禁止硬编码颜色与 `var(--el-*)`，使插件界面自动跟随用户主题；需感知当前主题 id 时用 `window.__PLUGIN_CTX__.theme.getCurrent()`。
