# share 模块说明

## 一句话职责
把用户选中的作品/作品集发布为分享会话（复用 export 数据面收集 → 与盲转中继建立出站隧道注册会话 → 生成端到端加密分享链接），并管理会话生命周期（在线保活/断线重连/撤销/取消/状态查询）；发布**直跑**（Service 内受监督 goroutine 驱动，不经任务模块），生命周期自首次在线起持久化为 **share_record 分享记录**（active/revoked/expired/failed），有效期内每次启动 App 自动复原（原 token bind 重绑，链接不变）。收件侧接收分享链接（`library-squirrel://` 深链拉起 / 手动粘贴）经中继拉取分享方数据并回灌导入本库，仍建模为 **share-receive 任务**（断点续传/暂停恢复/重试归任务标准能力）。

## 边界
- 与 export：export 是数据面（manifest 契约 + 收集/规划），share 是发起方与编排者——经接口注入 export 的 Collect/Plan 能力，不重复收集；share 不落盘打包（无 ZIP）。
- 与 import：收件侧回灌入库经注入的 `ManifestIngestor` 能力（与 import handler 同一实例，app.go 装配）——share 负责拉取/暂存/E2E 解密与编排，入库/查重/落盘全链复用 import。
- 与 task/taskManager：仅**收件拉取**（`Receive`）创建任务并立即启动（task_type=`share-receive`，经 `BuiltinTaskControl` 能力接口由 task.Service+taskManager 装配实现，执行面为 `ReceiveExecution`）；分享方发布不走任务（语义错位：任务面板心智是「有始有终的工作」，隧道维持是长驻服务）——发布/复原主体为 Service 内 goroutine（panic recover 监督），状态经 share-events 实时推、跨重启语义由 share_record 承载。
- 会话运行态为进程内注册表；分享参数与终态持久化于 share_record（运行态 connecting/online/reconnecting 不入表）。

## 对外接口（Handler）
| 方法 | 作用 |
| --- | --- |
| `SharePublish(workIDs, workSetIDs, options)` | 发布分享（直跑）：立即返回 shareID（`share-{毫秒时间戳}`），进度/完成/会话状态经 `share-events` 推送。options：标题/有效期秒（-1=中继默认、0=无限期、>0=自定义）/访问密码（落记录行的只有有无标记，摘要属运行时载荷） |
| `ShareCancelPublish(shareID)` | 取消发布中的分享（发布弹窗「取消」）：直接终止会话主体；未到首次在线不产生记录行（发布中止经 `share-events` 推 complete 失败收尾） |
| `ShareRevoke(shareID)` | 撤销分享：在线（会话在注册表）经 REVOKE 帧直达中继即时生效；离线（active 记录行无在驻会话）本地落 revoked——中继侧存续至到期 |
| `ShareSessions()` | 全部会话快照（含终态，创建时间升序；运行态实时源，视图按 share_id 与分享记录关联） |
| `ShareRecords()` | 全部分享记录（历史分享账本：状态/链接重建要素/规划统计/失败原因，create_time 倒序） |
| `ShareDeleteRecord(shareID)` | 删除分享记录（物理删行；在驻会话先撤销——活跃分享删除即链接失效，离线记录仅本地删行） |
| `ShareReceive(link, password)` | 创建并启动 share-receive 任务（返回任务 ID）。link 接受深链 `library-squirrel://share/{中继}/{token}#k={密钥}` 与落地页链接 `https://{中继}/s/{token}#k={密钥}`；password 为设有访问密码时的明文（落载荷只有摘要） |
| `ShareConsumePendingLink()` | 取走深链到达时缓存的待处理链接（前端启动衔接：深链事件先于前端就绪的兜底；空串=无） |
| `ShareProtocolRegStatus()` / `ShareUnregisterProtocol()` | 深链协议注册状态查询 / 取消自注册（便携版无卸载器的清理入口；仅 HKCU 视图） |

