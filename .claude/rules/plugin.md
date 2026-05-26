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
- **两种类型**：运行时插件（Go DLL 子进程）和纯 UI 插件（仅 `plugin.json`）
- **三个扩展点**：TaskHandler、SiteBrowser（运行时）、Slot（通过 `plugin.json` 声明式）
- **插件 SDK**：`github.com/lvfeng-z/library-squirrel-plugin-sdk`（本地 replace 指令）
- **静态资源服务地址**：`http://wails.localhost:{backend-port}/plugin/{id}/{ver}/...`

## 初始化时序

主程序初始化必须按以下顺序执行，否则插件事件通道不可用：

1. `NewApp()` — 创建 App（**不加载插件**）
2. `SetEventEmitter(emitter, onEvent)` — 设置 Wails 事件发射器和前端事件监听函数
3. `LoadPlugins()` — 加载并激活插件（此时事件通道已就绪）

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

每个 slot 声明对应 `backend/base/model/dto/plugin_types.go` 的 `SlotDeclaration` 结构：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 插槽唯一标识（插件内唯一） |
| `name` | string | 显示名称 |
| `slotType` | string | `embed` \| `panel` \| `view` \| `menu` \| `siteBrowserList` |
| `contentType` | string | `precompiled` \| `vueSource` \| `code` \| `html` |
| `content` | object | 根据 contentType 不同而不同（见下方） |
| `position` | string | embed: `topbar`\|`toolbar`\|`statusbar`\|`dialog`；panel: `left-sidebar`\|`right-sidebar`\|`bottom` |
| `contributionId` | string | 关联的 TaskHandler ID（如 `"main"`） |
| `props` | object | 传递给组件的额外属性 |
| `children` | array | 嵌套子 slot（仅 menu 类型） |

### content 字段格式

| contentType | content 格式 |
|-------------|-------------|
| `precompiled` | `{"js": "path/to/file.js", "css": "path/to/style.css"}` |
| `vueSource` | `{"entry": "path/to/Component.vue"}` |
| `html` | `{"html": "path/to/file.html"}` |
| `code` | 无需 content，代码直接在 props 中传递 |

content 中的相对路径会自动解析为 `/plugin/{publicId}/{version}/...` 形式的完整 URL。

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
- `SlotResponse`：`backend/slot/dto.go` — IPC 响应 DTO
- 前端接口：`frontend/src/model/model/interface/SlotConfigs.ts` — 按类型做可辨识联合

## 插件开发规范

- **Slot 注册**：通过 `plugin.json` 的 `extensions.slots` 声明式注册，调用 `RegisterSlot()` 的这种方式已不再被支持
- **静态资源**：在 `extensions.staticResources.directories` 声明可访问目录
- **入口函数**：运行时插件导出 `func Activate(ctx pluginsdk.PluginContext)`
- **PRECOMPILED_OVER_VUESOURCE** (P0): 新组件优先使用 `precompiled` contentType，`vueSource` 需要运行时 SFC 编译开销更大
- **FACTORY_IMPORT_AS** (P0): 预编译组件中禁止使用 `import { X as Y }` 的 `as` 语法（Vite 工厂插件不兼容），需要别名时直接修改变量名
- **LAZY_EMITTER_CLOSURE** (P1): 主程序中引用 `emitter` 必须通过闭包延迟读取，禁止在初始化阶段直接持有 emitter 引用
