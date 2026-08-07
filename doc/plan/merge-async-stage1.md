# 合并异步化（阶段1·止血）

> 派生自「耗时操作接入任务模块」派生图的『阶段1·合并异步化止血』节点——该节点同时是发布前主要决策定稿树「合并异步化」阻塞项的实现主体。
> 前置阶段0（合并必修缺陷修复）已交付（2026-07-20）：① merge_service 入口去重幂等 ② merged 挂 GenerationDerived ③ 启动清理 ls-merge-* 临时残留。
> 本文档是阶段1 实施方案：合并挪 goroutine + ffmpeg 进度回调 + cancel + 前端迷你 UI，**只止血、不进 taskManager 控制面**（执行面剥离/崩溃恢复属阶段2，发布后由翻译/AI标签/查重等纯计算型操作触发）。

---

## 审查摘要

### 关键声明（抽查项）

- **声明1**：合并当前**同步阻塞 IPC**——ffmpeg 在 IPC handler 的 goroutine 内同步跑 `cmd.Run()`，大文件分钟级占用 IPC 线程。锚点：`backend/resource/merge_service.go:148`（MergeRemux 调用）、`backend/merge/ffmpeg.go:73`（cmd.Run 阻塞）、`backend/resource/handler.go:23-29`（同步返回结果）。
- **声明2**：ffmpeg 的 stderr 当前**仅作失败诊断**（截末尾 512 字节），不解析进度、不 emit。锚点：`backend/merge/ffmpeg.go:69-71`（stderrBuf）、`ffmpeg.go:61-67`（cmd 参数无 `-progress`）。
- **声明3**：TaskProgressPusher（taskManager 的进度推送接口）推 `task-events` topic，前端 `MainIpcListener.ts` 分发到 `useTaskStore`（任务面板 store）。锚点：`backend/taskManager/progress_pusher.go:46-48`（emitTaskEvent→"task-events"）、`frontend/src/MainIpcListener.ts:18-31`（→ useTaskStore.updateTask/updateTaskSchedule）。
- **声明4**：代码库长任务 goroutine 用 `context.WithCancel(context.Background())` **脱离 handler ctx**（handler 返回后其 ctx 不保证存活）。锚点：`backend/taskManager/model.go:409`。
- **声明5**：现有去重守卫只查"已存在 merged store"（幂等），不含"in-flight 进行中合并"——异步化后两连点会绕过此守卫并发起第二个合并，故需新增 in-flight 注册表守卫。锚点：`backend/resource/merge_service.go:105-110`。
- **声明6**：取消即 `exec.CommandContext` 的 ctx 取消（Windows 上为 TerminateProcess 强杀子进程），已内建于 MergeRemux。锚点：`backend/merge/ffmpeg.go:61`（CommandContext）、`ffmpeg.go:76-78`（ctx 取消归"被中断"）。
- **声明7**：merge 包为纯能力（`MergeRemux` 输入输出全文件路径，不耦合 store/resource）；进度解析属 ffmpeg 机制，归 merge 包，回调签名用裸数字保持纯能力。锚点：`backend/merge/ffmpeg.go:56`。
- **声明8**：前端合并 UI 当前 = 单按钮 + `:loading` 弱保护，合并结果同步 await。锚点：`frontend/src/components/dialogs/WorkDialog.vue:167-182`、`frontend/src/apis/http/wrappers/resource.ts:14-17`。
- **声明9**：阶段0 启动清理已落地（`MergeService.CleanupResidualTempFiles` 扫 `os.TempDir()` 删 ls-merge-* 残留），覆盖异步合并崩溃后的临时文件残留。锚点：`backend/resource/merge_service.go:241-256`。

### 待决策（需用户拍板）

