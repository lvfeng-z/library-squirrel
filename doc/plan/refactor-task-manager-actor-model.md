# 重构:任务调度 per-task actor 化(及前置/配套改进)

> 状态:项一(插件 reader 响应 ctx)已落地;项三 A+B 阶段(actor 外壳+命令投递、删 `pauseCh` 统一 `runCtx` + 删 `liveGoroutines`)已实施(编译/测试全绿,运行时验证待做);项二(终态即时落盘)、项三 C 阶段(RPC 超时 + `GracefulShutdown` 用 `actorDone` + 删 `doneOnce`)待做。
> 前置:`refactor-task-single-goroutine-invariant.md`(dispatch 不变量重构)已完成。
> 本计划把 per-task actor 全重构作为**核心必做项**,并明确其与两项配套工作的依赖关系。新会话接手时**先读对应函数的当前代码**确认现状(行号会漂移,下文按函数名引用)。

## 背景与决策

dispatch 不变量重构(已完成)用 `dispatchState` CAS + `pendingResume` 补偿建立了"一任务至多一条 executeTask goroutine"不变量,消除了高频启停的并发失稳。目前 CAS + pendingResume 路径**无已知边界 bug**。

但该路径的并发安全靠"散落在 `ManagedTask` 多个 atomic 字段 + 补偿逻辑"维持,而非结构保证。`ManagedTask` 上挂了 `state`/`dispatchState`/`pendingResume`/`liveGoroutines`/`progressTotal`/`progressFinished` 等 atomic 字段,以及 `pauseMu`、`streamController.mu`、`doneOnce` 等锁/单次原语,每个不变量(无双 goroutine、不丢唤醒、pause/stop 一致性)都靠它们之间的精心协调来保证。这套模型留下三类**结构性脆弱**:

1. **滞后 goroutine + 陈旧标志**:任务 257 已发过——插件 `reader.Read` 卡在网络 ~4.4s 不响应 ctx 取消,滞后的 goroutine 退出时读到陈旧的 `pendingResume=true`(滞后窗口里用户 Resume 过一次),按陈旧意图重派发,违背用户随后又 Pause 的最新意图。当前靠 `PauseTaskTree`/`StopTaskTree` 开头对所有子任务 `pendingResume.Store(false)` 硬兜。这套补偿能 work 的前提是"每个破坏意图的入口都记得清标志",漏一个入口或出现新组合即复现。
2. **executeTask 退出契约是复杂状态机**(manager.go `executeTask` 的 defer):释放槽位 → 查 `pendingResume` → 判断可恢复 → 同 goroutine 重取槽位续跑或入队由别人调度 → `dispatchState` 跃迁。整套复杂度唯一源于"goroutine 按需创建、退出时要自己决定是否重 spawn"。
3. **多把字段级锁的顺序脆弱**:`newManagedTask` 的状态回调注释记录过一个**已发生的死锁**——`m.mu` → `refreshMu` 锁顺序与 `cleanupFinishedTask` 冲突,导致"并发完成时 cleanupFinishedTask 阻塞 → executeTask 不退出 → 信号量槽泄漏 → 并行度逐渐降到 1"。只要还有多把字段级锁,就有锁顺序踩雷的可能,且这类 bug 极难复现排查。

**决策**:不再等"CAS 路径暴露新边界 bug"才动手,主动用 actor 单 goroutine 模型把不变量从"靠协调维持"换成"靠结构保证"——所有任务级可变状态只在一条常驻 goroutine 内修改,命令通过 channel 投递、按序处理,从结构上消灭滞后重派发与丢失唤醒。

## 本计划的范围

三项工作,actor 化为核心,另两项为前置/配套:

- **项一(前置)**:插件 reader 响应 ctx 取消。I/O 层根因修复,是 actor 能干净落地的硬前提。
- **项二(配套)**:终态状态即时落盘。持久化策略,与 actor 正交。
- **项三(核心)**:per-task actor 全重构。含插件 RPC 超时设计、Pause/Stop 命令投递的天然并行。

## 实施顺序与依赖

**项一 → 项二 → 项三**。

| 依赖 | 说明 |
|---|---|
| **项一 → 项三(硬依赖)** | actor 的"命令串行、最新意图最后生效"只有在 actor 不被不可中断的 `reader.Read` 钉死时才成立。项一不落地,actor 处理 `download` 命令时卡在 `copyLoop` 的网络读里,`pause`/`stop` 命令塞进 `cmdCh` 也读不到,暂停延迟仍是秒级,结构优势无法体现。项一修 I/O 层,项三修调度层,缺一不可。 |
| **项二 ↔ 项三(无依赖)** | 持久化层与执行模型解耦,可任意先后。项二改动小、风险低,建议先做先受益。 |

