# 图文紧密结合资源（专栏正文：富文本 + 内嵌图）设计方案

> **派生自**：bilibili 插件专栏(cv)实现（multitrack-resource-lineage 谱系 A 节点）。实现 cv 时发现专栏是图文紧密结合的富文档，当前离散文件存储丢失结构。
> **状态**：设计阶段（类型1 存储格式选型待用户拍板后展开实施）。
> **范围（2026-07-21 修订）**：本任务**聚焦类型1——图文交织资源**（正文 + 位置相关内嵌图，bilibili cv 驱动）。**类型2——站点现成的文档文件（pdf/doc/docx/txt/rtf 等）**因存储组织本质不同（原文件单 store、内嵌内容不拆、只缺前端多格式查看器），**reveal 为延后分支**（见文末"延后分支"），不在本任务统一选型，不强求格式转换。

## 一、问题与范围界定

"富文本资源"的数据源实质分两类，存储组织本质不同，强行统一格式不现实（需把每类反向转换，有损且引入无关复杂度）：

| 类型 | 代表 | cohesive 单位 | 正确存储 | 真正缺的能力 |
|---|---|---|---|---|
| **1. 图文交织**（本任务） | bilibili 专栏(cv) | 正文 + 图是可分离的独立资源，图位置相关、有独立价值 | 正文（Markdown/AST）+ N 图 store 引用——**新存储形态** | 资源模型（已就位，见 §三）+ md 渲染 + 图URL解析 |
| **2. 现成文档**（延后） | pdf/doc/docx/txt/rtf | 文件本身即 cohesive 单位，内嵌内容不拆 | **原文件单 store 做 main**（现有模型已支持） | 前端多格式查看器（纯渲染层） |

本任务 = 类型1。类型2 的存储层零改动、纯前端查看器工作，独立成延后分支。

> 注：图文动态(opus)也存在正文，但其形态偏"文字 caption + 图片分离"，单流图片 + 文字介绍尚可接受；**专栏**才是图文交织、强结构化的典型，最需要 cohesive 存储。

## 二、当前资源模型（约束，已核验 2026-07-21）

- `Resource`（属 Work）挂 N 个 `ResourceStore`，每个 store 指 `persistent_store`（一个离散文件）。
- `store_type` 是**开放枚举**（`backend/base/model/entity/resource_store.go:29` `StoreType string`，GORM 仅 `column:store_type`，**无 enum tag、无 DB CHECK、无白名单**；常量块 `resource_store.go:9-15` 是约定非强制）。新增类型（`article` 等）**不改表结构、无需后端改动**。
- `generation`：`downloaded`（流式可续传）/ `derived`（一次性派生）。
- 现有 store 都是单文件（图片/视频/缩略图）。**没有"多文件带结构"的 cohesive 文档类型**——但模型本身**已能表达**（1 Resource 挂 N store），缺口仅在"插件如何产出 + 前端如何渲染"，非模型层。

→ 模型层零改动（见 §三 核验）。

## 三、后端零改动核验（2026-07-21，两根支柱）

**结论**：本功能后端（主程序）**零逻辑改动**——是已有 P0 纪律叠加的必然结果，非权宜。

**支柱1·`store_type` 开放无校验**（已核验）：见 §二，插件可自由用 `article`/`document` 等值，后端不校验、迁移不约束。

**支柱2·前端获取 stores + file_path 的数据流已就位**（已核验）：
- 后端 DTO：`NewResourceFullDTO`（`resource_dto.go:67`）产出 `Stores []ResourceStoreDTO`（全量多轨，主数据源），每项含 `StoreType`/`Generation`/`Store`（`PersistentStoreDTO`，含 `FilePath`/`FileName`/`FilenameExtension`/`Width`/`Height`，`persistent_store_dto.go:14-24`）。
- 后端组装：Work 详情 service 调 `NewResourceFullDTO` 嵌入 `WorkFullDTO.Resource`（`backend/work/service.go:1181`）。前端通过 **Work 详情接口**拿到含 stores 的 Resource。
- 前端消费：`frontend/src/utils/ResourceUtil.ts` 已读 `resource.stores[].storeType` 与 `rs.store.filePath`（`isResourceMergeable`/`getResourceOpenPath`），证明数据流已通到前端。

→ 图文渲染所需的"图引用 → `/store/` URL 解析"，前端已有全部数据基础（stores + filePath）。

