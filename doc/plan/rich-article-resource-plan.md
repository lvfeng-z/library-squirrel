# 图文紧密结合资源（专栏正文：富文本 + 内嵌图）设计方案

> **派生自**：bilibili 插件专栏(cv)实现（multitrack-resource-lineage 谱系 A 节点）。实现 cv 时发现专栏是图文紧密结合的富文档，当前离散文件存储丢失结构。
> **状态**：设计阶段（选型待定，需用户拍板存储格式后展开实施）。

## 一、问题

bilibili 专栏（cv）等是**图文紧密结合**的富文档：大段格式化正文（标题/段落/列表）+ **位置相关**的内嵌图片。当前实现只提取 `MODULE_TYPE_TOP` 的相册图（每图一个 `main` 子任务），**丢失了正文与图文相对位置**——文章被拆成一堆图，不再是可读的文章。

> 注：图文动态（opus）也存在正文，但其形态偏"文字 caption + 图片分离"，单流图片 + 文字介绍尚可接受；**专栏**才是图文交织、强结构化的典型，最需要 cohesive 存储。

## 二、当前资源模型（约束）

- `Resource`（属 Work）挂 N 个 `ResourceStore`，每个 store 指 `persistent_store`（一个**离散文件**）。
- `store_type` 是**开放枚举**（`main`/`thumbnail`/`videoTrack`/`audioTrack`/`merged`…），新增类型不改表结构。
- `generation`：`downloaded`（流式可续传）/ `derived`（一次性派生）。
- 现有 store 都是**单文件**（图片/视频/缩略图）。**没有"多文件带结构"的 cohesive 文档类型**。

→ 缺口：无法表达"一份正文 + N 张内嵌图（位置相关）"的富文档。

## 三、源结构（bilibili cv，待确认）

专栏正文在 opus 页 `__INITIAL_STATE__.detail.modules[MODULE_TYPE_CONTENT].module_content.paragraphs[]`：
- 文本段：`para_type`+`text.nodes[]`（`word.words`/`rich.text`）—— 已能解析（图文动态复用）。
- **内嵌图段**：`paragraphs[]` 中应有图片节点（`para_type` 区分图文）——**结构待用一篇"图多+正文长"的真实专栏确认**（之前 cv dump 在 MODULE_TYPE_AUTHOR 截断，未看到 CONTENT 的图片节点）。

> 实施第一步：取一篇富专栏，dump MODULE_TYPE_CONTENT，确认内嵌图的节点形态（预计类似 `module_top.display.album.pics` 或 paragraph 内 `pic` 字段）。

## 四、设计选项（存储格式，需拍板）

| 方案 | main store | 内嵌图 | 渲染 | 取舍 |
|---|---|---|---|---|
| **A. Markdown + 独立图 store**（推荐） | `.md`（derived，插件据结构生成） | 各图为 `downloaded` store，`.md` 按文件名引用 | 前端 markdown 渲染器 + 图 URL 解析 | 可编辑/可搜索/标准；需 md 渲染器 + 图引用解析 |
| B. 自包含 HTML | `.html`（derived，图内联 base64） | 内联 | WebView | 自包含、富格式；base64 膨胀、不可编辑 |
| C. 结构化 JSON | `.json`（derived，AST：段落/文本/图引用） | 独立 store | 自定义渲染器 | 结构精确；需专用渲染器、不通用 |
| D. PDF | `.pdf`（渲染产物） | 内嵌 | PDF 查看器 | 便携/固定版式；不可编辑、丢可搜索性、需渲染管线 |

**推荐 A（Markdown）**：个人资源库场景下可编辑/可全文搜索/格式标准，图作为独立 store 复用现有下载/存储/备份链路。资源形态：1 个 `main`=`.md`（derived）+ N 个图 store（downloaded，`.md` 按相对文件名引用），全部挂同一 Resource。

## 五、实施阶段（选型确定后）

