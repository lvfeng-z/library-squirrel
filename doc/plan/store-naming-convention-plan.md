# 多 store/多资源落盘命名规约（任务 N）

> **⚠️ v3 决策修订（2026-07-26）—— D1 回到位置绑定（A 方案），前置命名全部废弃**
>
> **修订理由**：重新审视前置命名（`host.ResolveStorePath`）的需求来源——它为 bilibili article 的 `.md` 正文引用内嵌图物理 basename（`![001.jpg](001.jpg)`）而设计。但"元数据内容依赖资源文件命名"本身是耦合设计，不合理：`.md` 是内容（document store），不应依赖图片（image store）的物理文件名。**问题在 bilibili 插件，不在主程序架构**。主程序不该为让这个错误耦合工作而引入前置命名整套架构（RPC + sameRoleCount + 改命名规则 + 跨仓库改动）。
>
> **正确机制（已实现）**：`.md` 占位是逻辑符号，渲染层做逻辑→物理映射——位置绑定（A 方案）。前端 `MarkdownView.vue` 的 `renderer.rules.image` 第 k 个 image token → 第 k 个 image store（按 store_seq 升序）。R 阶段3 已实测通过。
>
> **Phase 1**：命名函数去方法化（commit `3155626`）经回退（commit `10bcd75`）撤销——代码回到 `33ba595` 位置绑定状态（编译通过）。历史保留（`3155626` + revert 可追溯）。
>
> **废弃**：v2 的前置命名规约设计（§5.1-5.8）+ Phase 2-4 实施（SDK RPC + 主程序 HostDeps 反查）+ D1/D3-D7 决策（均关联前置命名）。以下 v2 内容保留供推导追溯，**不再执行**。
>
> **N 剩余治理（延后，暂不做）**：位置绑定不变量守卫、D2 模板空兜底、占位符语义、命名规约文档。

---

> **审查摘要**（v2 历史内容，前置命名相关已由 v3 废弃，保留供推导追溯）
> - **核心机制**：前置命名 + 确定性命名函数。命名规则归主程序（`resolveStorePath` 实现不外移），通过 SDK `host.ResolveStorePath(role, seq, format)` 暴露给插件在生成期调用；两端各自调用同一确定性函数、结果必然一致，**无 RPC 往返协调**。一致性靠三同源保证：规则同源（主程序独占实现）+ 作品元数据同源（主程序 Create 时单点持有，插件不传）+ seq 同源（插件产出顺序 = spec 数组顺序 = store_seq）。
> - **关键不变量**：`store_seq`（同 role 内 0-based 序号）在 DB 字段（`mountResourceStores` `model.go:1148-1177`）+ 文件名后缀（`resolveStorePath` `model.go:1996-2011`）+ resume 身份键（`model.go:1896-1914`）+ `.md` 引用 四处统一。
> - **决策已定**：D1 前置命名/确定性函数；D2 settings 强制非空 + DB 主键派生兜底；D3 规约文档 + 架构落地（Phase 1-4 核心原子发布、5-8 配套）。**v2 补 sub-决策**：D4 缩略图纯派生（§5.6）、D5 `${downloadTime*}` 剥离（§5.5）、D6 `.md` 占位生成延后到 Start 期（§5.7）、D7 回滚与多插件发布策略（§六末）。
> - **自曝风险**：① 锚点行号需按当前 HEAD 复核（审查已发现 `resolveStorePath` 实际 1996 非 1994）；② `resolveMainPath` 当前绑在 `ManagedTask` 上（依赖 `m.workResp` 作品元数据 / `m.deps` 设置注入），解耦为纯函数须审计全部调用点（清单见 §六 Phase 1）；③ SDK proto 改动**向前兼容**（新增 RPC 不破坏旧插件），但 bilibili 须升级 SDK 才能用；④ bilibili `.md` 当前在 `parseOpusState`（解析期）生成，D6 已定延后到 Start 期；⑤ `${downloadTime*}` 若用户存量模板在用，剥离后该占位符失效（日志提示），唯一性转由 DB 主键兜底——**不破坏存量文件**（只影响新生成）。

## 一、背景与派生

**派生自上游任务 R（图文紧密结合资源），非阻塞引出**：R 实施 article（类型1：document `.md` + N 内嵌图 image）时发现——作品:资源从 1:1（1 作品 1 文件）演进到 1:N（1 作品 N 资源、每资源 N store）后，store 的 `file_path` 落盘命名规约**从未系统设计**。R 暂用"序号 basename + 前端位置绑定"推进。本任务（N）正式补齐：防跨资源/跨 store 碰撞、保证确定性、理顺 `.md` basename 与落盘名的关系。