**已知非阻塞缺口**：`backend/resource/handler.go` 仅有 `GetById`/`ListByWorkId` 返回**瘦** `ResourceDTO`（无 Stores），无 `GetFullById` 直供方法。前端目前只能通过 Work 详情**间接**拿 Resource Full。对图文渲染（总发生在 Work/卡片上下文）**不阻塞**；若将来需在非 Work 上下文单独取 Resource Full，再补一个透传 service 既有能力的 handler 方法（非新逻辑）。

**执行面**（2026-07-20 已评估，见 §八）：`.md`(derived + `io.NopCloser`)复用 `streamController` 通用路径，内嵌图走标准下载流，**零改动**。

## 四、源结构（bilibili cv，阶段0 待确认）

专栏正文在 opus 页 `__INITIAL_STATE__.detail.modules[MODULE_TYPE_CONTENT].module_content.paragraphs[]`：
- 文本段：`para_type`+`text.nodes[]`（`word.words`/`rich.text`）—— 已能解析（图文动态复用）。
- **内嵌图段**：`paragraphs[]` 中应有图片节点（`para_type` 区分图文）——**结构待用一篇"图多+正文长"的真实专栏确认**（之前 cv dump 在 MODULE_TYPE_AUTHOR 截断，未看到 CONTENT 的图片节点）。

> 阶段0 第一步：取一篇富专栏，dump MODULE_TYPE_CONTENT，确认内嵌图节点形态（预计类似 `module_top.display.album.pics` 或 paragraph 内 `pic` 字段）。

## 五、设计选项（类型1 的存储格式，需拍板）

| 方案 | main store | 内嵌图 | 渲染 | 取舍 |
|---|---|---|---|---|
| **A. Markdown + 独立图 store**（推荐） | `.md`（derived，插件据结构生成） | 各图为 `downloaded` store，`.md` 按文件名引用 | 前端 md 渲染器 + 图 URL 解析 | 可编辑/可搜索/标准；需 md 渲染器 + 图引用解析 |
| B. 自包含 HTML | `.html`（derived，图内联 base64） | 内联 | WebView | 自包含、富格式；base64 膨胀、不可编辑 |
| C. 结构化 JSON | `.json`（derived，AST） | 独立 store | 自定义渲染器 | 结构精确；需专用渲染器、不通用 |

> ~~D. PDF~~：不适合类型1（不可编辑、丢可搜索性、需渲染管线）；PDF 是**类型2**的现成文件形态，归延后分支。

**推荐 A（Markdown）**：个人资源库场景下可编辑/可全文搜索/格式标准，图作为独立 store 复用现有下载/存储/备份链路。资源形态：1 个 `main`=`.md`(derived) + N 图 store(downloaded，`.md` 按相对文件名引用)，全部挂同一 Resource。

## 六、实施阶段（后端零改动版，选型确定后）

工作面仅两块：**插件侧**（bilibili cv 提取）+ **前端侧**（md 渲染）。后端阶段作废。

- **阶段 0 · 调研**：取富专栏确认 MODULE_TYPE_CONTENT 内嵌图节点；调研前端 markdown 渲染器（Vue 生态）+ 图引用→store URL 解析方案。
- **阶段 1 · 插件提取**（bilibili）：cv 富文本 → markdown（文本段 + `![](图)`），内嵌图作 downloaded store；声明各 store 的 `StoreSpec`（main `.md` derived + 图 downloaded）。定 `store_type`（复用 `main` 还是新增 `article`）与 `.md` 引用图的命名约定（图 store 的相对文件名 / basename）。
- **阶段 2 · 前端渲染**：资源详情/预览组件据 main store 的 format（`.md`）或 store_type 分发到 markdown 渲染器；`.md` 内 `![](文件名)` 经"文件名 → store `/store/` URL"映射 resolve（前端读 Resource.stores 的 filePath 构建）。
- **阶段 3 · 集成验证**：粘一篇富专栏 → 建任务 → 下载 `.md`+图 → 前端渲染为完整图文文章。

## 七、开放问题（需用户定）

1. **存储格式**：A（Markdown，推荐）/ B（HTML）/ C（JSON）？
2. **store_type**：复用 `main`（`.md`）+ 图 store，还是新增 `article` 类型？
3. **可编辑性**：是否要求 `.md` 可在库内编辑（影响是否纯 derived）？
4. **图引用解析**：`.md` 内图引用如何映射到图 store 的 `/store/` URL（按文件名 basename？前端 resolve 已有数据基础，见 §三）。
5. **范围**：仅 bilibili 专栏，还是设计成通用"图文交织资源"能力（其他 HTML 解析站点可用）？——已倾向**通用**（类型1 抽象，非 bilibili 专属）。

