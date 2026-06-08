# 插件系统 gRPC 重构计划

## 需求概述

引入 `hashicorp/go-plugin` + gRPC 重构当前插件系统，用标准化的 gRPC 协议替代自定义 JSON-RPC 2.0 over UDS 协议，获得进程生命周期管理、AutoMTLS 安全通信、健康检查、崩溃检测等能力。本期仅涉及 Go 语言插件。

## 架构对比

### 当前架构

```
主程序 (Go)
  └─ PluginProcess（手动 exec.Cmd + UDS 监听）
       └─ FrameCodec（自定义二进制帧协议: 4字节长度 + 1字节类型 + 载荷）
            ├─ JSON 帧 (0x01): JSON-RPC 2.0 消息
            └─ 二进制帧 (0x02): 流式数据（带 streamID）
```

**局限**：手写进程管理、帧协议、流控、错误恢复。二进制流需要专用 channel 路由。

### 目标架构

```
主程序 (Go) — hashicorp/go-plugin Host
  ├─ 进程生命周期管理（启动/停止/健康检查/崩溃重启）
  ├─ AutoMTLS 安全通信
  ├─ 版本协商
  └─ gRPC 通道
       ├─ PluginService（插件实现）：TaskHandler、SiteBrowser、Lifecycle
       └─ HostService（主程序实现，通过 GRPCBroker）：PluginContext
```

**优势**：
- 进程隔离 + 崩溃自动检测 + 重新启动
- 标准化的安全模型（AutoMTLS）
- 内置健康检查和版本协商
- 流式传输使用 gRPC 原生 streaming，无需自定义帧协议
- Go 插件 SDK 可直接使用 hashicorp/go-plugin 库，GRPCBroker 原生支持双向通信

## 技术方案

### 一、Protobuf 接口定义

定义四个核心 gRPC 服务：

