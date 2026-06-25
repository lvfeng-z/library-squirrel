# 插件系统从 Go plugin 迁移到子进程模式

## Context

当前插件系统使用 Go 标准库 `plugin.Open()` 加载 `.so`/`.dll` 共享库，存在两个根本问题：
1. **Windows 不支持** `buildmode=plugin`，而 Windows 是主要运行平台
2. 插件与主进程共享地址空间，插件崩溃会导致主进程崩溃

**目标**：将**运行时插件**（含 taskHandlers / siteBrowsers）改为独立子进程运行，通过 Unix Domain Socket 进行 JSON-RPC 通信，实现跨平台支持和进程隔离。

### 前置条件（已完成）

以下工作已在 "静态资源服务与声明式插槽注册重构" 中完成，是本计划的基础：

1. **Slot 注册改为声明式**：由 `app.go:loadInstalledPlugins()` 直接读取 `plugin.json` 的 `extensions.slots` 完成，不经过 PluginContext RPC。子进程协议无需包含 Slot 注册/注销方法。
2. **纯 UI 插件支持**：无 `taskHandlers` 且无 `siteBrowsers` 的插件在 `loadInstalledPlugins()` 中跳过 DLL 加载。子进程模式下同样跳过——纯 UI 插件不需要子进程。
3. **静态资源服务**：`StaticResourceService` + `PluginAwareAssetHandler` 提供独立的 `resource://plugin/{publicId}/...` 文件服务。子进程迁移**不影响**此机制——主进程直接读取插件目录文件，无需子进程参与。
4. **ContentType 统一**：`vueSource` / `precompiled` / `code` / `html`，前端通过 `resource://` URL 加载资源，不依赖插件运行时。

### 前置事项（待完成）

- **SDK 版本升级**：当前 `v0.0.2` 的 `PluginContext` 接口仍包含 `RegisterSlot` / `UnregisterSlot`（主程序侧已实现为返回错误的 stub）。需发布 SDK 新版本，将这两个方法标记为 deprecated 或移除，确保子进程侧不需要实现无意义的 RPC 调用。

## 方案概述

**通信机制**：JSON-RPC 2.0 over Unix Domain Socket (UDS) + 长度前缀二进制帧

选择 UDS 作为统一传输层的原因：
- **指令通信 + 流式传输合一**：UDS 是全双工字节流，单条连接同时承载 JSON-RPC 指令和二进制数据帧，通过帧头类型区分
- **跨平台统一**：`net.Listen("unix", path)` / `net.Dial("unix", path)` 在 Windows 10 1803+、Linux、macOS 上接口完全一致，无需平台分支代码
- **无端口管理**：本地内核传输，不经过网络协议栈，无防火墙问题
- **低延迟**：内核级缓冲区，延迟远低于 TCP
- **依赖最简**：仅依赖 Go 标准库，不引入 gRPC/protobuf 等重量级依赖

**Socket 文件管理**：
```go
// 应用数据目录下，按 publicId 隔离
socketDir := filepath.Join(util.RootPath(), "sockets")
os.MkdirAll(socketDir, 0700)
socketPath := filepath.Join(socketDir, publicId+".sock")
// Listen 前清理残留文件（上次崩溃未清理的情况）
os.Remove(socketPath)
```

**连接建立流程**：
1. 主进程 `net.Listen("unix", socketPath)` 创建监听
2. 主进程 `exec.Command(pluginExe, "--socket", socketPath)` 启动子进程
3. 子进程 `net.Dial("unix", socketPath)` 建立连接
4. 主进程 `listener.Accept()` 获得连接
5. 开始通信

**核心挑战**：`TaskHandler.Start()` 当前返回 `io.ReadCloser`（二进制流），需要跨进程传输。
解决方案：JSON-RPC 响应返回元数据 + streamId，随后通过同一 UDS 连接以二进制帧持续发送数据块。主进程侧 `StreamReader` 实现 `io.ReadCloser`，对接现有 `ManagedTask.run()` 的 32KB 分块读取循环。

## 帧协议

UDS 连接上所有数据使用长度前缀帧格式，全双工双向复用：

```
[4字节: 消息长度 big-endian uint32] [1字节: 消息类型] [载荷]
```

消息类型：
- `0x01` = JSON-RPC 消息（UTF-8 JSON 文本）
- `0x02` = 二进制数据帧（streamId + 数据块）

