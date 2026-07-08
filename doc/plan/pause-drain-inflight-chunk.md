# 优雅暂停:排空在途 chunk,使磁盘 stat 对齐中断点

> 状态:**已实施并经运行时验证(2026-07-09)** —— 暂停/恢复延迟降至毫秒级,频繁启停无资源损坏。实施记录与偏离原文的修正见第 12 节。
> 类型:架构改进(暂停语义从「立即切断·在途可丢失」升级为「排空在途·无损对齐」)
> 范围:主程序 `backend/taskManager` 为主;SDK 基本不动;插件不动
> 关联:`doc/bug/频繁启停资源内容错位-Resume并发竞态.md`(方案A 已修复并发竞态,本方案消除在途丢失)
> 注:本方案**不涉及 reader 复用,也不为连接复用打基础**——连接复用走共享 Transport,与 reader 复用正交(详见 7.5)

## 1. 背景与目标

### 1.1 现状

方案A 已消除 Resume 路径一 reader 跨边界并发竞态(高频启停资源损坏根因)。但当前暂停策略仍遗留一个特性:**立即取消 gRPC stream**,导致在途 chunk(主程序已发 PullRequest、插件 reader.Read 已完成、但 send 未被主程序 recv 落盘)被 SDK `serveSpecsPull` 丢弃(≤32K)。这是「即时暂停 ⟺ 在途可丢失」权衡中选了即时性那一端的结果,由 Resume 从磁盘 stat 重下兜底。

### 1.2 问题

- **磁盘 stat 与真实中断点存在缝隙**:插件 reader 的 validBytes 可能领先于主程序落盘点,缝隙 = 在途丢失的 chunk。Resume 从 stat 重下缝隙,产生重复下载。
- **reader 状态不可信**:因缝隙存在,Resume 不能复用旧 reader(其 validBytes 虚高),必须每次新建(方案A)。这阻断了连接复用——Transport 随 reader 新建而新建,空闲连接无法 keep-alive。

### 1.3 目标

用「优雅暂停」替代「立即切断」:**暂停只阻止新数据发起,不阻止在途数据完成落盘**。达成:

1. **零在途丢失**:暂停时已发起的 chunk 全部落盘。
2. **磁盘 stat 天然对齐中断点**:缝隙消失,stat 即权威中断点,Resume 无重复下载。
3. **不回退方案A 的并发安全**:reader 仍每次新建(不跨 RPC),不引入跨 Pause→Resume 的 reader 状态保留。

> 本方案核心价值仅上述两条(零丢失 + stat 对齐)。**不涉及 reader 复用、不涉及连接复用**——见 7.5 的澄清。

### 1.4 核心约束(用户明确)

> **主程序提供给插件的磁盘 stat 依然是权威,插件仍应基于它计算偏移量。** 新方案不改变这一权威关系,只是让 stat 在理论上天然与中断点对齐(因在途已排空)。插件 Resume 逻辑不变(从 StreamOffsets 读 stat → 计算偏移 → 新建 reader)。

## 2. 当前实现回顾

暂停链路(`backend/taskManager/model.go`):

```
用户暂停 → Pause() → postCmd(cmdPause)
  → cmdWatcher 收到 cmdPause
       └─ m.runCancel()                    [model.go:595] ★立即取消 runCtx
  → runCtx 取消经项一传播:stream ctx 取消
       ├─ copyLoop: select <-runCtx.Done() → handlePause   [model.go:1139] 立即退出
       └─ copyLoop 阻塞 reader.Read → ctx 取消 → readErr
            └─ runCtx.Err()!=nil → handlePause             [model.go:1168] 立即退出
  → handlePause: drain(buf)+Sync+Close                     [model.go:1234]
       (pull 模型下 drain 空操作:reader 是 pullReadCloser,ctx 取消后 Read 返回 error)
  → copyLoop 返回 streamResult{resultPaused}
  → downloadLoop wg.Wait → anyStreamPaused → runResultPaused [model.go:1108]
  → handleRunCmd: close(stopWatcher)+runCancel+result 处理 [model.go:538-560]
  → handlePauseCmd: Pausing + runCancel(幂等) + pluginExec.Pause + Paused [model.go:637]
```