```protobuf
syntax = "proto3";
package plugins;

// ========== 共享消息类型 ==========

message Empty {}

message Task {
  int64 id = 1;
  string taskName = 2;
  string url = 3;
  string siteName = 4;
  string siteWorkId = 5;
  string pluginTaskId = 6;
  string pluginData = 7;
  int64 parentTaskId = 8;
  int32 status = 9;
  string resourcePath = 10;
  int64 createTime = 11;
  int64 updateTime = 12;
}

message WorkSet {
  int64 id = 1;
  string name = 2;
  string siteName = 3;
  string siteWorkSetId = 4;
  string coverPath = 5;
  string description = 6;
  int64 createTime = 7;
  int64 updateTime = 8;
}

message Site {
  int64 id = 1;
  string name = 2;
  string displayName = 3;
  string description = 4;
  int64 createTime = 5;
  int64 updateTime = 6;
}

message LocalAuthorDTO {
  string name = 1;
  repeated string siteNames = 2;
}

message LocalTagDTO {
  string name = 1;
  string categoryName = 2;
  repeated string siteNames = 3;
}

message WorkResponse {
  string title = 1;
  string siteName = 2;
  string siteWorkId = 3;
  string description = 4;
  string coverUrl = 5;
  repeated LocalAuthorDTO authors = 6;
  repeated LocalTagDTO tags = 7;
  repeated WorkSet workSets = 8;
  repeated TaskResourceDTO resources = 9;
}

message TaskResourceDTO {
  string fileName = 1;
  string fileExtension = 2;
  int64 fileSize = 3;
  int32 order = 4;
  string downloadUrl = 5;
  string siteResourceId = 6;
  string pluginData = 7;
}

message TaskCreateChildResponse {
  string taskName = 1;
  string siteWorkId = 2;
  string url = 3;
  string pluginData = 4;
  string siteName = 5;
}

message TaskCreateResponse {
  string pluginTaskId = 1;
  string taskName = 2;
  string url = 3;
  string siteName = 4;
  repeated TaskCreateChildResponse children = 5;
}

message TaskResParam {
  Task task = 1;
  string resourcePath = 2;
}

// ========== 1. 插件生命周期服务（主程序 → 插件）==========

service PluginLifecycle {
  rpc Activate(ActivateRequest) returns (ActivateResponse);
  rpc Shutdown(Empty) returns (Empty);
}

message ActivateRequest {
  string pluginPublicId = 1;
  string pluginData = 2;
  string rootPath = 3;
  uint32 hostServiceId = 4;  // GRPCBroker service ID for HostService
}

message ActivateResponse {}

// ========== 2. 任务处理器服务（主程序 → 插件）==========

service TaskHandlerService {
  rpc Create(CreateRequest) returns (CreateResponse);
  rpc CreateWorkInfo(CreateWorkInfoRequest) returns (WorkResponse);
  // server streaming：流式返回文件数据
  rpc Start(StartRequest) returns (stream StreamChunk);
  rpc Retry(RetryRequest) returns (WorkResponse);
  rpc Pause(TaskResParamMessage) returns (Empty);
  rpc Stop(TaskResParamMessage) returns (Empty);
  // server streaming：从断点续传
  rpc Resume(TaskResParamMessage) returns (stream StreamChunk);
}

message CreateRequest {
  string url = 1;
  string contributionId = 2;
}

message CreateResponse {
  repeated TaskCreateResponse responses = 1;
}

message CreateWorkInfoRequest {
  Task task = 1;
  string contributionId = 2;
}

message StartRequest {
  Task task = 1;
  string contributionId = 2;
}

message RetryRequest {
  Task task = 1;
  string contributionId = 2;
}

message TaskResParamMessage {
  TaskResParam param = 1;
  string contributionId = 2;
}

// 流式数据块
message StreamChunk {
  oneof payload {
    WorkResponse workResponse = 1;  // 首个 chunk 携带作品信息
    bytes data = 2;                 // 后续 chunk 携带文件数据
    bool eof = 3;                   // 结束标记
    string error = 4;               // 错误信息
  }
}

// ========== 3. 站点浏览器服务（主程序 → 插件）==========

service SiteBrowserService {
  rpc Open(OpenBrowserRequest) returns (Empty);
  rpc Close(CloseBrowserRequest) returns (Empty);
}

message OpenBrowserRequest {
  string contributionId = 1;
}

message CloseBrowserRequest {
  string contributionId = 1;
}

// ========== 4. 主程序宿主服务（插件 → 主程序，通过 GRPCBroker）==========

service HostService {
  // 扩展点注册
  rpc RegisterTaskHandler(RegisterExtensionRequest) returns (Empty);
  rpc RegisterSiteBrowser(RegisterExtensionRequest) returns (Empty);
  rpc UnregisterSiteBrowser(UnregisterRequest) returns (Empty);

  // 插件数据持久化
  rpc GetPluginData(Empty) returns (PluginDataResponse);
  rpc SetPluginData(PluginDataRequest) returns (Empty);

  // 加密存储
  rpc StoreEncryptedValue(EncryptRequest) returns (EncryptResponse);
  rpc GetDecryptedValue(DecryptRequest) returns (DecryptResponse);
  rpc RemoveEncryptedValue(DecryptRequest) returns (Empty);

  // 业务查询
  rpc GetWorkSetBySiteWorkSetId(WorkSetQueryRequest) returns (WorkSetQueryResponse);
  rpc AddSite(AddSiteRequest) returns (Empty);

  // 任务管理
  rpc RegisterUrlListener(UrlListenerRequest) returns (Empty);
  rpc UnregisterUrlListener(Empty) returns (Empty);
  rpc CreateTask(CreateTaskRequest) returns (CreateTaskResponse);

  // 路径
  rpc GetPluginRoot(GetPluginRootRequest) returns (GetPluginRootResponse);

  // 日志
  rpc Log(LogRequest) returns (Empty);
}

message RegisterExtensionRequest {
  string contributionId = 1;
  string name = 2;
  string description = 3;
}

message UnregisterRequest {
  string contributionId = 1;
}

message PluginDataResponse {
  string data = 1;
}

message PluginDataRequest {
  string data = 1;
}

message EncryptRequest {
  string key = 1;
  string value = 2;
}

message EncryptResponse {
  string key = 1;
}

message DecryptRequest {
  string key = 1;
}

message DecryptResponse {
  string value = 1;
}

message WorkSetQueryRequest {
  string siteName = 1;
  string siteWorkSetId = 2;
}

message WorkSetQueryResponse {
  WorkSet workSet = 1;
}

message AddSiteRequest {
  Site site = 1;
}

message UrlListenerRequest {
  string listenerId = 1;
  repeated string patterns = 2;
}

message CreateTaskRequest {
  string url = 1;
}

message CreateTaskResponse {}

message GetPluginRootRequest {
  string pluginPublicId = 1;
}

message GetPluginRootResponse {
  string rootPath = 1;
}

message LogRequest {
  int32 level = 1;  // 0=debug, 1=info, 2=warn, 3=error
  string message = 2;
}
```

