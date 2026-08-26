# share 模块说明

## 一句话职责
把用户选中的作品/作品集发布为分享会话（复用 export 数据面收集 → 与盲转中继建立出站隧道注册会话 → 生成端到端加密分享链接），并管理会话生命周期（在线保活/断线重连/撤销/状态查询）。分享方 App 即内容源：App 运行期间链接可拉取，关闭即失效（设计语义）。

## 边界
- 与 export：export 是数据面（manifest 契约 + 收集/规划），share 是发起方与编排者——经接口注入 export 的 Collect/Plan 能力，不重复收集；share 不落盘打包（无 ZIP）。
- 与 import：收件人侧（拉取/回灌）归二期阶段4，本模块只有分享方（host）侧。
- 与 task 模块：一期不接入任务（会话状态由前端 UseShareStore + share-events 事件维护）；二期 share-host 任务化归任务模块接入（方案 G）。

## 对外接口（Handler）
| 方法 | 作用 |
| --- | --- |
| `SharePublish(workIDs, workSetIDs, options)` | 启动异步分享发布（立即返回 shareID；进度/完成/状态经 `share-events` 推送）。options：标题/有效期秒（-1=中继默认、0=无限期、>0=自定义）/访问密码 |
| `ShareCancelPublish(shareID)` | 取消进行中的发布（无则 no-op；已在线会话的停止走 Revoke） |
| `ShareRevoke(shareID)` | 撤销会话：在线经 REVOKE 帧直达中继即时生效；离线仅本地终止（中继侧存续至到期） |
| `ShareSessions()` | 全部会话快照（含终态，创建时间升序） |

## 核心概念
- **出站隧道**（frp 模式）：单条 TCP 长连承载全部收件人拉取流（HELLO register → WELCOME{token} → STREAM_OPEN/DATA/STREAM_CLOSE 多路复用 + PING/PONG 保活 + 断线 bind 重连）。线协议契约：`../library-squirrel-relay/PROTOCOL.md`（协议 version 1，帧 12 字节头，封闭帧类型白名单）。
- **会话状态机**：`connecting → online ⇄ reconnecting`（可逆）；`revoked / expired / failed`（终态不可逆，不重连）。中继 ERROR code 决定终态（revoked/expired/banned/not_found/malformed）或重试（rate_limited/limit/server_error）。
- **E2E 加密**：32 字节随机密钥 AES-256-GCM，记录 = `nonce(12) || GCM(明文)`、一帧一记录；**密钥不经中继**——只放链接 fragment（`https://{relay}/s/{token}#k=<base64url>`）。
- **流内应用层协议**（收件人侧二期实现，契约定稿于 session.go 头注）：请求单条 JSON 记录（`{"type":"manifest"}` / `{"type":"file","path":包内路径,"offset":N}`）；应答首记录 JSON 头（ok/kind/size/offset 或 error）+ 内容分块记录 + STREAM_CLOSE 收尾。
- **拉取白名单**（本机安全边界）：收件人请求的 path 仅作 `manifest.files[].path` 的 map 查找键，实际读文件只经 manifest 声明的 storePath——请求内容永不进入文件系统（路径穿越/任意读结构性排除）；并发流上限 8（对齐中继默认）+ 单流限速 16MiB/s（背压策略：半关闭请求-响应模式无回向确认通道，取 PROTOCOL.md §11.3 的「分块限速」分支）。
- **设备绑定实例 ID**：`config/share-instance-id` 持久化（16 字节 hex），中继溯源锚点；换机/重装即新实例。

## 依赖关系
- 依赖：`ExportCollector`/`ExportPlanner`（发起方定义接口，`export.Service`/`export.Packer` 实现，app.go 装配注入）；中继地址（`settings.shareSettings.relayAddress`）；workDir（settings）；Wails 事件发射器（`share-events` topic，延迟闭包读 emitter）。
- 被依赖：前端分享入口（MainView 多选操作栏 [分享] + 分享管理弹窗 + `UseShareStore`）。

## 关键设计
- **发布异步壳**（照 export runner 形态）：Publish 立即返回 shareID，goroutine 执行 Collect→Plan→生成密钥→注册（进度经 `share-events` progress 事件），首次在线（或终态失败）推 complete；会话随后常驻（不落库——App 退出即失效，持久化/恢复归二期任务接入）。
- **manifest 无 sha256**：分享不预计算全量哈希（发布即在线，避免大库发布前全盘读 IO 翻倍）；完整性由每记录 GCM 认证标签 + 应答头 size 字节计数守门（截断可检测）。
- **离线撤销的边界**：撤销是隧道帧，隧道断开期间无法送达——Revoke 离线路径仅本地终止，中继侧会话存续至有效期到期（或举报/管理端处置）。
- **中继地址两用**：TCP 拨号补默认端口 9527；链接 host 保留用户书写形态（官方中继经 443 反代时填裸域名即可）。
