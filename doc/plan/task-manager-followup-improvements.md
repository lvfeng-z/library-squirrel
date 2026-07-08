# [已废弃]任务调度后续改进(dispatch 不变量重构之后)

> 注意，本计划已废弃，本计划是 `refactor-task-single-goroutine-invariant.md`("其他延后"节)的细化,独立可接手。四项彼此独立,可分别实现。新会话接手时**先读对应函数的当前代码**确认现状(行号会漂移,下文按函数名引用)。

## 背景与共同证据

dispatch 不变量重构(已完成)消除了高频启停下"双 goroutine / Finished 被拉回 / 文件锁"等并发失稳。stress 回归仍暴露两类**非并发**问题,以及一项持久化窗口:

- **启停后状态切换有肉眼可见延迟**:根因是插件 gRPC 同步耦合 + 信号量排队,与已修复的 `pendingResume` 陈旧问题无关。
- **任务 257 在 setup-pause 后 goroutine 卡了 4.4 秒才退出**(日志 `doc/plan/...` 同目录的 stress 日志、原 `子任务执行未正确受到控制问题日志`):根因是插件 reader 的 `Read` 不响应任务 ctx 取消。

四项改进按建议顺序:**② reader ctx → ① gRPC 超时+并行 → ③ 终态即时落盘 → ④ actor 升级(仅必要时)**。

---

## ① 插件 gRPC 同步耦合:加超时 + 并行化

### 现状
- `executeTask` 持信号量全程同步调用插件 `Start`/`Resume`(经 `run()` → `runSectionCombo`/`resumeFromPersistedState`)。插件卡住 → 信号量槽被钉死 → 全局并行度下降。
- `ManagedTask.Pause()`(model.go)、`ManagedTask.Stop()`(model.go)在状态转换中**同步**调用 `pluginExec.Pause`/`Stop`。
- `PauseTaskTree`/`StopTaskTree`(manager.go)第二阶段**串行**遍历子任务调 `child.Pause()`/`child.Stop()`:N 个 Processing 子任务 = N 次串行 gRPC 往返,启停延迟随子任务数线性放大。

### 方案
1. **给所有插件 RPC 调用加 context 超时**:`CreateWorkInfo`/`Start`/`Resume`/`Pause`/`Stop`。建议分两类超时:
   - 数据传输类(`Start`/`Resume`):长超时(如 60s,或随 spec 总大小动态),保护"插件下载卡死钉死槽位"。
   - 控制类(`Pause`/`Stop`/`CreateWorkInfo`):短超时(如 10s),这些应快速返回。
   实现:用 `context.WithTimeout(ctx, d)` 包裹调用前;超时返回的错误按现有失败路径处理(`comboFail`/`setFailed`)。注意 `run()` 内已有自己的 panic recovery,加超时不破坏它。
2. **`PauseTaskTree`/`StopTaskTree` 子任务并行化**:第二阶段对每个 Processing/Pausing 子任务起 goroutine 调 `child.Pause()`(或 `Stop`),`sync.WaitGroup` 等全部返回。各 `child.Pause` 操作的是独立 `ManagedTask`,无共享可变状态(各自的状态机 + pauseCh + 插件句柄),可安全并行。
   - 注意:第一阶段(清队列、清 `pendingResume`)必须在第二阶段之前完成,顺序不变。
   - `cleanupStoppedTree`(Stop 末尾)仍在所有 Stop goroutine 之后调用。

### 改动点
- `backend/taskManager/model.go`:`Pause()`、`Stop()`、`runSectionCombo`(Start 调用处)、`resumeFromPersistedState`(Resume 调用处)。
- `backend/taskManager/manager.go`:`PauseTaskTree` 第二阶段、`StopTaskTree` 第二阶段(改并行)。
- 超时常量建议放 taskManager 包级常量(如 `const pluginPauseTimeout = 10 * time.Second`)。

### 验证
- 注入"插件 Pause 永不返回"的桩,断言 `Pause()` 在超时后返回、槽位不泄漏、任务进入 Failed/Paused。
- 启停含 N=7 子任务的父任务,断言总耗时 ≈ 单次 gRPC 往返(并行),而非 7×。
- `go test -race` 确认并行 Pause 无 data race。

### 风险
- 超时取值需平衡(下载类太短会误杀大文件,控制类太长无防护)。
- 并行 Pause 的日志时序交错(无功能影响)。
- 超时触发后插件子进程可能仍在执行被放弃的操作(资源已可能半写),需确认插件侧对超时取消的清理(与 ② reader ctx 联动)。

---

## ② 插件 reader 响应 ctx 取消(SDK + 插件,跨三仓库)

### 现状
- `copyLoop`(model.go)结构:`select { case <-m.pauseSignal()/m.ctx.Done(): ...; default: }` 后跟 `s.reader.Read(buf)`。`Read` 阻塞在网络,**不响应** `m.ctx` 取消。
- 实测:任务 257 setup 阶段被 Pause(cancel ctx)后,goroutine 仍卡在 `reader.Read` 约 4.4 秒才退出(插件 reader 等待网络帧)。
- 这放大了"滞后的 goroutine"窗口,曾是 `pendingResume` 陈旧重派发 bug 的载体(已用 Pause 清 pendingResume 兜底,但根上的滞后未消除)。

### 根因
reader 由 SDK 的 `transport.serveSpecsPull` 提供(pull 模式):`reader.Read` 触发主程序向插件发一帧 `PullRequest`,再 `Recv` 一帧 `StreamChunk`。`Recv` 阻塞在网络,未在 `select` 中纳入 ctx。任务 ctx 取消时,reader 不会主动中断 Recv。