**双向复用规则**：
- 主进程 → 插件：仅发送 `0x01`（JSON-RPC 请求/通知）
- 插件 → 主进程：发送 `0x01`（JSON-RPC 响应/请求）和 `0x02`（二进制数据帧）
- 同一条 UDS 连接承载两个方向，通过帧类型区分，全双工无阻塞

**插件日志**：使用独立 stderr 输出（不经过 UDS），主进程可选捕获或丢弃。

二进制帧载荷格式（`0x02` 类型）：
```
[1字节: streamId长度] [N字节: streamId] [剩余字节: 数据块]
```
数据块长度为零时表示流结束（EOF）。

## JSON-RPC 方法映射

### 主进程 → 插件

| 方法 | 对应接口 | 参数 | 返回 |
|------|----------|------|------|
| `activate` | `Activate(PluginContext)` | `{pluginPublicId, pluginData, rootPath}` | 无（通知） |
| `taskHandler/create` | `TaskHandler.Create(url)` | `{url}` | `TaskCreateResponse[]` |
| `taskHandler/createWorkInfo` | `TaskHandler.CreateWorkInfo(task)` | `{task}` | `WorkResponse` |
| `taskHandler/start` | `TaskHandler.Start(task)` | `{task}` | `{workResponse, streamId}` |
| `taskHandler/retry` | `TaskHandler.Retry(task)` | `{task}` | `WorkResponse` |
| `taskHandler/pause` | `TaskHandler.Pause(param)` | `TaskResParam` | 无 |
| `taskHandler/stop` | `TaskHandler.Stop(param)` | `TaskResParam` | 无 |
| `taskHandler/resume` | `TaskHandler.Resume(param)` | `TaskResParam` | `WorkResponse` |
| `siteBrowser/open` | `SiteBrowser.Open()` | 无 | 无 |
| `siteBrowser/close` | `SiteBrowser.Close()` | 无 | 无 |
| `shutdown` | 生命周期 | 无 | 无（通知） |

### 插件 → 主进程（PluginContext 回调）

> **注**：`RegisterSlot` / `UnregisterSlot` 已改为声明式（plugin.json），不经过 RPC 协议。子进程侧 `PluginContextClient` 中这两个方法直接返回错误，不发送 RPC 请求。

| 方法 | 对应 PluginContext 方法 |
|------|------------------------|
| `ctx/registerTaskHandler` | `RegisterTaskHandler(id, name, desc, handler)` |
| `ctx/registerSiteBrowser` | `RegisterSiteBrowser(id, name, desc, browser)` |
| `ctx/unregisterSiteBrowser` | `UnregisterSiteBrowser(id)` |
| `ctx/getPluginData` | `GetPluginData()` |
| `ctx/setPluginData` | `SetPluginData(data)` |
| `ctx/storeEncryptedValue` | `StoreEncryptedValue(plainValue, desc)` |
| `ctx/getDecryptedValue` | `GetDecryptedValue(storageKey)` |
| `ctx/removeEncryptedValue` | `RemoveEncryptedValue(storageKey)` |
| `ctx/getWorkSetBySiteWorkSetId` | `GetWorkSetBySiteWorkSetId(siteWorkSetId, siteName)` |
| `ctx/addSite` | `AddSite(sites)` |
| `ctx/registerUrlListener` | `RegisterUrlListener(contributionId, patterns)` |
| `ctx/unregisterUrlListener` | `UnregisterUrlListener()` |
| `ctx/createTask` | `CreateTask(url)` |
| `ctx/getPluginRoot` | `GetPluginRoot(isRelative)` |
| `ctx/infof` / `ctx/debugf` / `ctx/warnf` / `ctx/errorf` | 日志方法 |

## 流式传输流程（Start 方法）

```
1. 主进程通过 UDS 发送 JSON-RPC 请求 taskHandler/start {task}
2. 插件调用内部 TaskHandler.Start()，获得 io.ReadCloser + WorkResponse
3. 插件立即返回 JSON-RPC 响应 {workResponse, streamId: "stream-xxx"}
4. 插件启动 goroutine：从 io.ReadCloser 读取数据块，以二进制帧写入 UDS 连接
5. 主进程的 StreamReader 从 UDS 帧解析器读取数据，实现 io.ReadCloser
6. ManagedTask.run() 中的 32KB 分块读取循环无需修改
7. EOF 时：插件发送零长度二进制帧
8. Stop 时：主进程发送 taskHandler/stop，插件关闭 reader，流自然终止
9. 插件崩溃时：UDS 连接关闭，StreamReader.Read() 返回错误
```