### 二、通信模型

```
┌─────────────────────────────────┐
│         主程序 (Host)            │
│                                 │
│  hashicorp/go-plugin Client     │
│  ├─ 启动插件子进程               │
│  ├─ AutoMTLS 握手               │
│  └─ 版本协商                    │
│                                 │
│  gRPC Client Stub:              │
│  ├─ PluginLifecycle             │
│  ├─ TaskHandlerService          │
│  └─ SiteBrowserService          │
│                                 │
│  gRPC Server (via GRPCBroker):  │
│  └─ HostService                 │
└──────────┬──────────────────────┘
           │ gRPC (AutoMTLS)
┌──────────┴──────────────────────┐
│      插件子进程 (Plugin)         │
│                                 │
│  hashicorp/go-plugin Serve      │
│                                 │
│  gRPC Server Stub:              │
│  ├─ PluginLifecycle             │
│  ├─ TaskHandlerService          │
│  └─ SiteBrowserService          │
│                                 │
│  gRPC Client (via GRPCBroker):  │
│  └─ HostService                 │
└─────────────────────────────────┘
```

**关键交互流程**：

```
1. Host 通过 hashicorp/go-plugin 启动插件进程
2. Host 调用 PluginLifecycle.Activate(init)
   ├─ init 包含 hostServiceId（GRPCBroker ID）
   └─ 插件使用 GRPCBroker.Dial(hostServiceId) 获取 HostService 客户端
3. 插件调用 HostService.RegisterTaskHandler(...)
4. 插件调用 HostService.RegisterUrlListener(...)
5. [用户触发任务]
6. Host 调用 TaskHandlerService.Create(url)
7. Host 调用 TaskHandlerService.Start(task)
   └─ 插件通过 gRPC server streaming 返回 StreamChunk
8. Host 调用 PluginLifecycle.Shutdown()
```

### 三、与当前系统的关键差异

| 维度 | 当前系统 | 新系统 |
|------|---------|--------|
| **RPC 框架** | 自定义 JSON-RPC 2.0 | gRPC (protobuf) |
| **传输协议** | 自定义二进制帧（4+1+N） | gRPC (HTTP/2 framing) |
| **流式传输** | 手写二进制帧 + StreamManager + StreamReader | gRPC server streaming |
| **进程管理** | 手动 exec.Cmd + UDS + watchProcess goroutine | hashicorp/go-plugin Client |
| **安全性** | 无（UDS 本地） | AutoMTLS + Magic Cookie |
| **健康检查** | 无 | 内置 Ping() |
| **崩溃恢复** | 手动 watchProcess + 清理注册表 | 内置检测 + 可配置重启 |
| **版本协商** | 无 | 两层版本控制（core + app） |
| **握手** | `--socket` 参数 + `activate` 通知 | Magic Cookie + stdout 协议行 |
| **双向通信** | 同一连接上的双向 JSON-RPC | GRPCBroker（原生支持） |
| **日志** | JSON-RPC Notify → 主程序处理 | stdout/stderr 劫持 + HostService.Log |
| **streamID 路由** | 手动 atomic counter + channel map | gRPC 内置流管理 |

### 四、Go SDK 重构

当前的 `library-squirrel-sdk` 从零依赖的自定义 RPC 框架重构为基于 hashicorp/go-plugin + gRPC 的标准插件框架。

**新目录结构**：
```
library-squirrel-sdk/
├── go.mod
├── proto/
│   └── plugin.proto              # protobuf 定义
├── gen/                          # protoc 生成的代码
│   ├── plugin.pb.go
│   └── plugin_grpc.pb.go
├── plugin.go                     # hashicorp/go-plugin 的 GRPCPlugin 实现
├── host.go                       # 主程序侧：Client 配置、HostService gRPC 服务端
├── context.go                    # 插件侧：PluginContext 接口（不变）
├── context_client.go             # 插件侧：gRPC 实现 PluginContext（通过 HostService 客户端）
├── task_handler.go               # 插件侧：TaskHandler 接口（不变）
├── browser.go                    # 插件侧：SiteBrowser 接口（不变）
├── dto.go                        # DTO 类型（保持现有定义，增加 toProto/fromProto 转换）
├── entity.go                     # Entity 类型（保持现有定义，增加 toProto/fromProto 转换）
├── types.go                      # 共享类型
└── logger.go                     # 日志接口（简化：直接输出到 stdout/stderr）
```

