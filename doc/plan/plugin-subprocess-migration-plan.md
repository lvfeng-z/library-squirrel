# 插件系统从 Go plugin 迁移到子进程模式

## Context

当前插件系统使用 Go 标准库 `plugin.Open()` 加载 `.so`/`.dll` 共享库，存在两个根本问题：
1. **Windows 不支持** `buildmode=plugin`，而 Windows 是主要运行平台
2. 插件与主进程共享地址空间，插件崩溃会导致主进程崩溃

**目标**：将插件改为独立子进程运行，通过 stdin/stdout 进行 JSON-RPC 通信，实现跨平台支持和进程隔离。

## 方案概述

**通信机制**：JSON-RPC 2.0 over stdin/stdout + 长度前缀二进制帧

选择 stdin/stdout 而非 gRPC/HTTP 的原因：
- 无端口管理、无防火墙问题，Windows 完美支持
- LSP 协议已验证此模式的成熟性
- 仅依赖 Go 标准库，不引入 gRPC/protobuf 等重量级依赖

**核心挑战**：`TaskHandler.Start()` 当前返回 `io.ReadCloser`（二进制流），需要跨进程传输。
解决方案：JSON-RPC 响应返回元数据 + streamId，随后通过 stdout 以二进制帧持续发送数据块。主进程侧 `StreamReader` 实现 `io.ReadCloser`，对接现有 `ManagedTask.run()` 的 32KB 分块读取循环。

## 帧协议

stdout 上所有数据使用长度前缀帧格式：

```
[4字节: 消息长度 big-endian uint32] [1字节: 消息类型] [载荷]
```

消息类型：
- `0x01` = JSON-RPC 消息（UTF-8 JSON 文本）
- `0x02` = 二进制数据帧（streamId + 数据块）

主进程只通过 stdin 发送 JSON-RPC 消息给插件。插件通过 stdout 发送 JSON-RPC 响应和二进制流。stderr 保留给插件日志输出。

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