- **决策1（阻塞实施）·进度推送通道边界**：推荐**独立 `merge-events` topic** + 在 resource 包内定义 `MergeEventEmitter`（复用 WailsEventEmitter 推送基础设施与 `ipcEvent{type,data}` 信封模式 + 前端 `Events.On` 分发范式），**不复用** taskManager 的 `TaskProgressPusher` 接口/`task-events` topic。理由见下文『进度推送通道边界』。此决策解决 longops 选型方案『待决策点』第4点（"复用 TaskProgressPusher 但合并不进 taskManager 是否可接受 task-topic-无-task-记录"）。需用户确认。
- **决策2（不阻塞，建议确认）·MergeResource 同步→异步**：handler 的 `MergeResource` 由同步返回 `*MergeResult` 改为**立即返回 started ack**（启动确认），合并结果经 `merge-events` 的 `complete` 事件交付。前端 wrapper 不再同步拿 MergeResult。需同步 `wails3 generate bindings -ts`。
- **决策3（不阻塞，建议）·前端状态载体**：用组合式函数 `useMergeProgress`（模块级单例 reactive Map<resourceId, 合并状态>），非 Pinia store——当前仅 WorkDialog 一处消费，阶段1 保持最小；消费方增多再升格 store。

### 自曝风险（作者没把握 / 可能错的地方）

- **风险1·进度精度依赖 ffmpeg 输出**：percent 需从 ffmpeg `-progress` 的 `out_time=` 与 banner 的 `Duration:` 解析。输入无 Duration（异常容器）或 ffmpeg 版本输出差异 → percent 不可用，降级为不定态转圈（不显示数字）；正常情况 clamp 在 [0,99]，完成跳 100%。
- **风险2·无崩溃恢复（阶段1 已知缺口）**：合并 goroutine 跑在进程内，进程退出/崩溃则合并中断、无恢复。临时残留由声明9 的启动清理兜底；崩溃恢复属阶段2 接控制面后的能力，本阶段不补。
- **风险3·in-flight 注册表是进程内内存态**（`map + sync.Mutex`），无持久化——重启后丢失；与风险2 一致，阶段1 可接受。
- **风险4·onProgress 回调线程**：`cmd.Stderr` 的 sink.Write 由 os/exec 内部 copy goroutine 调用（与 MergeRemux 的 Run goroutine 并发），故 `PushProgress` 会从该内部 goroutine 触发。`MergeEventEmitter.PushProgress` 仅做无状态 emit（不碰共享可变状态），须保证 goroutine 安全（Wails Events.Emit 跨 goroutine 安全）。
- **风险5·取消与 overwrite 删除的时序**：overwrite 策略删除原轨发生在 MergeRemux 成功之后（`merge_service.go:180-190`），取消发生在 MergeRemux 阶段（其前），故取消不会误删原轨——已验证逻辑顺序，无需额外保护。
- **风险6·取消后立即再点合并**：runMerge 执行序为「MergeRemux →（仅成功路径）落盘/挂 store/重算/overwrite → PushComplete → finally delete(jobs)」，delete(jobs) 在最后，故删表时前一次合并已彻底收尾（成功已落盘 / 取消已清理临时产物），再点安全；取消后 goroutine 退出前的窗口内 `jobs[resourceId]` 仍在 → in-flight 守卫拒绝再点（返回"该资源正在合并中"）。已知轻微 UX 瑕疵：用户刚取消即再点会收到"正在合并中"提示（取消尚未拆除完）——阶段1 接受，未来可加"取消中"过渡态。
- **风险7·Wails 事件可能乱序**：progress 与 complete 即便同 topic 也不保证严格 FIFO（`SnapshotPusher` 的存在即为此）。前端 `useMergeProgress` 须把 complete 视为权威终态：收到 complete 后对该 resourceId 置终态并**忽略后续 progress**（防"已完成→倒退回 running"闪烁），并定时清理终态条目。
- **风险8·Windows 取消的进程范围**：Go `exec.CommandContext` 在 Windows 上仅杀直接子进程（不杀孙进程，需 Job Object 才能进程树级杀）。合并用 `-c copy`（无重编码 remux，ffmpeg 单进程、不 fork 辅助进程），故直接杀 ffmpeg 即彻底，当前不设 `SysProcAttr`；若未来改为带编码合并，需评估是否加进程组/Job Object。

---

## 一、现状与目标