**删除的文件**：
- `rpc_codec.go` — 自定义帧协议 → gRPC 内置
- `rpc_client.go` — JSON-RPC 客户端 → gRPC 客户端
- `rpc_server.go` — JSON-RPC 服务端 → gRPC 服务端
- `rpc_stream_writer.go` — 自定义二进制流写入 → gRPC server streaming
- `host.go`（旧版） → 新 `host.go`（HostService gRPC 服务端）

#### `plugin.go` — GRPCPlugin 桥接

```go
package pluginsdk

import (
    "context"
    "google.golang.org/grpc"
    "github.com/hashicorp/go-plugin"
)

// Handshake 配置
var Handshake = plugin.HandshakeConfig{
    ProtocolVersion:  1,
    MagicCookieKey:   "LIBRARY_SQUIRREL_PLUGIN",
    MagicCookieValue: "ls_plugin_magic_cookie_value",
}

// PluginMap 定义插件类型映射
var PluginMap = map[string]plugin.Plugin{
    "library_squirrel": &LibrarySquirrelPlugin{},
}

// LibrarySquirrelPlugin 实现 hashicorp/go-plugin 的 GRPCPlugin 接口
type LibrarySquirrelPlugin struct {
    plugin.NetRPCUnsupportedPlugin
    Impl       TaskHandler
    Browser    SiteBrowser
    OnActivate func(ctx PluginContext)
}

func (p *LibrarySquirrelPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
    // 注册 PluginLifecycleServer、TaskHandlerServiceServer、SiteBrowserServiceServer
    // lifecycle 的 Activate 回调中：
    //   1. 通过 broker.Dial(init.HostServiceId) 获取 HostService 客户端
    //   2. 构建 PluginContext 实现
    //   3. 调用 p.OnActivate(ctx)
    return nil
}

func (p *LibrarySquirrelPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
    // 主程序侧不使用此路径（主程序侧直接使用生成的 gRPC 客户端）
    return nil, nil
}
```

#### 插件入口函数

```go
// Serve 启动插件进程（插件开发者调用）
func Serve(impl TaskHandler, opts ...ServeOption) {
    // 应用选项（browser、onActivate 等）
    // 调用 plugin.Serve(&plugin.ServeConfig{
    //     HandshakeConfig: Handshake,
    //     Plugins:         PluginMap,
    //     GRPCServer:      plugin.DefaultGRPCServer,
    // })
}
```

#### `host.go` — 主程序侧

```go
package pluginsdk

// NewClientConfig 创建 hashicorp/go-plugin 的 Client 配置
func NewClientConfig(cmd *exec.Cmd) *plugin.ClientConfig {
    return &plugin.ClientConfig{
        HandshakeConfig: Handshake,
        Plugins:         PluginMap,
        Cmd:             cmd,
        AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
    }
}

// HostServiceServer 实现 HostService 的 gRPC 服务端
// 委托给主程序的 provider 接口
type HostServiceServer struct {
    providers PluginContextDeps
}
```

#### `context_client.go` — 插件侧 PluginContext

```go
// grpcPluginContext 通过 gRPC 客户端调用主程序的 HostService
type grpcPluginContext struct {
    hostClient gen.HostServiceClient
}

func (c *grpcPluginContext) GetPluginData() (string, error) {
    resp, err := c.hostClient.GetPluginData(context.Background(), &gen.Empty{})
    return resp.Data, err
}
// ... 其余方法同理
```

### 五、主程序改动范围

#### 需要修改的文件

