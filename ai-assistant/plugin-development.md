# LibrarySquirrel 插件系统开发指南

## 概述

插件系统是 LibrarySquirrel 扩展性的核心，允许通过插件支持不同的站点（如 pixiv、bilibili）。本指南介绍插件的架构和开发方法。

## 核心概念

### 插件目录结构

```
plugin/package/
└── [插件名]/
    ├── package.json           # 插件元信息
    ├── index.js               # 插件入口文件
    └── views/                 # 插件视图（可选）
```

### 插件清单 (package.json)

```json
{
  "name": "pixiv",
  "version": "1.0.0",
  "author": "LibrarySquirrel",
  "librarySquirrel": {
    "activationType": "url"
  }
}
```

### 激活类型

| 类型     | 说明                            |
| -------- | ------------------------------- |
| `url`    | URL 激活，通过解析特定 URL 触发 |
| `manual` | 手动激活，用户主动启用          |
| `auto`   | 自动激活，启动时加载            |

## 插件接口

### BasePlugin 接口

重构后的 `BasePlugin` 接口简化为只包含 `pluginId`：

```typescript
export interface BasePlugin {
  pluginId: number
}
```

### 插件实例 (PluginInstance)

```typescript
interface PluginInstance {
  plugin: Plugin                    # 插件实体
  contributions?: ContributionMap   # 贡献点映射
  entryPoint?: PluginEntryPoint      # 入口点
}
```

## 贡献点 (Contributions)

插件可以贡献以下功能：

### 1. TaskHandler - 任务处理器

处理资源下载任务：

```typescript
class MyTaskHandler implements TaskHandler {
  async handle(params: TaskHandlerParams): Promise<TaskResult> {
    # 下载逻辑
    return { success: true, works: [] }
  }
}
```

### 2. SiteBrowser - 站点浏览器

提供站点内容浏览能力：

```typescript
class MySiteBrowser implements SiteBrowser {
  async search(keyword: string): Promise<SearchResult[]> {
    # 搜索逻辑
  }
}
```

> **⚠️ 重要区分：站点浏览器注册 vs 站点浏览器列表插槽**
>
> 这两个概念容易被混淆，请注意区分：
>
> | 概念                   | API                               | 用途                                                                   |
> | ---------------------- | --------------------------------- | ---------------------------------------------------------------------- |
> | **站点浏览器**         | `app.registerSiteBrowser()`       | 注册站点浏览器本身（业务数据），使系统知道这个插件提供了站点浏览器功能 |
> | **站点浏览器列表插槽** | `slots.registerSiteBrowserSlot()` | 在"站点浏览"页面的卡片列表中添加一个入口（UI），让用户能看到并点击进入 |
>
> **两者必须同时注册**，站点浏览器功能才能完整工作：
>
> 1. 先用 `app.registerSiteBrowser()` 注册站点浏览器
> 2. 再用 `slots.registerSiteBrowserSlot()` 在 UI 中添加入口
>
> ```javascript
> // 激活函数中
> async activate() {
>     // 1. 注册站点浏览器（业务功能）
>     await this.context.app.registerSiteBrowser({
>         contributionId: 'main',
>         pluginPublicId: this.manifest.id,
>         name: 'PixivSiteBrowser',
>     })
>
>     // 2. 注册站点浏览器列表插槽（UI入口）
>     this.context.slots.registerSiteBrowserSlot({
>         slotId: `siteBrowser-${this.manifest.id}-main`,
>         contributionId: 'main',
>         pluginPublicId: this.manifest.id,
>         name: 'PixivSiteBrowser',
>         imagePath: '/plugins/icon.png',
>         order: 0
>     })
> }
> ```

## 插件管理器

`PluginManager` 负责插件的加载、缓存和生命周期管理：

```typescript
import PluginManager from '@main/plugin/PluginManager'

const pluginManager = new PluginManager()

# 加载插件
const instance = await pluginManager.load(pluginId)

# 卸载插件
await pluginManager.unload(pluginId)
```

## 插件上下文 (PluginContext)

插件运行时上下文，提供服务访问：

```typescript
interface PluginContext {
  pluginId: number
  localAuthorService: LocalAuthorService
  localTagService: LocalTagService
  siteService: SiteService
  workSetService: WorkSetService
  secureStorageService: SecureStorageService
}
```

## 插槽系统 (Slots)

插件可以通过插槽贡献 UI：

### 视图插槽 (ViewSlot)

```typescript
const viewSlot: ViewSlotConfig = {
  id: 'my-plugin-view',
  name: '我的插件视图',
  icon: 'Document',
  order: 10,
  component: () => import('./views/MyView.vue')
}
```

### 面板插槽 (PanelSlot)

```typescript
const panelSlot: PanelSlotConfig = {
  id: 'my-plugin-panel',
  name: '我的插件面板',
  icon: 'Panel',
  component: () => import('./views/MyPanel.vue')
}
```

