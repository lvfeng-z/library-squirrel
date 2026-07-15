# pixiv 连接复用：handler 级共享 Transport

> 谱系：multitrack-resource-lineage · 节点 F（派生自 D）
> 创建：2026-07-12 · 状态：设计方案（待 review）
> 范围：`library-squirrel-plugin-pixiv`（跨仓库）；SDK/主程序不动
> 关联：`doc/bug/频繁启停资源内容错位-Resume并发竞态.md` §68-74（F 的原始待办）、`doc/plan/pause-drain-inflight-chunk.md` §7.5（reader 复用与连接复用正交）

## 1. 背景与目标

### 1.1 现状：全插件"反连接复用"

pixiv 插件所有 HTTP 出口都**主动禁用连接复用**，以规避代理环境的 `Unsolicited response received on idle HTTP channel`：

| 出口 | 位置 | 复用策略 | 频率 |
|---|---|---|---|
| 图片下载 | `pixivRequestFn`（task_handler.go:440-456） | **每次请求新建** `Client+Transport`（:450-453） | 高（每个 Range chunk / 重连一次） |
| App API | `internal/pixivapi/app_api.go:26-32` | `DisableKeepAlives: true` | 中（Create/CreateWorkInfo/分页） |
| Browser API | `internal/pixivapi/browser_api.go:22` | `DisableKeepAlives: true` | 中 |
| Pixpedia API | `internal/pixivapi/pixpedia_api.go:21` | `DisableKeepAlives: true` | 低 |
| Login | `internal/pixivapi/login.go:128` | `DisableKeepAlives: true` | 极低（登录） |

**最大浪费在下载路径**：一次含 N 个 Range 请求的下载（续传/重试）产生 N 次独立 TLS 握手；高频启停下每次 Start/Resume 重建连接。`pixivRequestFn` 注释（:439）明确记录了"每次独立 Transport"是为规避代理 `Unsolicited response`。

### 1.2 目标

将下载路径的 `http.Transport` 提升为 **handler 级共享**（连接池 keep-alive），使 TLS 握手（~300ms）在多次 Range 请求与多次 Start/Resume 间摊销。

### 1.3 非目标

- **reader 复用**：正交，不做（见谱系节点 E，已放弃——reader 复用零收益）。
- 主程序/SDK 改动：无（F 全在 pixiv 插件内）。
- API client 的连接复用：次要范围（见 §5.3），优先级低于下载路径。

## 2. 障碍：代理 Unsolicited response

### 2.1 机制

Go `http.Transport` 从连接池取一条空闲连接复用时，若该连接已被代理/服务端单方面关闭、或残留上一请求的尾字节，Go 会读到"非预期响应"，打出 `Unsolicited response received on idle HTTP channel` 并丢弃该连接、重试一次。代理环境（尤其 HTTP/1.1 keep-alive + 代理连接回收）下该现象频发，历史上曾导致 pixiv 下载不稳定，故全插件采取"不复用"策略。

### 2.2 缓解手段（连接池调优，而非禁用）

| 参数 | 作用 | 建议值 |
|---|---|---|
| `IdleConnTimeout` | 空闲连接多久后关闭（默认 90s） | **缩短至 30s**，减少取到陈旧连接的概率 |
| `ResponseHeaderTimeout` | 等待响应头的最长时间 | 设定（如 30s），让挂在坏连接上的请求快速失败 |
| `MaxIdleConnsPerHost` | 每主机空闲连接上限 | 适度（如 4），避免池过大 |
| `ForceAttemptHTTP2` | 尝试 HTTP/2（多路复用，单连接多请求） | 开启（若 CDN 支持，可绕过部分 HTTP/1.1 代理问题） |
| `DisableKeepAlives` | 核武器（当前 API client 用） | **false**（否则 F 无意义） |

核心思路：**用"短 IdleConnTimeout + 响应超时"换取"连接活但陈旧"窗口的最小化**，保留复用收益的同时降低 Unsolicited response 概率。无法降到零——故交付模型需让用户可回退（§4）。

## 3. 设计方案

### 3.1 共享 Transport 注入

`PixivTaskHandler` 增字段，由 `Activate` 注入（与现有 tokenManager/logger 同模式；handler 在 `main.go:9` 字面量构造，`NewPixivTaskHandler` 为未调用的死代码，**不**通过它初始化）：

```go
type PixivTaskHandler struct {
    // ... 现有字段
    transport http.RoundTripper // 共享连接池 Transport（连接复用；由 Activate 注入，下载路径 Start/Resume 共用）
}
```

`Activate` 读 connectionReuse 设置后注入（见 §3.3/§5）：