## 核心概念
- **出站隧道**（frp 模式）：单条 TCP 长连承载全部收件人拉取流（HELLO register/bind → WELCOME{token} → STREAM_OPEN/DATA/STREAM_CLOSE 多路复用 + PING/PONG 保活 + 断线 bind 重连）。线协议契约：`../library-squirrel-relay/PROTOCOL.md`（协议 version 1，帧 12 字节头，封闭帧类型白名单）。
- **会话状态机**：`connecting → online ⇄ reconnecting`（可逆）；`revoked / expired / failed`（终态不可逆，不重连）。中继 ERROR code 决定终态（revoked/expired/banned/not_found/malformed）或重试（rate_limited/limit/server_error）。
- **发布直跑**（`Publish` → `hostSessionBody`）：收集→规划→注册（新密钥/新 token）→首次在线推 complete + 落 active 记录行→隧道维持（goroutine 常驻）至会话终态（revoked→记录 revoked、expired→expired、failed→failed 记原因）。进度按阶段经 share-events 推送（collecting → registering → online complete）。取消（`CancelPublish`）直接取消主体 ctx：未在线推「已取消」收尾弹窗、不落记录；已在线则会话本地移除（中继侧存续至到期，下次启动复原拉起）。主体 goroutine 以 recover 包装监督：panic 时推 complete 失败/记录落 failed 收尾。
- **分享记录（share_record）**：分享方持久化账本。写入点：注册成功（首次 online）落 active 行（token/密钥/标题/分享对象/中继地址/有效期配置/规划统计）；撤销→revoked（记撤销时刻）；到期/中继终态→expired；复原失败→failed 记原因。运行态不入表（内存 session + state 事件实时推，视图按 share_id 关联两源）。E2E 密钥明文入表（本地库本含全部内容）；密码只记有无标记。
- **启动自动复原（RestoreAll）**：app 启动稳定后（onDomReady）对全部 active 行逐条独立 goroutine 执行「重新收集（work_ids）→ 规划 → bind 重绑（原 token，PROTOCOL.md bind 语义）」——链接不变、剩余有效期由中继侧会话管理（bind 不重传 expiresAt）。复原失败按原因落终态：收集失败（作品已删）/中继 not_found→failed；中继不可达→会话重连循环（reconnecting 不落表，下次启动重试）。
- **share-receive 任务生命周期**（`ReceiveExecution`）：拉 manifest → **查重确认（方案《分享接收查重接入普通下载轨道》设计五）**——manifest 作品键 + 板块角色三分类，命中冲突弹覆盖确认（WaitReplaceConfirm，复用 ConfirmReplace 整体答复），零交集命中静默并入自动增补，确认替换后按交集角色软删旧 store 并经 SetTerminalRollback 登记回滚清单 → 逐文件拉取暂存（被裁决跳过作品的文件不拉）→ ManifestIngestor 回灌导入（替换集经 IngestOptions 注入）→ 成功清理暂存置 Finished。**断点续传锚 = 暂存文件已落盘字节数**（完整=跳过、部分=按 offset 续传、异常残留=截断重拉）；瞬态错误（网络/分享方离线/中继限流）退避重试（1s→8s×3 次）后置失败，暂停/停止保留暂存（恢复/重试从暂存续传，非中止清理——决策12）。**软删后中断窗口三态收口**（设计六）：暂停→恢复重跑重查落零交集分类延续替换不重弹；停止/失败经控制面 setFailed 单点按登记清单复活软删行；软删前中断（含跳过）重试重新弹窗。分享会话终态（撤销/过期/不存在）→ 失败终态+用户可读文案（「分享已失效」）；密码错误→「访问密码错误」。进度按字节上报（total=清单声明总字节）。
- **收件人拉取协议**（`receive_client.go`）：每次请求一条 TCP 连接（= 隧道内一条流，streamID 恒 1）——HELLO(role=recipient, token, instanceId, passwordHash) → 单条加密请求记录 → STREAM_CLOSE 半关闭 → 读加密应答至对端关流；应答首记录 JSON 头校验（kind/offset/size 与请求锚一致）+ 字节计数守门（截断可检测）。
- **深链拉起**（三通道汇入 `NotifyIncomingLink`）：① 已运行实例转发（wails SingleInstance 二启 WM_COPYDATA 转发 argv，main.go 回调）② 冷启动 URL 事件（`ApplicationLaunchedWithUrl`）③ argv 前缀扫描兜底（dev 模式事件判定不满足）。深链轻校验后缓存+推 `share-events` receive-link 事件；事件先于前端就绪时由 `ShareConsumePendingLink` 消费式拉取兜底；窗口期同链去重。深链二启早退探针（Windows，main.go）：argv 带深链且单实例互斥体已被占时以极简应用实例转发退出，跳过重初始化。
- **协议注册**：安装版经 build/config.yml `protocols` 渲染 NSIS 安装/卸载段（HKLM）；便携版每次启动幂等自写 HKCU\Software\Classes（exe 移动后悬空路径自愈；被他软件占用记日志跳过），设置页提供取消注册清理入口。
- **任务载荷**（`task.payload`）：share-receive `{schemaVersion, relayDial, relayHost, token, keyB64, passwordHash}`（E2E 密钥与密码摘要落本机任务行，属收件人自有数据）；版本自管，高于支持版本 fail-fast；密码明文不落库。
- **E2E 加密**：32 字节随机密钥 AES-256-GCM，记录 = `nonce(12) || GCM(明文)`、一帧一记录；**密钥不经中继**——只放链接 fragment（`https://{relay}/s/{token}#k=<base64url>`；深链同构 `library-squirrel://share/{relay}/{token}#k=<base64url>`），发布方另入 share_record（复原凭原密钥 bind，链接不变）。
- **流内应用层协议**（契约定稿于 session.go 头注）：请求单条 JSON 记录（`{"type":"manifest"}` / `{"type":"file","path":包内路径,"offset":N}`）；应答首记录 JSON 头（ok/kind/size/offset 或 error）+ 内容分块记录 + STREAM_CLOSE 收尾。
- **拉取白名单**（本机安全边界）：收件人请求的 path 仅作 `manifest.files[].path` 的 map 查找键，实际读文件只经 manifest 声明的 storePath——请求内容永不进入文件系统（路径穿越/任意读结构性排除）；并发流上限 8（对齐中继默认）+ 单流限速 16MiB/s（背压策略：半关闭请求-响应模式无回向确认通道，取 PROTOCOL.md §11.3 的「分块限速」分支）。
- **暂存清扫**：启动时 `CleanupOrphanReceiveStaging` 回收任务行已不存在的暂存目录（成功任务执行尾已自清；任务删除/崩溃残留由此兜底，任务行仍存在的暂存保留供续传）。
- **设备绑定实例 ID**：`config/share-instance-id` 持久化（16 字节 hex），中继溯源锚点（收发两用）；换机/重装即新实例。

