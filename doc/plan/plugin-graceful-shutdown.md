# 插件优雅关闭（Graceful Shutdown）实现计划

## 目标

在插件进程被杀死前，宿主通过 gRPC 调用 `PluginLifecycle.Shutdown` RPC，给插件一个执行清理逻辑（刷写缓存、关闭文件句柄、持久化状态等）的机会。超时后仍强制终止。

## 现状分析

| 层级 | 现状 |
|---|---|
| Proto | `PluginLifecycle.Shutdown(Empty)` 已定义 |
| SDK 服务端 (`lifecycleServer`) | 空实现 `return &gen.Empty{}, nil` |
| SDK 公共 API (`plugin.go`) | 只有 `WithActivate`，无关闭选项 |
| SDK `LSPlugin` | `OnActivate` 字段存在，无 `OnShutdown` |
| 宿主 `Loader.UnloadPlugin()` | 直接 `client.Kill()`，不调用 Shutdown RPC |
| 宿主 `app.shutdownPlugins()` | 调用 `UnloadAll()` → 逐个 `UnloadPlugin()` → `client.Kill()` |

## 变更范围

涉及两个仓库、共 5 个文件：

### SDK 仓库 (`library-squirrel-sdk`) — 4 个文件

1. **`transport/plugin_server.go`** — `lifecycleServer` 添加 `onShutdown` 回调，实现 `Shutdown()` 方法
2. **`transport/lsplugin.go`** — `LSPlugin` 添加 `OnShutdown` 字段，`GRPCServer()` 中注入
3. **`plugin/plugin.go`** — `serveConfig` 添加 `onShutdown` 字段，新增 `WithShutdown()` 选项

### 主仓库 (`library-squirrel`) — 2 个文件

4. **`backend/plugin/extension/loader.go`** — `UnloadPlugin()` 在 `client.Kill()` 前调用 `Shutdown` RPC
5. **`app.go`** — `shutdownPlugins()` 无需改动（已通过 `UnloadAll()` → `UnloadPlugin()` 走同一流程）

## 详细实现步骤

### 步骤 1：SDK — `lifecycleServer` 添加 onShutdown 回调

**文件**: `library-squirrel-sdk/transport/plugin_server.go`

```go
type lifecycleServer struct {
    gen.UnimplementedPluginLifecycleServer
    onActivate func(dto.PluginContext)
    onShutdown func()              // 新增
    broker     *plugin.GRPCBroker
}

func (s *lifecycleServer) Shutdown(ctx context.Context, req *gen.Empty) (*gen.Empty, error) {
    if s.onShutdown != nil {
        s.onShutdown()
    }
    return &gen.Empty{}, nil
}
```

### 步骤 2：SDK — `LSPlugin` 添加 OnShutdown 字段

**文件**: `library-squirrel-sdk/transport/lsplugin.go`

```go
type LSPlugin struct {
    plugin.NetRPCUnsupportedPlugin
    Handler    dto.TaskHandler
    Browser    dto.SiteBrowser
    OnActivate func(dto.PluginContext)
    OnShutdown func()              // 新增
    HostDeps   *HostDeps
}

func (p *LSPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
    gen.RegisterPluginLifecycleServer(s, &lifecycleServer{
        onActivate: p.OnActivate,
        onShutdown: p.OnShutdown,   // 新增注入
        broker:     broker,
    })
    // ... 其余不变
}
```

### 步骤 3：SDK — 新增 `WithShutdown()` 公共 API

**文件**: `library-squirrel-sdk/plugin/plugin.go`

```go
type serveConfig struct {
    browser    dto.SiteBrowser
    onActivate func(dto.PluginContext)
    onShutdown func()              // 新增
}

// WithShutdown 设置 Shutdown 回调（插件进程被关闭前调用，用于清理资源）
func WithShutdown(fn func()) ServeOption {
    return func(c *serveConfig) { c.onShutdown = fn }
}

// Serve 中注入 OnShutdown
func Serve(handler dto.TaskHandler, opts ...ServeOption) {
    // ...
    lsPlugin := &transport.LSPlugin{
        Handler:    handler,
        Browser:    cfg.browser,
        OnActivate: cfg.onActivate,
        OnShutdown: cfg.onShutdown,  // 新增
    }
    // ...
}
```