**在途丢失根因**:`cmdWatcher` 的 `runCancel`(model.go:595)是「立即取消」动作,它同时切断两端(主程序 recv + 插件 send)。copyLoop 在 `runCtx.Done` 或 `reader.Read 返回 ctx error` 时立即 `handlePause` 退出,**不等在途 chunk 落盘**。插件侧 `serveSpecsPull` 同样在 `ctx.Done` 丢弃已读未 send 的 chunk。

## 3. 优雅暂停方案核心思想

**暂停只阻止「新数据发起」,不阻止「在途数据完成」。**

- `cmdWatcher` 收到 cmdPause:**不立即 runCancel**,而是置 `softPause` flag + 启动 drain 超时定时器。
- `copyLoop`:完成当前在途往返(`reader.Read` 返回 + `storeWriter.Write` 落盘)后,检查 `softPause`,此时退出 → 在途已落盘,无损。
- **drain 超时兜底**:若在途迟迟不完成(插件 reader 卡死/网络黑洞),定时器触发 `runCancel` 强制切断,退化为当前有损行为,保证最坏即时性。
- `cmdStop` 不变:停止是放弃,仍立即 `runCancel`(无需 drain)。

SDK `serveSpecsPull` 与插件 Pause/Resume **语义不变**:常态 drain 期间 stream 不取消,在途自然完成;drain 完成后 `handleRunCmd` 的 `runCancel` 才取消 stream(此时无在途,SDK 丢包窗口不触发);插件 Pause RPC 仍是 drain 后的清理。

## 4. 差异分析(当前 vs 新方案)

| 环节 | 当前实现 | 新方案 |
|---|---|---|
| cmdWatcher 收到 cmdPause | `runCancel()` 立即取消 [595] | `softPause.Store(true)` + 启动 drain 定时器,**不 runCancel** |
| copyLoop 中断点 | `runCtx.Done` / reader.Read ctx error → 立即 handlePause [1139/1168] | reader.Read 返回 + Write 落盘后,检查 `softPause` → handlePause(在途已落盘) |
| runCtx 取消时机 | cmdWatcher 立即(drain 前) | drain 完成(handleRunCmd 长任务返回后)或超时兜底触发 |
| 在途 chunk | 丢失(≤32K) | 落盘(零丢失) |
| 磁盘 stat vs 中断点 | 有缝隙(stat < 真实读取点) | 天然对齐 |
| pluginExec.Pause 时机 | drain 前(ctx 已取消) | drain 后(在途已落盘,清理性质) |
| SDK serveSpecsPull | ctx.Done 丢包窗口常态触发 | 常态不触发(仅超时兜底/正常清理) |
| cmdStop | 立即 runCancel | **不变**(立即 runCancel) |

## 5. 详细实现方案

### 5.1 `ManagedTask` 结构增字段(model.go:335)

```go
// actor 通信与生命周期(现有块内新增)
softPause   atomic.Bool     // 软暂停标志:cmdWatcher 置位,copyLoop 完成在途往返后据此退出
drainTimer  *time.Timer     // drain 超时兜底定时器:触发则强制 runCancel(退化有损)
```

### 5.2 `cmdWatcher` 改动(model.go:590-606)

cmdPause 与 cmdStop 分流:**只有 Pause 走 softPause + drain;Stop 仍立即 runCancel**。

```go
func (m *ManagedTask) cmdWatcher(stop <-chan struct{}) {
    for {
        select {
        case c := <-m.cmdCh:
            if c.kind == cmdPause {
                // 优雅暂停:不立即取消 runCtx,通知 copyLoop 完成当前在途往返后退出
                m.softPause.Store(true)
                // drain 超时兜底:在途迟迟不完成则强制取消,退化为有损立即暂停
                m.drainTimer = time.AfterFunc(drainTimeout, func() {
                    if m.softPause.Load() && m.runCancel != nil {
                        logger.Log.Warnf("[TaskManager] 任务 %d drain 超时,强制取消(退化有损)", m.taskId)
                        m.runCancel()
                    }
                })
                m.pendingCmds = append(m.pendingCmds, c)
                return
            }
            if c.kind == cmdStop {
                // 停止是放弃,立即取消(不走 drain)
                if m.runCancel != nil {
                    m.runCancel()
                }
                m.pendingCmds = append(m.pendingCmds, c)
                return
            }
            m.pendingCmds = append(m.pendingCmds, c)
        case <-stop:
            return
        }
    }
}
```