**根因**：`.md` 占位 basename（插件解析期生成）与图片实际落盘名（主程序模板拼）是两套体系，源于**命名时序错位**——插件生成内容远早于主程序命名。解法不是"主程序预命名后下发"（协议倒换，RPC 往返重），而是**把命名逻辑做成确定性纯函数、两端共用**：命名规则仍归主程序，插件生成期调用同一函数，结果必然一致。

## 二、现状审计（Explore 2026-07-26，跨四仓库）

### 2.1 命名来源全景：3 层 + 1 派生 + 1 兜底
命名权集中在主程序，插件只贡献素材（`Format` 扩展名 / `SuggestName` 建议名 / `Markdown` 占位）。但 article 的 `.md` 占位与图片落盘名走**两套互不通气**的体系。

### 2.2 各命名来源清单（主程序 / SDK / 插件）

| # | 位置 | 命名逻辑 | 风险 |
|---|------|----------|------|
| 1 | `backend/taskManager/model.go:1968-1992` `resolveMainPath` | 读 `FileNameFormat` 模板 → `ExtractTokenData` 取占位数据 → `FormatFileName` 替换 → `SanitizeFileName` 净化 + ext；`store/resource/<authorDir>/<fileName>` | 中（`${downloadTime*}` 不确定） |
| 2 | `backend/taskManager/model.go:1996-2011` `resolveStorePath` | 同 role 多 store 加 `_%03d` 后缀（从 0 起），同步改 relPath 末段 | 低（已修 relPath 同步） |
| 3 | `backend/taskManager/model.go:2154-2170` | 缩略图派生：`store/thumbnail/` + basename + `_thumbnail.<format>` | 低（依赖主资源 relPath） |
| 4 | `backend/taskManager/model.go:2022-2038` `buildSuggestedFileName` | **模板空兜底**：`SuggestName` → 否则 `task`/`TaskName` + ext | **高**（撞名无声覆盖） |
| 5 | `backend/settings/model.go:65` + `service.go:29,98-100` | 默认模板 `"[${author}]_[${siteWorkId}]_${siteWorkName}"` | 中（占位符未识别保留字面） |
| 6 | `backend/util/filename/template.go:36-106` + `sanitize.go:6-21` | `strings.NewReplacer` 替换 `${...}`；sanitize 非法字符→全角 | 低（不替换 `[]`/`${}`） |
| 7 | `backend/resource/merge_service.go:140-145,183-190` + `persistentStore/service.go:640-651` | merged 派生：源 relPath + `_merged` | 低 |
| 8 | `backend/persistentStore/service.go:216-260` `StoreStream` + `dir.go:18-36` | `filepath.Join(workDir, relPath)`；**已有记录删旧建新** | **关键防线**（碰撞=无声覆盖） |

- **SDK**：`dto/handler_dto.go:113` `StoreSpec.SuggestName` + `proto/plugin.proto:284` 透传契约。⚠️ `gen/plugin.pb.go:1032` 残留旧 `TaskResourceDTO.SuggestName`（已废弃，Phase 8 清理）。
- **pixiv**（`task_handler.go:344-356`）：唯一给 `SuggestName` 的现役插件。
- **bilibili**（`E:/code/lvfeng/library-squirrel-plugin-bilibili/`）：全不给 `SuggestName`。article `.md` 占位见 2.3。
- **local**（`task_handler.go:162-218`）：`SiteWorkID=fileHash` 唯一。

### 2.3 关键耦合：article `.md` 占位 basename（本方案解决对象）
```go
// bilibili internal/bilibiliapi/dynamic.go:255-257
basename := fmt.Sprintf("%03d%s", len(r.InlineImages)+1, extFromURL(pic.URL))  // 从 1 起:001,002
mdParts = append(mdParts, "!["+basename+"]("+basename+")")                      // ![001.jpg](001.jpg)
```
图片实际落盘名 = 主程序模板拼 `[作者]_[workId]_作品名_000.jpg`（从 0 起）。**两套 basename 体系、序号起点错位（1 vs 0）**，当前靠前端 `MarkdownView.vue:27-37` 位置绑定兜底。

## 三、问题与风险

### 3.1 高风险碰撞场景
1. **模板空 + 插件不给 SuggestName**（bilibili/local）→ 落 `task.ext`/`<TaskName>.ext` → `StoreStream` 删旧建新无声覆盖。**最严重**。
2. `${siteWorkId}` 缺失 → 字面保留 → 同作者同名碰撞。
3. `${downloadTime*}` 不确定（秒级孤儿、日级同日重下碰撞）。