### 步骤 4：宿主 — `Loader.UnloadPlugin()` 调用 Shutdown RPC

**文件**: `library-squirrel/backend/plugin/extension/loader.go`

在 `UnloadPlugin()` 的 `client.Kill()` 之前，增加优雅关闭调用：

```go
func (l *Loader) UnloadPlugin(pluginPublicId string) error {
    l.mu.Lock()
    entry, ok := l.processes[pluginPublicId]
    if ok {
        delete(l.processes, pluginPublicId)
    }
    l.mu.Unlock()

    if ok {
        // 优雅关闭：通知插件进程执行清理逻辑，超时 5 秒后强制终止
        l.gracefulShutdown(entry)
        entry.client.Kill()
    }

    l.taskHandlerRegistry.UnregisterAll(pluginPublicId)
    l.siteBrowserRegistry.UnregisterAll(pluginPublicId)
    logger.Log.Info("插件已卸载", "plugin", pluginPublicId)
    return nil
}
```

新增私有方法：

```go
const shutdownTimeout = 5 * time.Second

// gracefulShutdown 通知插件进程执行清理逻辑
func (l *Loader) gracefulShutdown(entry *pluginEntry) {
    // 检测插件进程是否已崩溃，避免对已死进程发起 gRPC 调用
    if entry.client.Exited() {
        return
    }
    ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
    defer cancel()

    _, err := entry.services.Lifecycle.Shutdown(ctx, &gen.Empty{})
    if err != nil {
        logger.Log.Warnf("插件优雅关闭失败（将强制终止）: %v", err)
    }
}
```

### 步骤 5（可选）：宿主 — `UnloadAll()` 并行优雅关闭

当前 `UnloadAll()` 是逐个串行调用 `UnloadPlugin()`，5 个插件最坏情况需要 5×5=25 秒。可优化为：先并行发送所有 Shutdown RPC，等待全部完成后统一 Kill。

暂不实现此优化，保持串行逻辑简单。若后续插件数量增多导致关闭缓慢，再优化。

## 调用流程（实现后）

```
应用关闭 / 插件卸载
  └─ UnloadPlugin(publicId)
       ├─ gracefulShutdown(entry)
       │    └─ gRPC: Lifecycle.Shutdown(timeout=5s)
       │         └─ 插件端 onShutdown() 回调执行
       ├─ client.Kill()          // 无论 Shutdown 是否成功，最终都强制终止
       ├─ UnregisterAll TaskHandlers
       └─ UnregisterAll SiteBrowsers
```

## 插件使用示例

```go
package main

import (
    pluginsdk "github.com/lvfeng-z/library-squirrel-sdk/plugin"
)

func main() {
    pluginsdk.Serve(myHandler,
        pluginsdk.WithActivate(func(ctx dto.PluginContext) {
            // 初始化...
        }),
        pluginsdk.WithShutdown(func() {
            // 清理：刷写缓存、关闭文件等
        }),
    )
}
```

## 风险与注意事项

1. **超时兜底**：Shutdown RPC 设置 5 秒超时，超时后直接 Kill，不阻塞主程序关闭流程
2. **崩溃兼容**：调用 Shutdown 前检查 `client.Exited()`，避免对已崩溃进程发起无效 gRPC 调用
3. **向下兼容**：未注册 `WithShutdown` 的插件，`onShutdown` 为 nil，Shutdown RPC 直接返回空，不影响现有插件
4. **Proto 无需改动**：`Shutdown(Empty)` RPC 已在 proto 中定义，gRPC 生成的客户端/服务端代码均已存在
