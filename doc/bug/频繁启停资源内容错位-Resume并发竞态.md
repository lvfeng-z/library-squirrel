# 频繁启停资源内容错位 → Resume 路径一 reader 并发竞态(已定位)

> 类型:已确认缺陷(根因定位 + 方案A修复落地)
> 定位日期:2026-07-07
> 状态:根因已定位,修复(方案A:废弃 Resume 路径一)已实施并经运行时验证确认(2026-07-07,高频启停资源损坏不再复现)
> 严重程度:高(用户实测:文件大小正确但 JPEG 底部窄带错位/变色)
> 日志:`log/pixiv任务频繁启停出现资源损坏(资源大小与正常资源相同但是图像出现错位或变色)-2.log`
> 演进(2026-07-08):方案A 修复的是 reader 并发竞态(reader 不跨 RPC);与之正交的"在途 chunk 常态丢失"已由优雅暂停消除(常态排空落盘、磁盘 stat 对齐中断点;仅 drain 超时 2s 兜底时有损),见 `doc/plan/pause-drain-inflight-chunk.md`。方案A 的并发不变量不变。

## 现象

pixiv 父任务(254,7 个子任务)高频暂停/恢复后,部分下载完成的图片资源**文件大小正确,但 JPEG 解码错位/变色**,错位集中在**底部较窄区域**(任务258 错位量约 12288 字节,占 1.4MB 文件的 0.9%)。

## 根因:Resume 路径一复用 reader,与旧 serveSpecsPull 残留 goroutine 并发

`[serveSpecsPull] ctx 取消...` 诊断日志排除了 serveSpecsPull 自身 select 的丢包窗口(均走"Close 中断"分支,非"丢包窗口命中")。即便丢包窗口触发,也符合 Pause 的数据持久化边界(在途 chunk 允许丢失,Resume 从磁盘 stat 重下兜底)——它不是缺陷,详见 `doc/plugin-dev-guide.md`「Pause 的数据持久化边界」。真凶是**跨 Pause→Resume 边界的 reader 并发访问**。

### 字段撕裂铁证

`RetryReadCloser.ensureResponse` 建连日志 `offset=r.validBytes`(打印时刻)、`contentLength=resp.Header`(本次请求响应),二者必须自洽。任务258 多次 Resume 出现撕裂:

| 日志行 | offset | contentLength | 自洽 |
|---|---|---|---|
| L190(第一次 Resume Probe) | 171220 | 1238474 | ✓(1409694−171220) |
| **L195**(第一次 Resume Probe) | **183508** | **1238474** | ✗(183508 应得 1226186) |
| L240(第二次 Resume Probe) | 511188 | 898506 | ✓ |
| **L244**(第二次 Resume copyLoop) | **548052** | **898506** | ✗(548052 应得 861642) |

撕裂只能由"`requestFn` 用 validBytes=171220 发出请求、但 `log.Printf` 执行前另一 goroutine 把 `r.validBytes` 改成 183508"产生。`r.validBytes` 只在 `Read` 的 Range 分支 `+= n`(retry_reader.go:247)——**确凿有一个并发的 `reader.Read`**。

### 并发源:旧 serveSpecsPull 残留 goroutine

该并发 Read 不是 Probe(Probe 不读 body)、不是 copyLoop B(ResumeMount L198 之后才启动,L195 在它之前)——只能来自**首次 Start 的 serveSpecsPull 残留 goroutine**:它的最后一个 `reader.Read` 阻塞在 `body.Read`(pixiv 慢),迟至 Resume Probe 期间(约3.5秒后)才完成。日志 L195 的字段撕裂就发生在它完成的瞬间。

### 错位机制

1. 残留 goroutine 的 `reader.Read` 完成 n 字节 → `validBytes += n`、**消费了 response.body 的 n 字节**
2. 它 `send(Data n)` 给**已退出的 copyLoop A** → chunk 丢失(走 `case res` 分支 send 失败,不打"丢包窗口命中"日志)
3. Resume 路径一 Probe 建的新 response.body,**前 n 字节已被残留 goroutine 消费**
4. copyLoop B 读 response.body,实际从第 n 字节开始(=原图 offset+n 处),却写到文件 offset
5. 文件 [offset, …] 内容整体前移 n 字节 → **错位 n 字节**(12288),数据量靠后续 Resume 补齐到完整大小 → 文件大小正确、视觉窄带错位

### 反证:第三次 Resume 干净

L288-298:`SetValidBytes(1375444)` 重置后,Probe 建连 offset=1375444、contentLength=34250 **完全自洽**,无撕裂、无虚高——此时残留 goroutine 早已退出。这次数据正确,任务 Finished。

## 架构教训:调度不变量作用域未覆盖 reader 访问者

per-task actor 重构建立了"一任务一 goroutine"不变量(创建层 claim + 派发层 dispatch CAS),但它**只覆盖主程序 taskManager 调度层**。reader 的实际访问者是**插件进程的 serveSpecsPull goroutine**(per-RPC,受 gRPC stream 控制),与主程序 actor 命令切换**不同步**——不在守卫范围内。

主程序 actor 认为"旧 copyLoop 已退出 ⇒ reader 空闲 ⇒ Resume 可安全复用",但看不见插件侧那个阻塞在 `body.Read` 的残留 goroutine,它持有 reader 引用,跨越 Pause→Resume 边界。**频繁启停**制造了这个窗口(Pause→Resume 间隔短,残留 goroutine 未退出);正常使用间隔长,残留早已退出,不触发。