`drainTimeout` 为包级常量(建议 `2 * time.Second`:一个 chunk 往返+落盘通常 <1s,2s 足够覆盖,且暂停延迟可接受)。

### 5.3 `copyLoop` 改动(model.go:1135-1186)

在 `reader.Read` 返回 + `Write` 落盘**之后**插入 softPause 检查(保证在途落盘后才退出):

```go
func (s *streamController) copyLoop(m *ManagedTask) streamResult {
    buf := make([]byte, 32*1024)
    for {
        // runCtx 取消(超时兜底强制取消 / 外部取消):立即退出
        select {
        case <-m.runCtx.Done():
            return s.handlePause(buf)
        default:
        }

        n, readErr := s.reader.Read(buf)   // 当前在途往返(softPause 时这是最后一次)
        if n > 0 {
            written, writeErr := s.storeWriter.Write(buf[:n])   // 在途落盘
            // ... 现有 writeErr 处理不变 ...
            if written > 0 { s.mu.Lock(); s.written += int64(written); s.mu.Unlock() }
            m.reportProgress()
        }
        // 优雅暂停:在途已落盘,drain 完成,退出(此刻磁盘 stat = 真实中断点)
        if m.softPause.Load() && m.runCtx.Err() == nil {
            return s.handlePause(buf)
        }
        if readErr != nil {
            // ... 现有 readErr 处理不变(含 runCtx.Err()!=nil → handlePause 超时兜底路径) ...
        }
    }
}
```

关键:**softPause 检查在 Write 之后**。这保证「已发起的 chunk 必先落盘,再退出」。`runCtx.Err()==nil` 守卫区分两条路径:正常 drain(softPause,无损)vs 超时兜底(runCancel 已触发,走 readErr 分支的有损路径)。

### 5.4 `handleRunCmd` 改动(model.go:538-542)

长任务返回后(drain 完成或超时兜底已切断),停止 drain 定时器:

```go
close(stopWatcher)
if m.drainTimer != nil {
    m.drainTimer.Stop()   // drain 正常完成则停定时器;已触发(超时)则 Stop 返回 false,无副作用
    m.drainTimer = nil
}
if m.runCancel != nil {
    m.runCancel()         // 取消 stream 清理(drain 完成后此时无在途)
    m.runCancel = nil
}
```

### 5.5 `prepareForResume` 改动(model.go:1477)

**必须重置 softPause**,否则 Resume 后 copyLoop 会立即误判 softPause 退出:

```go
func (m *ManagedTask) prepareForResume() {
    m.softPause.Store(false)   // 新增:清除上一轮暂停标志
    if m.drainTimer != nil { m.drainTimer.Stop(); m.drainTimer = nil }  // 防御性清理
    m.closeStreamReaders()
    m.streams = nil
    m.resumeFromDB = m.task.PendingResourceID.Valid
    // ...
}
```

### 5.6 `handlePauseCmd` 基本不变(model.go:637)

- `runCancel()`(model.go:647)此时幂等(handleRunCmd 已 runCancel),保留或移除均可,建议保留(防御)。
- `pluginExec.Pause`(model.go:656)语义不变:此时 drain 已完成、stream 已取消、插件 serveSpecsPull 已退出,通知插件关 reader 是清理性质。
- 契约不变:`Pause` 仅 Processing 返回 nil,否则 `ErrTaskNotProcessing`。

### 5.7 SDK 改动(最小)

`serveSpecsPull`(library-squirrel-sdk/transport/plugin_server.go:195)**不改**:
- 常态 drain 期间 stream 不取消,在途 reader.Read + send 自然完成,主程序不再发新 PullRequest → `recvPull` 阻塞等下一个(等不到)→ handleRunCmd 的 runCancel 取消 stream → `ctx.Done` → `reader.Close` + 退出(**此时无在途,丢包窗口不触发**)。
- `ctx.Done` 丢包窗口逻辑(plugin_server.go:257-268)**保留**,作为超时兜底退化路径的语义兼容 + 长期哨兵日志。