项一跨三仓库(SDK + pixiv 插件 + 主程序受益但 `copyLoop` 无需改),发布周期长,应**最早启动**。若跨仓库发布滞后,主程序侧项二、项三可并行推进,三者汇合后做完整 stress 回归。**不能把项三当成项一的替代**——两者修的是不同层。

---

## 项一:插件 reader 响应 ctx 取消(前置 · 跨三仓库)

### 现状
- `copyLoop`(model.go)结构:`select { case <-m.pauseSignal()/m.ctx.Done(): ...; default: }` 后跟 `s.reader.Read(buf)`。`Read` 阻塞在网络,**不响应** `m.ctx` 取消。
- 实测:任务 257 setup 阶段被 Pause(cancel ctx)后,goroutine 仍卡在 `reader.Read` 约 4.4 秒才退出(插件 reader 等待网络帧)。
- 它是 `pendingResume` 陈旧重派发 bug 的载体(已用 Pause 清 pendingResume 兜底,但根上的滞后未消除),也是项三 actor 模型能否体现命令时序优势的前提。

### 根因
reader 由 SDK 的 `transport.serveSpecsPull` 提供(pull 模式):`reader.Read` 触发主程序向插件发一帧 `PullRequest`,再 `Recv` 一帧 `StreamChunk`。`Recv` 阻塞在网络,未在 `select` 中纳入 ctx。任务 ctx 取消时,reader 不会主动中断 Recv。

### 方案
- **SDK 侧**(`library-squirrel-sdk/transport/plugin_server.go`,`serveSpecsPull`):reader 的 `Read`/`Recv` 路径用 `select { case <-ctx.Done(): return error/EOF; case chunk := <-recvCh: ... }`,ctx 取消时关闭 bidi stream 并返回错误,使主程序 `copyLoop` 收到非 nil 错误后走 pause/cancel 分支。
- **插件侧**(`library-squirrel-plugin-pixiv/internal/download/retry_reader.go`):确认 reader 在 Recv 路径上能感知流关闭并快速返回(若 retry_reader 自己有阻塞读,也需纳入 ctx 或流关闭信号)。
- **协议侧**:ctx 取消关闭 bidi stream → 插件 `Recv` 收到 EOF → 插件清理该 task 的 reader。无需新协议帧。

### 责任边界与契约
取消意图经 gRPC bidi stream 传递,上游网络连接的处理完全在插件侧。明确以下分工:
1. **主程序只发取消意图**:cancel 任务 ctx + 关闭 bidi stream,不触达插件到站点的连接(也够不到——插件是独立子进程,上游连接句柄只存在于插件进程内)。
2. **reader 返回 EOF/错误 ≠ 必须关闭上游连接**。reader 返回是 gRPC stream 层面的事(插件停止向 stream 写数据 / 关闭 stream 发送端),与插件到站点的连接无关。**插件可以在保留上游连接的情况下让 reader 返回**——这是推荐的(HTTP keep-alive 连接池复用,避免高频启停下反复 TLS 握手)。
3. **插件的义务是"不泄漏内部资源",而非"关闭上游连接"**:取消时插件需让它阻塞在上游 `Body.Read` 的 goroutine 退出(把 ctx 传给上游 HTTP request,让 in-flight 读返回)。连接的去留是插件的实现自由(连接池复用 / 丢弃,由插件的 HTTP client 决策),主程序只关心"reader 返回了"这个结果。
4. **主程序对不合规插件的强制力有限**:只能关闭 stream 让 gRPC 调用在主程序侧返回错误;插件进程内部若不响应则主程序观察不到、管不着。故约束手段是**SDK 把"响应 ctx 的 reader"做进基线实现**(用 SDK 的插件默认合规)+ 契约文档,而非运行时强制。这是项一必须跨三仓库同步发布的根本原因。

### 改动点
- `library-squirrel-sdk/transport/plugin_server.go`:`serveSpecsPull` 的 Read 实现。
- `library-squirrel-plugin-pixiv/internal/download/retry_reader.go`:阻塞读路径。
- 主程序 `copyLoop` 无需改(已用 ctx.Done + Read 错误判定),受益于 reader 提前返回。

### 验证
- 任务下载中点 Pause,断言 `copyLoop` 在百毫秒级退出(而非等网络秒级)。
- `liveGoroutines`(model.go 的 atomic 断言字段)在 Pause 后及时归零。
- 跨三仓库协议一致性回归(pull 流程正常完成 + 取消两条路径)。

