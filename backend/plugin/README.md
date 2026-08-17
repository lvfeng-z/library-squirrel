# plugin 模块说明

## 一句话职责

插件**生命周期管理**与**自存信息**：从 ZIP 包安装、激活、卸载、重装插件，维护插件记录与运行状态；并通过 `plugin_storage`（统一 KV）提供插件自存信息与用户设置（`extensions.settings`）的存取。插件系统的架构（扩展点、SDK、前端扩展通信、初始化时序）详见 `.claude/rules/plugin.md`，本文件只描述模块职责与对外接口。

## 边界

- 与 **plugin/extension**：本模块（`backend/plugin`）管插件记录与生命周期入口；`extension/` 子包管运行时扩展点加载（TaskHandler / SiteBrowser / 前端扩展的注册与桥接）。
- 与 **plugin.md rules**：rules 讲"插件系统怎么设计"（协议、时序、前端扩展数据流），本文件讲"这个 Go 模块提供什么"。

## 对外接口（Handler）

| 方法 | 作用 |
| --- | --- |
| `InstallFromPath(packagePath, trusted)` | 从 ZIP 包安装插件；trusted 透传用户知情同意结果 |
| `InstallBundled(packagePath)`（Service） | 安装捆绑插件（**仅 pre-Run** 由启动期 `InstallBundledPlugins` 调用，检查更新流的检测入口）；bundled 已装记录按构建身份 buildId 检测变化，分支优先级：已装契约不兼容强制直装（记 forced 待办）> 拒绝标记等值静默跳过 > 未打标（buildId 空）回落 version 静默升级 > 记 available 待办保留旧版（前端红点提醒、答复走 `ApplyPendingUpgrade`）；包加载失败记 error 待办；不激活 |
| `Reinstall(pluginPublicId, trusted)` | 重新安装（已安装插件） |
| `ReinstallFromPath(pluginPublicId, packagePath, trusted)` | 从指定包重装 |
| `SetTrusted(pluginPublicId, trusted, force)` | 设置信任状态：true 时落标记并激活；false 时**即时停用运行时**（停进程+参与者清痕迹，不删文件），停用成功才落标记（参与者否决则保持运行、标记不变），force=true 跳过否决检查 |
| `Uninstall(pluginPublicId)` | 卸载插件 |
| `SetUninstalled(pluginId)` | 标记为已卸载（残留清理） |
| `Save` / `Update` | 保存 / 更新插件记录 |
| `GetById` / `GetByPublicId` | 按ID / 公共ID查询 |
| `Page(query)` | 分页查询 |
| `CheckInstalled(publicId)` | 检查是否已安装 |
| `GetPluginRoot()` | 获取插件运行时根目录 |
| `GetPluginStatus(pluginPublicId)` | 获取插件运行状态 |
| `GetPendingUpgrades()` | 获取检查更新待办（available 可答复计入红点；forced/error 只读告知） |
| `ApplyPendingUpgrade(pluginPublicId)` | 答复「升级」：对 available 待办执行运行期换版（当次会话生效；运行中任务被参与者否决） |
| `DeclinePendingUpgrade(pluginPublicId)` | 答复「跳过此构建」：持久化拒绝标记（`UpgradeDeclinedBuildID`），下次启动对等值 buildId 静默跳过 |
| `RestorePendingUpgrade(pluginPublicId)` | 「重新提示」反悔入口：清除拒绝标记并立即重跑检测重建待办 |

### 插件设置（SettingHandler）

| 方法 | 作用 |
| --- | --- |
| `GetSettings(pluginPublicId)` | 获取插件用户设置项（声明 + 当前值，加密项已解密） |
| `SaveSetting(pluginPublicId, key, value)` | 保存单个设置项（按声明 `encrypted` 路由加密/明文） |
| `ResetSetting(pluginPublicId, key)` | 重置设置项为默认值 |

## 核心概念

