# LibrarySquirrel 插件系统开发指南

## 概述

插件系统是 LibrarySquirrel 扩展性的核心，允许通过 Go 共享库（`.dll`/`.so`）或纯 UI 配置支持不同的站点（如 pixiv、bilibili）。

### 两种插件模式

| 模式 | 入口文件 | DLL 加载 | 适用场景 |
|------|----------|----------|----------|
| **运行时插件** | 需要 `entryFile` | 需要 | 含 TaskHandler 或 SiteBrowser 的完整插件 |
| **纯 UI 插件** | 不需要 | 不需要 | 仅提供 Slot 扩展（图标、菜单项、静态页面等） |

插件接口（`PluginContext`、`TaskHandler`、`SiteBrowser`、`SlotType`、`ContentType` 等）定义在独立的 SDK 库 `github.com/lvfeng-z/library-squirrel-plugin-sdk` 中，主程序和插件共同依赖此 SDK。

### 注册方式

| 扩展点 | 注册方式 | 说明 |
|--------|----------|------|
| TaskHandler | 运行时（DLL `Activate` 函数） | 需要代码实现 |
| SiteBrowser | 运行时（DLL `Activate` 函数） | 需要代码实现 |
| Slot | **声明式**（`plugin.json` 配置） | 主程序读取配置自动注册 |

## 核心概念

### 插件目录结构

```
plugin/package/
└── [pluginPublicId]/
    └── [version]/
        ├── plugin.json           # 插件清单（必需）
        ├── entry.dll / entry.so  # Go 共享库（运行时插件必需，纯 UI 插件不需要）
        ├── views/                # Vue/JS/CSS 资源
        ├── assets/               # 图标、图片等静态资源
        └── ...
```

### 插件清单 (plugin.json)

```json
{
  "id": "com.example.pixiv",
  "name": "Pixiv Plugin",
  "version": "1.0.0",
  "author": "LibrarySquirrel",
  "description": "Pixiv 站点支持插件",
  "entryFile": "entry.dll",
  "activation": { "type": 1 },
  "extensions": {
    "taskHandlers": [
      { "id": "pixiv-task", "name": "Pixiv任务", "description": "处理Pixiv下载任务" }
    ],
    "siteBrowsers": [
      { "id": "pixiv-browser", "name": "Pixiv浏览器", "description": "浏览Pixiv站点" }
    ],
    "slots": [
      {
        "id": "pixiv-slot",
        "name": "Pixiv入口",
        "description": "Pixiv站点浏览器入口",
        "slotType": "siteBrowserList",
        "contentType": "precompiled",
        "content": { "js": "views/browser.js", "css": "views/browser.css" },
        "title": "Pixiv",
        "icon": "assets/icon.png",
        "order": 0,
        "contributionId": "pixiv-browser"
      }
    ],
    "staticResources": {
      "directories": ["views", "assets"]
    }
  }
}
```

#### extensions 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `taskHandlers` | `[]TaskHandlerDeclaration` | 任务处理器声明（运行时注册） |
| `siteBrowsers` | `[]SiteBrowserDeclaration` | 站点浏览器声明（运行时注册） |
| `slots` | `[]SlotDeclaration` | 插槽声明（**声明式注册**，主程序自动处理） |
| `staticResources` | `StaticResourcesConfig` | 允许访问的目录白名单 |

#### 静态资源访问

主程序通过 `StaticResourceService` 提供统一的静态资源 HTTP 服务：

- **URL 格式**：`resource://plugin/{publicId}/{version}/{relativePath}`
- **目录白名单**：仅 `staticResources.directories` 中声明的目录可访问
- **安全验证**：路径遍历防护（`..` 检查）、目录白名单校验
- **缓存策略**：ETag + Cache-Control immutable（基于版本号）

`plugin.json` 中的相对路径（`icon`、`content` 中的路径）在注册时由后端自动转换为完整 `resource://` URL。

### 激活类型

| Type | 说明                 |
| ---- | -------------------- |
| `0`  | 手动激活，用户主动启用 |
| `1`  | 启动时自动加载         |