| 方法 | 对应 PluginContext 方法 |
|------|------------------------|
| `ctx/registerTaskHandler` | `RegisterTaskHandler(id, name, desc, handler)` |
| `ctx/registerSiteBrowser` | `RegisterSiteBrowser(id, name, desc, browser)` |
| `ctx/registerSlot` | `RegisterSlot(id, name, desc, slotType, content, contentType, ...)` |
| `ctx/unregisterSlot` | `UnregisterSlot(id)` |
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
1. 主进程发送 JSON-RPC 请求 taskHandler/start {task}
2. 插件调用内部 TaskHandler.Start()，获得 io.ReadCloser + WorkResponse
3. 插件立即返回 JSON-RPC 响应 {workResponse, streamId: "stream-xxx"}
4. 插件启动 goroutine：从 io.ReadCloser 读取数据块，以二进制帧写入 stdout
5. 主进程的 StreamReader 从 stdout 帧解析器读取数据，实现 io.ReadCloser
6. ManagedTask.run() 中的 32KB 分块读取循环无需修改
7. EOF 时：插件发送零长度二进制帧
8. Stop 时：主进程发送 taskHandler/stop，插件关闭 reader，流自然终止
9. 插件崩溃时：stdout 管道关闭，StreamReader.Read() 返回错误
```

## 插件生命周期

**启动**：
1. 主进程读取数据库中的插件记录
2. 解析可执行文件路径（`.exe`）
3. `exec.Command(pluginExe)` 启动子进程，stdin/stdout 为管道
4. 发送 `activate` 通知
5. 插件在 Activate 中通过 RPC 注册扩展点

**运行时**：
- 主进程发送 `ping` 请求（每 30s），超时则认为插件死亡
- 插件崩溃时，`cmd.Wait()` 返回非零退出码，主进程注销该插件的所有扩展点

**关闭**：
- 发送 `shutdown` 通知，等待 5s，超时则 `cmd.Process.Kill()`

## 文件变更

### plugin-sdk 库（`library-squirrel-plugin-sdk`）

| 文件 | 操作 | 说明 |
|------|------|------|
| `rpc/codec.go` | 新建 | 帧读写器：长度前缀协议，区分 JSON/二进制帧 |
| `rpc/client.go` | 新建 | JSON-RPC 2.0 客户端：请求/响应关联，通知发送 |
| `rpc/server.go` | 新建 | JSON-RPC 2.0 服务端：接收请求，分发给处理函数 |
| `rpc/stream_writer.go` | 新建 | 将 io.ReadCloser 数据以二进制帧写入 stdout |
| `rpc/types.go` | 新建 | RPC 消息类型定义：PluginContextInit、StartResponse 等 |
| `context_client.go` | 新建 | PluginContextClient：实现 PluginContext 接口，每个方法映射为一次 RPC 调用 |
| `host.go` | 新建 | PluginHost 接口 + 默认实现：处理插件→主进程的 ctx/* 调用 |

### 主项目（`library-squirrel`）

| 文件 | 操作 | 说明 |
|------|------|------|
| `backend/plugin/extension/stream_reader.go` | 新建 | io.ReadCloser 实现，从 channel 读取二进制帧数据 |
| `backend/plugin/extension/plugin_process.go` | 新建 | 管理单个插件子进程：启动、停止、健康检查、重启 |
| `backend/plugin/extension/plugin_host.go` | 新建 | JSON-RPC 服务端，处理 ctx/* 调用，委托给 PluginContext |
| `backend/plugin/extension/task_handler_proxy.go` | 新建 | 实现 dto.TaskHandler，通过 JSON-RPC 代理到子进程 |
| `backend/plugin/extension/loader.go` | 修改 | 新增 LoadPluginProcess() 方法，支持 .exe 子进程模式 |
| `backend/plugin/extension/task_executor.go` | 微调 | 使用新的 task_handler_proxy |
| `app.go` | 修改 | loadInstalledPlugins 根据 entryFile 后缀选择加载方式 |

### 插件（`library-squirrel-plugin-pixiv-go`）

| 文件 | 操作 | 说明 |
|------|------|------|
| `main.go` | 新建 | 程序入口：解析 stdin/stdout，启动 JSON-RPC 服务，注册处理器 |
| `activate.go` | 修改 | 从导出函数 Activate 改为由 main.go 内部调用 |
| `task_handler.go` | 微调 | Start() 返回的 io.ReadCloser 由 RPC 框架捕获并流式传输 |
| `site_browser.go` | 无变更 | 实现不变，由 RPC 框架调用 |
| `token_manager.go` | 无变更 | 使用 PluginContext 接口（现在由 PluginContextClient 实现） |

## 实施阶段

### Phase 1: SDK 传输层（plugin-sdk）
1. `rpc/codec.go` — 帧读写器
2. `rpc/client.go` — JSON-RPC 客户端
3. `rpc/server.go` — JSON-RPC 服务端
4. `rpc/stream_writer.go` — 二进制流写入器
5. `rpc/types.go` — 共享类型
6. `context_client.go` — PluginContext 的 RPC 客户端实现
7. `host.go` — 主进程侧 RPC 服务处理

### Phase 2: 主项目基础设施
8. `stream_reader.go` — io.ReadCloser 子进程实现
9. `plugin_process.go` — 子进程生命周期管理
10. `plugin_host.go` — ctx/* RPC 服务端
11. `task_handler_proxy.go` — TaskHandler RPC 代理
12. 修改 `loader.go` — 新增子进程加载路径

### Phase 3: 集成与适配
13. 修改 `app.go` — 根据 entryFile 后缀自动选择加载方式
14. 适配 Pixiv 插件：创建 `main.go`，修改 `activate.go`

### Phase 4: 清理
15. 移除 `plugin.Open()` 代码路径
16. 更新构建脚本（.dll → .exe）
17. 更新文档

## 验证方式

1. **单元测试**：codec 的帧读写、JSON-RPC 请求/响应序列化、二进制流传输
2. **集成测试**：用简单 echo 插件验证完整流程（Activate → 注册 → Create → Start 流式传输 → Stop）
3. **Pixiv 插件端到端**：编译为 .exe，通过主进程启动，验证下载任务完整流程
4. **崩溃恢复**：手动 kill 插件进程，验证主进程检测到并清理注册信息
