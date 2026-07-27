# 插件查资源路径能力（兄弟文件真实引用）

> **审查摘要**
> - **核心机制**：SDK 新增 `PluginContext.GetStoreRelPath(role, storeSeq) (relPath string, err error)`，插件查**当前任务资源**的某 store 真实落盘路径（严格从 DB `resource_store`→`persistent_store.file_path` 读，不预算/不推算）。查不到返回**结构化错误**（尚未创建/创建失败/resource 不存在等），插件据此决策。
> - **关键锚点**：SDK 接口 `library-squirrel-sdk/dto/context.go:4 PluginContext` + 实现 `transport/client.go:13 PluginContextClient`（:150 GetPluginRoot 模式）；主程序 Provider `backend/plugin/extension/plugin_context.go:15-55`（接口+Deps）+ HostDeps 适配器群 `backend/plugin/extension/loader.go:352-424`（**拟在此追加** `hostStorePathProvider`）；HostService rpc `library-squirrel-sdk/proto/plugin.proto:308-339`（**拟在此 service 块追加** rpc）；DB 查询 `backend/resource/resource_store_repository.go:26 ListByResourceId` + `backend/taskManager/model.go:230 StoreReader`（PersistentStore.file_path）；bilibili basename 源 `library-squirrel-plugin-bilibili/internal/bilibiliapi/dynamic.go:255-256/316-320` + document 生成 `download.go:144-156`；resource_store 创建时序 startDownload 事务（`backend/taskManager/model.go:929` ExecInTransaction → specs 遍历 StoreStream :936-948 → 提交 :959）。
> - **document 定稿时序已定 X2（lazy ReadCloser，2026-07-27 用户拍板）**：插件侧 document ReadCloser 为 lazy wrapper，被 pull 时由插件生成（查 `GetStoreRelPath` + 拼 .md）。主程序被动（不调插件生成，只 pull + 提供 SDK 能力 + 保证 resource_store 时序）。X1（两阶段 Start）/X3（derived 独立后处理）让主程序越界主导生成流程，否决；X4（占位+主程序替换）不符"插件自行占位"，否决。详见 §五。
> - **自曝风险**：① **resource_store 时序不变量**（X2 隐式依赖）：`downloadLoop` 必须在 `startDownload`/`resumeFromPersistedState` 事务提交之后（startDownload 事务 model.go:929→提交:959→downloadLoop:988；resume 事务 model.go:1576→downloadLoop:1647），否则 document 的 lazy 查路径落空（事务未提交，DB 不可见）。需在代码注释固化，防未来重构破坏；② **跨仓发布兼容窗口**：发布顺序 SDK（rpc+能力）→ 主程序（HostService 实现）→ bilibili（lazyMDReader）；bilibili 不能先于 SDK/主程序发布（否则 genFunc 调 `GetStoreRelPath` 在旧 SDK/主程序无此 rpc → 失败）；回滚：bilibili 回退 NopCloser 占位（位置绑定兜底，已验证可用）；③ **命名权边界**：插件用 `GetStoreRelPath` 拿 relPath（DB file_path），.md 引用用 `filepath.Base(relPath)` 提取 basename（同目录兄弟引用）——插件不需懂 N 规约，basename 直接来自 DB file_path，一致性天然保证；④ **genFunc 错误传播**：GetStoreRelPath 返回 NOT_CREATED（事务保证下不发生，防御性）时 genFunc 行为（建议 fail 暴露问题）+ RPC deadline 设定（如 5s），影响 document 生成健壮性。

## 一、背景与根因

**根因（用户 2026-07-27 定位）**：bilibili 插件的 .md 生成策略错误——在 **Create 阶段**就生成 .md 内容（`PluginData.Markdown`），先于资源文件创建。此时 image specs 未产、未落盘，插件不知 image 文件名，被迫用解耦占位（`001.jpg`）+ 前端位置绑定兜底。

**D4 演变**：D4 原始（2026-07-24）"basename==落盘名"→ 被 M D3-B + 位置绑定取代（2026-07-25）。位置绑定是"策略错误的补救"（绕过 basename 不一致），非正确解法——.md 作为独立兄弟文件引用失效（外部/导出不可读）。