### 5.8 插件改动(无)

- pixiv `Pause`(task_handler.go:466):`reader.Pause()`(closeResponse)语义不变,时机后移到 drain 完成后(由主程序 handlePauseCmd 调用),仍是清理。
- pixiv `Resume`(task_handler.go:513):**完全不变**。仍从 `StreamOffsets`(磁盘 stat)读 offset → 新建 reader + `SetValidBytes(offset)` → GetHeaders → Store。
- **磁盘 stat 权威性不变**:插件始终基于主程序给的 stat 计算偏移。新方案只是让 stat 在理论上 = 中断点(无缝隙),插件无需感知这一变化。

## 6. 时序对比

### 当前(立即切断,在途丢失)
```
cmdPause → cmdWatcher runCancel ──┐
                                   ├─ copyLoop runCtx.Done 立即退出(在途丢)
                                   └─ serveSpecsPull ctx.Done 丢包窗口(在途丢)
copyLoop handlePause → downloadLoop runResultPaused
handleRunCmd: runCancel + result
handlePauseCmd: pluginExec.Pause(清理)
磁盘 stat < 真实中断点(有缝隙)
```

### 新方案(优雅暂停,在途排空)
```
cmdPause → cmdWatcher: softPause=true + 启动 drainTimer(不 runCancel)
copyLoop: reader.Read(在途)完成 → Write 落盘 → 检查 softPause → handlePause 退出
          (若无在途,reader.Read 立即返回或 softPause 检查直接退出)
serveSpecsPull: 在途 reader.Read+send 自然完成;主程序不再发 Pull → 阻塞
downloadLoop wg.Wait → runResultPaused
handleRunCmd: 停 drainTimer + runCancel(此时无在途,stream 取消,serveSpecsPull 干净退出)
handlePauseCmd: pluginExec.Pause(清理)
磁盘 stat = 真实中断点(对齐)

[异常] drain 超时 → drainTimer 触发 runCancel → copyLoop readErr→handlePause(有损) → 同当前
```

## 7. 关键设计决策

### 7.1 磁盘 stat 权威性不变(约束落实)
插件 Resume 始终从主程序给的 `StreamOffsets`(= os.Stat)计算偏移。新方案**不改变这一权威关系**,只是因 drain 排空在途,stat 从「落后于真实读取点」变为「等于中断点」。插件代码零改动即可享受对齐收益。

### 7.2 drain 超时兜底(兼顾无损与最坏即时性)
常态 drain 一个 chunk 往返(<1s)无损完成;异常(插件卡死/网络黑洞)时 `drainTimeout`(2s)强制 `runCancel`,退化为当前有损行为。这让方案在「常态无损」与「异常即时」间自适应,**不牺牲最坏情况下的暂停可用性**。

### 7.3 多流 drain 一致性
`softPause` 与 `drainTimer` 均为任务级(一个 flag、一个定时器)。多流场景下每个 stream 的 copyLoop 各自完成在途后退出,`downloadLoop` `wg.Wait` 等全部。各 stream 的磁盘 stat 各自对齐其中断点,Resume 按 `StreamOffsets`(每轨独立 offset)续传,一致。

### 7.4 Stop 不走 drain
Stop 是放弃(用户主动终止/删除),无需保存在途。仍立即 `runCancel`,与当前一致。只有 Pause(暂停,Resume 要续)走 drain。

### 7.5 本方案不复用 reader,也不为连接复用打基础

此节澄清一个容易误判的点(方案早稿曾错把本方案与连接复用挂钩)。

**方案A 问题的两个维度,优雅暂停只解决其一:**

| 维度 | 含义 | 当前(立即切断) | 优雅暂停 |
|---|---|---|---|
| 维度1:reader 状态数值准确性 | `validBytes` 是否如实反映已交付主程序的数据 | ❌ 虚高(reader.Read 完成的 chunk 被 serveSpecsPull 丢弃时 `validBytes` 仍 `+= n`) | ✅ drain 让 chunk 送达落盘,`validBytes` = stat |
| 维度2:reader 访问串行性 | reader 是否只被一个 goroutine 访问 | 复用时 ❌(旧 serveSpecsPull 残留 goroutine + 新 copyLoop 并发) | ❌ 仍未解决(主程序无法确定插件侧旧 serveSpecsPull 何时退出) |

