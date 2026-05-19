---
description: "插件系统架构与规则，适用于修改 plugin/ 目录或插件相关代码时加载"
globs:
  - "plugin/**"
  - "**/plugin.json"
---

# 插件系统架构与规则

## 插件系统
- 插件位于 `plugin/`，由 `app.go` 的 `loadInstalledPlugins()` 加载
- **两种类型**：运行时插件（Go DLL 子进程）和纯 UI 插件（仅 `plugin.json`）
- **三个扩展点**：TaskHandler、SiteBrowser（运行时）、Slot（通过 `plugin.json` 声明式）
- **插件 SDK**：`github.com/lvfeng-z/library-squirrel-plugin-sdk`（本地 replace 指令）
- **静态资源服务地址**：`http://wails.localhost:{backend-port}/plugin/{id}/{ver}/...`

## 插件开发规范

- **Slot 注册**：通过 `plugin.json` 的 `extensions.slots` 声明式注册，调用 `RegisterSlot()` 的这种方式已不再被支持
- **静态资源**：在 `extensions.staticResources.directories` 声明可访问目录
- **入口函数**：运行时插件导出 `func Activate(ctx pluginsdk.PluginContext)`