### 风险
- 跨三仓库(主/SDK/插件)行为变更,需同步发布;reader 关闭需幂等(可能被 copyLoop defer 与 ctx 取消双重关闭)。
- 取消时插件需让它阻塞在上游 `Body.Read` 的内部 goroutine 退出(把 ctx 传给上游 HTTP request),避免 goroutine 泄漏;**上游连接本身不必关闭**(可连接池复用,见上"责任边界与契约"),主程序不关心其去留。

---

## 项二:终态状态即时落盘(配套)

### 现状
- `addToPending`(manager.go)把状态写入 `pendingStatusUpdates`,由 `flushLoop` 每 200ms 批量刷库。
- **终态**(`Finished`/`Failed`/`PartlyFinished`)也走这条批量通道 → 崩溃窗口最长 200ms 内终态可能未落盘 → 重启后从 DB 读到的状态滞后。
- 注意:`addToPending` 对终态**已经**同步调用 `repo.UpdateRedownloadSections`(清空执行模式),但**状态本身**却是批量的——存在不一致。

### 方案
终态状态写**即时**落盘,非终态(`Paused`、进度)仍批量(避免进度高频写放大)。
- 在 `addToPending`:若 `isClearableTerminal(status)`,同步调 `repo.BatchSetStatus`(单条 map)即时写该任务状态,**不入** `pendingStatusUpdates`;非终态维持原批量逻辑。
- 即时写须与 `doFlush` 的批量写清晰区分:不能交叉(同任务终态即时写后,doFlush 不应再写过时批量值——可在即时写后从 `pendingStatusUpdates` 删除该 taskId 兜底)。

### 改动点
- `backend/taskManager/manager.go`:`addToPending` 分流(终态即时 / 非终态批量)。
- `backend/task` Repository:确认 `BatchSetStatus` 支持单条调用(已支持 map,传单条即可),或加 `SetStatus`。

### 验证
- 终态写入后立即 kill 进程,重启读 DB 状态正确(Finished/Failed)。
- 进度推送仍走批量(无写放大回归):`pendingProgressUpdates` 路径不变。
- `go test -race` 确认即时写与 flushLoop 无竞态。

### 风险
- 即时写增加单次 DB 写开销(终态低频,可接受)。
- 即时写与 doFlush 的并发顺序(见上"方案"兜底)。

---

## 项三:per-task actor 全重构(核心)

### 现状与根因
见本计划"背景与决策"。当前执行模型的并发安全靠散装 atomic 字段 + 补偿逻辑(`pendingResume`、退出契约、PauseTaskTree 清标志)维持,而非结构保证,存在滞后重派发、退出契约复杂、字段级锁顺序脆弱三类结构性脆弱。

### 方案

#### 1. 常驻 goroutine + command channel
每个 `ManagedTask` 持**一条常驻 goroutine**(对象创建时启动、终态时退出)与一个 command channel。命令集:`start` / `pause` / `resume` / `stop`。所有任务级可变状态的修改只发生在该 goroutine 内。

#### 2. 外部操作退化为命令投递
`PauseTaskTree`/`ResumeTaskTree`/`StopTaskTree`/`dispatch` 全部退化为"向 `cmdCh` 投递命令":
- `ResumeTaskTree` 投 `resume` 命令——**`pendingResume` 字段整体删除**。Resume 只是队列里的一条命令,channel 天然记忆,不存在"丢失唤醒";也不需要"退出契约检查 pendingResume 决定是否重 spawn",因为 actor 不退出(直到终态),命令按序处理,最新意图最后生效。
- `PauseTaskTree`/`StopTaskTree` 投 `pause`/`stop` 命令——也不需要开头"对所有子任务清 pendingResume"的补偿,因为命令队列里 `pause` 排在滞后的 `resume` 之后,actor 处理到 `pause` 时自然覆盖。

#### 3. dispatchState 退化为 started 标志
`dispatchState`(dsIdle/dsQueued/dsRunning 三态)退化为 actor 的 `started` 布尔(对象创建时启动 actor、终态时退出)。创建层 `claimTask`/`claimParent`(保证同一 taskId 只有一个 `ManagedTask` 对象)保留,仍是第一道闸。

#### 4. 信号量获取移入 actor
信号量槽位的获取/释放改由 actor 内部在处理 `start`/`resume` 命令时进行(actor 进入下载前取槽位、终态或暂停时释放)。`waitingQueue` 仍保留,但入队/出队由 actor 内部驱动而非外部 dispatch。