| 文件 | 改动说明 |
|------|---------|
| `app.go` | `loadInstalledPlugins` 使用 hashicorp/go-plugin Client 启动插件 |
| `backend/plugin/extension/loader.go` | 重写：用 hashicorp/go-plugin Client 管理 plugin 生命周期 |
| `backend/plugin/extension/plugin_host.go` | 重写为 HostService 的 gRPC 服务端实现 |
| `backend/plugin/extension/task_handler_proxy.go` | 重写为 TaskHandlerService 的 gRPC 客户端封装 |
| `backend/plugin/extension/stream_reader.go` | 重写为 gRPC stream reader（从 StreamChunk 读取） |
| `backend/plugin/extension/convert.go` | 更新类型转换（gRPC message ↔ entity/DTO） |
| `backend/plugin/extension/plugin_context.go` | 重构为 HostServiceServer 的依赖注入 |

#### 可以保留的文件

| 文件 | 说明 |
|------|------|
| `slot_registry.go` | Slot 注册逻辑不受 RPC 层影响 |
| `task_handler_registry.go` | 注册表逻辑不受 RPC 层影响 |
| `site_browser_registry.go` | 注册表逻辑不受 RPC 层影响 |
| `static_resource_service.go` | 静态资源服务不受影响 |
| `wails_pusher.go` | Wails 事件桥接不受影响 |
| `task_executor.go` | 任务执行器接口不变 |
| `errors.go` | 错误定义不变 |

#### 需要删除的文件

| 文件 | 替代方案 |
|------|---------|
| `plugin_process.go` | hashicorp/go-plugin 管理进程生命周期 |

#### go.mod 新增依赖

```
github.com/hashicorp/go-plugin
google.golang.org/grpc
google.golang.org/protobuf
```

### 六、插件侧改动（pixiv 插件为例）

当前插件入口：
```go
func main() {
    socketPath := flag.String("socket", "", "")
    flag.Parse()
    conn, _ := net.Dial("unix", *socketPath)
    // ... 手动建立 RPC
    pluginsdk.Serve(ctx, handlers...)
}
```

新插件入口：
```go
func main() {
    handler := &PixivTaskHandler{}
    browser := &PixivSiteBrowser{}

    pluginsdk.Serve(handler,
        pluginsdk.WithBrowser(browser),
        pluginsdk.WithActivate(func(ctx pluginsdk.PluginContext) {
            ctx.RegisterTaskHandler("main", "Pixiv Suite", "Pixiv 作品下载", handler)
            ctx.RegisterSiteBrowser("main", "Pixiv 浏览器", browser)
            ctx.RegisterUrlListener("main", []string{...})
        }),
    )
}
```

**关键变化**：
- 不再需要 `--socket` 参数
- 不再需要手动建立 UDS 连接
- `hashicorp/go-plugin` 通过 `plugin.Serve()` 处理所有底层细节
- `Activate` 回调在 GRPCPlugin 的 `Activate` RPC 处理中触发

## 开发步骤

### Phase 1: Protobuf 定义与 Go SDK 重构

1. **定义 `proto/plugin.proto`**
   - 映射所有 SDK DTO/Entity 为 protobuf message
   - 定义 4 个 gRPC service
   - 使用 `oneof` 处理 StreamChunk 变体
   - 使用 `stream` 标记 Start/Resume 的流式返回

2. **生成 Go 代码**
   - `protoc --go_out=. --go-grpc_out=. proto/plugin.proto`
   - 验证生成的代码编译通过

3. **实现 hashicorp/go-plugin 桥接** (`plugin.go`)
   - `LibrarySquirrelPlugin`（GRPCPlugin 接口）
   - `PluginLifecycleServer`（处理 Activate/Shutdown）
   - `TaskHandlerServiceServer`（委托给 Go TaskHandler 接口，处理 streaming）
   - `SiteBrowserServiceServer`（委托给 SiteBrowser 接口）
   - `HostServiceServer`（委托给 provider 接口）
   - `Serve()` 入口函数

4. **重写 SDK 的插件侧 API**
   - 新的 `PluginContext` 实现（通过 GRPCBroker 获取 HostService gRPC 客户端）
   - 移除手动的 UDS/RPC 连接代码

5. **更新 `dto.go` / `entity.go`**
   - 保留现有 Go 类型定义
   - 添加 `toProto()` / `fromProto()` 转换函数

6. **删除旧 RPC 文件**
   - 删除 `rpc_codec.go`、`rpc_client.go`、`rpc_server.go`、`rpc_stream_writer.go`

### Phase 2: 主程序适配

