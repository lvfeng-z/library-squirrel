# 插件 RPC 活动检测(G·重新核定版)

> 关联:多轨资源谱系 · 节点 G。**本版取代先前同文件的 model.go `WithTimeout` 方案**(三重缺陷,作废)。
> 范围:主程序(`backend/plugin/extension`) + SDK(`library-squirrel-sdk`)。
> **实施状态(2026-07-09)**:G-1(keepalive 双端)+ G-2(proxy unary deadline)已实施,SDK 与主程序编译通过;G-3(reader idle)归 followup ②。keepalive 运行时生效待实测(kill 插件子进程,断言 RPC 在 `PingInterval+PingTimeout` 内失败、槽位释放)。

## 核定:原 model.go WithTimeout 方案作废

读码核查(`task_handler_proxy.go` / `lsplugin.go` / `loader.go` / `plugin/plugin.go`)后,原方案三重缺陷:

1. **unary 调用的 ctx 被丢弃**:Pause/Stop/CreateWorkInfo/Create/Retry 在 proxy 用 `context.Background()`(proxy:94/141/156/168),忽略调用方 ctx → model.go 包 `WithTimeout` 对它们**无效**。
2. **Start/Resume 的 reader 绑定 stream ctx**:proxy 用 `WithCancel(ctx)` 建 stream,reader 生命周期绑定该 ctx(proxy:111-113)→ 给 Start 包 `WithTimeout` 的 deadline 会**杀 reader**。
3. **绝对 deadline 不区分活动性**:把"慢但进展中"与"完全无响应"一视同仁。

## 重新认知:"无响应"有三种,单一机制覆盖不了

| 无响应类型 | 场景 | 检测机制 | 不误杀"长连接"的依据 |
|---|---|---|---|
| **连接级** | 插件进程崩溃/网络断 | gRPC keepalive | 对端活着就回 ping → 容忍任意长操作 |
| **应用级·流** | Start/Resume reader 长期无 chunk | idle timeout | 有 chunk 流动即不超时 |
| **应用级·unary** | CreateWorkInfo/Pause/Stop handler 卡死 | 短绝对 deadline | unary 正常秒级,远超正常耗时的 deadline 触发即异常 |

洞察:"慢但进展"只在流式(reader)有意义(用 idle);unary 无进度信号但正常就该快(用短 deadline);Start/Resume"长"是正常的,**不能**给绝对 deadline(会杀 reader)。

## 架构原则:策略汇聚,实现各在其层

三种机制分处不同生命周期阶段(keepalive=连接建立时配置 / unary deadline=每次调用包裹 / reader idle=流读取循环判定),**强行合并到一个处理器是伪聚合**(调用点仍散、职责混乱)。正确做法:

- **策略单一源**:SDK 新增 `liveness` 包,集中所有阈值常量 + gRPC options 构造。改阈值、看全貌、保证双端参数一致,只看这一处。
- **实现各在其位**:各层只引用 `liveness` 包,不在本地定义阈值/options。

## 方案

### 汇聚层:SDK `liveness` 包(策略单一源)

```go
package liveness

const (
	KeepalivePingInterval = 10 * time.Second // client 空闲多久后发 ping
	KeepalivePingTimeout  = 5 * time.Second  // ping 后多久无 pong 判连接死
	UnaryRPCTimeout       = 30 * time.Second // CreateWorkInfo/Pause/Stop(handler 卡死兜底)
	ReaderIdleTimeout     = 60 * time.Second // Start/Resume 流无 chunk(归 followup ②)
)

// ClientDialOptions 主程序 loader 注入:WithKeepaliveParams(PermitWithoutStream=true)
func ClientDialOptions() []grpc.DialOption { ... }
// ServerOptions 插件 plugin.go 注入:ServerParameters + EnforcementPolicy(MinTime≤Interval, PermitWithoutStream=true)
func ServerOptions() []grpc.ServerOption { ... }
```

### G-1:gRPC keepalive(连接级)

- 主程序 `backend/plugin/extension/loader.go:151`:`GRPCDialOptions: liveness.ClientDialOptions()`。
- SDK `library-squirrel-sdk/plugin/plugin.go:53`:`GRPCServer` 从 `goPlugin.DefaultGRPCServer`(签名 `func([]grpc.ServerOption) *grpc.Server`,默认 `grpc.NewServer(opts...)` 不加 options)换为同签名自定义函数:**保留** go-plugin 传入的 opts(框架注入的 TLS/拦截器等必需)、`append(opts, liveness.ServerOptions()...)` 追加 keepalive 再 NewServer,不可丢弃 opts。