## 插件生命周期

> **注**：当前 `app.go:loadInstalledPlugins()` 已实现声明式 Slot 注册和纯 UI 插件检测。以下仅描述**运行时插件**（含 taskHandlers / siteBrowsers）的子进程生命周期。纯 UI 插件和声明式 Slot 注册的流程保持不变。

**启动**：
1. 主进程读取数据库中的插件记录
2. 读取并解析 `plugin.json`，执行声明式 Slot 注册 + 静态资源注册（已有逻辑，不变）
3. 检测到运行时扩展点（taskHandlers / siteBrowsers），解析可执行文件路径（`.exe`）
4. 主进程 `net.Listen("unix", socketPath)` 创建 UDS 监听（先清理残留 socket 文件）
5. 主进程 `exec.Command(pluginExe, "--socket", socketPath)` 启动子进程
6. 子进程 `net.Dial("unix", socketPath)` 连接，主进程 `Accept()` 获得连接
7. 主进程通过 UDS 发送 `activate` 通知
8. 插件在 Activate 中通过 RPC 注册 taskHandler / siteBrowser 扩展点（不再注册 slot）

**运行时**：
- 主进程发送 `ping` 请求（每 30s），超时则认为插件死亡
- 插件崩溃时：UDS 连接关闭 + `cmd.Wait()` 返回非零退出码，主进程注销该插件的 taskHandler / siteBrowser 扩展点
- 声明式 Slot 不受子进程崩溃影响，静态资源仍由主进程直接提供

**关闭**：
- 发送 `shutdown` 通知，等待 5s，超时则 `cmd.Process.Kill()`
- 清理 socket 文件：`os.Remove(socketPath)`
- Slot / 静态资源的清理由外层（StaticResourceService / SlotRegistry）负责，与子进程无关

## 文件变更

### plugin-sdk 库（`library-squirrel-sdk`）