优雅暂停解决维度1(数值可信),**不解决维度2(访问串行性)**。所以即便 drain 后 reader 数值可信,复用 reader 仍可能重现方案A 维度2 的并发竞态(需额外设计旧 serveSpecsPull 退出确认握手,超出本方案)。**因此本方案保持「Resume 仍新建 reader」**,方案A 两维度一并由「不跨 RPC」守住,不回退。

**连接复用与 reader 复用正交,本方案对它无直接价值:**

`doc/bug/频繁启停资源内容错位-Resume并发竞态.md` 已确认:reader(应用层状态)与 Transport(连接层资源)正交,方案A 不阻塞连接复用。连接复用的正确路径是**提升 `http.Transport` 为 handler 级共享**(连接池 keep-alive),reader 仍每次新建——reader 复用既非连接复用的前提,也非其障碍。连接复用的真正障碍是代理环境 `Unsolicited response`(待办④),与优雅暂停无关。**故本方案不为连接复用扫清任何障碍,也无需为此复用 reader。**

### 7.6 不回退方案A 的并发安全
本方案不保留任何跨 Pause→Resume 的 reader 状态(reader 仍每次新建)。drain 只改变「主程序何时停止 recv」,不引入插件侧状态保留。方案A 的并发不变量(一任务一 goroutine 串行 + reader 不跨 RPC)完整保留。

### 7.7 多板块 drain 边界(收益按 generation 不同)

当前多板块分两类 generation:`downloaded`(main,可续传)与 `derived`(thumbnail,一次性派生)。恢复时二者分流(`resumeFromPersistedState`):downloaded 未完成按 `os.Stat` 续传,derived 未完成**整轨重产**(不续传)。

本方案 `softPause` 为任务级 flag,所有板块的 copyLoop 都 drain,但**收益按 generation 不同**:

| 板块 | drain 效果 | 价值 |
|---|---|---|
| downloaded(main) | 在途落盘 → stat 对齐中断点 → Resume 精确续传 | **核心受益**(本方案目标) |
| derived(thumbnail) | 在途落盘 → 恰好 EOF 完成则 Resume 跳过(省一次重产);仍未完成则 Resume 整轨重产,drain 字节被重建覆盖 | 无害,价值有限 |

采用**任务级统一 drain**(不按 generation 区分):① 简单;② derived 轨通常小且派生快,在途(≤32K)drain 延迟可忽略;③ derived drain 无副作用。本方案的核心收益仅在 downloaded 轨,derived 轨的"避免重产"是附带小收益,非目标。

### 7.8 前瞻兼容性:多 downloaded 板块一视同仁

本方案的暂停/重启改动是 **generation-based、role-agnostic** 的,天然支持未来「大型非主要板块与主资源下载流程统一」的演进方向。

**当前下载流程本就无 role 分支**:

| 层 | 是否按 role 分支 | 依据 |
|---|---|---|
| copyLoop(单流读写) | 否 | model.go:1135 |
| downloadLoop(多流聚合) | 否 | model.go:1081 |
| startDownload(建 streamController) | 否 | model.go:882 |
| handleEOF(完整性校验) | 按 generation,非 role | model.go:1208 |
| resumeFromPersistedState(恢复续传) | 按 generation,非 role | model.go:1420 |

**本方案改动同样 role-agnostic**:`softPause` 任务级、copyLoop drain 不看 role、`resumeFromPersistedState` 只要是 `downloaded` 就续传。

**演进保证**:未来新增 `videoTrack`/`audioTrack`/`merged` 等大型板块,只要声明 `GenerationDownloaded`(handler_dto.go),**无需任何下载/暂停/续传层改动**,自动获得与 main 完全一致的能力(相同 copyLoop、drain、stat 续传、handleEOF 校验)。这正是「下载时一视同仁、仅使用层区别」的愿景。

**generation 分支的本质**:不是「主/非主」分支,而是「下载型/派生型」分支——downloaded(HTTP 可 Range 续传)vs derived(计算产物不可续传)。把「非主要但大型」的资源归到 downloaded,即消除下载分支,符合 generation 区分的设计初衷。真正的派生产物(thumbnail 等)保留 derived。