**现状**（声明1）：用户点合并 → IPC `Handler.MergeResource` → `mergeSvc.MergeResource` 在 IPC goroutine 内同步跑 ffmpeg（`ffmpeg.go:73` cmd.Run，超时 10min 兜底），IPC 线程被占用至完成；前端只显示按钮 loading，无进度、不可取消。

**阶段1 目标**（止血，4 项）：
1. 合并挪到独立 goroutine，**不阻塞 IPC**（handler 立即返回）。
2. ffmpeg **stderr 进度回调**（解析 `-progress` 输出推 percent）。
3. **主动取消**（前端按钮触发，调 cancel 杀 ffmpeg 子进程）。
4. 前端**迷你 UI**（进度 + 取消按钮）。

**非目标（阶段边界，勿扩张）**：不进 taskManager 控制面（无 actor/信号量/状态机/统一任务面板/崩溃恢复）；不定义 ExecutionStrategy；不做崩溃恢复与跨重启续传。

---

## 二、进度推送通道边界（决策1 详述）

### 两选与取舍

| 维度 | A 方案：独立 `merge-events` topic（推荐） | B 方案：复用 `task-events` + TaskProgressPusher |
|---|---|---|
| 包依赖 | resource 包内自建 `MergeEventEmitter`（注入 WailsEventEmitter），**不 import taskManager** | resource 须 import taskManager 取 TaskProgressPusher 接口 → resource→taskManager 耦合 |
| 控制面 | 完全不碰 taskManager，符合"阶段1 不进控制面"（含包级） | 复用了控制面推送件，与"不进控制面"张力 |
| 任务面板 | 独立 topic，前端分发到 `useMergeProgress`，**不污染** `useTaskStore` | 走 `task-events` → MainIpcListener → `useTaskStore`，会向任务面板塞入幽灵任务/或被 updateExisting 丢弃（声明3） |
| 语义键 | resourceId（per-resource 合并） | TaskProgressPusher 按 taskId 键、total/finished 字节模型——与合并（percent 模型）不匹配，需伪造 taskId |
| 复用度 | 复用**推送基础设施**（WailsEventEmitter + `ipcEvent{type,data}` 信封 + 前端 `Events.On` 范式），非全新并行系统 | 复用 Go 接口本体 |

**推荐 A 方案**。关键否决理由：B 方案① 让 resource 反向依赖 taskManager 包，破坏"阶段1 不进 taskManager 控制面"的包级纯洁性；② TaskProgressPusher 按 taskId 键、字节进度模型，与合并的 resourceId 键、percent 模型不匹配，强行套用（伪造 taskId、percent 塞 total/finished）是语义扭曲；③ 会污染统一任务面板。A 方案复用的是"推送基础设施与信封范式"（即 longops 方案『待决策点』第4点所欲避免的"全新并行事件系统"），而非 taskManager 的控制面件，忠实于"不进控制面"。

> 通道 DTO 设计（merge-events topic，ipcEvent 信封）：
> - `{type:"progress", data:{resourceId, percent}}`（percent∈[0,100]，-1=不定态）
> - `{type:"complete", data:{resourceId, success, mergedStoreId, errMsg}}`

---

## 三、后端设计

### 3.1 merge 包：进度解析（纯能力，声明7）

`backend/merge/ffmpeg.go`：

1. **MergeRemux 签名加进度回调**（`ffmpeg.go:56`）：`func (m *FFmpegMuxer) MergeRemux(ctx, videoPath, audioPath, outPath string, onProgress func(percent int)) error`。`onProgress=nil` 表示不上报（保留纯合并调用方）。`Merger` 接口（`merge_service.go:42-44`）同步加该参数。
2. **cmd 加进度参数**（`ffmpeg.go:61-67`）：追加 `-nostats -progress pipe:2`，让 ffmpeg 向 stderr 输出结构化 key=value 进度块。
3. **stderr 改为 sink**（替换 `ffmpeg.go:70` 的 `bytes.Buffer`）：自定义 `stderrSink` 实现 `io.Writer`——全量累积字节（失败诊断 tail 复用 `ffmpegFailureDetail`，`ffmpeg.go:92-98`）+ 按 `\n` 切完整行回调进度解析器。
4. **进度解析**（merge 包内纯函数）：
   - 首遇 `Duration: HH:MM:SS.xx` 取各输入 Duration 的 **max** 作为 total（remux 无 `-shortest`，输出≈最长输入）。
   - 遇 `out_time=HH:MM:SS.xx`（机器可读形式，避免 `out_time_ms` 的历史微秒歧义）→ `percent = clamp(outSec/totalSec*100, 0, 99)`，调 `onProgress`。
   - cmd 退出成功 → `onProgress(100)`。
   - 无 Duration 解析到 → 首次 `onProgress(-1)` 标不定态（风险1）。