**正确原则（用户）**：元数据↔资源不依赖；**兄弟文件间可互相依赖**（.md 通过文件名引用 image 合理）。故 .md 应通过**真实文件名**引用 image，由插件生成（非主程序后处理）。前提：插件能查到真实路径 → 本能力。

## 二、决策（用户 2026-07-27 拍板）

1. **SDK 能力形态**：`ctx.GetStoreRelPath(role, storeSeq) → (relPath string, err error)`（单点查）。
2. **bas（N 命名规约的基准文件名，如 `[作者]_[workId]_作品名`）时序**：**严格从真实来源（DB）获取**，不预算/推算。查 `resource_store`→`persistent_store.file_path`。
3. **在建 vs 已建**：同 ②——皆从 DB 真实记录读（resource_store 创建时 file_path 即写入，文件未完成也算"在建"路径可知）。
4. **document 定稿时机**：不限制何时生成，但**只能在所有 image 路径被确认后定稿**（注意：不是"image 完成"，未完成的在建 image 路径也可查）。

## 三、SDK 能力设计（`library-squirrel-sdk`）

> **实施期对原设计的两处精化（2026-07-27，已落地）**：
> 1. **SDK 签名加 `taskId` 形参**——原写 `GetStoreRelPath(role, storeSeq)`。验证锚点后发现：插件 `Start(ctx, task, ...)` 在 `startDownload` 事务(`model.go:918`→`:929`)**之前**被调,首次运行时 `task.PendingResourceID` 尚未置位(资源在事务内创建);document 的 lazy gen 在 `downloadLoop`(pull 时,事务提交后)触发,此时宿主侧任务记录的 `PendingResourceID` 已就位,但插件 Start 时捕获的 task 仍无此值。故插件唯一可靠标识是 `task.ID`,RPC 须携带 `task_id` 由主程序映射到当前 `PendingResourceID`(即 §四"rpc 携带 taskId"路线,§三原签名与 §四内部不一致,以 §四为准)。落地签名:`GetStoreRelPath(taskId int64, role string, storeSeq int) (string, error)`。
> 2. **去掉 `StorePathStatus` enum**——原设计 OK/NOT_CREATED/RESOURCE_ABSENT 三态。插件 document genFunc 在任何非 OK 都 fail document(§五),三态不改变行为;保留则宿主适配器需 `(relPath, status, err)` 而 pluginContext 方法是 `(string, error)`,需额外 deps 透传接线。改为统一 `(string, error)` + gRPC error(`fmt.Errorf` 带语义原因:无 PendingResourceID / 无此 role+seq / DB 错),原因文本保留可调试性,契约最小。

- `dto/context.go PluginContext` 加方法：`GetStoreRelPath(taskId int64, role string, storeSeq int) (string, error)`。
- `dto/providers.go` 加 `StorePathQueryProvider`(HostDeps 契约):`GetStoreRelPath(ctx, taskId, role, storeSeq) (string, error)`。
- `transport/client.go PluginContextClient` 加实现(仿 `GetPluginRoot`):调 HostService gRPC,**5s deadline**(防主程序异常时 document lazy gen 无限阻塞)。
- `transport/host.go HostDeps` 嵌入 `dto.StorePathQueryProvider`;`HostServiceServer.GetStoreRelPath` 调 deps,失败 `fmt.Errorf` 返回(转 gRPC error)。
- `proto/plugin.proto HostService` 加 rpc:
  ```proto
  rpc GetStoreRelPath(GetStoreRelPathRequest) returns (GetStoreRelPathResponse);
  message GetStoreRelPathRequest { int64 task_id = 1; string role = 2; int32 store_seq = 3; }
  message GetStoreRelPathResponse { string rel_path = 1; }   // workDir 相对路径;失败走 gRPC error
  ```
- 重生成 `gen/`(protoc `--go_out` + `--go-grpc_out` + `module=github.com/lvfeng-z/library-squirrel-sdk`)。

## 四、主程序实现（`backend/`）

- `plugin/extension/plugin_context.go` 加 `StorePathQueryProvider` 接口（:15-44 模式）+ 注入 `PluginContextDeps:55`：
  ```go
  type StorePathQueryProvider interface {
      // StoreRelPath 按 (当前任务 PendingResourceID, role, storeSeq) 查真实落盘路径。
      // status: OK / NOT_CREATED(resource_store 无此 role+seq) / RESOURCE_ABSENT(无 PendingResourceID)
      StoreRelPath(ctx context.Context, resourceId int64, role string, storeSeq int) (relPath string, status int, err error)
  }
  ```