**边界(不在本方案范围,记录为未来泛化点)**:路径层 `resolveMainPath`(model.go:875)以 main 为基准、`allStoreRoles`(model.go:57)硬编码 main+thumbnail——这些属「下载完成后的组织/使用」层,不影响暂停/重启。若未来出现「多个并列大型下载板块、无主次之分」(如多图作品每张图平等),需泛化路径基准假设,但与本方案正交,可独立演进。

## 8. 测试方案

### 8.1 单元测试(backend/taskManager)
- 改写 `TestDownloadLoop_PauseBroadcast`:验证 softPause 路径下,copyLoop 在 Write 后退出(非 runCtx.Done 立即退出)。
- 新增 `TestCopyLoop_SoftPause_DrainsInflight`:构造一个 mock reader,暂停时已 Read 出 n 字节未 Write,断言 Write 发生后 才 handlePause(在途落盘)。
- 新增 `TestCmdWatcher_PauseSoftNotCancel`:断言 cmdPause 后 runCtx 未立即取消(softPause=true),cmdStop 后 runCtx 立即取消。
- 新增 `TestDrainTimeout_ForceCancel`:模拟在途不完成(drainTimeout 内 reader.Read 阻塞),断言定时器触发 runCancel,copyLoop 走有损路径退出。
- 新增 `TestPrepareForResume_ResetsSoftPause`:断言 Resume 后 softPause=false,copyLoop 不误退出。
- 现有 4 个必破坏测试(actor 专项)回归通过。

### 8.2 运行时验证(task dev)
- pixiv 父任务高频启停:`[serveSpecsPull] 丢包窗口命中` 日志**不再出现**(常态 drain 成功);仅人为制造 reader 卡死时出现(超时兜底)。
- 暂停后磁盘 stat 对齐:对比暂停瞬间插件 reader validBytes 与 os.Stat,二者相等(无缝隙)。
- Resume 续传:从 stat 直接续,无重复下载(当前会有 ≤32K 重下)。
- 暂停延迟:常态 <1s(一个往返),用户无感。

## 9. 风险与权衡

| 风险 | 评估 | 缓解 |
|---|---|---|
| 暂停延迟增加(一个往返) | 常态 <1s,用户无感;仅异常时 2s | drain 超时兜底封顶 2s |
| drain 期间任务「暂停中」状态可见性 | copyLoop drain 时 state 仍 Processing(未到 handlePauseCmd) | UI 可接受(Pausing 本就是瞬态);若需反馈,可在 cmdWatcher 置 softPause 时即 setState(Pausing) |
| softPause flag 漏重置导致 Resume 误退 | prepareForResume 已重置 | 测试覆盖 + prepareForResume 是 cmdResume 唯一入口 |
| 多流 drain 时单个 stream 卡死拖累整体 | drainTimer 任务级,2s 封顶 | 超时强制取消,不无限等 |
| runCtx 取消时机后移影响项一(快速暂停) | 项一价值是 reader 响应 ctx 中断;新方案常态靠 softPause,超时靠 runCancel,项一机制保留用于兜底 | 项一落地不回退 |

## 10. 不在范围内

- **链接复用**(待办④):**与本方案无关**。连接复用走「共享 Transport」(reader 与 Transport 正交,reader 仍每次新建),其障碍是代理环境 `Unsolicited response`,独立评估,不依赖本方案。
- **逐 chunk ACK 协议**:不引入(优雅暂停用边界 drain 代替常态确认,零常态开销)。
- **插件本地缓存**:不引入(规避跨边界状态/竞态)。
- **SDK serveSpecsPull 重写**:不改(丢包窗口逻辑保留作兜底)。

## 11. 实施步骤

### 11.0 实施上下文(新会话定位)

- **主改文件**:`backend/taskManager/model.go`(全部改动集中于此包)
- **不改**:SDK(`library-squirrel-sdk/transport/plugin_server.go`)、插件(`library-squirrel-plugin-pixiv/task_handler.go`)、插件 Resume 逻辑
- **前置阅读**(理解背景,必读):
  - 本文档第 2-3 节(当前暂停链路 + 优雅暂停核心思想)
  - `doc/bug/频繁启停资源内容错位-Resume并发竞态.md`(方案A 背景;本方案**不回退**其「reader 不跨 RPC」并发不变量)
  - 本文 7.5(reader 不复用)、7.7(多板块 drain 边界)、7.8(前瞻兼容性)