### 3.2 merge_service：异步执行 + in-flight 注册表 + cancel

`backend/resource/merge_service.go`：

1. **MergeService 增字段**：`jobs map[int64]*mergeJob`（resourceId→进行中合并）+ `jobsMu sync.Mutex`；注入 `emitter MergeEventEmitter`（接口，resource 包内定义）。
2. **mergeJob 结构**：`{ctx context.Context; cancel context.CancelFunc}`。
3. **MergeResource 改异步**（`merge_service.go:98`）：
   - 前置校验仍**同步**在 handler goroutine 内做（返回错误给前端）：merger 可用性（`merge_service.go:99`）/ 已存在 merged 幂等（声明5，`merge_service.go:105`）/ 缺轨（`merge_service.go:113-126`）。
   - **新增 in-flight 守卫**：加锁查 `jobs[resourceId]`，命中 → 返回 `ErrMergeInProgress`（避免两连点并发起第二个合并，补声明5 的窗口）。
   - 创建 **detached ctx**（声明4）：`ctx, cancel := context.WithTimeout(context.Background(), defaultMergeTimeout)`，写 `jobs[resourceId]`。
   - `go s.runMerge(ctx, resourceId, videoPS, audioPS, ...)`，handler **立即返回** started ack（无 MergeResult）。
4. **runMerge goroutine**：`merger.MergeRemux(ctx, videoAbs, audioAbs, tmpOut, onProgress)`，其中 `onProgress = func(p int){ s.emitter.PushProgress(resourceId, p) }`（风险4：由 exec 内部 goroutine 调用，仅 emit）。MergeRemux 返回后：
   - 成功：沿用现有落盘/挂 store/重算 complete/overwrite 逻辑（`merge_service.go:152-193`）→ `emitter.PushComplete(resourceId, true, mergedPsId, "")`。
   - 失败/取消：`emitter.PushComplete(resourceId, false, 0, errMsg)`（取消的 errMsg 透出"已取消"）。
   - finally：加锁 `delete(jobs, resourceId)`。
5. **CancelMerge(ctx, resourceId)**：加锁查 `jobs[resourceId]`，命中 → `job.cancel()`（声明6：杀 ffmpeg 子进程）；未命中 → no-op（已结束/未开始）。
6. **MergeEventEmitter 接口**（resource 包内）：`PushProgress(resourceId int64, percent int)`、`PushComplete(resourceId int64, success bool, mergedStoreId int64, errMsg string)`。实现 `wailsMergeEmitter` 包 `WailsEventEmitter`（与 taskManager 的 `WailsTaskProgressPusher` 同款 Emit + ipcEvent 信封，topic 用 `merge-events`）。`app.go` 构造时注入。

### 3.3 handler：异步接口

`backend/resource/handler.go`：

1. `MergeResource`（`handler.go:23`）返回类型由 `*ApiResponse[*MergeResult]` 改 started ack（`*ApiResponse[any]`，success=true 表示已启动）；内部调 `mergeSvc.MergeResource`（语义改"启动"）。
2. 新增 `MergeCancel(ctx, resourceId int64) *ApiResponse[any]` → `mergeSvc.CancelMerge`。
3. `wails3 generate bindings -ts` 重生前端类型。

---

## 四、前端设计

### 4.1 事件分发与状态载体（决策3）