### 3.2 规约盲区
- `.md` 占位与落盘名两套体系（靠位置绑定，无强不变量守卫）。
- 模板空兜底无防护。
- SuggestName 语义分裂（pixiv 给、bilibili/local 不给）。

### 3.3 已合规保留
rootDir + relPath 拼接、`validatePath` 白名单、store_seq 四处共用、模板+sanitize 管线。

## 四、决策（2026-07-26 定案）

| 编号 | 决策 | 理由 |
|---|---|---|
| **D1** | 前置命名 / 确定性命名函数 | 命名权留主程序（实现 `resolveStorePath`），SDK 暴露 `host.ResolveStorePath`，两端各自调用、确定性保证一致，无 RPC 往返。非"位置绑定"（运行时耦合）、非"主程序改写 .md"（破坏职责边界）、非"协议倒换"（RPC 重）|
| **D2** | settings 强制非空 + DB 主键派生兜底 | 双保险防撞名 |
| **D3** | 规约文档 + 架构落地 | Phase 1-4 四端原子发布核心、5-8 配套 |
| **D4** | 缩略图 = 主资源 relPath 的纯字符串派生 | 解决缩略图命名时序（详见 5.6）|
| **D5** | `${downloadTime*}` 剥离，不参与命名 | 确定性硬约束（详见 5.5）|
| **D6** | `.md` 占位生成延后到 Start 期 | 解决 workMeta 时序（详见 5.7）|
| **D7** | 回滚靠 proto 向前兼容 + 前端位置绑定保留为兜底 | 详见 §六末 |

## 五、规约设计

### 5.1 命名权与时序
- **命名规则归主程序**：`resolveStorePath` 实现留在主程序，插件不持有规则、不见模板与元数据。
- **命名时序前置**：命名函数在资源生成期即可调用。插件生成内容（含 `.md` 占位、跨 store 引用）时，调用主程序函数取真实落盘名。
- **执行模型**：两端各自调用同一确定性函数，**非主程序算完下发**。无 RPC 往返、无时序依赖。

### 5.2 命名函数契约
```
主程序纯函数:
  ResolveStorePath(workMeta, role, seq, format) → (relativePath, fileName)
  - workMeta: 作品元数据(author/siteWorkId/siteWorkName),主程序 Create 时持有
  - role: entity.StoreType 之一(image/document/thumbnail/videoTrack/audioTrack/merged)
  - seq: 同 role 内 0-based 序号(插件产出顺序)
  - format: 扩展名(内容生成期得知)
  - 纯函数:同输入恒同输出,无副作用,无时变依赖

SDK 暴露(插件调用形态):
  host.ResolveStorePath(role, seq, format) → relativePath
  - 插件只传三参;主程序 HostDeps 实现内部用当前 task 的 workMeta + settings 模板算
  - 返回相对 workDir 的 relativePath
```

### 5.3 一致性保证（三同源 + 确定性）
| 输入 | 归属 | 同源原因 |
|---|---|---|
| 命名规则 | 主程序独占 | 插件经 SDK 调主程序实现，不见规则 |
| 作品元数据 | 主程序单点持有 | Create 时从插件 WorkResponse 收到，插件不传 |
| seq | 插件产出顺序 | = spec 数组顺序 = store_seq（主程序按序赋值） |

函数确定性 + 三同源 → 两端任意时刻调用结果必然一致。

### 5.4 确定性约束（强不变量）
- 命名输入**禁止时变**：前置命名要求同一张图两次调用（插件生成期 + 主程序落盘期）必须同名；若命名依赖"当前时间"，两次调用时间不同 → 名不同 → 破坏一致性。
- **残留检测**：sanitize 后若残留字面 `${...}`（占位符未识别，如 `${siteWorkId}` 站点未返回），视为命名异常 → 走 D2 兜底名（保留原因为日志诊断）。

### 5.5 `${downloadTime*}` 处理（D5 定案）
- **剥离**：命名函数计算 file_path 时**忽略** `${downloadTime*}` 占位符（不参与文件名）。理由：时间戳本质时变，与确定性硬约束冲突，无法安全参与命名。
- 用户模板含 `${downloadTime*}` 时：命名时剥离 + 日志提示"时间戳占位符不参与命名，唯一性由作品元数据保证"。
- 文件唯一性由"作品元数据（author+siteWorkId+siteWorkName）+ 同 role seq + D2 DB 主键兜底"保证，不依赖时间戳。
- **不破坏存量**：已落盘文件不受影响，只影响新生成文件的命名。

