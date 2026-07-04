# 修复:资源续传数据损坏(SDK push 解耦 + 续传不以落盘偏移为权威)

## 核心思路(三层契约)

本次修复围绕"主程序落盘偏移量"这一唯一权威展开,是所有修复的指导原则。

### 第一层(地基):主程序保证持久化

主程序从资源流中**收到**的每一字节,必须**落盘持久化**。由此,主程序对外提供的落盘偏移量 `diskOffset` ≡ 主程序已落盘字节数,真实、完整、无在途水分。

**隐含约束**:资源 reader 的前进**不得领先主程序的落盘**——即 `reader 已交付量 ≡ 主程序落盘量`,reader 不得独立预读、把数据推送到主程序来不及落盘的缓冲。当前 SDK 的 push 模式违背此约束(见差距分析)。

### 第二层(职责边界):主程序恢复时只提供偏移量

恢复任务时,主程序只把 `diskOffset` 交给插件,**不决定续写位置**。续写用旧流还是新流,由插件判断。(当前实现已符合:通过 `StreamOffsets` 提供。)

### 第三层(插件职责):以主程序偏移量为接续权威

插件**不假设**"自身推送的数据已被主程序完整持久化"——即**不信**自己的 `reader.validBytes` 等于主程序落盘量。接续时以主程序提供的 `diskOffset` 为**唯一判断依据**:

- 旧 reader 仍可用 → 复用,但**起点重设为 `diskOffset`**(`SetValidBytes(diskOffset)` + `Probe Range@diskOffset`)。
- 旧 reader 不可用 → 新建,同样 `SetValidBytes(diskOffset)`。

两条路的数据起点都钉死在 `diskOffset`,与主程序写入起点天然对齐。`reader.validBytes` 退化为 reader 内部管理连接的辅助量,**不参与接续决策**。

### 两层都必须修

- **第一层**(主程序保证持久化)= 效率与干净:reader 不领先落盘,从源头消除在途浪费与 Pause 丢失。
- **第三层**(以 diskOffset 为权威)= 正确性:接续对齐,且独立于第一层是否完美。
- **任一缺失都留隐患**:只修第三层 → 正确但每次恢复要重发在途量,浪费且 Start 阶段预读丢失需重下;只修第一层 → `reader.validBytes == diskOffset` 成立,但接续逻辑仍误用 validBytes,未来回归风险。故两层同时修,互为兜底。

---

## 当前实现 vs 契约的差距

### 差距一(违背第一层):SDK push 模式使 reader 领先落盘

资源传输链路(实测 + 代码确认):

```
插件 streamStoreSpecs goroutine: reader.Read(推进 validBytes) → stream.Send
        ↓ gRPC server-streaming(HTTP/2 window ~64KB 在途)
主程序 demuxStream goroutine:    stream.Recv → pw.Write(io.Pipe)
        ↓ io.Pipe(同步握手,至多 1 帧在途)
taskManager copyLoop:            pr.Read → storeWriter.Write(落盘)
```

三处解耦(gRPC buffer + io.Pipe + copyLoop buf)使 `reader.validBytes` 持续领先主程序落盘量。Pause 时,领先部分(gRPC buffer + pipe 帧 + copyLoop buf)丢失,`diskOffset < reader.validBytes`。

**关键代码**:
- `library-squirrel-sdk/transport/plugin_server.go:187-233`(`streamStoreSpecs`):每 spec 一个 goroutine,`reader.Read → stream.Send` 循环到 EOF,**不等主程序**。
- `backend/plugin/extension/task_handler_proxy.go:254,260-296`(`demuxStream`):后台 goroutine 被动 `Recv → pw.Write`,与 copyLoop 解耦。
- `library-squirrel-sdk/proto/plugin.proto:174,178`:Start/Resume 为 server-streaming(单向,主程序无反向 Read 请求通道)。

**实测证据(2026-07-03,任务 264,contentLength=1032816)**:

