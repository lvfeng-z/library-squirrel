# Pause 边界 EOF 丢失 → 潜在 Resume 416 假失败

> 类型：潜在缺陷（代码路径分析，**未实测**）
> 分析日期：2026-07-05
> 状态：理论路径成立，尚未在日志中观察到 416 实例
> 严重程度：中（若触发：数据未损坏，但已完整的资源被标记 Failed）

## 范围说明（务必先读）

本文档分析的是一个**基于代码结构推断的潜在缺陷**（下称"本机理"），回答的是："如果 Pause 恰好落在某 role 的 EOF 窗口，会发生什么？" **它不是已实测的 bug。**

**不要与 264/265 资源损坏混淆**——那是另一个已回退的事件（过滤器 bug），与本机理无关：

| | 本机理（本文档） | 264/265（已回退，另一事件） |
| --- | --- | --- |
| 前提 | 文件**已下完** | 文件**未下完**（仅写了 84% / 78%） |
| 机制 | Pause 落在 EOF 窗口 → Status 未对齐 → Resume 用"完整大小"当 offset → 416 | 过滤器误用 `offset >= spec.Size`（spec.Size 是**剩余字节**）把半成品判成完成 → 截断损坏 |
| 实测 | **无**（日志全程无 416） | 有 |
| 修复点 | 插件侧 Resume 自判 | 已回退过滤器 |

两者易混，因为都涉及"完成判定"和 `spec.Size` 语义。但触发条件、后果、修复点都不同。

## 假设性现象

若以下条件同时成立：

1. 某 role 的资源已由 reader 返回了全部字节（磁盘文件已写完整）
2. 用户在"最后一批字节已写入磁盘"与"下一次返回 `io.EOF` 的 Read 尚未执行"之间触发 Pause
3. 任务随后被恢复（Resume）

则：

- `persistent_store` 该 role 的 `Status` 仍为 `Incomplete`（Pause 路径不置 Complete，见根因第 3 层）
- 但磁盘文件大小已等于资源完整大小
- `resumeFromPersistedState` 用磁盘大小作为续传 offset 传给插件 → 插件发 `Range: bytes={完整大小}-` → 起点 ≥ 文件末尾 → HTTP 416 → 任务 Failed

## 根因：Pause 边界的 EOF 丢失

主程序把某 role 标记为完成的**唯一触发点**是 `handleEOF` 内 `reader.Read` 返回 `io.EOF` 后调用 `storeWriter.Complete()`（置 DB `Status=Complete`）。但 Pause 会让那"最后一次 Read"永远不执行，EOF 信号被丢弃。分四层：

### 1. io.Reader 语义：EOF 与最后一批字节不在同一次 Read

Go `io.Reader` 约定：读到流末尾时，Read 先返回最后那批字节（`n>0, err=nil`），EOF 要等下一次 Read 才返回（`n=0, err=io.EOF`），EOF 不与最后一批数据同次返回。所以"磁盘写完整"与"读到 EOF"之间必然隔一次 Read。

### 2. copyLoop 的时序窗口卡在两次 Read 之间

`backend/taskManager/model.go` 的 `copyLoop`：每轮先在顶部 `select` 非阻塞检查 `pauseSignal`，再到底部 `Read + 处理 EOF`：

```go
for {
    select {
    case <-m.pauseSignal():
        return s.handlePause(buf)      // Pause 走这里 → 不调 Complete
    case <-m.ctx.Done(): ...
    default:
    }
    n, readErr := s.reader.Read(buf)
    ...
    if readErr == io.EOF {
        return s.handleEOF(m)          // 唯一调 Complete 的路径
    }
}
```

时序窗口（完整文件最后字节在 Read#K 返回）：

```
Read#K   : n = 最后 N 字节, err = nil   ← 文件在此刻写完整
            （顶部 select 检查 pauseSignal —— Pause 落在此处则走 handlePause）
Read#K+1 : n = 0, err = io.EOF          ← 这次才触发 handleEOF → Complete
```

Pause 落在"Read#K 写完最后字节"与"Read#K+1 读到 EOF"之间的顶部 select 检查点上，就走 `handlePause` 而非 `handleEOF`，那一次本该返回 EOF 的 Read 永远不被调用。

### 3. handlePause 设计上不调 Complete