效果:进程活、RPC 在跑(任意慢)→ ping 正常 → 不中断;进程死/网络断 → 不回 ping → `PingInterval+PingTimeout` 内连接判死 → 进行中 RPC 收 transport 错误。
局限:handler goroutine 卡死但连接层活(照常回 ping)→ keepalive 探测不到,交 G-2/G-3。

### G-2:unary 短 deadline(汇聚在 proxy 内部)

在 `task_handler_proxy.go` 的 unary 方法(Pause/Stop/CreateWorkInfo/Create/Retry)**内部统一包裹** `WithTimeout(context.Background(), liveness.UnaryRPCTimeout)`:
- proxy 本就是所有 unary 调用的必经入口,deadline 逻辑只在此一处;
- **model.go 不动**(不散落各调用点);
- **不改 `dto.TaskHandler` 接口签名**(不波及 pixiv/local/bilibili 插件实现)。

```go
func (p *TaskHandlerProxy) Pause(param *pluginsdkdto.TaskResParam) error {
	client, _ := p.getTaskClient()
	ctx, cancel := context.WithTimeout(context.Background(), liveness.UnaryRPCTimeout)
	defer cancel()
	_, err = client.Pause(ctx, &gen.TaskResParamMessage{...})
	return err
}
```

代价:proxy 内部用 `Background`,不响应任务 ctx 取消;但"取消传播"是独立问题,不在 G 的"无响应检测"范畴。

### G-3:reader idle timeout(归 followup ②)

`serveSpecsPull` 的 Recv 循环用 `liveness.ReaderIdleTimeout`:两次 Recv 间无 chunk 超阈值判应用级无响应。与 followup ②(reader 响应 ctx)同源,合并处理。

### Start/Resume 不加绝对 deadline

reader 绑定 stream ctx,deadline 杀 reader;其"长"是正常的(大文件),靠 idle(G-3)区分,不用绝对 deadline。

## 落点与范围

| 子项 | 仓库 | 文件 | 改动 |
|---|---|---|---|
| 汇聚层 liveness 包 | SDK | library-squirrel-sdk/liveness/(新) | 常量 + ClientDialOptions/ServerOptions |
| G-1 client keepalive | 主程序 | backend/plugin/extension/loader.go:151 | GRPCDialOptions 引用 liveness |
| G-1 server keepalive | SDK | library-squirrel-sdk/plugin/plugin.go:48 | GRPCServer 注入 liveness.ServerOptions |
| G-2 unary deadline | 主程序 | backend/plugin/extension/task_handler_proxy.go | unary 方法内部包 WithTimeout |
| G-3 reader idle | SDK+插件 | serveSpecsPull 等 | 引用 liveness.ReaderIdleTimeout(归②) |

## 验证

1. **keepalive 连接级**:kill 插件子进程,断言 client RPC 在 `PingInterval+PingTimeout` 内返回 transport 错误(非永久阻塞)、槽位释放。
2. **keepalive 不误杀长操作**:慢建连/大文件下载(>keepalive 周期),断言 ping 正常、RPC/下载不被中断。
3. **unary deadline**:注入"插件 Pause handler 永不返回"桩,断言 `UnaryRPCTimeout` 后 proxy 返回超时、任务状态仍推进。
4. **reader idle**(G-3):注入"插件 reader 无 chunk"桩,断言 idle 超时后 copyLoop 退出(归 ②)。
5. **liveness 包单一源**:断言双端 keepalive 参数取自同一常量(MinTime ≤ PingInterval),无散落定义。

## 风险

- **go-plugin keepalive 双端配合**:`MinTime`/`PermitWithoutStream` 须一致,需实测 ping 实际生效(go-plugin 版本行为差异)。
- **跨主程序+SDK 两仓库**,需同步发布。
- keepalive 探测不到 handler 卡死(连接层活)→ 必须配合 G-2/G-3 才完整,单做 G-1 不够。

## 边界

- **G** = 连接级(keepalive)+ unary 控制层(短 deadline):覆盖"进程死"与"unary handler 卡死"。
- **followup ②** = reader 流 idle:覆盖"连接活但流无数据"。两者互补,共同构成"无响应 vs 长连接"区分。
- "任务取消传播到 unary"(proxy 用 Background)是独立问题,不在 G 范围。

## 关联

- 任务图:`.claude/workflow/active/multitrack-resource-lineage/TREE.md` 节点 G
- 取代:本文件先前的 model.go `WithTimeout` 方案(作废)
- 互补:followup ②(reader idle,跨 SDK+插件)