- `loader.go:352-424` 现有适配器群后追加 `hostStorePathProvider`（SDK HostDeps → Provider，仿 `hostPluginRootProvider:380` 模式）。**resource_id 来源**：HostService rpc 携带 extensionId/taskId → 主程序映射到当前任务 PendingResourceID（仿现有 rpc 的 taskId→resource 上下文）。
- DB 查询：`resource_store_repository.go:26 ListByResourceId(resourceId)` → `[]*ResourceStore`（含 store_type/store_seq/store_id），按 (role, store_seq) 过滤得 store_id → `StoreReader.GetById(store_id).FilePath`（relPath，workDir 相对）。
- **不暴露 bas 算法**：纯 DB 读，命名权仍集中主程序（N 规约）。
- **reportProgress 进度核算调整（lazy document 副作用）**：document spec 改 lazy 后 `Size=0`（生成前大小未知）。原 `reportProgress`(`model.go:1787`)对 `total` 只计 `size>0` 轨、对 `finished` 计全部轨 written——document 的 written 会令 `finished>total` 致进度 >100%（文本密集型专栏明显）。调整为 `size<=0` 轨既不计 total 也不计 finished（未知大小轨不参与进度核算,语义自洽);前端 `TaskOperationBarActive` 已处理 `total===0`(纯文本专栏仅 document 一轨时进度走 indeterminate,不除零)。完整性校验(`model.go:1370`)只校验 `GenerationDownloaded`,derived document 不受 Size=0 影响。

## 五、document 定稿时序（X2：lazy ReadCloser，已定）

用户决策④：document 在所有 image 路径确认后定稿。用户原则：**主程序是被动支持者，不插手插件资源生成流程**。X1（两阶段 Start）/X3（derived 独立后处理）让主程序越界主导生成（调插件产 derived / 规定何时产），否决；X4（占位+主程序替换）不符"插件自行占位"，否决。

**X2 核心**：document 的 ReadCloser 是 **lazy wrapper（插件侧）**，内容在被读（pull）时由插件生成。主程序不调插件生成，只正常 pull + 提供 SDK 能力 + 保证 resource_store 时序。

### 数据流时序

当前 document = `io.NopCloser(bytes.NewReader(mdBytes))`（Start 时定）改为 lazy：

```
插件 Start → document.ReadCloser = lazyWrapper(genFunc)          ← 占位,无内容
    ↓ (gRPC pull 模型,主程序 pull chunk)
主程序 startDownload 事务(model.go:929-959) → 所有 store 的 resource_store 写 DB 并提交(image 路径确认)
    ↓ 事务提交
主程序 downloadLoop(model.go:988,1218) 并行 pull 各 stream(go func per stream:1233-1235,wg.Wait:1253)
    ├─ image 流:正常下载
    └─ document 流:pull → 插件 serveSpecsPull 读 lazyWrapper → 触发 genFunc:
            · 遍历 InlineImages,调 ctx.GetStoreRelPath(taskId, "image", seq) 查 DB(已提交,必命中)
            · 用 filepath.Base(relPath) 提取 basename,替换正文 {{IMG:seq}} 占位
            · sync.Once 生成+缓存
         → 发 chunk → 主程序落盘 document(真实内容)
```

### 多 store 流并行 + 顺序不受控（不影响 X2）

downloadLoop（model.go:1218-1253）每 stream 一个 goroutine（1233 `for streams { go func }`），`wg.Wait`（1253）等所有完成。流并行、顺序由 OS 调度不受控——**但不影响 X2**：resource_store 建立（事务）与流读取（downloadLoop）是分离阶段，document 流无论先/后/并发 pull，image resource_store 都已在 DB（事务已提交）。

### resource_store 时序保证（X2 核心不变量）

**downloadLoop（model.go:988）一定在 startDownload 事务提交（model.go:959）之后**——代码顺序保证（非锁/信号），依赖事务提交的 ACID 可见性：

