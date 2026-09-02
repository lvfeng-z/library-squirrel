# settings 模块说明

## 一句话职责
应用运行时用户设置（`config/settings.json`）的加载、读取与保存——工作目录及作品/导入/插件/回收站/外观/合并/监控/备份治理/导出/分享各设置组的单一来源；同时拥有**工作目录未配置态**的领域概念：哨兵错误、前端通知发射口与请求期拒绝收口函数均定义于此，供全后端消费。

## 对外接口（Handler）
| 方法 | 作用 |
| --- | --- |
| `Get` | 获取全部设置 |
| `Save(changes)` | 按变更列表保存（每项 path+value，如 `workdir`、`fsmonitor.autoRepairEnabled`） |
| `Reset` | 重置为默认设置 |

## 核心概念
- **Settings**：`WorkDir` 顶层字段 + 11 个设置组，koanf 两层合并（代码默认值 → settings.json 文件层；文件缺失/字段缺失/解析失败均回落默认值）
- **未配置 = 空串**：`GetWorkDir()` 返回空串即「工作目录未配置」，是合法且显式的状态——各依赖模块须自行处理该状态（启动期不启动 / 请求期拒绝），禁止把空串当合法路径拼接到文件系统操作（空串会被路径层静默相对化为进程工作目录）
- **未配置三件套**（`unconfigured.go`）：
  - `ErrWorkDirNotConfigured`：哨兵错误（文案引导用户前往设置页配置），消费方 `errors.Is` 判定
  - `NotifyWorkDirUnconfigured(source)`：统一发射口——发 Wails 事件 `workdir:unconfigured`（payload `{source}` = 发现未配置的模块名）；发射器闭包未接线/未就绪时静默跳过
  - `RefuseIfUnconfigured(workdir, source)`：请求期拒绝的收口入口——空串时发射通知并返回哨兵错误，已配置返回 nil；拒绝点一行调用，判定+通知+错误返回不散落
- **afterSave 回调**：保存/重置后在写锁内同步调用（app 注入），联动需即时生效的设置项（storeRegistry suppression 开关、`/store/` 文件服务 workDir 快照）；回调内不得调用本 Service 方法（写锁重入死锁）

## 依赖关系
- 依赖：koanf（配置合并）、logger（解析失败日志）；发射器闭包由 app.go 在 `SetEventEmitter` 时序内接线（`SetUnconfiguredEmitter`，闭包延迟读取发射器）
- 被依赖：
  - 拒绝点消费方（`RefuseIfUnconfigured` / 既有守卫空值分支接 `NotifyWorkDirUnconfigured`）：persistentStore、backup、recycleBin、appLauncher、resource（替换回滚）、taskManager（下载前置）、export、share
  - 启动期消费方（判空串跳过启动）：fsmonitor（`Start` 短路）、recycleBin（`StartCleanup` 跳过）
  - 前端：设置页（Get/Save/Reset）、`UseWorkdirStatusStore`（启动拉取 GetSettings 判定未配置态 + 监听 `workdir:unconfigured` 事件）

## 关键设计
- **读取侧判空串、保存侧 Trim**：读取侧（`GetWorkDir`）不 Trim，空串即未配置的判定值；保存侧（`SaveSettings`）对 `workdir` 变更做 TrimSpace 后落库，空白串等同未配置——两侧职责分离，未配置判据单一（空串）
- **未配置通知只发请求期、不发启动期**：启动期的初始未配置判定由前端拉取完成（MainLayout 挂载水合 `GetSettings`），规避「发射早于前端 `Events.On` 注册即丢失」的时序缺口；请求期拒绝点经 `RefuseIfUnconfigured` 发射。既有守卫点（taskManager/export/share 的领域错误）同权接入发射——**事件统一、文案不统一**（各方保留带领域上下文的错误文案）
- **配置后生效形态**：`/store/` 文件服务的 workDir 快照经 afterSave 联动即时刷新；fsmonitor/recycleBin 的启动依赖重启生效（保存成功后前端提示重启），已启动态改目录走 `fsmonitor:workdir-changed` 事件提示重启