## 依赖关系
- 依赖：`Repository`（share_record 仓储，app.go 装配注入）；`ExportCollector`/`ExportPlanner`（发起方定义接口，`export.Service`/`export.Packer` 实现，app.go 装配注入）；`ManifestIngestor`（import 能力接口，share-receive 执行器注入）；`BuiltinTaskControl`（share-receive 任务创建/启动能力，app.go 以延迟闭包适配器组合 task.Service + taskManager.Manager——ShareService 先于二者创建）；中继地址（`settings.shareSettings.relayAddress`，收件侧中继取自链接；复原取记录行的中继地址——链接所指，不受当前配置切换影响）；workDir（settings）；Wails 事件发射器（`share-events` topic，延迟闭包读 emitter）。
- 被依赖：taskManager（`ReceiveExecution` 按 task_type 注册进 Manager 执行面策略表，app.go 装配）；main.go/app.go（深链三通道接线 + 单实例回调 + onDomReady 触发 RestoreAll）；前端分享入口（MainView 多选操作栏 [分享] 发布弹窗 + 分享视图 `ShareManage`（菜单「分享」：进行中会话区/历史记录双源，接收分享入口）；`UseShareStore`/`UseShareReceiveStore` + MainLayout 挂载接收对话框）。

## 关键设计
- **发布直跑、收件任务化**：发布是长驻服务语义（隧道维持无「完成」时刻），直跑 goroutine + share_record 承载跨重启；收件是有始有终的数据搬运，任务模块承接暂停/续传/重试。`Publish` 前置校验（选择非空/中继已配置/workDir 非空）通过即返回 shareID（`share-{毫秒时间戳}`，同毫秒顺延保唯一），主体异步执行。
- **manifest 无 sha256**：分享不预计算全量哈希（发布即在线，避免大库发布前全盘读 IO 翻倍）；完整性由每记录 GCM 认证标签 + 应答头 size 字节计数守门（截断可检测）。
- **暂存即续传状态**：不另记传输状态文件——暂存文件大小对齐 manifest 声明即完成、偏差即续传锚（每分块 GCM 认证保证大小对齐的文件即密码学完整）；分享方现报缺失的条目置清单 Missing 走导入缺席降级（对齐导出决策4）。
- **离线撤销的边界**：撤销是隧道帧，隧道断开期间无法送达——离线撤销仅本地终态（记录行 revoked，复原不再拉起），中继侧会话存续至有效期到期（或举报/管理端处置）。删除活跃分享同边界：在线删除撤销帧直达中继即时失效，离线删除仅本地删行。
- **中继地址两用**：TCP 拨号补默认端口 9527；链接 host 保留用户书写形态（官方中继经 443 反代时填裸域名即可）。