- **阶段 0 · 调研**：取富专栏确认 MODULE_TYPE_CONTENT 内嵌图节点；调研前端 markdown 渲染器（Vue 生态）+ 图引用→store URL 解析方案。
- **阶段 1 · 资源/存储契约**：定 store_type（复用 `main` 还是新增 `article`）、`.md` 引用图的命名/解析约定、derived `.md` 的生成与落盘。
- **阶段 2 · 插件提取**（bilibili）：cv 富文本 → markdown（文本段 + `![](图)`），内嵌图作 downloaded store；声明 `InvolvedRoles`。
- **阶段 3 · 主程序存储/渲染支撑**：derived `.md` store 的落盘/备份；前端 markdown 渲染 + 图 URL 解析（引用 → 对应图 store 的 `/store/` URL）。
- **阶段 4 · 集成验证**：粘一篇富专栏 → 建任务 → 下载 `.md`+图 → 前端渲染为完整图文文章。

## 六、开放问题（需用户定）

1. **存储格式**：A（Markdown，推荐）/ B（HTML）/ C（JSON）/ D（PDF）？
2. **store_type**：复用 `main`（`.md`）+ 图 store，还是新增 `article` 类型？
3. **可编辑性**：是否要求 `.md` 可在库内编辑（影响是否纯 derived）？
4. **图引用解析**：`.md` 内图引用如何映射到图 store 的 `/store/` URL（按文件名约定？需主程序 resolve）？
5. **范围**：仅 bilibili 专栏，还是设计成通用"富文档资源"能力（其他站点/插件可用）？

## 七、恢复定位（新会话续作所需——先读此节）

### 上游谱系与本任务位置
- 派生自 **multitrack-resource-lineage** 谱系 **A 节点（bilibili 插件）**——bilibili 专栏(cv)实现时发现图文紧密结合富文档、当前离散存储丢失结构，引出本平台级能力。
- 本任务独立树：`.claude/workflow/active/rich-article-resource/TREE.md`；设计计划：本文档。
- bilibili 插件已完整可用（视频/图文动态/专栏单流图片），本任务**不阻塞**它。

### 关键代码定位
- **当前资源/store 模型**（要扩展的目标）：
  - `backend/base/model/entity/resource.go`（Resource：属 Work，挂 N 个 store）
  - `backend/base/model/entity/resource_store.go`（ResourceStore：`store_type` 开放枚举 `main/thumbnail/videoTrack/audioTrack/merged` + `generation` `downloaded/derived` + `OrderIdx`；**新增类型不改表结构**）
  - `backend/base/model/entity/persistent_store.go`（store 文件：`file_path`/`width`/`height`）
  - 路径/存储规则：`CLAUDE.md` + `.claude/rules/database.md`（workDir 相对路径、`store/resource/...` 子目录、`buildStoreUrl` 编码）
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
- MODULE_TYPE_CONTENT 正文：`module_content.paragraphs[].text.nodes[]`，节点取 `word.words`（文字）或 `rich.text`（emoji/富文本）。**内嵌图节点形态未确认**（见下一步）。
- 标题在 `MODULE_TYPE_TITLE.module_title.text`；相册图在 `MODULE_TYPE_TOP.module_top.display.album.pics[].url`；作者在 `MODULE_TYPE_AUTHOR`/`basic.uid`。

### 立即下一步（阶段 0，选型拍板前后均可做）
1. **确认 cv 内嵌图节点**：取一篇"图多+正文长"的专栏，复用 bilibili 插件 `getRaw(页面)`+`initialStateRe` 抓 `__INITIAL_STATE__`，dump `MODULE_TYPE_CONTENT` 看 image paragraph/node 形态（预计 `para_type` 区分图文，或 paragraph 内 `pic`/`image` 字段；也可能图在 `module_top` 之外的独立模块）。
2. **调研前端 markdown 渲染器**（Vue 生态，如 markdown-it/vue-markdown）+ 图引用→store `/store/` URL 解析方案。
3. **用户拍板存储格式**（默认 Markdown，见 §四）后，进阶段 1（资源/存储契约：store_type 选型、`.md` 引用图命名/解析约定）。

### 设计倾向（供新会话参考，非定论）
- 推荐 **A（Markdown + 独立图 store）**：1 个 `.md`(derived, `main` 或新 `article` 类型) + N 内嵌图(downloaded) 挂同一 Resource；`.md` 按图文件名引用，前端渲染时解析为图 store 的 `/store/` URL。
- 富文档提取应在**插件侧**（bilibili 据 cv 结构生成 markdown），主程序只管存储/渲染（**模块边界 MODULE_BOUNDARY_PURITY**：插件产出文档，主程序落盘/渲染）。