| 时间 | 事件 | reader.validBytes | 主程序落盘 |
|---|---|---|---|
| Start 后 312ms | SDK goroutine 预读 | 65536 | 0(setup-pause 未消费 Start stream) |
| Resume Load | 路径一读到 | 65536 | 0(offsets=main:0) |

Start 阶段 SDK goroutine 已预读 65536(HTTP/2 window 上限附近),主程序从未 Recv Start stream;恢复时 reader.validBytes 已计入这段,Range 从此起,跳过这段。

### 差距二(违背第三层):续传以 reader.validBytes 为起点

pixiv Resume 路径一(复用缓存 reader)用 `reader.ValidBytes()` 作为 Probe 的 Range 起点,而非主程序提供的 `StreamOffsets["main"]`(diskOffset)。

- `library-squirrel-plugin-pixiv/task_handler.go:518-532`(路径一):`r.Probe()` 直接以 reader.validBytes 发 Range,未 `SetValidBytes(diskOffset)`。
- 对照路径二(`:544`):`reader.SetValidBytes(offset)` 正确(故路径二从未出错)。

**实测证据(`[ResumeMount]` 写入起点 vs reader `建连 offset` 数据起点)**:

| 任务 | `[ResumeMount]` writeOffset | reader Range offset | 损坏形式 |
|---|---|---|---|
| 263 | 0(StoreStream) | 68821 | 错位:[0,68821) 永久缺 |
| 264 | 0(StoreStream) | 73728 | 错位:[0,73728) 永久缺 |
| 265 | 0(StoreStream) | 101590 | 错位:[0,101590) 永久缺 |
| 266 | 0(StoreStream) | 73728 | 错位:[0,73728) 永久缺 |
| 265(第三轮) | 73728(ResumeStream) | 1273046 | 空洞:[73728,1273046) 缺 |

`writeOffset ≠ Range` 即 reader 没把前段交给主程序,主程序却从 writeOffset 写 → 错位/空洞。`caller` 诊断日志进一步确认:每次 `reader.Read` 的调用方都是 `streamStoreSpecs.func1`(SDK 转发 goroutine),排除插件内别的消费点。

### 第二层:无差距

主程序恢复时通过 `StreamOffsets` 提供偏移量、不决定续写位置,符合契约。

---

## 修复方案

### 第一层修复:SDK push → pull(主程序驱动 Read)

**目标**:reader.Read 由主程序 copyLoop 的 Read 驱动,reader 不领先主程序落盘,从源头消除在途数据。

> server-streaming 无法承载 pull(主程序无反向"给我下一块"通道);HTTP/2 window 调小只能限领先量、不能归零。故必须改为 bidi-streaming,由主程序按需发 PullRequest。这是落实第一层的必要代价。

**改造范围(SDK transport 内部,插件 TaskHandler 接口不变)**:

1. **proto 协议**(`library-squirrel-sdk/proto/plugin.proto:174,178`):Start/Resume 由 server-streaming 改为 **bidi-streaming**,新增反向 `PullRequest`。
   ```proto
   rpc Start(stream StartFrame) returns (stream StreamChunk);
   rpc Resume(stream ResumeFrame) returns (stream StreamChunk);
   message PullRequest { string role = 1; int32 max_bytes = 2; }
   // StartFrame / ResumeFrame 为 oneof:首帧带 StartReq / TaskResumeParam,后续帧为 PullRequest
   ```
   `StreamChunk` 结构不变(role + data/eof/error)。

2. **插件侧**(`library-squirrel-sdk/transport/plugin_server.go:187-233` `streamStoreSpecs` 重写):删掉"每 spec 独立 goroutine 主动 drain"。改为单线程循环:`req := stream.Recv()` → 按 `req.role` 选 reader → `reader.Read(buf[:req.maxBytes])` → `stream.Send(data)`;读到 EOF 发该 role 的 Eof。
   - 串行化由 Recv 循环天然保证,去掉 `sendMu`。
   - 多 role 共享一条 bidi stream,严格按请求顺序响应(保证 Send/Recv 配对)。