- document 流的 genFunc 调 GetStoreRelPath 走**独立 DB 连接**（gRPC server handler，不在 startDownload 事务连接内）
- 事务未提交时查询看不到（DB 隔离），但 downloadLoop 在提交后，所以 genFunc 查时**事务已提交，可见所有 resource_store**
- resume 同理（resumeFromPersistedState:1576 事务 → 1647 downloadLoop）

**不变量（需代码注释固化）**：`downloadLoop` 必须在 `startDownload`/`resumeFromPersistedState` 事务提交之后调用，否则 derived 的 lazy 查路径落空。

### 主程序：纯被动

| 主程序做什么 | 主程序**不**做什么 |
|---|---|
| 正常 downloadLoop pull chunk（不改） | ❌ 不调插件"生成 derived"（X1/X3 越界） |
| 提供 SDK `GetStoreRelPath`（查 DB） | ❌ 不规定 document 何时/如何生成 |
| 保证 resource_store 时序（事务先于 downloadLoop，已有） | ❌ 不主导生成流程 |

### 插件：完全主动

插件提供 lazy wrapper（document ReadCloser），内部 genFunc 自行决定查哪些路径、怎么拼 .md、错误如何处理。实现轮廓：

```go
type lazyMDReader struct {
    once    sync.Once
    content []byte
    gen     func() ([]byte, error)  // 查路径 + 拼 .md
}
func (r *lazyMDReader) Read(p []byte) (int, error) {
    r.once.Do(func() { r.content, _ = r.gen() })  // 首次 Read 触发生成
    // 从 content 续读...
}
```

bilibili 把 document spec 的 `ReadCloser: io.NopCloser(bytes.NewReader(mdBytes))` 换成 `newLazyMDReader(gen)`，gen 内遍历 InlineImages 调 `ctx.GetStoreRelPath(task.ID, "image", seq)` 拿真实路径,`filepath.Base` 提取 basename 替换正文 `{{IMG:seq}}` 占位拼 .md。正文结构(`![](<{{IMG:seq}}>)` 占位 + 文本段)存 PluginData.Markdown,不含真实文件名;真实文件名在 document 落盘侧(pull 时)替换。

### 保护机制

| 保护 | 来源 |
|---|---|
| resource_store 时序保证（查必命中 DB） | startDownload 事务代码顺序（model.go:929→提交:959→downloadLoop:988） |
| gRPC RPC 超时（不无限阻塞） | gRPC context deadline 机制 |
| 无死锁 | GetStoreRelPath 独立 unary RPC，主程序 gRPC server 独立 goroutine 处理（只读 DB，不持 downloadLoop 的 wg/锁）；pull stream 与 GetStoreRelPath 多路复用并发 |

### 待实施设计的细节

1. **genFunc 错误处理**：GetStoreRelPath 返回 NOT_CREATED（理论上事务保证下不发生，防御性）时 genFunc 行为——建议 fail document（触发任务失败暴露问题，不静默降级）。
2. **GetStoreRelPath RPC deadline**：设合理超时（如 5s），避免主程序异常时 document 流无限阻塞拖慢 downloadLoop。
3. **lazy wrapper 只触发一次**：sync.Once 生成 + 缓存，后续 Read 读缓存（document 是一次性小 .md）。

## 六、bilibili 插件适配

- `.md` 生成从 **Create 阶段**移到 **document 落盘侧(pull 时 lazy 生成)**:PluginData.Markdown 存正文结构(文本段 + `![](<{{IMG:seq}}>)` 图片占位,**不含真实文件名**);InlineImages 存图 URL 顺序(决定 store_seq)。
- dynamic.go 两处图片占位(CONTENT 内嵌图 + TOP 相册 fallback)用 `![](<{{IMG:seq}}>)`(`seq` = 追加前 `len(InlineImages)`,0-indexed,与 startArticle 的 `for seq := range InlineImages` 一致)。
- document spec 用 `newLazyMDReader(genArticleMarkdown(task, data))`;gen 遍历 InlineImages 调 `ctx.GetStoreRelPath(task.ID, "image", seq)` 取真实路径,`filepath.Base` 替换 `{{IMG:seq}}`。
- `BilibiliTaskHandler` 加 `ctx sdkdto.PluginContext` 字段(Activate 注入),供 lazy gen 调 `GetStoreRelPath`。
- 退役 `extFromURL`(dynamic.go,占位不再需扩展名——真实扩展名来自 DB file_path basename)+ 修正过时注释(:327-328 D4 残留)。
- 前端位置绑定(MarkdownView)可降级为容错(basename 真实匹配为主)——本次未改,留作可选。