1. `frontend/src/MainIpcListener.ts`（`MainIpcListener.ts:18` 区附近）：加 `Events.On('merge-events', ...)`，按 `{type}` 分发 `progress`/`complete` 到 `useMergeProgress`。
2. `frontend/src/composables/useMergeProgress.ts`（新）：模块级单例——`reactive(new Map<number, MergeState>())`（MergeState = `{percent:number, status:'running'|'done'|'failed'|'canceled'}`）；progress 事件更新对应条目，complete 事件置终态后定时清理（如 3s 删条目）。暴露 `getMergeState(resourceId): MergeState | undefined`。WorkDialog 经 computed 派生当前资源的 merging/percent/cancelable。

### 4.2 迷你 UI（声明8 改造）

`frontend/src/components/dialogs/WorkDialog.vue`：

1. `handleMergeButtonClick`（`WorkDialog.vue:169-182`）：改为**异步启动**——调 `resourceMerge(resourceId)`（不再 await 结果，仅启动），不再 try/catch 同步结果；终态由 complete 事件驱动。
2. 合并按钮区（`WorkDialog.vue:684-692`）：running 时按钮文案 `合并 N%`（percent=-1 时 `合并中…`）+ 禁用；旁置**取消**小按钮（仅 running 显示）→ `resourceMergeCancel(resourceId)`。
3. complete 事件监听（经 useMergeProgress 的 status 变化 watch，或 listen）：success → `getWorkInfo()` 刷新 + 成功 toast；failed/canceled → 对应 toast。
4. `merging` ref 改由 `useMergeProgress(currentResourceId)` 派生（取代 `WorkDialog.vue:167` 的本地 ref），天然处理"合并中切换作品再切回"的状态恢复（Map 保留 running 条目）。

### 4.3 wrapper

`frontend/src/apis/http/wrappers/resource.ts`（`resource.ts:14-17`）：`resourceMerge` 改返回 started ack（不再 `.data` 取 MergeResult）；新增 `resourceMergeCancel(resourceId)`。

---

## 五、文件改动清单

**后端**
- `backend/merge/ffmpeg.go` — MergeRemux 加 onProgress 参数 + `-progress pipe:2` + stderrSink 进度解析（声明7 纯能力）。
- `backend/merge/ffmpeg_test.go` — 3 处调用补 `nil`（行65/90/110）+ 新增进度回调测试（验证回调被调、范围 0~100）。
- `backend/resource/merge_service.go` — Merger 接口同步、jobs 注册表 + mutex、MergeResource 异步化、runMerge、CancelMerge、MergeEventEmitter 接口 + wailsMergeEmitter。
- `backend/resource/handler.go` — MergeResource 返回 started ack + 新增 MergeCancel。
- `app.go` — 构造 wailsMergeEmitter 注入 NewMergeService。
- `wails3 generate bindings -ts`。

**前端**
- `frontend/src/composables/useMergeProgress.ts`（新）— 单例 Map + getMergeState。
- `frontend/src/MainIpcListener.ts` — merge-events 分发。
- `frontend/src/apis/http/wrappers/resource.ts` — resourceMerge 改 ack + resourceMergeCancel。
- `frontend/src/components/dialogs/WorkDialog.vue` — 异步启动 + 进度/取消 UI。

**验证**
- `go build ./...`、`go test ./backend/merge/... ./backend/resource/...`、`cd frontend && yarn build`。

---

## 六、阶段边界（不做什么·守住勿扩张）

- **不进 taskManager 控制面**：无 actor 串行、无信号量限流、无状态机、无统一任务面板、无崩溃恢复（决策1 选 A 方案正是为此）。
- **不定义 ExecutionStrategy**：执行面剥离是阶段2，推迟到"下一个纯计算/API 型操作要进控制面时"启动（发布后翻译/AI标签/查重落地时）。
- **无崩溃恢复/跨重启续传**：进程退出合并丢失（风险2），临时残留靠阶段0 启动清理兜底（声明9）。
- **复用推送基础设施、不复用控制面件**：决策1 的 A 方案是"复用 WailsEventEmitter+ipcEvent 信封+Events.On 范式"，非 taskManager 的 TaskProgressPusher 接口本体。