3. **主程序聚合 ReadCloser**(`backend/plugin/extension/task_handler_proxy.go:215-296` `recvSpecsAndDemux`/`demuxStream` 重写):删掉 `io.Pipe` + 后台 demux goroutine。改为自定义 `pullReadCloser` 实现 `io.ReadCloser`,其 `Read(p)`:`stream.Send(&PullRequest{role, len(p)})` → `chunk := stream.Recv()` → copy `chunk.Data` 到 p;收到 Eof 返 io.EOF,Error 返错。
   - 多 role 共享一条 bidi stream:各 `pullReadCloser` 的 Send+Recv 配对需互斥(共享锁,Send 与 Recv 作为原子对),避免请求/响应串扰。

**保持不变**:
- 插件 `dto.TaskHandler` 接口(`Start/Resume` 返回 `[]*StoreSpec`,spec 含 `ReadCloser io.ReadCloser`)——插件作者无感。
- `backend/taskManager/model.go` 的 `streamController.reader`(`:305`)、`copyLoop`(`:843-900`)、`drain`(`:954-969`)——只调 `Read`,不关心 ReadCloser 内部是 io.Pipe 还是 pull 适配器。

**效果**:copyLoop Read 一次 → 触发一次 PullRequest → 插件 reader.Read 一次 → 主程序落盘。reader 不领先落盘,`reader.validBytes ≡ 主程序落盘量`。Pause 时 reader 已交付的都已落盘(copyLoop Read→Write 同循环,Pause 检查在循环顶),无在途丢失。

### 第三层修复:pixiv Resume 以 diskOffset 为接续权威

`library-squirrel-plugin-pixiv/task_handler.go` Resume 路径一,Probe 前 `SetValidBytes(offset)`(`offset = param.StreamOffsets["main"]`),与路径二统一:

```go
if val, ok := h.activeReaders.LoadAndDelete(param.Task.ID); ok {
    if r, ok := val.(*download.RetryReadCloser); ok {
        log.Printf("[PixivResume] 路径一:Load 缓存 reader: taskId=%d readerPtr=%p validBytes=%d", param.Task.ID, r, r.ValidBytes())
        r.SetValidBytes(offset) // 以主程序落盘偏移为准,不信 reader.validBytes
        r.Unpause()
        if hdr, sz, probeErr := r.Probe(); probeErr == nil {
            reader, headers, size = r, hdr, sz
        } else {
            log.Printf("[PixivResume] 路径一:链接已失效 (%v),走路径二", probeErr)
            r.Close()
        }
    }
}
```

两路径数据起点都 = diskOffset,`reader.validBytes` 不再参与接续决策。

### 第二层:无需修改

---

## 历史背景:首轮防御修复(已落地,保留)

首轮基于初始推测定位的缺陷 A-F 已实施,作为防御层保留,与本次三层契约修复正交:

| # | 位置 | 作用 |
|---|---|---|
| A | `taskManager/model.go` `handleEOF` | Size 未知/为 0 时仍校验 `written>0`,防空产物假完成 |
| B | `taskManager/model.go` `Pause` setup 分支 | setup 暂停通知插件,补全契约 |
| C | `backup/store_backup_orchestrator.go` | 还原静默跳过改可观测 + 失败上报 |
| D | `plugin-pixiv/task_handler.go` `Resume` | Resume 返回 Size/Format |
| E | `plugin-pixiv/retry_reader.go` `Probe` | 强制重建连接探活 |
| F | `taskManager/manager.go` `PauseTaskTree` | 补全 `WaitingForInput` 处理 |

> 首轮 D/E 让 Resume 能拿到正确 Size、能探活连接,但未解决"reader 起点与主程序写入起点不对齐"——即本次三层契约修复所针对的根因。

---

## 验证方案

| 场景 | 预期 |
|---|---|
| setup 阶段暂停 → 恢复(多任务) | reader.validBytes == 主程序落盘量(无预读领先);Resume 数据起点 = diskOffset;产物完整、可解码 |
| download 阶段暂停 → 恢复 | Pause 无在途丢失(pull 驱动,reader 不领先);恢复续传对齐 |
| 跨重启续传(进程重启) | 路径二(SetValidBytes(diskOffset))正常;reader.validBytes == diskOffset |
| 路径一 / 路径二切换 | 两路径数据起点都 = diskOffset,产物一致 |
| 多流并发(main + thumbnail) | bidi stream 多 role 分发正确,各 pullReadCloser Send/Recv 配对无串扰 |
| 诊断日志对照 | `[ResumeMount]` writeOffset == reader `建连 offset` |

