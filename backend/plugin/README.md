# plugin 模块说明

## 一句话职责

插件**生命周期管理**与**自存信息**：从 ZIP 包安装、激活、卸载、重装插件，维护插件记录与运行状态；并通过 `plugin_storage`（统一 KV）提供插件自存信息与用户设置（`extensions.settings`）的存取。插件系统的架构（扩展点、SDK、前端扩展通信、初始化时序）详见 `.claude/rules/plugin.md`，本文件只描述模块职责与对外接口。

## 边界

- 与 **plugin/extension**：本模块（`backend/plugin`）管插件记录与生命周期入口；`extension/` 子包管运行时扩展点加载（TaskHandler / SiteBrowser / 前端扩展的注册与桥接）。
- 与 **plugin.md rules**：rules 讲"插件系统怎么设计"（协议、时序、前端扩展数据流），本文件讲"这个 Go 模块提供什么"。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `InstallFromPath(packagePath, installType)` | 从 ZIP 包安装插件 |
| `Reinstall(pluginPublicId, installType)` | 重新安装（已安装插件） |
| `ReinstallFromPath(...)` | 从指定包重装 |
| `Uninstall(pluginPublicId)` | 卸载插件 |
| `SetUninstalled(pluginId)` | 标记为已卸载（残留清理） |
| `Save` / `Update` | 保存 / 更新插件记录 |
| `GetById` / `GetByPublicId` | 按ID / 公共ID查询 |
| `Page(query)` | 分页查询 |
| `CheckInstalled(publicId)` | 检查是否已安装 |
| `GetPluginRoot()` | 获取插件运行时根目录 |
| `GetPluginStatus(pluginPublicId)` | 获取插件运行状态 |

### 插件设置（SettingHandler）

| 方法 | 作用 |
| --- | --- |
| `GetSettings(pluginPublicId)` | 获取插件用户设置项（声明 + 当前值，加密项已解密） |
| `SaveSetting(pluginPublicId, key, value)` | 保存单个设置项（按声明 `encrypted` 路由加密/明文） |
| `ResetSetting(pluginPublicId, key)` | 重置设置项为默认值 |

## 核心概念

- **运行时插件 vs 纯 UI 插件**：前者有 Go 入口（DLL 子进程），后者仅 plugin.json。
- **installType**：安装类型（如正式 / 开发模式）。
- **PluginStatus**：插件运行时状态（进程存活、激活情况等）。
- **PluginStorage（插件自存信息）**：统一 KV 存储（`plugin_storage` 单表），取代旧的 `plugin.plugin_data` 与 `secure_storage`。明文项直接读写，加密项 `SetValueEncrypted` 存密文（`util/crypto` 加解密）、读取自动解密。

## 依赖关系

- 依赖：`extension/`（扩展点加载）、插件 SDK（HostService 桥接）、settings（激活配置）、`util/crypto`（自存信息加解密）
- 被依赖：app.go 初始化（`LoadPlugins`）、前端插件管理页

## 关键设计

- **初始化时序约束**：必须先 `SetEventEmitter` 再 `LoadPlugins`，否则插件事件通道不可用（详见 plugin.md）。