## 八、与 taskManager 执行面的关系（不加深耦合·2026-07-20 评估）

**结论**：本功能实现**不加深** taskManager 执行面/控制面耦合，`.md` 复用既有的 derived 通用路径，执行面零改动。

**理由**（推荐方案 A：`.md` derived + 内嵌图 downloaded）：
- **`.md`（derived）**：插件 `TaskHandler.Start` 返回 `{Role:main/article, Generation:derived, ReadCloser:io.NopCloser(mdBytes)}`，走 `streamController` 的 downloaded/derived 通用 reader→writer 拷贝路径（`backend/taskManager/model.go:298`）。缩略图（thumbnail）已是 `derived + NopCloser` 的现有案例（`model_multi_stream_test.go`），`.md` 与之同款。
- **内嵌图（downloaded）**：走标准下载流（`StoreSpec` 带 ReadCloser 流），与现有图片下载完全一致。
- **执行面改动**：零。只在数据模型层新增一个 `store_type` 常量（复用 `main` 或新增 `article`，开放枚举不改表）。

**衍生认知**（已同步到 `doc/plan/longops-task-integration-plan.md` §三）：`NopCloser` 是 derived store 的**标准产出方式**（非"伪装/撞墙"），StoreSpec 的 ReadCloser 契约既已包容派生文件产出。真正撞该契约的是纯计算/API 型操作（翻译/AI/查重，无文件产出），与本功能无关。

→ 本功能不触发 longops 阶段2（执行面剥离），可安心实现。

## 九、恢复定位（新会话续作所需——先读此节）

### 上游谱系与本任务位置
- 派生自 **multitrack-resource-lineage** 谱系 **A 节点（bilibili 插件）**——bilibili 专栏(cv)实现时发现图文紧密结合富文档、当前离散存储丢失结构，引出本平台级能力。
- 本任务独立树：`.claude/workflow/active/rich-article-resource/TREE.md`；设计计划：本文档。
- bilibili 插件已完整可用（视频/图文动态/专栏单流图片），本任务**不阻塞**它。

### 关键代码定位
- **当前资源/store 模型**（已核验零改动，见 §三）：
  - `backend/base/model/entity/resource.go`（Resource：属 Work，挂 N store）
  - `backend/base/model/entity/resource_store.go:29`（`StoreType string` 开放枚举，无校验）
  - `backend/base/model/entity/persistent_store.go`（`file_path`/`width`/`height`）
  - 路径/存储规则：`CLAUDE.md` + `.claude/rules/database.md`（workDir 相对路径、`store/resource/...` 子目录、`buildStoreUrl` 编码）
- **DTO 数据流**（前端拿 stores 的路径，已通）：
  - `backend/base/model/dto/resource_dto.go:67`（`NewResourceFullDTO` → `Stores[]`）
  - `backend/base/model/dto/persistent_store_dto.go:14-24`（`FilePath` 等）
  - `backend/work/service.go:1181`（Work 详情嵌入 ResourceFullDTO）
  - 前端 `frontend/src/utils/ResourceUtil.ts`（已消费 stores + filePath）
  - **非阻塞缺口**：`backend/resource/handler.go` 无 `GetFullById`（渲染在 Work 上下文，不影响）
- **bilibili 插件 cv 提取**（要扩展为产出 markdown 的源）：
  - `library-squirrel-plugin-bilibili/internal/bilibiliapi/article.go`：`GetArticleImages`（cv → `parseOpusState`）、`initialStateRe`（提取 `__INITIAL_STATE__`）
  - `library-squirrel-plugin-bilibili/internal/bilibiliapi/dynamic.go`：`parseOpusState`（解析 `detail.modules`：TITLE/CONTENT/TOP/AUTHOR）、`OpusInitialState`、`fetchOpusPageState`、`fetchOpusDetail`
  - `library-squirrel-plugin-bilibili/bilibili_task_handler.go`：`createImage`（cv/dynamic 建 parent+每图子任务、单流 main）——富文档需改为产出 `.md`(derived) + 内嵌图(downloaded)
  - `library-squirrel-plugin-bilibili/urls.go`：`ArticlePageURL`/`OpusPageURL`/`DynamicDetailURL`