1. **重写 `loader.go`**
   - 用 hashicorp/go-plugin Client 替代 `PluginProcess`
   - 插件启动：`client := plugin.NewClient(pluginsdk.NewClientConfig(cmd))`
   - 获取 gRPC 连接：`conn, _ := client.Client()`
   - 通过 gRPC client 调用 PluginLifecycle.Activate
   - 插件停止：`client.Kill()`

2. **删除 `plugin_process.go`**
   - 所有进程管理由 hashicorp/go-plugin 接管

3. **重写 `plugin_host.go`**
   - 实现 `HostServiceServer` 的 gRPC 服务端
   - 委托给现有的 provider 接口（PluginDataProvider、SecureStorageProvider 等）
   - 在 GRPCBroker 中注册 HostService

4. **重写 `task_handler_proxy.go`**
   - `TaskHandlerProxy` 使用 TaskHandlerService 的 gRPC 客户端
   - `SiteBrowserProxy` 使用 SiteBrowserService 的 gRPC 客户端

5. **重写 `stream_reader.go`**
   - 新的 `StreamReader` 从 gRPC stream 接收 `StreamChunk`
   - 处理 `oneof payload`：workResponse / data / eof / error

6. **更新 `convert.go`**
   - 添加 gRPC message ↔ entity/DTO 转换函数
   - 保留现有 entity ↔ SDK 转换（SDK 类型定义不变）

7. **更新 `plugin_context.go`**
   - 重构为 HostServiceServer 的依赖注入配置
   - provider 接口不变

### Phase 3: pixiv 插件迁移

1. **适配 pixiv 插件到新 SDK**
   - 将入口改为使用 `pluginsdk.Serve()`
   - TaskHandler/SiteBrowser 实现不变（接口不变）
   - 在 `Activate` 回调中注册扩展点和 URL 监听器

2. **编译与打包**
   - 更新 go.mod 依赖新版 SDK
   - 编译为 `pixiv_plugin.exe`
   - `plugin.json` 无需修改（`entryFile` 字段不变）

### Phase 4: 测试与验证

1. **集成测试**
   - 安装修改后的 pixiv 插件
   - 验证任务创建、流式下载、暂停/恢复
   - 验证站点浏览器 Open/Close
   - 验证 Slot 注册
   - 验证 URL 监听器匹配

2. **稳定性测试**
   - 插件进程崩溃后主程序正确检测并清理
   - 健康检查正常工作
   - AutoMTLS 安全通信正常
   - 日志正确劫持和转发

3. **性能对比**
   - gRPC 与 JSON-RPC 的延迟对比
   - 流式传输吞吐量对比
   - 内存使用对比

## 关键技术决策

### 决策 1：一次性切换（已决定）

不保留旧版 JSON-RPC 插件系统。切换后所有插件必须使用新的 gRPC 协议。

**影响范围**：
- pixiv 插件需重新编译适配（Phase 3 处理）
- SDK 完全重写，不向后兼容
- 主程序侧一次性替换所有插件相关代码

### 决策 2：GRPCBroker 用于插件回调主程序

GRPCBroker 是 hashicorp/go-plugin 提供的双向通信机制，用于"插件需要调用主程序方法"的场景。

**工作原理**：
```
主程序 ──gRPC Client──▶ 插件（gRPC Server）   ← 主程序调用插件方法
主程序 ◀──gRPC Server── 插件（gRPC Client）    ← 插件回调主程序方法

"反向通道"通过 GRPCBroker 建立：
1. 主程序注册 HostService 到 Broker，获得 serviceId
2. 主程序在 Activate 请求中传入 serviceId
3. 插件通过 broker.Dial(serviceId) 获得到主程序的 gRPC 连接
4. 插件创建 HostService 客户端，调用 PluginContext 方法
```

**Go 插件端**：hashicorp/go-plugin 库原生支持 GRPCBroker，直接调用 `broker.Dial(hostServiceId)` 即可获取到 HostService 的 gRPC 连接。无需手动实现协议。

### 决策 3：本期仅支持 Go 插件（已决定）

本期专注 Go 语言插件。多语言支持（Node.js、Python 等）留作后续扩展。

**简化**：
- `plugin.json` 格式不变（无需 `runtime` 字段）
- 入口文件仍为编译后的 Go 可执行文件
- 不需要多语言引导逻辑

## 技术风险与应对

### 1. 流式传输的数据格式

**风险**：当前 `Start()` 需要同时返回 `WorkResponse` 和文件流数据。

