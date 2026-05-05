# LibrarySquirrel 插件系统开发指南

## 概述

插件系统是 LibrarySquirrel 扩展性的核心，允许通过 Go 共享库（`.dll`/`.so`）插件支持不同的站点（如 pixiv、bilibili）。插件在 `Activate` 函数中接收 `PluginContext`，自主注册所需的扩展点。

## 核心概念

### 插件目录结构

```
plugin/package/
└── [pluginPublicId]/
    └── [version]/
        ├── plugin.json           # 插件清单
        ├── entry.dll / entry.so  # Go 共享库（入口文件）
        └── views/                # 插件视图资源（可选）
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
  "contributes": [
    { "type": "taskHandler", "id": "pixiv-task" },
    { "type": "siteBrowser", "id": "pixiv-browser" },
    { "type": "slot", "id": "pixiv-slot" }
  ]
}
```

### 激活类型

| Type | 说明                 |
| ---- | -------------------- |
| `0`  | 手动激活，用户主动启用 |
| `1`  | 启动时自动加载         |

## 插件入口函数

每个插件 DLL 必须导出 `Activate` 函数：

```go
package main

import (
    "github.com/library-squirrel/wails/backend/plugin/extension"
    "github.com/library-squirrel/wails/backend/base"
)

// Activate 插件入口函数，主程序加载 DLL 后调用
func Activate(ctx extension.PluginContext) {
    // 注册任务处理器
    ctx.RegisterTaskHandler("pixiv-task", "Pixiv任务", "处理Pixiv下载任务", &PixivTaskHandler{})

    // 注册站点浏览器
    ctx.RegisterSiteBrowser("pixiv-browser", "Pixiv浏览器", "浏览Pixiv站点", &PixivBrowser{})

    // 注册插槽（UI扩展）
    ctx.RegisterSlot(
        "pixiv-slot", "Pixiv入口", "Pixiv站点浏览器入口",
        base.SlotTypeSiteBrowserList,   // 插槽类型
        "",                             // 内容
        base.ContentTypeComponent,      // 内容类型
        "Pixiv",                        // 标题
        "pixiv-icon.png",               // 图标
        0,                              // 排序
    )

    // 注册 URL 监听器（任务路由）
    ctx.RegisterUrlListener("pixiv-task", []string{`https?://www\.pixiv\.net/.*`})
}
```

### 关键约定

1. **一个 DLL 可以注册多个扩展点**：`Activate` 中可多次调用 `RegisterXxx`
2. **主程序传递 PluginContext**：插件通过 `PluginContext` 访问主程序全部能力
3. **panic 安全**：主程序会 recover 插件 Activate 中的 panic 并回滚已注册的扩展点

## PluginContext

`PluginContext` 是主程序提供给插件的完整 API，每个插件拥有独立的实例。

### 接口总览

```go
type PluginContext interface {
    // 扩展点注册
    RegisterTaskHandler(id, name, desc string, handler dto.TaskHandler) error
    RegisterSiteBrowser(id, name, desc string, browser SiteBrowser) error
    RegisterSlot(id, name, desc string, slotType base.SlotType, ...) error

    // 扩展点注销
    UnregisterSlot(id string) error
    UnregisterSiteBrowser(id string) error

    // 插件数据持久化
    GetPluginData() (string, error)
    SetPluginData(data string) error

    // 加密存储
    StoreEncryptedValue(plainValue, description string) (string, error)
    GetDecryptedValue(storageKey string) (string, error)
    RemoveEncryptedValue(storageKey string) error

    // 业务查询
    GetWorkSetBySiteWorkSetId(siteWorkSetId, siteName string) (*entity.WorkSet, error)
    AddSite(sites []*entity.Site) error

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

### 1. TaskHandler — 任务处理器

处理资源下载任务的完整生命周期：

```go
type TaskHandler interface {
    Create(url string) ([]*TaskCreateResponse, error)
    CreateWorkInfo(task *entity.Task) (*WorkResponse, error)
    Start(task *entity.Task) (io.ReadCloser, *WorkResponse, error)
    Retry(task *entity.Task) (*WorkResponse, error)
    Pause(param *TaskResParam) error
    Stop(param *TaskResParam) error
    Resume(param *TaskResParam) (*WorkResponse, error)
}
```

**调用链**：`TaskManager → TaskExecutorImpl → Loader.GetTaskHandler() → Registry → 插件 DLL`

### 2. SiteBrowser — 站点浏览器

提供站点内容浏览能力：

```go
type SiteBrowser interface {
    Open() error
    Close() error
}
```

> **⚠️ 站点浏览器注册 vs 站点浏览器列表插槽**
>
> | 概念                   | API                       | 用途                     |
> | ---------------------- | ------------------------- | ------------------------ |
> | **站点浏览器**         | `RegisterSiteBrowser()`   | 注册浏览器功能（业务）   |
> | **站点浏览器列表插槽** | `RegisterSlot(SlotTypeSiteBrowserList)` | 在 UI 中添加入口（展示） |
>
> **两者必须同时注册**，站点浏览器功能才能完整工作。

### 3. Slot — 插槽（UI 扩展）

插件通过插槽贡献 UI 内容（Vue 组件、HTML 等）。

#### 插槽类型

| SlotType              | 位置                | 说明                       |
| --------------------- | ------------------- | -------------------------- |
| `embed`               | 嵌入到指定位置      | 小组件（topbar/toolbar 等）|
| `panel`               | left/right/bottom   | 侧边或底部面板             |
| `view`                | 主视图              | 完整页面                   |
| `menu`                | 侧边菜单            | 菜单项                     |
| `siteBrowserList`     | 站点浏览器列表      | 站点浏览器卡片入口         |

#### 内容类型

| ContentType         | 说明         |
| ------------------- | ------------ |
| `vueSource`         | Vue SFC 源码 |
| `html`              | HTML 内容    |
| `component`         | 组件引用     |

## 注册中心

三个扩展点各有独立的线程安全注册中心：

| Registry               | 存储                                    | 关键文件                            |
| ---------------------- | --------------------------------------- | ----------------------------------- |
| `TaskHandlerRegistry`  | `map[string]*Extension[dto.TaskHandler]` | `extension/task_handler_registry.go`|
| `SiteBrowserRegistry`  | `map[string]*Extension[SiteBrowser]`    | `extension/site_browser_registry.go`|
| `SlotRegistry`         | `map[string]*Extension[*SlotConfig]`    | `extension/slot_registry.go`        |

key 格式：`pluginPublicId/extensionId`

## 插件加载流程

### 启动引导

```
NewApp()
  ├── initBaseServices()
  ├── initAdvancedServices()    ← 创建 Loader、各注册中心
  ├── initHandlers()
  └── loadInstalledPlugins()    ← 遍历已安装插件
        └── for each plugin:
              ├── 创建 PluginInfo
              ├── 创建 PluginContext（独立实例）
              └── Loader.LoadPlugin(dllPath, publicId, ctx)
                    ├── plugin.Open(dllPath)
                    ├── Lookup("Activate")
                    ├── Activate(ctx)         ← 插件内部注册扩展点
                    └── recover panic → UnloadPlugin
```

### 安装流程

```
InstallFromPath(zipPath)
  ├── 解析 plugin.json
  ├── 验证清单（ID、Name、Version、Author、Contributes、EntryFile）
  ├── 创建备份
  ├── 解压到 plugin/package/{publicId}/{version}/
  └── 保存到数据库（plugin 表）
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
| `backend/plugin/extension/registrar.go` | Registrar 接口实现（扩展点注册） |
| `backend/plugin/extension/plugin_context.go` | PluginContext 接口与实现、Provider 接口定义 |
| `backend/plugin/extension/task_handler_registry.go` | TaskHandler 注册中心 |
| `backend/plugin/extension/site_browser_registry.go` | SiteBrowser 注册中心 |
| `backend/plugin/extension/slot_registry.go` | Slot 注册中心 + WailsSlotPusher |
| `backend/plugin/extension/wails_pusher.go` | Slot 事件的 Wails Events 桥接 |
| `backend/plugin/extension/task_executor.go` | TaskManager ↔ 插件桥接 |
| `backend/plugin/service.go` | 插件安装/卸载（ZIP + 数据库） |
| `backend/plugin/handler.go` | Wails Handler（前端 CRUD） |
| `backend/base/model/dto/task_handler.go` | TaskHandler 接口定义 |
| `backend/base/model/dto/plugin_types.go` | PluginManifest、PluginContribute |
| `backend/base/slot.go` | SlotConfig、SlotType、ContentType 枚举 |
| `backend/base/model/extension.go` | Extension[T] 泛型包装、ExtensionMetadata |
| `app.go` | 启动引导、loadInstalledPlugins、适配器 |

## 最佳实践

1. **错误处理**：所有异步操作都需处理 error
2. **资源清理**：插件 Activate 中的 panic 会自动回滚已注册的扩展点
3. **日志记录**：使用 PluginContext 的 `Infof`/`Errorf` 等方法（自动带插件名前缀）
4. **敏感数据**：使用 `StoreEncryptedValue` / `GetDecryptedValue` 存取
5. **插件数据**：使用 `GetPluginData` / `SetPluginData` 持久化插件状态

## 更新记录

### 2026-05-05
- [修改] 目录结构调整：`internal/` → `backend/`，`pkg/` → `backend/base/`
- [修改] 插件接口定义从 `internal/plugin/extension` 提取到 `backend/base/plugin`（公共包）

### 2026-05-04

- [重构] 插件系统从 TypeScript/Electron 迁移到 Go/Wails
- [重构] 入口函数从 `PluginEntry()` 改为 `Activate(PluginContext)`
- [新增] PluginContext 接口，封装主程序提供给插件的完整 API
- [新增] Registrar 接口，支持插件自主注册多个扩展点
- [新增] Provider 接口（依赖倒置），隔离 PluginContext 与内部服务
- [新增] loadInstalledPlugins 启动引导
- [修改] Slot 同步从 SSE 改为 Wails Events

### 2026-03-17

- [新增] 添加插件贡献点注册流程详解
- [新增] 添加 UI 插槽加载流程详解

### 2026-03-16

- [修改] BasePlugin 接口简化为只包含 pluginId
- [修改] 更新 PluginManager 说明