被打破的不是调度不变量本身,而是它的**派生假设**(reader 单线程访问),该假设从未被显式声明、也未被守卫覆盖。这也解释了之前所有修复(Store 前 check ctx、SetCtx 收尾)都无效——它们堵的是 reader 状态一致性,而根因是 reader 的**并发访问**本身。

## 关键事实:路径一从未带来连接复用收益

`pixivRequestFn`(task_handler.go)`requestFn` 每次调用都 `&http.Client{Transport: &http.Transport{...}}`——**新建 Transport = 新连接池 = 不复用 TCP/TLS 连接**。即便路径一复用了 reader 对象,reader 内部建连仍每次新建 Transport。所以**路径一是"纯风险、零收益"的代码**:复用的只是 `validBytes` 计数器(Resume 时还被 `SetValidBytes` 重置覆盖),却引入了跨 RPC 的并发竞态。这也是方案A 零代价的依据。

## 修复(方案A,已实施)

废弃 Resume 路径一,每次 Resume 新建 reader(原路径二逻辑):
- reader 不跨 RPC ⇒ 只被当前 serveSpecsPull 单一 goroutine 访问 ⇒ 消除跨 Pause→Resume 边界的并发
- 与 per-task actor 串行不变量对齐(可变状态只在一个串行上下文中访问)
- 零代价:连接本就没复用,reader 复用也无实质收益

改动:`library-squirrel-plugin-pixiv/task_handler.go` `Resume` 删除路径一(LoadAndDelete + Probe 复用),总走新建 reader + SetValidBytes + GetHeaders + Store。

## 待办:pixiv 插件连接复用(路径一清理之后,暂不实现)

方案A 把 reader 不复用做对了,但 Transport 层仍每次新建(连接不复用)。若未来建连开销成为瓶颈,可做连接复用:
- 提升 `http.Transport` 为 handler 级共享(连接池自动 keep-alive 复用空闲连接)
- reader(应用层状态)与 Transport(连接层资源)正交,可独立演进——方案A 不阻塞连接复用
- **障碍**:`pixivRequestFn` 注释记录的历史教训——代理环境下 keep-alive 连接复用会触发 Go 的 `Unsolicited response received on idle HTTP channel`(代理把已关闭/错配的连接派发给新请求)。需实测代理兼容性,可能需对代理 Transport 单独配置(更激进的连接健康检查 / `IdleConnTimeout` / `ResponseHeaderTimeout`)
- **优先级低**:单次 Resume 建连约 300ms(TLS),相对几 MB 传输占比小;频繁启停是人操作(秒级频率),建连开销不构成瓶颈;当前"每次新建 Transport"在代理环境下反而更可靠

## 验证

- [x] `CGO_ENABLED=0 go build` pixiv + SDK 通过
- [x] gofmt 合规
- [x] 运行时验证(2026-07-07):pixiv 父任务高频启停复现,资源损坏不再出现;`[PixivResume] 新建 reader` 日志为单一路径(无"路径一/路径二"分支)

## 本次修复引出的其他待办

1. **核查其他插件的 reader 复用**(✅ 已完成,2026-07-07):核查 localImport,其 `Resume` 每次新建 `*os.File`(`os.Open` + `Seek(offset)`),不复用缓存 reader;`readers sync.Map` 仅用于 Pause/Stop 的 `closeReader` 关闭当前句柄。无同类竞态,无需修复。两个插件中仅 pixiv 曾有路径一,已修。
2. **架构教训同步**(✅ 已完成,2026-07-07):已记入 `doc/plan/refactor-task-manager-actor-model.md` "后续注记:调度不变量的作用域边界"节 + memory(`actor-invariant-scope-gap.md`)。
3. **bug A 独立修复**(✅ 已完成,2026-07-08):`retry_reader.GetHeaders` 新增 `fullSize`,206 据 Content-Range 还原完整大小;`reportProgress` total 公式连锁调整。详见 `doc/bug/specSize取剩余字节-Resume完整性校验失效.md`。
4. **Probe/Unpause 清理**(✅ 已完成,2026-07-08):核查确认 `Probe`/`Unpause` 是 pixiv `internal/download/retry_reader.go` 的方法(非 SDK),路径一删除后无任何调用方,已按死代码删除;`Pause`/`ValidBytes`/`SetCtx` 仍在用,保留。
5. **serveSpecsPull 诊断日志去留**(✅ 已完成,2026-07-08):精简为只保留"丢包窗口命中"(n>0)作长期哨兵;常态(Read 阻塞或 n=0)静默,降低 SDK 公共库日志噪音。

## 相关

- 配套缺陷(必要条件,使错位文件能过校验判 Finished):`doc/bug/specSize取剩余字节-Resume完整性校验失效.md`
- 调度不变量背景:`doc/plan/refactor-task-manager-actor-model.md`(per-task actor 重构)
- 代码定位:
  - `library-squirrel-plugin-pixiv/task_handler.go` `Resume`(已重写,删路径一)、`pixivRequestFn`(每次新建 Transport,连接不复用的现状)
  - `library-squirrel-plugin-pixiv/internal/download/retry_reader.go` `Read`(Range 模式 validBytes += n)、`ensureResponse`(建连日志 offset 字段撕裂点)
  - `library-squirrel-sdk/transport/plugin_server.go` `serveSpecsPull`(per-RPC goroutine,ctx.Done 丢包窗口诊断日志)
  - `backend/taskManager/model.go` `resumeFromPersistedState`(主程序侧续传,writeOffset = 磁盘 stat)