**诊断日志清理**:第一层 pull 改造落地、回归通过后移除:
- `plugin-pixiv/internal/download/retry_reader.go`:`[RetryReader] Read 调用方`(`logCaller`)、`readCount`/`lastCaller` 字段。
- `plugin-pixiv/task_handler.go`:`[PixivStart/Resume/Pause] ... validBytes` 系列(`[ResumeMount]` 保留至回归通过)。
- `taskManager/model.go`:`[ResumeMount]`。

---

## 涉及文件

**第一层(SDK pull 改造,主要工作量)**:
- `library-squirrel-sdk/proto/plugin.proto`:Start/Resume 改 bidi-streaming,新增 `PullRequest`
- `library-squirrel-sdk/gen/`:`protoc` 重新生成 gRPC 代码
- `library-squirrel-sdk/transport/plugin_server.go`:`streamStoreSpecs` 重写为 pull(按需 Read)
- `backend/plugin/extension/task_handler_proxy.go`:`recvSpecsAndDemux`/`demuxStream` 重写为 `pullReadCloser`(去 io.Pipe)

**第三层(续传以 diskOffset 为权威)**:
- `library-squirrel-plugin-pixiv/task_handler.go`:Resume 路径一 Probe 前 `SetValidBytes(diskOffset)`

**诊断日志(改造后清理)**:
- `plugin-pixiv/internal/download/retry_reader.go`、`plugin-pixiv/task_handler.go`、`taskManager/model.go`

**编译/绑定**:
- SDK 改 proto 后重新生成 gRPC 代码;主程序与所有插件用新 SDK 重编译(transport 层变更)
- 未触及 Wails 绑定签名,bindings 无需重生

---

## 实施状态(2026-07-03)

- **第三层**(pixiv Resume 路径一 `SetValidBytes(diskOffset)`):已实施,编译通过。
- **第一层**(SDK push→pull):已实施,全链编译通过(SDK + 主程序 + pixiv + local 插件)。
  - proto:Start/Resume 改 bidi-streaming + `PullRequest`/`StartFrame`/`ResumeFrame`,`protoc` 重新生成 `gen/*.pb.go`。
  - 插件侧 `transport/plugin_server.go`:`streamStoreSpecs` → `serveSpecsPull`(单线程 `Recv(PullRequest) → reader.Read(max_bytes) → Send(data/eof/error)`)。
  - 主程序侧 `backend/plugin/extension/task_handler_proxy.go`:`recvSpecsAndDemux`/`demuxStream` → `recvSpecsAndPull`/`pullReadCloser`(去 `io.Pipe`;`Read` 一次 = `Send(PullRequest)` + `Recv(StreamChunk)` 一帧;多 role 共享一条 bidi,`mu` 串行化 Send/Recv 配对;全部 role `Close` 后 `cancel` ctx 关 stream)。
- **接口不变**:插件 `dto.TaskHandler`、`StoreSpec.ReadCloser`、`copyLoop`/`drain` 均未改,插件作者无感。
- **待回归**(我无法运行,需手工):多图 setup-pause / download-pause / 跨重启续传;验证产物可解码、`reader.validBytes == diskOffset`、无重发浪费。
- **诊断日志暂留**:回归通过后清理 `retry_reader.go` 的 `logCaller`/`readCount`/`lastCaller`、`task_handler.go` 的 `[PixivStart/Resume/Pause] validBytes`、`model.go` 的 `[ResumeMount]`。
- **文档影响核查**:本次改造是 SDK transport internals,对外接口不变;`.claude/rules/plugin.md`、`CLAUDE.md`、backend 模块 README 均未描述资源流传输机制,无需同步。