### 嵌入插槽 (EmbedSlot)

```typescript
const embedSlot: EmbedSlotConfig = {
  id: 'my-plugin-embed',
  name: '我的嵌入组件',
  component: () => import('./components/MyEmbed.vue')
}
```

## 开发示例

### 创建简单插件

1. 创建插件目录结构

```
plugin/package/my-plugin/
├── package.json
└── index.js
```

2. 编写 package.json

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "author": "YourName",
  "librarySquirrel": {
    "activationType": "url"
  }
}
```

3. 编写入口文件

```javascript
export default {
  pluginId: 1,

  onLoad(context) {
    console.log('插件加载', context.pluginId)
  },

  contributions: {
    taskHandler: {
      # 任务处理器
    }
  }
}
```

## 主程序端管理

### 插件服务 (PluginService)

负责插件的 CRUD 操作：

```typescript
import PluginService from '@main/service/PluginService'

const pluginService = new PluginService()

# 获取所有插件
const plugins = await pluginService.list()

# 安装插件
await pluginService.install(pluginPath)

# 卸载插件
await pluginService.uninstall(pluginId)
```

### 插件数据库表

| 表名          | 说明         |
| ------------- | ------------ |
| `plugin`      | 插件基本信息 |
| `site`        | 站点配置     |
| `site_tag`    | 站点标签     |
| `site_author` | 站点作者     |

## IPC 通信

插件通过 IPC 与主进程通信：

```typescript
# 主进程端
Electron.ipcMain.handle('plugin-method', async (event, args) => {
  # 处理逻辑
})

# 插件端
window.api.pluginMethod(args)
```

## 最佳实践

1. **错误处理**：所有异步操作都需要 try-catch
2. **资源清理**：卸载时释放所有资源
3. **版本兼容**：检查主程序版本
4. **日志记录**：使用 LogUtil 记录关键操作
5. **配置存储**：使用 SecureStorageService 存储敏感信息

## 更新记录

### 2026-03-17

- [新增] 添加插件贡献点注册流程详解
- [新增] 添加 UI 插槽加载流程详解

### 2026-03-16

- [修改] BasePlugin 接口简化为只包含 pluginId
- [修改] 更新 PluginManager 说明

## 插件贡献点注册流程

### 设计理念：插件主动提供，主程序被动发现

LibrarySquirrel 的插件系统采用**事件驱动/观察者模式**的变体：插件激活时主动注册自己的贡献点，主程序在需要时动态发现和调用。这种设计实现了：

- **解耦**：主程序不需要知道有哪些插件
- **灵活**：多个插件可以监听同一 URL 模式
- **可扩展**：新增贡献点类型时无需大幅修改主程序

### 1. 任务功能注册流程

#### 注册阶段（插件激活时）

插件通过 `PluginContext.app.registerUrlListener()` 方法注册任务监听器：

```typescript
// PluginManager.ts:200-206
registerUrlListener: (conditions: { contributionId: string; listenerPatterns: RegExp[] }[]) => {
  const listenerManager = getPluginTaskUrlListenerManager()
  for (const condition of conditions) {
    const pluginWithContribution = new PluginWithContribution(plugin, 'taskHandler', condition.contributionId)
    listenerManager.register(pluginWithContribution, condition.listenerPatterns)
  }
}
```

- 插件在激活时调用 `registerUrlListener`，传入贡献点 ID 和正则表达式数组
- `PluginTaskUrlListenerManager` 将每个正则表达式与对应的插件关联存储

**关键文件**：

- `src/main/core/pluginTaskUrlListener.ts` - 任务 URL 监听器管理器

#### 触发阶段（创建任务时）

当用户创建任务时，`TaskService` 会查询匹配的插件：

```typescript
// TaskService.ts:53-55
const taskPlugins = await getPluginTaskUrlListenerManager().listListener(url)
```

- 根据 URL 匹配所有注册了监听器的插件
- 按顺序尝试每个插件处理任务

### 2. 站点浏览器功能注册流程

#### 注册阶段（插件激活时）

```typescript
// PluginManager.ts:215-218
registerSiteBrowser: (siteBrowser: SiteBrowserDTO) => {
  const manager = getSiteBrowserManager()
  siteBrowser = new SiteBrowserDTO(siteBrowser)
  manager.register(siteBrowser)
}
```

- 插件调用 `registerSiteBrowser` 注册一个站点浏览器
- `SiteBrowserManager` 将站点浏览器信息存入缓存

**关键文件**：

- `src/main/core/classes/SiteBrowserManager.ts` - 站点浏览器管理器
- `src/main/core/siteBrowserManager.ts` - 站点浏览器管理器导出

#### 使用阶段

在 `MainProcessApi.ts` 中处理前端打开站点浏览器的请求：

```typescript
ipcMain.handle('siteBrowser-open', async (pluginPublicId, contributionId) => {
  const siteBrowser = siteBrowserManager.getById(`${pluginPublicId}-${contributionId}`)
  const siteBrowserInstance = await pluginManager.getContribution(...)
})
```

### 3. UI 插槽加载流程

#### 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│  主进程                                                       │
│  1. 插件激活 → 调用 context.slots.registerXxxSlot()          │
│  2. SlotSyncService.registerSlot() → 缓存 + 推送IPC事件    │
└─────────────────────┬───────────────────────────────────────┘
                      │ IPC: slot-register / slot-batch-register
                      ▼
┌─────────────────────────────────────────────────────────────┐
│  渲染进程                                                     │
│  3. useSlotSyncListener 监听IPC事件                          │
│  4. 转换配置 → 调用 SlotRegistryStore 注册                   │
│  5. Store 更新 → 触发响应式更新                              │
│  6. 渲染器组件读取 Store → 渲染插件UI                        │
└─────────────────────────────────────────────────────────────┘
```