## 插件入口函数（运行时插件）

运行时插件（含 TaskHandler/SiteBrowser）必须导出 `Activate` 函数：

```go
package main

import (
    pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
)

// Activate 插件入口函数，主程序加载 DLL 后调用
func Activate(ctx pluginsdk.PluginContext) {
    // 注册任务处理器
    ctx.RegisterTaskHandler("pixiv-task", "Pixiv任务", "处理Pixiv下载任务", &PixivTaskHandler{})

    // 注册站点浏览器
    ctx.RegisterSiteBrowser("pixiv-browser", "Pixiv浏览器", "浏览Pixiv站点", &PixivBrowser{})

    // 注意：Slot 注册已改为声明式（通过 plugin.json），ctx.RegisterSlot() 调用会返回错误

    // 注册 URL 监听器（任务路由）
    ctx.RegisterUrlListener("pixiv-task", []string{`https?://www\.pixiv\.net/.*`})
}
```

### 关键约定

1. **一个 DLL 可以注册多个扩展点**：`Activate` 中可多次调用 `RegisterXxx`
2. **主程序传递 PluginContext**：插件通过 `PluginContext` 访问主程序全部能力
3. **panic 安全**：主程序会 recover 插件 Activate 中的 panic 并回滚已注册的扩展点
4. **Slot 不在运行时注册**：Slot 通过 `plugin.json` 声明，主程序在启动时自动读取和注册

## PluginContext

`PluginContext` 是主程序提供给插件的完整 API，每个插件拥有独立的实例。

### 接口总览

```go
type PluginContext interface {
    // 扩展点注册（运行时）
    RegisterTaskHandler(id, name, desc string, handler TaskHandler) error
    RegisterSiteBrowser(id, name, desc string, browser SiteBrowser) error
    RegisterSlot(id, name, desc string, slotType SlotType, ...) error  // 已废弃，返回错误

    // 扩展点注销
    UnregisterSlot(id string) error          // 已废弃，返回错误
    UnregisterSiteBrowser(id string) error

    // 插件数据持久化
    GetPluginData() (string, error)
    SetPluginData(data string) error

    // 加密存储
    StoreEncryptedValue(plainValue, description string) (string, error)
    GetDecryptedValue(storageKey string) (string, error)
    RemoveEncryptedValue(storageKey string) error

    // 业务查询（返回 SDK 等效类型）
    GetWorkSetBySiteWorkSetId(siteWorkSetId, siteName string) (*WorkSet, error)
    AddSite(sites []*Site) error

    // 任务管理
    RegisterUrlListener(contributionId string, patterns []string) error
    UnregisterUrlListener() error
    CreateTask(url string) (*TaskCreateResult, error)

    // 路径
    GetPluginRoot(isRelative bool) string

    // 窗口管理
    GetMainWindow() WindowHandle
    CreateWindow(options WindowOptions) (WindowHandle, error)

    // 日志（带 Plugin[名称] 前缀）
    Infof(template string, args ...any)
    Debugf(template string, args ...any)
    Warnf(template string, args ...any)
    Errorf(template string, args ...any)
}
```

> **注意**：`RegisterSlot` 和 `UnregisterSlot` 已废弃。Slot 注册已改为声明式（通过 `plugin.json` 的 `extensions.slots` 配置），调用这些方法会返回错误。它们保留仅为了兼容 SDK 接口。

### Provider 接口（依赖倒置）

PluginContext 的服务依赖通过 Provider 接口注入，由 `extension` 包定义，各 `backend` 服务实现：

| Provider               | 方法                                            | 实现方                      |
| ---------------------- | ----------------------------------------------- | --------------------------- |
| `PluginDataProvider`   | `GetByPublicId`, `Update`                       | `plugin.Service`            |
| `SecureStorageProvider`| `StoreAndGetKey`, `GetValueByKey`, `Remove`     | `secureStorage.Service`     |
| `WorkSetQueryProvider` | `GetBySiteWorkSetIdAndSiteName`                 | `workSet.Service`           |
| `SiteSaveProvider`     | `Save`                                          | `site.Service`              |
| `TaskCreateProvider`   | `CreateTaskByURL`                               | `taskCreateAdapter`（适配） |
| `UrlListenerRegistry`  | `RegisterUrlListener`, `UnregisterUrlListener`  | `urlListenerAdapter`（适配）|

## 扩展点

### 1. TaskHandler — 任务处理器（运行时注册）

处理资源下载任务的完整生命周期：

```go
type TaskHandler interface {
    Create(url string) ([]*TaskCreateResponse, error)
    CreateWorkInfo(task *Task) (*WorkResponse, error)
    Start(task *Task) (io.ReadCloser, *WorkResponse, error)
    Retry(task *Task) (*WorkResponse, error)
    Pause(param *TaskResParam) error
    Stop(param *TaskResParam) error
    Resume(param *TaskResParam) (*WorkResponse, error)
}
```

**调用链**：`TaskManager → TaskExecutorImpl → Loader.GetTaskHandler() → Registry → 插件 DLL`

### 2. SiteBrowser — 站点浏览器（运行时注册）

提供站点内容浏览能力：

```go
type SiteBrowser interface {
    Open() error
    Close() error
}
```

> **⚠️ 站点浏览器注册 vs 站点浏览器列表插槽**
>
> | 概念                   | 注册方式                       | 用途                     |
> | ---------------------- | ------------------------------ | ------------------------ |
> | **站点浏览器**         | 运行时 `RegisterSiteBrowser()` | 注册浏览器功能（业务）   |
> | **站点浏览器列表插槽** | 声明式 `plugin.json` slot 配置 | 在 UI 中添加入口（展示） |
>
> **两者必须同时存在**，站点浏览器功能才能完整工作。

### 3. Slot — 插槽（声明式注册）

插件通过 `plugin.json` 的 `extensions.slots` 声明 UI 扩展，主程序在启动时自动注册。

#### 插槽类型

| SlotType              | 位置                | 说明                       |
| --------------------- | ------------------- | -------------------------- |
| `embed`               | 嵌入到指定位置      | 小组件（topbar/toolbar 等）|
| `panel`               | left/right/bottom   | 侧边或底部面板             |
| `view`                | 主视图              | 完整页面                   |
| `menu`                | 侧边菜单            | 菜单项                     |
| `siteBrowserList`     | 站点浏览器列表      | 站点浏览器卡片入口         |

#### 内容类型

| ContentType     | 说明                                    | Content 格式 |
| --------------- | --------------------------------------- | ------------ |
| `vueSource`     | Vue SFC 源码（运行时编译或预编译缓存）   | `{ "vue": "path.vue", "js": "path.js", "css": "path.css" }` |
| `precompiled`   | 预编译 JS/CSS 文件                      | `{ "js": "path.js", "css": "path.css" }` |
| `code`          | JavaScript 代码字符串（行内代码）        | 行内 JS 字符串（不经过路径转换） |
| `html`          | HTML 文件                               | `{ "html": "path.html" }` |

> `content` 中的相对路径由后端 `resolveContentURLs()` 自动转换为 `resource://plugin/{id}/{ver}/...` 完整 URL，前端直接使用。