- **关键函数定位**(model.go):`ManagedTask` 结构(335)、`cmdWatcher`(590)、`handleRunCmd`(497)、`copyLoop`(1135)、`handlePauseCmd`(637)、`prepareForResume`(1477)、`resumeFromPersistedState`(1271)
- **核心约束**:磁盘 stat 仍是唯一权威,插件 Resume 不变;本方案仅改主程序暂停时机,不碰 reader 复用/连接复用

### 11.1 改动步骤

1. `ManagedTask` 增 `softPause`/`drainTimer` 字段 + `drainTimeout` 常量
2. `cmdWatcher`:cmdPause 走 softPause + drainTimer;cmdStop 保持 runCancel
3. `copyLoop`:Write 后插 softPause 检查
4. `handleRunCmd`:长任务返回后停 drainTimer
5. `prepareForResume`:重置 softPause + 清 drainTimer
6. 单元测试:改写 + 新增(8.1)
7. `CGO_ENABLED=0 go build ./backend/...` + `go test ./backend/taskManager/...` + `go vet`
8. 运行时验证(8.2)
9. 文档同步:`doc/ai-assistant/task-execution-flow.md` 暂停阶段、`doc/plugin-dev-guide.md` Pause 数据持久化边界(「在途允许丢失」改为「常态排空,仅异常超时丢失」)、`doc/bug/频繁启停资源内容错位-Resume并发竞态.md` 现状节

## 12. 实施记录(2026-07-09 已实施)

按 11.1 步骤实施完毕,23 项单测全绿,运行时验证:暂停/恢复延迟降至毫秒级,频繁启停无资源损坏。

**三条偏离原文的修正(基于代码分析 + 运行时验证,详见 memory `pause-drain-inflight-plan`)**:

1. **copyLoop softPause 路径用 `handlePause(nil)` 而非 `handlePause(buf)`**:`pullReadCloser.Read`(`backend/plugin/extension/task_handler_proxy.go`)每次 = `sendPull`(PullRequest)+ `recvChunk`,Read 本身即发起新拉取。优雅暂停路径 `runCtx.Err()==nil`,`handlePause(buf)` 内部的 `drain(buf)` 会循环 Read 发起新 PullRequest 把剩余资源拉下来,违背"暂停只阻止新数据发起"。本轮 Read 的数据已 Write 落盘、buf 无残留,传 nil 跳过 drain 才符合第 6 节时序图"主程序不再发 Pull"的真实意图。

2. **handleRunCmd 派生 runCtx 前统一重置 softPause/drainTimer**:计划只在 `prepareForResume`(cmdResume 路径)重置,但 cmdStart/cmdConfirmReplace 不经它。Pause→Stop(终态)→Retry(cmdStart)复用 ManagedTask 时 softPause 残留会让 copyLoop 立即误退出。故在 handleRunCmd 派生 runCtx 前加统一重置点覆盖所有 run 命令,prepareForResume 的重置保留(冗余但自文档)。

3. **cmdWatcher 阶段感知(setup 立即 runCancel / downloadLoop softPause drain)**:运行时验证发现 setup 阶段(插件 Start RPC 含网络建连)收到 cmdPause 后,原 softPause 只在 copyLoop 生效、不中断 setup,导致暂停延迟 = setup 剩余 RPC 时间(实测任务 260 延迟 1.6s,因 Start 建连 1.18s)。setup 阶段无在途 chunk 需排空,立即 runCancel 无损且快速。故加 `inDownload atomic.Bool`(downloadLoop 入口置 true、defer 置 false),cmdWatcher 据此分流:downloadLoop(`inDownload=true`)走 softPause drain(保留在途排空价值),setup(`inDownload=false`)走立即 runCancel(恢复改造前的快速 setup 中断)。此修正扩展了原方案的作用域(原方案 5.x 只描述 downloadLoop 阶段的 softPause)。