```go
handler.transport = pixivapi.NewPixivTransport(!reuseEnabled) // reuseEnabled=true → disableKeepAlives=false
```

工厂 `NewPixivTransport`（N 已落地于 `internal/pixivapi/transport.go`）集中调参与代理注入，单一来源。

### 3.2 下载路径改造

`pixivRequestFn` 从"每次新建 Transport"改为"复用 handler 的 Transport"。因其是包级函数、当前签名 `pixivRequestFn(taskURL string)`，改造为接收 Transport：

```go
// pixivRequestFn 构造图片下载请求工厂；复用传入 Transport 的连接池，TLS 握手跨请求摊销。
func pixivRequestFn(taskURL string, transport http.RoundTripper) func(ctx context.Context, offset int64) (*http.Response, error) {
    client := &http.Client{Timeout: 5 * time.Minute, Transport: transport} // Client 轻量，Transport 共享
    return func(ctx context.Context, offset int64) (*http.Response, error) {
        // ... 现有 req 构造（Referer / Range）不变
        return client.Do(req)
    }
}
```

调用点（Start:328、Resume:534）改传 `h.transport`。`RetryReadCloser` 不变。

### 3.3 opt-in 开关（交付模型，见 §4）

实际工厂为 `NewPixivTransport(disableKeepAlives bool)`（N 已落地，参数语义与本文早期草稿的 `reuseEnabled` **相反**——以工厂实际签名为准）：
- `disableKeepAlives=false`（即 reuseEnabled=true）：调优的 keep-alive Transport——`IdleConnTimeout=30s`、`MaxIdleConnsPerHost=4`、`ResponseHeaderTimeout=30s`、`ForceAttemptHTTP2`（§2.2，缩短取到陈旧连接的窗口）。
- `disableKeepAlives=true`（即 reuseEnabled=false，默认）：禁用 keep-alive，等价当前行为，代理环境可靠默认。

> **F 需扩展工厂**：当前 `NewPixivTransport(false)` 仅设 `Proxy`+`DisableKeepAlives:false`，未设连接池调优参数；F 让复用模式补上 §2.2 的 IdleConnTimeout 等。

## 4. 交付模型选型（待 review 拍板）

| 模型 | 说明 | 优 | 劣 |
|---|---|---|---|
| **A. opt-in 插件设置，默认关**（推荐） | 新增 `extensions.settings` 项 `connectionReuse`（boolean，default false）。关=当前行为（可靠）；开=共享 Transport | 零回归（代理用户不受影响）；用户据自身网络决定；可逆 | 收益仅对开启者；多一项设置 |
| B. 始终启用共享 Transport | 删 `DisableKeepAlives`/per-request Transport，全用调优 Transport | 所有用户受益 | 代理环境回归风险；需充分实测才能上 |
| C. 按 Proxy 环境自动分流 | 检测 `HTTP_PROXY`/`HTTPS_PROXY`：有代理→保守，无→激进 | 自动适配 | 复杂；检测不可靠（pac/系统代理） |

**推荐 A**：F 优先级低（§6），opt-in 默认关 = 零回归交付，让网络条件好的用户主动获益，代理用户保持现状。后续若实测稳定可考虑切 B。

## 5. 改动点（全在 `library-squirrel-plugin-pixiv`）

### 5.1 主改动（下载路径）
1. `PixivTaskHandler` 增 `transport http.RoundTripper` 字段（task_handler.go:30）；`Activate` 读 connectionReuse 后注入 `handler.transport = NewPixivTransport(!reuseEnabled)`（`NewPixivTaskHandler` 为死代码，不用）
2. 扩展 `NewPixivTransport(disableKeepAlives)`（transport.go:45）：复用模式（false）补 `IdleConnTimeout`/`MaxIdleConnsPerHost`/`ResponseHeaderTimeout`/`ForceAttemptHTTP2`
3. `pixivRequestFn` 签名加 `transport http.RoundTripper` 参数（task_handler.go:442），内部 Client 复用传入 Transport；Start(task_handler.go:330)/Resume(task_handler.go:536) 调用点传 `h.transport`

### 5.2 opt-in 设置（模型 A）
4. `plugin.json` `extensions.settings` 加 `connectionReuse`（boolean，default "false"，group 网络）——主程序据此渲染开关，存 plugin_storage（统一 string，"true"/"false"）
5. `Activate`（activate.go:13 附近）读 `connectionReuse` 设置 → 决定 `reuseEnabled`

### 5.3 次要范围（API client，可选，建议二期）
6. `internal/pixivapi/*.go` 四个 client 的 `Transport` 改为接收外部注入（去掉硬编码 `DisableKeepAlives: true`），共享同一 handler Transport。频率低，收益小，可延后。