## 注册中心

三个扩展点各有独立的线程安全注册中心：

| Registry               | 存储                                           | 关键文件                            |
| ---------------------- | ---------------------------------------------- | ----------------------------------- |
| `TaskHandlerRegistry`  | `map[string]*Extension[pluginsdk.TaskHandler]` | `extension/task_handler_registry.go`|
| `SiteBrowserRegistry`  | `map[string]*Extension[pluginsdk.SiteBrowser]` | `extension/site_browser_registry.go`|
| `SlotRegistry`         | `map[string]*Extension[*SlotConfig]`           | `extension/slot_registry.go`        |

key 格式：`pluginPublicId/extensionId`

> **注意**：`Registrar` 接口不再包含 `RegisterSlot` 方法。Slot 由 `loadInstalledPlugins()` 直接通过 `SlotRegistry.Register()` 注册。

## 插件加载流程

### 启动引导

```
NewApp()
  ├── initBaseServices()
  ├── initAdvancedServices()    ← 创建 Loader、各注册中心、StaticResourceService
  ├── initHandlers()
  └── loadInstalledPlugins()    ← 遍历已安装插件
        └── for each plugin:
              ├── 读取 plugin.json
              ├── 解析 PluginManifest（extensions 结构）
              ├── StaticResourceService.RegisterPlugin()    ← 注册静态资源映射
              ├── for each slot in extensions.slots:
              │     ├── 构建 SlotConfig（Icon/Content 路径转为 resource:// URL）
              │     └── SlotRegistry.Register()             ← 声明式注册 Slot
              ├── 判断是否为纯 UI 插件（无 taskHandlers/siteBrowsers）
              │     └── 纯 UI 插件跳过 DLL 加载
              └── 运行时插件:
                    ├── 创建 PluginInfo
                    ├── 创建 PluginContext
                    └── Loader.LoadPlugin(dllPath, publicId, ctx)
                          ├── plugin.Open(dllPath)
                          ├── Lookup("Activate")
                          ├── Activate(ctx)         ← 插件注册 TaskHandler/SiteBrowser
                          └── recover panic → UnloadPlugin
```