**应对**：使用 `oneof` 的 `StreamChunk` 消息，首个 chunk 携带 `WorkResponse`，后续 chunks 携带 `bytes data`，最后 `eof=true`。

### 2. 多 contributionId 路由

**风险**：当前系统通过 `contributionId` 区分同一插件的不同 TaskHandler。gRPC service 如何路由？

**应对**：每个 RPC 请求中携带 `contributionId` 字段。SDK 侧的 `GRPCServer` 内部维护 `map[contributionId]TaskHandler`，根据请求中的 ID 路由到具体实现。

### 3. gRPC 与 Wails 环境兼容性

**风险**：Wails 运行在 Windows 上，gRPC 的 UDS 支持在 Windows 上可能有限制。

**应对**：hashicorp/go-plugin 在 Windows 上默认使用 TCP（而非 UDS），`127.0.0.1` 本地回环地址。性能与 UDS 相当，且无路径长度限制问题。

### 4. 大目录扫描耗时（为本地导入插件预留）

**风险**：`Create()` 是 unary RPC，大目录扫描可能耗时较长。

**应对**：gRPC 默认无超时限制（由客户端控制），Go 插件的 `Create` 可在合理时间内完成。后续可改为 client streaming 支持渐进式扫描。

## 验收标准

### Phase 1（Go SDK 重构）
- [ ] `plugin.proto` 定义完整，覆盖所有当前 SDK 接口
- [ ] protoc 生成的 Go 代码编译通过
- [ ] `plugin.Serve()` 能正确启动 gRPC 服务并完成 hashicorp/go-plugin 握手
- [ ] HostService 通过 GRPCBroker 正常工作
- [ ] TaskHandlerService 的 unary RPC 正常工作（Create、CreateWorkInfo）
- [ ] TaskHandlerService 的 streaming RPC 正常工作（Start 返回文件流）
- [ ] SiteBrowserService 的 RPC 正常工作（Open、Close）
- [ ] PluginContext 所有方法通过 HostService gRPC 客户端正常调用
- [ ] 旧 RPC 文件已删除，无编译错误

### Phase 2（主程序适配）
- [ ] loader.go 使用 hashicorp/go-plugin Client 启动/停止插件
- [ ] plugin_process.go 已删除
- [ ] HostService gRPC 服务端正确委托到 provider 接口
- [ ] TaskHandlerProxy 通过 gRPC 客户端调用插件方法
- [ ] StreamReader 从 gRPC stream 正确读取 StreamChunk
- [ ] convert.go 的 gRPC message ↔ entity/DTO 转换正确
- [ ] 插件崩溃后主程序正确检测并清理注册表

### Phase 3（pixiv 迁移）
- [ ] pixiv 插件在新系统下正常创建任务
- [ ] 流式下载正常工作
- [ ] SiteBrowser Open/Close 正常工作
- [ ] Slot 注册正常工作
- [ ] URL 监听器匹配正常

### Phase 4（测试）
- [ ] 进程崩溃恢复正常
- [ ] 健康检查正常
- [ ] AutoMTLS 安全通信正常
- [ ] 性能无明显退化

## 预计工作量

| 阶段 | 工作量 | 说明 |
|------|--------|------|
| Phase 1: Protobuf + Go SDK 重构 | 5-7 天 | 核心架构变更，proto 定义 + 桥接层 + 转换函数 |
| Phase 2: 主程序适配 | 3-4 天 | loader/host/proxy/stream/convert 重写 |
| Phase 3: pixiv 插件迁移 | 1-2 天 | 入口函数改造，接口不变 |
| Phase 4: 测试与验证 | 2-3 天 | 集成测试、稳定性测试、性能对比 |
| **合计** | **11-16 天** | |

## 后续扩展（不在本期范围）

1. **多语言插件支持**：在 gRPC 基础上，为 Node.js/Python 等语言开发 SDK（需实现 hashicorp/go-plugin 握手协议）
2. **本地文件导入插件**：基于多语言 SDK 用 TypeScript 重写旧版 JS 插件
3. **流式 Create**：将 `Create()` 改为 client streaming，支持大目录渐进式扫描
4. **插件热重载**：利用 hashicorp/go-plugin 的 reattach 能力
5. **插件市场**：标准化打包格式，支持下载安装