### 5.6 缩略图命名时序（D4：现状已满足，无需改造）
- **Phase 1 数据流验证**：缩略图名 = 主资源 relPath 的纯字符串派生。`resolveMainPath` 算出逻辑 `mainRelPath`（未落盘）→ `resolveStorePath` 接收 → `buildThumbnailRelPath`（`model.go:2154-2162`）消费该逻辑路径做纯变换（去 `store/resource/` → `store/thumbnail/` + basename + `_thumbnail.<format>`）。**缩略图派生本就不依赖落盘状态**，前置命名期天然可用。
- **结论**：Phase 1 无需缩略图相关代码改动；命名函数解耦（去方法化）保持了这一性质。
- 缩略图 seq：缩略图是 derived 且每 resource 至多 1 个（thumbnail 的 Max=1），无同 role 多 store 问题，seq 恒 0。

### 5.7 `.md` 占位生成时序（D6 定案）
- **问题**：bilibili `parseOpusState`（解析期，可能在插件 Create 内）生成 `.md` 时调 `host.ResolveStorePath` 需 workMeta 就绪，但 Create 期 task/workMeta 可能尚未建立 → RPC 无法定位 task。
- **解法**：`.md` 占位生成**延后到 Start 期**。`parseOpusState` 只收集段落结构 + InlineImages（不产最终 `.md`）；`startArticle`（Start 期，task/workMeta 必然已建立）调 `host.ResolveStorePath(image, k, ext)` 取真实 basename 后生成 `.md`。
- **验证点**：Start 期 host.ResolveStorePath 能定位到当前 task 的 workMeta（SDK 已有 task-scoped HostService 调用先例，如 `GetWorkSetBySiteWorkSetId`）。

### 5.8 `.md` basename 一致性（问题①解法）
bilibili 生成 `.md` 占位时，调 `host.ResolveStorePath(image, k, extFromURL(url))` 取真实 relativePath，用其 basename 替代 `fmt.Sprintf("%03d%s", ...)`：
```
// startArticle 改造后(伪代码,Start 期)
for k, picURL := range inlineImages {
    imgRelPath, _ := host.ResolveStorePath(image, k, extFromURL(picURL))
    basename := filepath.Base(imgRelPath)          // 真实落盘 basename
    mdParts = append(mdParts, "!["+basename+"]("+basename+")")
}
```
- 落盘名（主程序调同函数）与 `.md` 占位（插件调同函数）必然一致。
- 前端位置绑定**保留为兼容兜底**（D7），标准 markdown 渲染即可正确显示。

### 5.9 兜底防护（问题②解法，D2）
- **settings 层**：保存 `FileNameFormat` 时校验非空（空则拒绝或回退默认模板）。
- **buildSuggestedFileName 兜底改造**：模板空且无 SuggestName 时，落盘名改用 DB 主键派生 `<workId>-<resourceId>-<storeSeq>.<ext>`，relPath = `store/resource/<fileName>`（兜底场景省略 authorDir，因 workMeta 可能缺失），仍以 `store/resource` 开头 → 兼容 `validatePath` 白名单。DB 主键稳定，resume/重命名重算结果一致。

## 六、实施步骤

### 核心架构（Phase 1-4，四端原子发布）

**Phase 1 · 主程序命名函数解耦**（无依赖）
- 把 `resolveMainPath`（`model.go:1968-1992`）从 `ManagedTask` 方法抽为纯函数 `ResolveStorePath(workMeta, role, seq, format, settings) (relativePath, fileName)`，去掉对 `m.workResp`/`m.deps.FileNameFormatProvider` 的绑定，改显式传参。
- **当前 `resolveStorePath` 调用点清单**（须全部改调纯函数）：`model.go` 落盘主循环（downloadLoop 内 store 挂载处）+ resume 路径（`resumeArticle`/`resumeVideo`/`resumeImage` 对应的 store 重挂处）+ `mountResourceStores`（`model.go:1148-1177`）。执行前 grep `resolveStorePath\|resolveMainPath` 全部消费点确认无遗漏。
- 缩略图派生改造（D4）：`buildThumbnailRelPath` 输入源从已落盘 relPath 改为纯函数算出的主资源 relPath。

**Phase 2 · SDK HostService 暴露**（依赖 Phase 1）
- proto 加 `ResolveStorePath(role, seq, format) → StorePathResponse`（**向前兼容**：新增 RPC，旧插件不调用仍正常）。
- SDK `PluginContext` 加 `ResolveStorePath(role string, seq int, format string) (string, error)`。
- 主程序 `HostDeps` 实现：用当前 task 的 workMeta + settings 调 Phase 1 纯函数。