### 安装流程

```
InstallFromPath(zipPath)
  ├── 解析 plugin.json
  ├── 验证清单:
  │     ├── ID、Name、Version、Author 必填
  │     ├── extensions 必须存在
  │     ├── 至少含 taskHandlers/siteBrowsers/slots 之一
  │     └── entryFile 仅在有运行时扩展点时必填
  ├── 创建备份
  ├── 解压到 plugin/package/{publicId}/{version}/
  └── 保存到数据库（plugin 表）
```

### 静态资源请求流程

```
前端请求 resource://plugin/{id}/{ver}/path
  → Wails Asset Handler
    → PluginAwareAssetHandler.ServeHTTP()
      → 路径以 /plugin/ 开头?
        ├── 是 → StaticResourceService.ServeHTTP()
        │         ├── 查找 publicId → pluginResourceMapping
        │         ├── 安全校验（路径遍历、目录白名单）
        │         ├── 设置缓存头（ETag + Cache-Control immutable）
        │         └── http.ServeFile()
        └── 否 → 前端嵌入式资源（embed.FS）
```

## 插件服务 (PluginService)

| 方法               | 说明         |
| ------------------ | ------------ |
| `InstallFromPath`  | 从 ZIP 安装  |
| `Uninstall`        | 卸载         |
| `Reinstall`        | 重新安装     |
| `GetByPublicId`    | 按公开ID查询 |
| `Page`             | 分页查询     |

## 关键文件

| 文件 | 职责 |
|------|------|
| `backend/plugin/extension/loader.go` | 插件 DLL 加载、Activate 调用、panic 恢复 |
| `backend/plugin/extension/registrar.go` | Registrar 接口实现（TaskHandler/SiteBrowser 注册，**不含 Slot**） |
| `backend/plugin/extension/plugin_context.go` | PluginContext 实现、Provider 接口定义 |
| `backend/plugin/extension/convert.go` | SDK ↔ entity/DTO 类型转换、`taskHandlerAdapter` 适配器 |
| `backend/plugin/extension/task_handler_registry.go` | TaskHandler 注册中心 |
| `backend/plugin/extension/site_browser_registry.go` | SiteBrowser 注册中心 |
| `backend/plugin/extension/slot_registry.go` | Slot 注册中心（声明式注册的接收端） |
| `backend/plugin/extension/wails_pusher.go` | Slot 事件的 Wails Events 桥接 |
| `backend/plugin/extension/task_executor.go` | TaskManager ↔ 插件桥接 |
| `backend/plugin/extension/static_resource_service.go` | **静态资源服务**（路径映射、安全校验、缓存头、HTTP 文件服务） |
| `backend/plugin/extension/asset_handler.go` | **组合 Asset Handler**（前端资源 + 插件静态资源路由分发） |
| `backend/plugin/service.go` | 插件安装/卸载（ZIP + 数据库） |
| `backend/plugin/handler.go` | Wails Handler（前端 CRUD） |
| `backend/base/model/dto/plugin_types.go` | PluginManifest、PluginExtensions、SlotDeclaration 等类型 |
| `backend/base/slot.go` | SlotConfig、SlotType、ContentType 枚举 |
| `backend/base/model/extension.go` | Extension[T] 泛型包装、ExtensionMetadata |
| `app.go` | 启动引导、loadInstalledPlugins（声明式 Slot 注册 + resolveContentURLs）、适配器 |
| **SDK 库** `github.com/lvfeng-z/library-squirrel-plugin-sdk` | 插件接口定义（PluginContext、TaskHandler、SiteBrowser、SlotType、ContentType 等） |