| 文件 | 操作 | 说明 |
|------|------|------|
| `rpc/codec.go` | 新建 | 帧读写器：长度前缀协议，区分 JSON/二进制帧，基于 `net.Conn` 读写 |
| `rpc/client.go` | 新建 | JSON-RPC 2.0 客户端：请求/响应关联，通知发送 |
| `rpc/server.go` | 新建 | JSON-RPC 2.0 服务端：接收请求，分发给处理函数 |
| `rpc/stream_writer.go` | 新建 | 将 io.ReadCloser 数据以二进制帧写入 UDS 连接 |
| `rpc/types.go` | 新建 | RPC 消息类型定义：PluginContextInit、StartResponse 等（不含 Slot 相关类型） |
| `context_client.go` | 新建 | PluginContextClient：实现 PluginContext 接口。`RegisterSlot`/`UnregisterSlot` 直接返回错误（非 RPC 调用），其余方法映射为 RPC |
| `host.go` | 新建 | PluginHost 接口 + 默认实现：处理插件→主进程的 ctx/* 调用（不含 ctx/registerSlot、ctx/unregisterSlot） |

### 主项目（`library-squirrel`）

| 文件 | 操作 | 说明 |
|------|------|------|
| `backend/plugin/extension/stream_reader.go` | 新建 | io.ReadCloser 实现，从 UDS 连接的帧解析器读取二进制数据 |
| `backend/plugin/extension/plugin_process.go` | 新建 | 管理单个插件子进程：UDS 监听创建、子进程启动（`--socket` 参数）、连接接受、健康检查、停止时清理 socket 文件 |
| `backend/plugin/extension/plugin_host.go` | 新建 | JSON-RPC 服务端，处理 ctx/* 调用（不含 slot），委托给 PluginContext |
| `backend/plugin/extension/task_handler_proxy.go` | 新建 | 实现 dto.TaskHandler，通过 JSON-RPC 代理到子进程 |
| `backend/plugin/extension/loader.go` | 修改 | 新增 `LoadPluginProcess()` 方法，支持 .exe 子进程模式。`UnloadPlugin` 只清理 taskHandler/siteBrowser（slot 由外层管理） |
| `backend/plugin/extension/task_executor.go` | 微调 | 使用新的 task_handler_proxy |
| `app.go` | 修改 | 仅修改运行时插件加载路径：将 `Loader.LoadPlugin()`（DLL）替换为 `Loader.LoadPluginProcess()`（子进程）。声明式 Slot 注册、静态资源注册、纯 UI 插件检测逻辑不变 |

### 插件（`library-squirrel-plugin-pixiv-go`）

| 文件 | 操作 | 说明 |
|------|------|------|
| `main.go` | 新建 | 程序入口：解析 `--socket` 参数，`net.Dial("unix", socketPath)` 建立 UDS 连接，启动 JSON-RPC 服务，注册处理器 |
| `activate.go` | 修改 | 从导出函数 `Activate` 改为由 main.go 内部调用。移除 `RegisterSlot` 调用（已迁移到 plugin.json） |
| `task_handler.go` | 微调 | Start() 返回的 io.ReadCloser 由 RPC 框架捕获并通过 UDS 以二进制帧流式传输 |
| `site_browser.go` | 无变更 | 实现不变，由 RPC 框架调用 |
| `token_manager.go` | 无变更 | 使用 PluginContext 接口（现在由 PluginContextClient 实现） |

## 实施阶段

### Phase 0: SDK 接口清理（前置）
0. 发布 `library-squirrel-sdk` 新版本：
   - 从 `PluginContext` 接口移除 `RegisterSlot` / `UnregisterSlot`，或标记为 deprecated
   - 移除 `SlotType` / `ContentType` 类型定义（已改为配置驱动）
   - 主项目 `plugin_context.go` 中对应的 stub 方法可一并移除

### Phase 1: SDK 传输层（plugin-sdk）
1. `rpc/codec.go` — 帧读写器（基于 `net.Conn`，从 stdout pipe 改为 UDS 连接读写）
2. `rpc/client.go` — JSON-RPC 客户端
3. `rpc/server.go` — JSON-RPC 服务端
4. `rpc/stream_writer.go` — 二进制流写入 UDS 连接
5. `rpc/types.go` — 共享类型（不含 Slot 相关类型）
6. `context_client.go` — PluginContext 的 RPC 客户端实现（`RegisterSlot`/`UnregisterSlot` 直接返回错误，非 RPC）
7. `host.go` — 主进程侧 RPC 服务处理（不含 slot RPC handler）

### Phase 2: 主项目基础设施
8. `stream_reader.go` — io.ReadCloser 子进程实现（从 UDS 帧解析器读取数据）
9. `plugin_process.go` — 子进程生命周期管理（UDS 监听、`--socket` 参数传递、连接接受、socket 文件清理）
10. `plugin_host.go` — ctx/* RPC 服务端（仅 taskHandler/siteBrowser 相关，不含 slot）
11. `task_handler_proxy.go` — TaskHandler RPC 代理
12. 修改 `loader.go` — 新增子进程加载路径，`UnloadPlugin` 仅清理 taskHandler/siteBrowser

### Phase 3: 集成与适配
13. 修改 `app.go` — 仅修改运行时插件加载分支：`Loader.LoadPlugin()`（DLL）→ `Loader.LoadPluginProcess()`（子进程）。声明式 Slot 注册、静态资源注册、纯 UI 插件跳过逻辑不变
14. 适配 Pixiv 插件：创建 `main.go`（解析 `--socket`，UDS 连接），修改 `activate.go`（移除 `RegisterSlot` 调用，已迁移到 plugin.json）

### Phase 4: 清理
15. 移除 `plugin.Open()` 代码路径（`loader.go` 中的旧 `LoadPlugin` 方法）
16. 更新构建脚本（.dll → .exe）
17. 更新 `doc/ai-assistant/` 文档

## 验证方式

1. **单元测试**：codec 的帧读写、JSON-RPC 请求/响应序列化、二进制流传输
2. **集成测试**：用简单 echo 插件验证完整流程（Activate → 注册 → Create → Start 流式传输 → Stop）
3. **Pixiv 插件端到端**：编译为 .exe，通过主进程启动，验证下载任务完整流程
4. **崩溃恢复**：手动 kill 插件进程，验证主进程检测到并清理注册信息