#### 5. 命令中断机制(依赖项一)
actor 处理 `start`/`resume` 命令时会进入长时间运行的 `downloadLoop`。为使 `pause`/`stop` 命令能中断在途下载:
- actor 为每条 `start`/`resume` 命令派生**子 ctx**,收到 `pause`/`stop` 命令时 cancel 子 ctx。
- 子 ctx 取消经 `copyLoop` 的 `select` 传到 `reader.Read`——**此处依赖项一**(reader 在 Recv 循环响应 ctx)。项一不落地,actor 仍会被网络读钉死,`pause`/`stop` 命令塞在 `cmdCh` 也处理不到。
- `pauseCh`/`pauseMu`(model.go 的暂停广播通道)随之删除,暂停信号统一走子 ctx。

#### 6. 插件 RPC 超时(actor 内部命令处理的子设计)
actor 处理命令时对插件 RPC(`CreateWorkInfo`/`Start`/`Resume`/`Pause`/`Stop`)加 context 超时,防止单个 actor 被卡住的 RPC 钉死自己的槽位。**注意:这些超时只覆盖 RPC 调用本身**的卡死;真正的数据字节传输发生在 `Start`/`Resume` 返回之后的 `copyLoop`(`reader.Read`)中,传输停滞由项一(reader 响应 ctx + actor 子 ctx cancel)兜底,不在 RPC 超时管辖内。按 RPC 的实际语义分三类:
- **元数据类**(`CreateWorkInfo`):插件请求站点 API 拉取作品元数据,站点 API 不响应时卡死。固定中超时(如 30s)。
- **建连类**(`Start`/`Resume`):这两者返回 `[]*sdkdto.StoreSpec`(含流式 `ReadCloser` 句柄),RPC 本身只做"建立到上游的连接 + 准备 reader"(握手 / TLS / 上游就绪),**不包含数据传输**。固定中超时(按建连窗口估,如 60s)。**不随 spec 总大小动态**——总大小是传输阶段的事,与建连窗口无关。
- **控制类**(`Pause`/`Stop`):通知插件暂停 / 停止,应快速返回。固定短超时(如 10s)。
- 实现:用 `context.WithTimeout(actorSubCtx, d)` 包裹调用前;超时返回的错误按现有失败路径处理(`comboFail`/`setFailed`)。`run()` 内已有自己的 panic recovery,加超时不破坏它。超时常量放 taskManager 包级常量(如 `const pluginCreateWorkInfoTimeout = 30 * time.Second`、`const pluginConnTimeout = 60 * time.Second`、`const pluginControlTimeout = 10 * time.Second`)。
- **不暴露为用户设置项**:超时是健壮性兜底参数(正常不应触发),取值按 RPC 语义在代码内确定;不同插件网络特性差异大(pixiv 远程 vs localImport 本地),全局用户设置无法适配。若将来确需逃生口,放 `config.yaml`(部署层)而非 `config/settings.json`(用户偏好),但建议先用常量(YAGNI)。

#### 7. PauseTaskTree/StopTaskTree 的天然并行
`PauseTaskTree`/`StopTaskTree` 第二阶段变为 `for child { child.cmdCh <- pauseCmd }`——命令投递本身是非阻塞的 channel 发送(纳秒级),各 child actor 独立 channel、无共享可变状态,**天然并行**。各 actor 收到命令后各自调 `pluginExec.Pause`/`Stop`,这些调用散在不同 actor goroutine 里自动并行。无需为"串行 gRPC 往返随子任务数线性放大"单独设计并行化。第一阶段(清队列、清 `waitingQueue`)仍在第二阶段之前,顺序不变;`cleanupStoppedTree` 仍在所有命令投递之后调用。

#### 8. 多流并发与进度的诚实表述
"字段级锁全部消失"需精确限定范围:
- **消失的是任务级状态机相关的锁/补偿**:`state`/`dispatchState`/`pendingResume`/`liveGoroutines` 的 atomic、`pauseMu`/`pauseCh`、`doneOnce`、`executeTask` 退出契约。
- **保留的是流级并发与高频进度**:`downloadLoop` 仍派生 per-stream goroutine(多流并发下载,不能倒退为串行);`streamController` 间的进度汇总可通过 channel 回报 actor 单线程汇总,或保留 `progressTotal`/`progressFinished` 为 atomic(供 `BuildSnapshot` 在 `Manager.mu` 下并发读 + 高频写,纯数据无状态机语义,保留 atomic 是合理例外)。实施时按"能消的锁尽量消、纯数据并发保留 atomic"的原则审查每个字段,避免过度承诺。