**Phase 3 · 主程序落盘流程对齐**（依赖 Phase 1）
- taskManager 落盘时 `resolveStorePath` 改调纯函数；`store_seq` 赋值（`mountResourceStores`）与函数 seq 参数对齐（同源，确认无漂移）。
- 移除后置命名对 spec 顺序的隐式依赖，改显式 seq 契约。

**Phase 4 · bilibili 插件 `.md` 改造**（依赖 Phase 2）
- `parseOpusState`（`dynamic.go`）改为只收集段落结构 + InlineImages，不产最终 `.md`。
- `startArticle`（Start 期）调 `host.ResolveStorePath(image, k, ext)` 取真实 basename 生成 `.md`（D6）。
- resume：document 整轨重产时同样调 host 取真实名（image 已落盘则结果与首次一致）。

### 配套防护与文档（Phase 5-8）

**Phase 5 · D2 兜底防护**（独立）
- `backend/settings/service.go` 保存校验 `FileNameFormat` 非空。
- `buildSuggestedFileName`（`model.go:2022-2038`）兜底改 DB 主键派生名（§5.9）。

**Phase 6 · 占位符语义规约**（依赖 Phase 1）
- `${downloadTime*}` 剥离实现（§5.5）。
- sanitize 残留 `${...}` 检测 + 兜底回退（§5.4）。

**Phase 7 · 前端兼容层**（依赖 Phase 4）
- `MarkdownView.vue` 位置绑定**保留为兼容兜底**（支持真实名/占位两种 `.md`），不删除。标准 markdown 渲染即可正确显示真实名 `.md`。

**Phase 8 · 文档与清理**
- 新建 `doc/store-naming-convention.md`（命名规约：权属、时序、契约、确定性约束、占位符语义）。
- `doc/plugin-dev-guide.md` 补 `host.ResolveStorePath` 契约说明。
- 清理 SDK `gen/plugin.pb.go:1032` 废弃 `TaskResourceDTO.SuggestName`（确认无引用后）。

### D7 · 回滚与多插件发布策略

**回滚**：
- Phase 2 proto 新增方法**向前兼容**（不破坏旧契约），SDK/主程序可单独发布而不影响未升级插件。
- 前端位置绑定**保留为兼容兜底**（Phase 7 不删除），支持 bilibili 回退到占位模式时前端仍正确渲染。
- Phase 4 bilibili 可独立回滚（回占位模式，前端兜底）。
- Phase 1/3 主程序内部重构，git revert 即可。

**多插件发布顺序**：
- pixiv/local **不需改**（非 article，不用 `ResolveStorePath`）。
- SDK proto 新增方法向前兼容，pixiv/local **不强制升级**（旧 SDK 客户端不受影响）。
- 发布顺序：主程序 Phase 1-3 + SDK Phase 2 先发 → bilibili 升级 SDK + Phase 4 后发。pixiv/local 可保持现状。

## 七、验收点
1. article `.md` 落盘后，占位 basename 与 image store `file_path` basename **严格一致**（可脱离主程序查看）。
2. 前端标准 markdown 渲染（位置绑定作兜底）正确显示内嵌图。
3. 模板空时落盘名唯一（DB 主键派生，无 `task.ext` 撞名）。
4. 同输入多次调用 `ResolveStorePath` 结果一致（确定性）。
5. resume 后命名与首次一致（document 整轨重产、跨重启）。
6. 缩略图在主资源未落盘时命名正确（纯派生）。
7. `${downloadTime*}` 模板下命名仍确定（剥离生效）。
8. 四端编译通过 + 现有 taskManager/entity 单测无回归 + 新增命名函数确定性单测。

## 八、风险与边界
- **改动面**：Phase 1 解耦 `resolveMainPath` 须审计全部调用点（§六 Phase 1 清单）；Phase 2 proto 改动向前兼容但 bilibili 须升级 SDK。
- **bilibili 时序**：D6 已定 `.md` 延后到 Start 期，须确认 Start 期 host 可定位 task workMeta（§5.7 验证点）。
- **时间戳兼容**：D5 剥离 `${downloadTime*}`，存量文件不受影响，只影响新生成（日志提示）。
- **范围边界**：本方案聚焦"命名规约 + article `.md` 一致性"。前置命名的远期收益（跨资源引用、落盘前冲突预检、确定性身份作缓存键）是副产品，不在本方案实施范围，需时另起。
