# SDK 目录结构重组计划

## 目标

按切面划分 SDK 目录，根包保持中立，各切面子包职责单一。

## 目标结构

```
pluginsdk/
  dto/                        — 数据层：纯数据类型，无行为逻辑，无外部依赖
    handler_dto.go            ← TaskResParam, TaskCreateResponse, WorkResponse, TaskResourceDTO 等
    task_dto.go               ← TaskDTO, TaskProgressDTO, TaskProgressTreeDTO, CreateTaskRequest, TreeDataPageDTO
    work_dto.go               ← WorkDTO, WorkFullDTO
    work_set_dto.go           ← WorkSetDTO, WorkSetWithWorksResultDTO, WorkSetWithCoverDTO
    resource_dto.go           ← ResourceDTO
    site_dto.go               ← SiteDTO
    local_author_dto.go       ← LocalAuthorDTO, RankedLocalAuthor, RankedLocalAuthorWithWorkId
    local_tag_dto.go          ← LocalTagDTO, LocalTagWithBaseTagDTO
    site_author_dto.go        ← SiteAuthorDTO, RankedSiteAuthor*, SiteAuthorFullDTO, SiteAuthorLocalRelateDTO
    site_tag_dto.go           ← SiteTagDTO, SiteTagFullDTO, SiteTagLocalRelateDTO
    backup_dto.go             ← BackupDTO
    select_item.go            ← SelectItem
    search.go                 ← SearchType, SearchCondition, MediaType, operator 常量
    plugin_types.go           ← PluginManifest, PluginInstallDTO, SlotDeclaration 等插件元数据类型
    result.go                 ← CreateTaskResult（主程序 CreateTask 返回结果）

  transport/                  — 通信层：gRPC 实现（服务端、客户端、proto 转换）
    host.go                   ← HostDeps, HostServiceServer, Provider 接口, RegisterHostService, GRPCPluginClient, DiscoverPluginServices, host 侧 proto 转换
    plugin_server.go          ← lifecycleServer, taskHandlerServer, siteBrowserServer, 插件侧 proto 转换函数
    client.go                 ← PluginContextClient 实现
    logger.go                 ← grpcLogger 实现, NewGRPCLogger
    format.go                 ← FormatLogArgs
    handshake.go              ← Handshake, PluginMap, NewClientConfig（协议握手配置，通信层常量）

  plugin/                     — 插件框架层：插件开发入口和接口契约
    plugin.go                 ← Serve, ServeOption, serveConfig, LSPlugin（含 GRPCServer/GRPCClient 方法）
    context.go                ← PluginContext 接口
    task_handler.go           ← TaskHandler 接口
    browser.go                ← SiteBrowser 接口
    logger.go                 ← Logger 接口
    types.go                  ← WindowHandle, WindowOptions, TaskCreateResult, BatchResult, StreamResult, ErrPluginCrashed

  gen/                        — protobuf 生成代码（不变）
  proto/                      — protobuf 定义（不变）
  window/                     — 窗口操作（不变，内部引用改为 plugin 子包）
```

## 四层职责

| 层 | 包 | 职责 | 依赖 |
|---|---|---|---|
| **数据层** | `dto` | 纯数据结构，无方法，无外部包依赖 | 无 |
| **通信层** | `transport` | gRPC 实现细节：服务端/客户端、proto 转换、握手配置 | dto, gen |
| **插件框架层** | `plugin` | 插件开发入口（Serve）、核心接口（TaskHandler, SiteBrowser, PluginContext, Logger） | dto, transport |
| **平台层** | `window` | 平台窗口操作 | plugin |

依赖关系（无循环）：
```
dto
  ↑
transport ← gen
  ↑
plugin
  ↑
window
```

## 根包状态

根包（`pluginsdk`）不再包含任何 `.go` 文件。所有代码按切面归入子包。
可选添加 `doc.go` 仅用于包文档说明。

## 执行步骤

### 步骤 1：创建 `dto/` 子包

1. 创建 `dto/` 目录
2. 移入 13 个数据类型文件 + `plugin_types.go`，修改 `package` 为 `dto`
3. 从原 `dto.go` 拆分：`SiteDTO` → `site_dto.go`，`LocalAuthorDTO`/`LocalTagDTO` 合入已有文件
4. `CreateTaskResult` 从 `types.go` 移入 `dto/result.go`

### 步骤 2：创建 `transport/` 子包

1. 创建 `transport/` 目录
2. `host.go` 全部内容 → `transport/host.go`
3. 从 `plugin.go` 提取 gRPC 服务端和 proto 转换 → `transport/plugin_server.go`
4. `context_client.go` 全部内容 → `transport/client.go`
5. `logger.go` 的 grpcLogger 实现 → `transport/logger.go`
6. `FormatLogArgs` → `transport/format.go`
7. `Handshake`、`PluginMap`、`NewClientConfig` → `transport/handshake.go`

### 步骤 3：创建 `plugin/` 子包

1. 创建 `plugin/` 目录
2. `plugin.go` 剩余部分（Serve, LSPlugin 结构体）→ `plugin/plugin.go`
3. `context.go`（PluginContext 接口）→ `plugin/context.go`
4. `task_handler.go`（TaskHandler 接口）→ `plugin/task_handler.go`
5. `browser.go`（SiteBrowser 接口）→ `plugin/browser.go`
6. `Logger` 接口 → `plugin/logger.go`
7. `WindowHandle`, `WindowOptions`, `TaskCreateResult`, `BatchResult`, `StreamResult`, `ErrPluginCrashed` → `plugin/types.go`

### 步骤 4：更新跨包引用

**`plugin/` 内部**：
- 接口引用的 DTO 类型改为 `dto.XXX`
- LSPlugin 的 GRPCServer/GRPCClient 方法引用 `transport` 包的 server 构造和 proto 转换

**`transport/` 内部**：
- 所有 DTO 类型引用改为 `dto.XXX`
- `client.go` 中 PluginContextClient 引用 dto 类型

**`window/`**：
- `WindowOptions`、`WindowHandle` 引用路径改为 `plugin` 子包

### 步骤 5：更新主程序引用

| 旧引用 | 新引用 |
|---|---|
| `pluginsdk "github.com/.../plugin-sdk"` | 按需选择子包 |
| `pluginsdk.TaskDTO` | `pluginsdkdto "github.com/.../plugin-sdk/dto"` |
| `pluginsdk.HostDeps` | `pluginsdktransport "github.com/.../plugin-sdk/transport"` |
| `pluginsdk.Handshake` | `pluginsdktransport.Handshake` |
| `pluginsdk.CreateTaskResult` | `pluginsdkdto.CreateTaskResult` 或 `pluginsdkplugin.CreateTaskResult` |

### 步骤 6：验证编译

`CGO_ENABLED=0 go vet ./...` 在 SDK 和主程序分别验证。

## 关于 TaskCreateResult 的归属

`TaskCreateResult` 是插件 Create 方法的返回类型，带有 `IsStream()`/`Array()`/`Stream()` 方法。
它不是纯数据类型（有行为），归入 `plugin/types.go`。

`CreateTaskResult` 是主程序 CreateTask 的返回类型（纯数据），归入 `dto/result.go`。