## 最佳实践

1. **优先使用声明式注册**：UI 扩展通过 `plugin.json` 的 `extensions.slots` 声明，无需 DLL
2. **声明静态资源目录**：在 `extensions.staticResources.directories` 中列出所有需要访问的目录
3. **使用相对路径**：`plugin.json` 中的 `icon`、`content` 路径使用相对于插件根目录的相对路径
4. **错误处理**：所有异步操作都需处理 error
5. **资源清理**：插件 Activate 中的 panic 会自动回滚已注册的扩展点
6. **日志记录**：使用 PluginContext 的 `Infof`/`Errorf` 等方法（自动带插件名前缀）
7. **敏感数据**：使用 `StoreEncryptedValue` / `GetDecryptedValue` 存取
8. **插件数据**：使用 `GetPluginData` / `SetPluginData` 持久化插件状态

## 更新记录

### 2026-05-06（静态资源模块重构）
- [重构] Slot 注册从运行时 `PluginContext.RegisterSlot()` 改为声明式 `plugin.json` 配置
- [新增] `StaticResourceService`：线程安全的插件静态资源映射 + HTTP 文件服务
- [新增] `PluginAwareAssetHandler`：组合前端资源与插件静态资源的 asset handler
- [新增] `resolveContentURLs()`：自动将 Content 中的相对路径转为完整 `resource://` URL
- [新增] 纯 UI 插件支持：仅含 slots 的插件无需 DLL 加载
- [重构] `PluginManifest.Contributes []PluginContribute` → `PluginManifest.Extensions *PluginExtensions`
- [重构] `ContentType` 统一为 `vueSource/precompiled/code/html`，移除 `component`
- [修改] `Registrar` 接口移除 `RegisterSlot` 方法
- [修改] `Loader` 不再依赖 `SlotRegistry`
- [修改] `PluginContextDeps` 移除 `SlotRegistry` 字段
- [修改] `PluginContext.RegisterSlot()` 保留为存根（返回 config-driven 错误，兼容 SDK）
- [修改] 前端 `useSlotSyncListener`：用 `fetch(resource://)` 替代 `PluginHandler.ReadVueFile()`
- [新增] 前端 `html` 类型支持：`createHtmlComponent` 通过 `fetch` + Vue `template` 渲染

### 2026-05-06（SDK 迁移）
- [重构] 插件接口迁移至独立 SDK 库 `github.com/lvfeng-z/library-squirrel-plugin-sdk`
- [修改] PluginContext、TaskHandler、SiteBrowser 接口改为 SDK 定义
- [修改] 模块路径从 `github.com/library-squirrel/wails` 调整为 `github.com/library-squirrel`
- [新增] `convert.go`：SDK 类型与 entity/DTO 类型之间的转换函数 + `taskHandlerAdapter` 适配器
- [删除] `backend/base/plugin/` 目录（已被 SDK 替代）
- [修改] 插件入口函数签名：`func Activate(ctx pluginsdk.PluginContext)`

### 2026-05-05
- [修改] 目录结构调整：`internal/` → `backend/`，`pkg/` → `backend/base/`

### 2026-05-04
- [重构] 插件系统从 TypeScript/Electron 迁移到 Go/Wails
- [新增] PluginContext 接口、Registrar 接口、Provider 接口（依赖倒置）
- [新增] loadInstalledPlugins 启动引导
- [修改] Slot 同步从 SSE 改为 Wails Events