- **插件↔平台 store 契约**（如何发 store）：
  - SDK `dto/handler_dto.go`：`StoreSpec`（Role/Generation/ReadCloser/Format/Size/Continuable）
  - `doc/plugin-dev-guide.md`：StoreSpec 字段速查 + **Format 前导点契约**（downloaded 轨带点、derived 缩略图不带点——`.md` 是 derived 文档，Format 带点 `.md`）
  - 生成方式：`.md` = **derived**（插件据 cv 结构生成，`io.NopCloser(bytes)` 一次性产出）；内嵌图 = **downloaded**（流式下载，可续传）
- **前端资源渲染**（要加 markdown 渲染）：`frontend/src/views/`（资源相关视图）+ store 文件预览组件——需定位"资源详情/预览"组件，加 markdown 渲染器 + 图引用→`/store/` URL 解析。

### 已确认的源结构事实（上一会话学到，勿重测）
- bilibili 已合并**专栏+动态为 opus**（[bilibili-api #662](https://github.com/Nemo2011/bilibili-api/issues/662)）；cv 页与 opus 页 `__INITIAL_STATE__` **同构**（`detail.modules`）。
- `web-dynamic-v1/detail` API 对图文返回**遗留 DRAW 形态**（无正文文字）；正文在**页面 `__INITIAL_STATE__`** 的 `detail.modules[MODULE_TYPE_CONTENT]`。
- MODULE_TYPE_CONTENT 正文：`module_content.paragraphs[].text.nodes[]`，节点取 `word.words`（文字）或 `rich.text`（emoji/富文本）。**内嵌图节点形态未确认**（见阶段0）。
- 标题在 `MODULE_TYPE_TITLE.module_title.text`；相册图在 `MODULE_TYPE_TOP.module_top.display.album.pics[].url`；作者在 `MODULE_TYPE_AUTHOR`/`basic.uid`。

### 立即下一步（阶段 0，选型拍板前后均可做）
1. **确认 cv 内嵌图节点**：取一篇"图多+正文长"的专栏，复用 bilibili 插件 `getRaw(页面)`+`initialStateRe` 抓 `__INITIAL_STATE__`，dump `MODULE_TYPE_CONTENT` 看 image paragraph/node 形态（预计 `para_type` 区分图文，或 paragraph 内 `pic`/`image` 字段；也可能图在 `module_top` 之外的独立模块）。
2. **调研前端 markdown 渲染器**（Vue 生态，如 markdown-it/vue-markdown）+ 图引用→store `/store/` URL 解析方案（前端已有数据基础，见 §三）。
3. **用户拍板存储格式**（默认 Markdown，见 §五）后，进阶段 1（插件提取：store_type 选型、`.md` 引用图命名约定）。

### 设计倾向（供新会话参考，非定论）
- 推荐 **A（Markdown + 独立图 store）**：1 个 `.md`(derived, `main` 或新 `article` 类型) + N 内嵌图(downloaded) 挂同一 Resource；`.md` 按图文件名引用，前端渲染时解析为图 store 的 `/store/` URL（数据基础已就位）。
- 富文档提取在**插件侧**（bilibili 据 cv 结构生成 markdown），主程序只管存储/渲染（**模块边界 MODULE_BOUNDARY_PURITY**：插件产出文档，主程序落盘/渲染）。
- **后端零改动**（已核验，见 §三）：不为图文资源改数据模型/执行面/handler；只在插件侧产出 store、前端侧加渲染。

## 延后分支（类型2：现成文档文件的多格式查看器）

**派生自本任务的设计反思**（2026-07-21）：识别"富文本资源"时发现类型2（站点现成 pdf/doc/docx/txt/rtf）存储组织与类型1 本质不同，不应统一选型。

- **本质**：原文件本身即 cohesive 单位，内嵌内容不拆；存储 = 原文件单 store 做 main（`downloaded` 或 `derived`，现有模型已支持，**后端零改动**）。
- **真正工作**：前端多格式查看器（pdf viewer / docx 渲染 / 纯文本 / rtf）——纯渲染层，不涉及资源模型/插件提取/存储契约。
- **为何延后**：与类型1 工作面完全不重叠（类型1 是插件提取 + md 渲染，类型2 是多格式查看器），硬捆进同一 epic 模糊焦点。等类型1 交付或用户需要文档查看时再 focus。
- **追踪**：在 `.claude/workflow/active/rich-article-resource/TREE.md` 中作为 `[延后]` 节点。