### 改动点
- `backend/taskManager/model.go`:`ManagedTask` 结构大改(常驻 goroutine + `cmdCh` + `started` 标志);`run`/`downloadLoop`/`copyLoop`/`Pause`/`Stop`/`prepareForResume` 按命令处理模型重写;删除 `pendingResume`/`pauseCh`/`pauseMu`/`dispatchState`(三态)/退出契约相关字段与逻辑。
- `backend/taskManager/manager.go`:`executeTask`/`dispatch`/`PauseTaskTree`/`ResumeTaskTree`/`StopTaskTree`/`dispatchFromQueue`/`removeFromQueue` 改为命令投递与 actor 驱动;`claimTask`/`claimParent` 保留。
- 包级超时常量(数据传输类 / 控制类)。

### 验证(结构化验收标准)
项三从"可选"升格为"必做",不再有"等具体 bug 复现"作为验收依据,改为结构化标准:
1. **结构正确性**:任意时刻每任务至多一条常驻 actor goroutine(扩展 `liveGoroutines` 断言);任务级字段级锁(`pauseMu` 等)与补偿字段(`pendingResume`、退出契约)确实删除——若残留说明 actor 化不彻底。
2. **命令时序**:stress 下"暂停后滞后的 goroutine 按陈旧意图重派发"永不再现(actor 从结构上消灭);`pause`/`stop` 命令在项一落地后端到端百毫秒级生效。
3. **隔离性**:单个任务插件 RPC 卡住(注入桩)只影响该任务自己的槽位,不拖累全局并行度(actor 隔离 + 超时)。
4. **回归门禁**:`model_multi_stream_test.go` 的 dispatch 不变量并发用例全绿;`go test -race` 无 data race(待 CGO 可用环境补跑)。

### 风险
- 大重构,回归面广:pull/diskOffset 链路、信号量公平性、GracefulShutdown、多流并发需重新验证。建议以现有 `model_multi_stream_test.go` 并发用例为强制落地门禁。
- actor 与多流并发的边界:per-stream goroutine 仍存在,stream 间共享状态(如进度汇总)需重新设计通信,避免引入新锁顺序问题。
- 超时取值需平衡(建连类太短会误杀慢握手 / 高延迟站点,控制类太长则无防护);超时触发后插件子进程可能仍在执行被放弃的操作(资源半写),需确认插件侧对超时取消的清理(与项一联动)。
- 命令投递的死锁风险:若 actor channel 满且投递方持锁等待,需保证投递路径不持有 `Manager.mu`(投递应非阻塞或独立 select)。

---

## 后续注记:调度不变量的作用域边界(实施后补充,2026-07-07)

项三建立的 per-task actor 不变量(创建层 `claim` + 派发层 `dispatch CAS`)只覆盖**主程序 taskManager 调度层**:保证同一 ManagedTask 不会有两次 dispatch 并发。它的**作用域未延伸到插件进程的 transport 层 goroutine**——`serveSpecsPull` 是 per-RPC goroutine,受 gRPC stream 控制,与主程序 actor 命令切换不同步。

这个缺口在 pixiv 插件 Resume 路径一(跨 RPC 复用缓存 reader)时暴露为真实 bug:旧 `serveSpecsPull` 残留 goroutine(阻塞在 `body.Read`)跨越 Pause→Resume 边界,与新 Resume 的 Probe/copyLoop 并发访问同一 reader,消费 `response.body` 并撕裂 `validBytes`,导致续传内容错位(详见 `doc/bug/频繁启停资源内容错位-Resume并发竞态.md`,修复:废弃 reader 跨 RPC 复用,每次 Resume 新建 reader)。

**教训(适用于未来任何调度/不变量重构):**
- "一任务一 goroutine"隐含了"被该任务驱动的可变对象(如 reader)单线程访问"这一派生假设,但它从未被显式声明、也未被守卫覆盖。
- 复用**跨 RPC 生命周期**的有状态对象(reader/连接/缓冲区)前,必须确认其所有访问者都在调度不变量的串行覆盖内;若访问者跨越 RPC 边界(插件 transport goroutine),必须让对象生命周期跟 RPC 走(每次新建),而非跨 RPC 缓存复用。
- 建立调度不变量时,应显式标注其作用域边界(覆盖哪些 goroutine、不覆盖哪些),避免派生假设在边界外静默失效。

## 关联
- 前置:`doc/plan/refactor-task-single-goroutine-invariant.md`(dispatch 不变量重构,已完成)。
- 资源损坏修复(已完成,跨三仓库 pull 协议):`doc/plan/fix-task-resume-data-corruption.md`。
- 本计划取代 `doc/plan/task-manager-followup-improvements.md`(该文档把 actor 列为可选储备项;本计划将其升格为核心必做,并整合其全部独立改进点)。