- **运行时插件 vs 纯 UI 插件**：前者有 Go 入口（DLL 子进程），后者仅 plugin.json。
- **来源（Source）**：插件安装来源（bundled/local/url/marketplace），由主程序按安装入口判定、写入 `plugin.Source`，不由插件声明。
- **信任标记（Trusted）**：`plugin.Trusted`（bool）。bundled 默认 true；第三方经用户知情同意后 true；false 则不激活（运行门控）。
- **构建身份（BuildID）**：构建管线注入 plugin.json `buildId` 字段的 git describe 标识（同源码状态重构建永远同值，与构建环境无关）。`InstallBundled` 以它做捆绑插件升级检测；静态资产 URL/ETag 缓存键同源（激活时优先 buildId，未打标包回落 version，见 `app.go` activatePlugin）。原 zip 字节 SHA256 存证（IntegrityHash）已退役移除（无判据性读取方）。设计见 `doc/plan/插件构建身份与升级判据机制.md`。
- **检查更新待办（pendingUpgrade，内存态）**：启动期检测出的更新事项（available/forced/error 三类），进程生命周期、重启重检；前端「插件」菜单红点与管理页待更新区块消费。落库的只有拒绝标记 `UpgradeDeclinedBuildID`（「跳过此构建」持久化，重装全字段覆盖自然清零）。设计见 `doc/plan/插件检查更新方案.md`。
- **PluginStatus**：插件运行时状态（进程存活、激活情况等）。
- **PluginStorage（插件自存信息）**：统一 KV 存储（`plugin_storage` 单表），取代旧的 `plugin.plugin_data` 与 `secure_storage`。明文项直接读写，加密项 `SetValueEncrypted` 存密文（`util/crypto` 加解密）、读取自动解密。

## 依赖关系

- 依赖：`extension/`（扩展点加载）、插件 SDK（HostService 桥接）、settings（激活配置）、`util/crypto`（自存信息加解密）
- 被依赖：app.go 初始化（`LoadPlugins`）、前端插件管理页

## 关键设计

- **停用生命周期（能力分散、契约集中）**：停用/换版统一走 `stopRuntime`（`lifecycle.go`）——参与者 `PrepareStop` 否决检查 → 运行时停止器（loader.UnloadPlugin，停进程+清其所属注册表）→ 参与者 `OnStopped` 清痕迹；`removeFiles` 独立成原子操作。凡持有插件运行时痕迹的模块经 `RegisterLifecycleParticipant` 注册（app.go 装配静态资源/前端扩展/taskManager 三个参与者），注册表是停用清理完备性的唯一审计点。taskManager 参与者在卸载/更新/重装时否决存在运行中任务的操作（Processing/Pausing/Stopping/WaitingForInput；Paused 不拦），取消信任不否决（代价由前端确认框按 `GetActiveTaskCount` 明示后 force 强制停）。卸载=stopRuntime+removeFiles+标记；重装/换版=stopRuntime+removeFiles+installCore+Activate；取消信任=仅 stopRuntime。操作类型（`PluginStopOp`：uninstall/update/untrust）与 force 透传给参与者分级处置。**崩溃路径对称**：loader 崩溃清理后经 `SetCrashNotifier` 回调 `NotifyPluginCrashed`，执行同一参与者 OnStopped 集合（不否决、不停进程）。设计见 `doc/plan/插件热重载体系完善方案.md`。
- **初始化时序约束**：必须先 `SetEventEmitter` 再 `LoadPlugins`，否则插件事件通道不可用（详见 plugin.md）。
- **检查更新流（bundled 生产者）**：检测在 pre-Run（`InstallBundled`，仅此入口——强制分支直装会绕过参与者否决，运行期一律走 `ApplyPendingUpgrade`）；提醒=前端 mounted 拉取 `GetPendingUpgrades` 写 store，经通用菜单红点注册表在「插件」菜单按钮显示 available 数；答复当次生效（`ApplyPendingUpgrade` 走换版链热重载，插件管理页行内升级/跳过按钮 + 多选批量升级）；未打标包与拒绝标记等值时不进待办（维持静默/跳过）；非 bundled 网络检查更新留接口未实现（待办列表/DTO/handler 命名不绑死 bundled）。设计见 `doc/plan/插件检查更新方案.md`。
- **插件信任模型（最小集）**：来源追溯 + 知情同意 + 运行门控，非沙箱隔离。`LoadPlugins` → `loadInstalledPlugins` 按 trusted 门控（非真不激活）+ Restricted Mode（settings 开关，仅 bundled，来源未设置视作非 bundled）加载；`activatePlugin` 起始统一拦截 trusted 非真。完整设计见 `.claude/rules/plugin.md`「插件信任模型」节与 `doc/plan/插件信任模型最小集方案.md`。