### 方案
- **SDK 侧**(`library-squirrel-sdk/transport/plugin_server.go`,`serveSpecsPull`):reader 的 `Read`/`Recv` 路径用 `select { case <-ctx.Done(): return error/EOF; case chunk := <-recvCh: ... }`,ctx 取消时关闭 bidi stream 并返回错误,使主程序 `copyLoop` 收到非 nil 错误后走 pause/cancel 分支。
- **插件侧**(`library-squirrel-plugin-pixiv/internal/download/retry_reader.go`):确认 reader 在 Recv 路径上能感知流关闭并快速返回(若 retry_reader 自己有阻塞读,也需纳入 ctx 或流关闭信号)。
- **协议侧**:ctx 取消关闭 bidi stream → 插件 `Recv` 收到 EOF → 插件清理该 task 的 reader。无需新协议帧。

### 改动点
- `library-squirrel-sdk/transport/plugin_server.go`:`serveSpecsPull` 的 Read 实现。
- `library-squirrel-plugin-pixiv/internal/download/retry_reader.go`:阻塞读路径。
- 主程序 copyLoop 无需改(已用 ctx.Done + Read 错误判定),受益于 reader 提前返回。

### 验证
- 任务下载中点 Pause,断言 copyLoop 在百毫秒级退出(而非等网络秒级)。
- `liveGoroutines`(model.go 的 atomic 断言字段)在 Pause 后及时归零。
- 跨三仓库协议一致性回归(pull 流程正常完成 + 取消两条路径)。

### 风险
- 跨三仓库(主/SDK/插件)行为变更,需同步发布;reader 关闭需幂等(可能被 copyLoop defer 与 ctx 取消双重关闭)。
- 取消后插件侧的 HTTP 连接清理(避免连接泄漏)。

---

## ③ 终态即时落盘

### 现状
- `addToPending`(manager.go)把状态写入 `pendingStatusUpdates`,由 `flushLoop` 每 200ms 批量刷库。
- **终态**(`Finished`/`Failed`/`PartlyFinished`)也走这条批量通道 → 崩溃窗口最长 200ms 内终态可能未落盘 → 重启后从 DB 读到的状态滞后。
- 注意:`addToPending` 对终态**已经**同步调用 `repo.UpdateRedownloadSections`(清空执行模式),但**状态本身**却是批量的——存在不一致。

### 方案
终态状态写**即时**落盘,非终态(`Paused`、进度)仍批量(避免进度高频写放大)。
- 在 `addToPending`:若 `isClearableTerminal(status)`,同步调 `repo.BatchSetStatus`(单条 map)或新增 `repo.SetStatus` 即时写该任务状态,**不入** `pendingStatusUpdates`;非终态维持原批量逻辑。
- 即时写须在 `pendingMu` 之外或清晰区分:不能与 `doFlush` 的批量写交叉(同任务终态即时写后,doFlush 不应再写过时批量值——可在即时写后从 `pendingStatusUpdates` 删除该 taskId 兜底)。

### 改动点
- `backend/taskManager/manager.go`:`addToPending` 分流(终态即时 / 非终态批量)。
- `backend/task` Repository:确认 `BatchSetStatus` 支持单条调用(已支持 map,传单条即可),或加 `SetStatus`。

### 验证
- 终态写入后立即 kill 进程,重启读 DB 状态正确(Finished/Failed)。
- 进度推送仍走批量(无写放大回归):`pendingProgressUpdates` 路径不变。
- `go test -race` 确认即时写与 flushLoop 无竞态。

### 风险
- 即时写增加单次 DB 写开销(终态低频,可接受)。
- 即时写与 doFlush 的并发顺序(见上"方案"第二条兜底)。

---

## ④ per-task actor 全重构(可选,仅必要时)

### 触发条件
仅当 ① ② ③ 落地后,若 CAS+pendingResume 路径仍暴露边界问题(目前无),才考虑。**当前不建议做**。

### 方案概要
- 每个 `ManagedTask` 持**一条常驻 goroutine** + command channel(`start`/`pause`/`resume`/`stop` 命令);所有可变状态只在该 goroutine 内修改 → 字段级锁全部消失。
- `Pause`/`Resume`/`Stop`/`dispatch` 退化为"向 channel 投递命令";`dispatchState` 退化为 actor 的 `started` 标志(对象创建时启动 actor,终态时退出)。
- 信号量获取改由 actor 内部在处理 `start`/`resume` 命令时进行。

### 代价
实质重写 executeTask/downloadLoop/Pause/Stop 的执行模型;需重新验证 pull/diskOffset 链路、信号量公平性、GracefulShutdown。

### 改动点(若实施)
- `backend/taskManager/model.go`:`ManagedTask` 结构大改(常驻 goroutine + cmdCh)。
- `backend/taskManager/manager.go`:`executeTask`/`dispatch`/`PauseTaskTree`/`ResumeTaskTree`/`StopTaskTree` 改为命令投递。

### 风险
大重构,回归面广。决策前应明确列出"CAS 路径无法解决"的具体场景,否则不必动。

---

## 关联
- 来源:`doc/plan/refactor-task-single-goroutine-invariant.md` "其他延后(独立计划)"节。
- 已修复的相关 bug:`pendingResume` 陈旧重派发(高频启停"单独 Processing"),见重构计划状态段。
- 资源损坏修复(已完成,跨三仓库 pull 协议):`doc/plan/fix-task-resume-data-corruption.md`。