## 6. 优先级评估（诚实）

`doc/bug/频繁启停资源内容错位-Resume并发竞态.md` §74 已定调，F 维持**低优先级**：

- **收益小**：单次建连 ~300ms（TLS），相对几 MB 传输占比小；频繁启停是人操作（秒级频率），建连开销不构成瓶颈。
- **当前更可靠**：每次新建 Transport / `DisableKeepAlives` 在代理环境下规避了 Unsolicited response，是"安全默认"。
- **回归风险**：keep-alive 在代理环境的稳定性需实测，贸然 always-on（模型 B）可能引入难复现的下载失败。

**结论**：F 值得做但**不紧急**。推荐 opt-in（模型 A）零回归落地，把收益留给网络条件支持的用户，默认行为不变。若当前无代理实测条件，可仅交付本设计文档、实现延后。

## 7. 代理兼容性实测计划（实现后）

1. **直连环境**（无代理）：开启 connectionReuse，下载多图父任务 + 高频启停，确认 TLS 握手减少（抓包/日志）、无 Unsolicited response。
2. **代理环境**（HTTP/HTTPS proxy）：开启 connectionReuse，长时下载 + 空闲间隔，观察是否出现 `Unsolicited response received on idle HTTP channel`；调整 `IdleConnTimeout`（30s→更低）直至稳定或判定该代理不兼容（回退关闭）。
3. **回归**：默认关时行为与改动前完全一致（逐字节下载结果、启停无资源损坏）。

## 8. 不在范围

- reader 跨 RPC 复用（节点 E，已放弃）
- 主程序/SDK 连接治理（F 仅 pixiv 插件）
- HTTP/2 强制升级（依赖 CDN 支持，§2.2 仅 ForceAttemptHTTP2 试探）

## 9. 评估排除：lazy GetHeaders（非纯插件，不采用）

曾考虑让 GetHeaders 不再阻塞 Resume RPC、建连推迟到 copyLoop 首次 Read，以消除「恢复后进入进行中、进度推进延迟取决于网络」的体感延迟。**评估后排除**——非 pixiv 插件纯调整：

- **format 强依赖**：pixiv 的文件扩展名唯一来自 GetHeaders 的 `Content-Type`（`task_handler.go:552` detectFormat，URL 不含可靠扩展名）；主程序 `resolveMainPath`(`model.go:1869`)/`resolveStorePath`(`model.go:1881-1882,1912`) 在 Resume RPC 返回后**事务内立即**用 `spec.Format` 决定落盘路径/扩展名。lazy 化 → format 缺失 → 文件无名/错名，须改主程序 store 逻辑（占位名 + 首字节后 rename）。
- **size 协议可延后但消费点多**：`StoreSpec.Size` 协议支持 `-1 未知`（`handler_dto.go:73`），但主程序 `reportProgress`(`model.go:1723`) 用作进度分母、`handleEOF`(`model.go:1327`) 用作完整性校验；size=-1 时进度无分母、完整性退化为「未知」。补回真实 size 需在首 chunk 回传 → 改 SDK stream 协议 + 主程序 + 插件。

**结论**：lazy 必然跨出插件边界（动主程序 store/进度/完整性，可能动 SDK 协议），违背 F「纯插件、SDK/主程序不动」范围。恢复慢现象改由 F（handler 共享 Transport）缓解——它对症（见 §10）且纯插件。

## 10. 恢复慢定位（F 的对症依据）

现象：任务恢复后**状态很快进入「进行中」**，但**进度推进隔一段取决于网络的时长才开始**。

数据流验证（`backend/taskManager/model.go` resumeFromPersistedState）：
- `:1395 setState(Processing)` —— 前端见「进行中」（本地、快）
- `:1468 pluginExec.Resume` —— 插件 Resume RPC，内含 `GetHeaders`(`task_handler.go:539`) 到 pixiv CDN 的建连（DNS+TCP+TLS+Range 请求+等响应头，**唯一显著依赖外部网络**）
- 首次 `reportProgress`(`model.go:1265`) 在 copyLoop 首字节落盘后

故「进行中→首字节推进」间隔 ≈ GetHeaders 的 CDN 建连延迟（经代理 300ms~2s）。F 让短时暂停后 GetHeaders 复用 handler 池里的存活连接，省 DNS+TCP+TLS，把间隔压到一次 RTT。代理若激进回收空闲连接则复用失败、退回完整建连（该次无收益，但代价仅一次额外 RTT——幂等 GET 的 Unsolicited response 由 Go http.Client 透明重试，不进 reader 的 1s 退避）。