### 实测发现并修复的两处 sentinel bug(2026-07-27)

首版 sentinel 为 `![](lsimg:%d)`,实测暴露两处缺陷,已修正为 `![](<{{IMG:%d}}>)`:

1. **前缀碰撞(多图场景)**:`lsimg:1` 是 `lsimg:10` 的前缀,genArticleMarkdown 升序 `for seq := range` 替换时 seq=1 把 seq=10 占位里的 `lsimg:1` 一并替换,残留尾 `0`(实测 task 2 第 11 张图引用变成 `image_001.jpg0`)。task 1 仅 4 张图(seq 0-3)未触发,task 2 有 11 张(seq 0-10)触发。改用闭合标记 `{{IMG:seq}}`——`{{IMG:1}}` 非 `{{IMG:10}}` 子串,任意替换顺序均安全。
2. **空格断 markdown(文件名含空格)**:N 命名规约的 bas 用作品标题,标题可含空格(实测 `海特洛衣橱 ｜ 伊洛伊时装展示`),markdown 图片目标遇空格即截断、缺 `)` 闭合 → 不被解析为 image token(外部查看器与 app 内 markdown-it 均中招)。改用 CommonMark `<...>` 包裹目标,空格/方括号/Unicode 全兼容;前端位置绑定按 token 计数 + 覆盖 src,不受影响。

> 验证前提:旧 sentinel `lsimg:seq` 仅存在于本任务未发布的首版,rebuild 后须**新建**任务复验(既有任务的 PluginData.Markdown 仍存旧 sentinel,新代码替换 `{{IMG:seq}}` 找不到 → 保留残留);无需向后兼容未发布格式。

## 七、范围 + 实施顺序

1. **SDK**：proto rpc + dto 接口 + client 实现 + gen 重生成。
2. **主程序**：StorePathQueryProvider + hostStorePathProvider 适配 + DB 查询 + taskId→resourceId 上下文。
3. **主程序时序不变量固化**：在 `downloadLoop` 调用处加注释（必须在 startDownload/resume 事务提交后）+ StorePathQueryProvider/hostStorePathProvider 实现 + DB 查询。
4. **bilibili**：.md 生成时机调整 + 真实占位 + 退役 extFromURL。
5. **前端**：位置绑定降级（可选）。
6. **实测**：article 跨重启，验证 .md 含真实文件名 + 兄弟引用有效。

## 八、风险与边界

- **resource_store 时序不变量**（§五，X2 隐式依赖）：`downloadLoop`（model.go:988/1647）必须在 startDownload（:929→提交:959）/resume（:1576）事务提交后。需代码注释固化，防未来重构破坏。X2 落地后充分实测（article 跨重启）。
- **跨仓发布兼容窗口**：发布顺序 SDK（rpc+能力）→ 主程序（HostService 实现）→ bilibili（lazyMDReader）。**bilibili 不能先于 SDK/主程序发布**——否则 genFunc 调 `GetStoreRelPath` 在旧 SDK/主程序无此 rpc → 失败。proto 加 rpc 向前兼容（旧插件不调无影响），但 bilibili 用新能力需三端同步。**回滚**：bilibili 回退到 NopCloser 占位（位置绑定兜底，已验证可用）。
- **命名权边界**（插件引用名一致性）：插件用 `GetStoreRelPath` 拿 relPath（DB file_path），.md 引用用 `filepath.Base(relPath)` 提取的 basename——同目录兄弟引用。插件**不需懂 N 规约**（role/seq→basename 映射），basename 直接来自 DB file_path，一致性天然保证（image 落盘文件名 = DB file_path 的 basename）。
- **genFunc 错误传播**：GetStoreRelPath 返回 NOT_CREATED（事务保证下不发生，防御性）时 genFunc 行为——建议 fail document（暴露问题，不静默降级）。RPC deadline（如 5s）防主程序异常时 document 流无限阻塞拖慢 downloadLoop。
- **范围边界**：本能力只管"插件查路径"。命名规约（N）不变；位置绑定降级非必须。