#### 主进程注册阶段

插件通过 `PluginContext.slots` 注册插槽：

```typescript
// PluginManager.ts:235-276
slots: {
  registerEmbedSlot: (config) => {
    const fullConfig: EmbedSlotConfig = { ...config, pluginId, type: 'embed' }
    getSlotSyncService().registerSlot(fullConfig)
  },
  registerPanelSlot: (config) => { /* ... */ },
  registerViewSlot: (config) => { /* ... */ },
  registerMenuSlot: (config) => { /* ... */ },
  registerSiteBrowserSlot: (config) => { /* ... */ },
  registerSlots: (configs) => { /* 批量注册 */ }
}
```

**关键文件**：

- `src/main/core/SlotSyncService.ts` - 插槽同步服务
- `src/main/plugin/types/SlotTypes.ts` - 插槽类型定义

#### 主进程同步到渲染进程

`SlotSyncService` 通过 IPC 推送插槽变更到渲染进程：

```typescript
// SlotSyncService.ts:43-47
registerSlot(config: SlotConfig): void {
  const slotId = config.id
  registeredSlots.set(slotId, config)  // 存入内存缓存
  this.syncToRenderer(SlotEvent.REGISTER, config)  // 推送到渲染进程
}
```

#### 渲染进程接收

`useSlotSyncListener` 监听 IPC 事件并更新 Store：

```typescript
// src/renderer/src/composables/useSlotSyncListener.ts
export function initSlotSyncListener() {
  const store = useSlotRegistryStore()

  // 监听注册事件
  window.electron.onSlotRegister((...args) => {
    const config = args[0] as SyncSlotConfig
    if (config.type === 'view') {
      store.registerViewSlot(convertToViewSlot(config))
    } else if (config.type === 'embed') {
      store.registerEmbedSlot(convertToEmbedSlot(config))
    }
    // ... 其他类型
  })

  // 首次加载时同步所有插槽
  window.electron.getAllSlots().then((slots) => {
    slots.forEach(/* 注册到 store */)
  })
}
```

#### 渲染阶段

渲染器组件使用 Store 中的插槽数据：

- `EmbedSlotRenderer.vue` - 渲染 embed 插槽（topbar/statusbar/toolbar）
- `PanelSlotRenderer.vue` - 渲染 panel 插槽（sidebar/bottom）
- `ViewSlotRenderer.vue` - 渲染 view 插槽（视图页面）
- `DynamicSideMenu.vue` - 渲染 menu 插槽（侧边菜单）

**关键文件**：

- `src/renderer/src/store/SlotRegistryStore.ts` - 插槽注册 Store
- `src/renderer/src/composables/useSlotSyncListener.ts` - 插槽同步监听器

### 插槽类型

| 类型              | 位置                              | 说明                                             |
| ----------------- | --------------------------------- | ------------------------------------------------ |
| `embed`           | 可能在主程序的任何位置            | 主程序可能在任何位置提供这种插槽，都是一些小组件 |
| `panel`           | left-sidebar/right-sidebar/bottom | 侧边面板或底部面板（暂未实现）                   |
| `view`            | 页面视图                          | 完整的页面视图，会在主视图中渲染                 |
| `menu`            | 侧边菜单                          | 左侧侧边栏的菜单项                               |
| `siteBrowserList` | 站点浏览器列表                    | 站点浏览器卡片入口                               |

### 管理器初始化

所有管理器在主进程启动时初始化：

```typescript
// src/main/index.ts:228-232
createTaskQueue()
createPluginTaskUrlListenerManager() // 任务URL监听器
createSiteBrowserManager() // 站点浏览器管理器
createPluginManager() // 插件管理器
```

### 停用时清理

插件停用时，系统会自动清理注册的贡献点：

```typescript
// PluginManager.ts:400-404
deactivatePlugin(pluginId: number): void {
  // 清除激活类型
  cached.activationType = undefined
  // 注销插件贡献的所有插槽
  getSlotSyncService().unregisterSlotsByPluginId(pluginId)
}
```