`handlePause` 只 `drain + Sync + Close + state=streamPaused`，**不调 `Complete()`**。即使 `drain` 在排空时读到了 `io.EOF`，也只 `return`，不置 Complete。这是 Pause 的语义决定的——Pause = "未完成，等下次续传"，必须保持 `Status = Incomplete`。

### 4. pull 模式进一步放大窗口

pull 协议下数据需主程序主动 pull 才来。Pause 后 copyLoop 不再 pull → 插件那边即便还握着最后的 EOF 帧，主程序也不收取。窗口从"一次 Read 间隔"被放大成"Pause 后不再 pull 任何帧"。

## 为什么主程序侧修不了

主程序判定完成依赖两样东西，在 Pause 边界都拿不到：

1. **EOF 信号**：在 Pause 路径结构性丢失（上述四层）
2. **完整 size 字段**：`persistent_store` 表**没有 size 字段**（只有图像像素 `width`/`height`），主程序不持有"资源完整大小"的权威值，无法用"已写字节 == 完整 size"补救

跨重启恢复时 `resumeFromPersistedState` 判定某 role 完成的依据只有 `store.Status == Complete && 文件存在`，Status 一旦滞后，主程序没有任何旁路判据。

完整 size 的唯一权威来源是 HTTP（插件做 Range/HEAD 时的 Content-Length），只有插件能拿到。所以本问题主程序侧无法修复。

## 修复方向（待本机理被实证后再实施）

### 正解（插件侧）

由插件在 `Resume` 内自判：

- 插件收到 `StreamOffsets[role] = X`，做 HTTP 探测时已知完整 size = S
- 若 `X >= S`：该 role **不产出 spec**（主程序据返回 specs 中无该 role 视为已完成），**或**把 HTTP 416 转译为"该 role 已完成"的成功信号而非任务失败
- 主程序侧不做任何 size 推断，回到只信 `store.Status` 的简单逻辑

涉及仓库：`library-squirrel-plugin-pixiv`（可能含 SDK）。

### 局部修补（主程序侧，可选，仅缩窗口不根治）

在 `handlePause` 的 `drain` 中，若 reader 返回 `io.EOF`（说明上游确实结束了），调 `Complete()` 把 Status 置对。能修正"Pause 发生在 EOF 帧已可读"的子窗口。

**局限**：救不了"Pause 后主程序不再 pull、EOF 帧根本未被收取"的更常见情况（pull 模式下 drain 在 Pause 后不再 pull，立刻返回）。所以仍是局部修补，不替代插件侧正解。

## 教训（来自 264/265 误判）

本机理之所以一度被当成"已实测"记录，源于两个叠加失误：

1. **字段语义未核实**：把插件 Resume 返回的 `spec.Size`（Range Content-Length，**剩余字节**）当成完整大小，据此在主程序侧加 `filterCompletedRoles` 过滤器，用 `offset >= spec.Size` 判完成 → 半成品被误判完成 → 264/265 截断损坏（过滤器已回退）。
2. **文案与归因沿用了同一误解**：过滤器日志里写了"磁盘已满但 store.Status 未对齐"，这行文案本身就建立在"offset >= spec.Size 即下完"的错误前提上；后续分析又把 264/265（过滤器 bug）误当作本机理（EOF 丢失）的实测证据，让理论分析伪装成了实测 bug。

教训：作为完成判据前必须核实字段语义；描述缺陷时严格区分"代码路径推断"与"实测证据"，不混装。

## 潜在影响

- 数据未损坏（磁盘文件完整）
- 状态错误：已完整的子任务被标 Failed，父任务可能随之 PartlyFinished/Failed
- 触发概率：取决于 Pause 命中 EOF 窗口的频率；高频启停下窗口被 pull 模式放大，但本测试中未观察到 416 实例（264/265 是另一回事，见范围说明）

## 相关

- 计划文档：`doc/plan/refactor-task-single-goroutine-invariant.md`「纪律 4」段（含已回退的过滤方案与本机理的正解）
- 后续改进：`doc/plan/task-manager-followup-improvements.md` ②（reader 响应 ctx 取消，与本机理同源——Pause 边界的 reader 行为）
- 代码定位：`backend/taskManager/model.go`（`copyLoop`、`handleEOF`、`handlePause`、`drain`、`resumeFromPersistedState`）；`backend/persistentStore/service.go`（`storeWriter.Complete` 置 `Status=Complete`）
- 完成判定现状：`persistent_store.Status`（`StoreStatusIncomplete=0` / `StoreStatusComplete=1`），无 size 字段
