# 图文紧密结合资源（专栏正文：富文本 + 内嵌图）开发计划

> **派生自**：bilibili 插件专栏(cv)实现（multitrack-resource-lineage 谱系 A 节点）。实现 cv 时发现专栏是图文紧密结合的富文档，当前离散文件存储丢失结构。
> **状态**：设计 + 可执行实施（2026-07-21 重核验升级）。**⚠️ 依赖前置基建 K「资源类型规约体系」(`doc/plan/resource-display-kind-design.md`)——选型已走乙方案(article=kind=article+spec)，"后端零改动"结论作废、待 K `done` 后回归更新本文 D2/§四/§十**。前端渲染入口已定位；当前焦点在 K（见 `.claude/workflow/active/rich-article-resource/TREE.md`）。
> **范围**：本任务聚焦**类型 1——图文交织资源**（正文 + 位置相关内嵌图，bilibili cv 驱动）。**类型 2——站点现成的文档文件（pdf/doc/docx/txt/rtf）**reveal 为延后分支（见文末），不在本任务统一选型。

## 审查摘要（给审查者，≤25 行；本段为审查入口，详情见其后各节）

> 审查本文档只需读本段 + 抽查下列证据锚点 + 回答待决策项，无需通读全文。

**关键声明（抽查项）**——正文"已核验 / 已就位 / 零改动"类论断全部在此登记并挂证据：
- **C1. `store_type` 开放无校验（零迁移即可新增类型）** — 证据：`backend/base/model/entity/resource_store.go:29`（GORM 仅 `column:store_type`，无 enum tag / DB CHECK / 白名单）
- **C2. 前端拿 stores + filePath 的数据流已就位** — 证据：`backend/base/model/dto/resource_dto.go:67`(`NewResourceFullDTO.Stores[]`) → `backend/work/service.go:1181`(Work 详情嵌入) → `frontend/src/utils/ResourceUtil.ts:10-35`(已消费 storeType/filePath)
- **C3. `.md`(derived) 复用 streamController 通用路径，执行面零改动** — 证据：`backend/taskManager/model.go:298`(downloaded/derived 通用 reader→writer 拷贝)，缩略图已是 `derived + NopCloser` 同款案例
- **C4. resource handler 无 `GetFullById`，渲染在 Work 上下文不阻塞** — 证据：`backend/resource/handler.go:69`(`GetById` 返回瘦 ResourceDTO，无 Stores)
- **C5. Format 前导点按"角色路径"分两套，非"derived 一律不带点"（纠偏原 §九）** — 证据：`doc/plugin-dev-guide.md:291-293` + `backend/taskManager/model.go:1878-1895`(`resolveStorePath` 按 role 分流；`normalizeExt:1889-1895` 对 main 路径强制补点)。`.md` 走 main/article 路径 → Format 用 `.md`
- **C6. 前端无 store_type 分发，仅按扩展名判图；视频前端内不可预览；无 markdown 库** — 证据：`frontend/src/components/common/WorkCard.vue:36-47`、`dialogs/WorkDialog.vue:116-127`、`common/WorkSetCard.vue:34-45`(三处逻辑逐字重复)；`frontend/package.json`(无 md 渲染依赖)
- **C7. cv 现状=每图独立 Resource（单流 main），方案 A 要 1 Resource 挂 N store，是结构性重构** — 证据：`library-squirrel-plugin-bilibili/bilibili_task_handler.go:159-223`(`createImage` 每图一子任务一 main)
- **C8. 新增 store_type 需三处常量同步** — 证据：`resource_store.go:9-15` + `library-squirrel-sdk/dto/handler_dto.go:9-18` + `frontend/src/constants/sectionCode.ts:1-26`

**待决策（需用户拍板）**：
- **D1. 存储格式**：A(Markdown，推荐) / B(自包含 HTML) / C(结构化 JSON) — 是否阻塞实施：是
- **D2. `store_type`/资源类型**：**已升级（2026-07-21）为 kind=article**（走乙方案，依赖前置 K「资源类型规约体系」`doc/plan/resource-display-kind-design.md`）；article 作为 K 的预定义资源类型（main 必含 .md + 图可含）。原"复用 main 嗅探 vs 新增 store_type"二选一作废 — 是否阻塞：是（等 K 落地）
- **D3. 可编辑性**：`.md` 是否要求库内可编辑（影响是否纯 derived） — 是否阻塞：否
- **D4. 图引用命名约定**：`.md` 内 `![](?)` 用序号 basename（如 `001.jpg`）映射到图 store — 是否阻塞：否
- **D5. 范围**：仅 bilibili 专栏，还是设计成通用"图文交织"能力（倾向通用） — 是否阻塞：否

**自曝风险（作者没把握 / 可能错的地方）**：
- **R1. cv 内嵌图节点形态未实测确认** — 阶段 0 第一步需 dump 一篇富专栏的 `MODULE_TYPE_CONTENT` 看图片 paragraph 结构（之前 dump 在 MODULE_TYPE_AUTHOR 截断，未见 CONTENT 图节点）。`.md` 提取的文本/图穿插顺序依赖此结构。
- **R2. `createImage` 重构同时服务 cv 与 dynamic（图文动态），须保证 dynamic 现有"每图独立"行为不回归**。
- **R3. markdown 渲染的 XSS 防护** — 文本节点取自站点（`word.words`/`rich.text`），md 渲染器须配 sanitize；图 URL 由 store 解析（相对可控）。
- **R4. bilibili 插件目录（`E:/code/lvfeng/library-squirrel-plugin-bilibili`）不在本会话 additional working directories 列表** — 实施阶段需确保该目录可访问/已纳入开发环境。

---

## 一、问题与范围界定

"富文本资源"的数据源实质分两类，存储组织本质不同，强行统一格式不现实（需把每类反向转换，有损且引入无关复杂度）：

| 类型 | 代表 | cohesive 单位 | 正确存储 | 真正缺的能力 |
|---|---|---|---|---|
| **1. 图文交织**（本任务） | bilibili 专栏(cv) | 正文 + 图是可分离的独立资源，图位置相关、有独立价值 | 正文（Markdown）+ N 图 store 引用——**新存储形态** | 插件提取（md 生成 + 资源模型重构）+ 前端 md 渲染 + 图 URL 解析 |
| **2. 现成文档**（延后） | pdf/doc/docx/txt/rtf | 文件本身即 cohesive 单位，内嵌内容不拆 | **原文件单 store 做 main**（现有模型已支持） | 前端多格式查看器（纯渲染层） |

本任务 = 类型 1。类型 2 的存储层零改动、纯前端查看器工作，独立成延后分支（文末）。

> 注：图文动态(opus)也存在正文，但其形态偏"文字 caption + 图片分离"，单流图片 + 文字介绍尚可接受；**专栏**才是图文交织、强结构化的典型，最需要 cohesive 存储。

## 二、当前资源模型（约束，已核验 2026-07-21）

- `Resource`（属 Work）挂 N 个 `ResourceStore`，每个 store 指 `persistent_store`（一个离散文件）。
- `store_type` 是**开放枚举**（C1：`resource_store.go:29` `StoreType string`，GORM 仅 `column:store_type`，**无 enum tag、无 DB CHECK、无白名单**；常量块 `resource_store.go:9-15` 是约定非强制）。新增类型（`article` 等）**不改表结构、无需后端改动**（仅常量同步，见 C8）。
- `generation`：`downloaded`（流式可续传）/ `derived`（一次性派生）。
- 现有 store 都是单文件（图片/视频/缩略图）。**没有"多文件带结构"的 cohesive 文档类型**——但模型本身**已能表达**（1 Resource 挂 N store，视频任务的 videoTrack+audioTrack+thumbnail 即多 store 案例）。缺口在"插件如何产出 + 前端如何渲染"，非模型层。

→ **模型层零改动**。

## 三、后端零改动核验（两根支柱，已钉锚）

**结论**：本功能后端（主程序）**零逻辑改动**——是已有 P0 纪律（`MODULE_BOUNDARY_PURITY` + 开放枚举 + 通用执行路径）叠加的必然结果，非权宜。

**支柱 1 · `store_type` 开放无校验**（C1）：见 §二，插件可自由用 `article` 等值，后端不校验、迁移不约束。

**支柱 2 · 前端获取 stores + file_path 的数据流已就位**（C2）：
- 后端 DTO：`NewResourceFullDTO`（`resource_dto.go:67`）产出 `Stores []ResourceStoreDTO`（全量多轨，主数据源），每项含 `StoreType`/`Generation`/`Store`（`PersistentStoreDTO`，含 `FilePath`/`FileName`/`FilenameExtension`/`Width`/`Height`，`persistent_store_dto.go:14-24`）。
- 后端组装：Work 详情 service 调 `NewResourceFullDTO` 嵌入 `WorkFullDTO.Resource`（`backend/work/service.go:1181`）。前端通过 **Work 详情接口**拿到含 stores 的 Resource。
- 前端消费：`ResourceUtil.ts` 已读 `resource.stores[].storeType` 与 `rs.store.filePath`（`isResourceMergeable:10`/`getResourceOpenPath:25`）。

→ 图文渲染所需的"图引用 → `/store/` URL 解析"，前端已有全部数据基础。

**已知非阻塞缺口**（C4）：`backend/resource/handler.go` 仅有 `GetById`（`:69`，返回瘦 `ResourceDTO` 无 Stores）/`ListByWorkId`，无 `GetFullById` 直供方法。前端目前只能通过 Work 详情**间接**拿 Resource Full。对图文渲染（总发生在 Work/卡片上下文）**不阻塞**；若将来需在非 Work 上下文单独取 Resource Full，再补一个透传 service 既有能力的 handler 方法（非新逻辑）。

## 四、前端渲染入口定位（2026-07-21 新增核验，原计划 §九 标"需定位"已补全）

> 本节回答"markdown 渲染器到底挂在哪里"。结论：**当前根本没有基于 store_type 的分发**，三处渲染入口逻辑逐字重复，需抽公共分发函数。

**三处资源渲染入口**（C6，逻辑几乎完全相同：优先 thumbnailStore，再按扩展名过滤回退 workStore）：

| 组件 | 职责 | 计算位置 | 模板位置 |
|---|---|---|---|
| `frontend/src/components/common/WorkCard.vue` | 资源卡片（网格缩略图卡） | `:36-47`（`imagePath` + `IMAGE_EXTENSIONS`/`isDisplayableImage`） | `:92-107`（`<el-image>`） |
| `frontend/src/components/dialogs/WorkDialog.vue` | 资源详情弹窗（点卡片打开） | `:116-127` | `:397-410`（`<el-image>`） |
| `frontend/src/components/common/WorkSetCard.vue` | 作品集卡片封面 | `:34-45` | 图区 |

**关键事实**：
- 现有判定**只看文件扩展名**（`.jpg/.jpeg/.png/.gif/.webp/.bmp`），非图片类型 `imagePath` 为空、`el-image` 走 `#error` 占位。**无任何"图片 vs 视频 vs md"的 store_type 分发**。
- 视频资源（videoTrack/audioTrack/merged）**前端内完全不可预览**，仅能双击调外部应用打开（`getResourceOpenPath` → `appLauncherOpenImage`）。
- 唯一按 `storeType` 分发的代码在 `ResourceUtil.ts:getResourceOpenPath:25`（merged 优先，否则 workStore）。
- **`package.json` 无任何 markdown 渲染依赖**（无 markdown-it/marked/vue-markdown/remark/sanitize-html），阶段 2 须新装。
- 可复用工具：`buildStoreUrl`（`frontend/src/utils/UrlUtil.ts:5`）把 filePath 编码为 `/store/<segments>` URL——md 正文加载与图引用解析都应复用。
- store 角色常量在 `frontend/src/constants/sectionCode.ts:1-26`（含 `StoreRole`/`ALL_STORE_ROLES`/`StoreRoleLabels`），**无 `ARTICLE`**，新增类型时三处都要加（见 C8）。
- 数据已到位：`WorkCardItem.resource: ResourceFullDTO`（含 `stores[]` 的 `storeType`/`store.filePath`/`store.filenameExtension`）。

**推荐改法**：在 `ResourceUtil.ts` 抽公共分发纯函数（如同模式的 `isResourceMergeable`/`getResourceOpenPath`），如 `getResourcePreviewKind(resource): 'image' | 'markdown' | 'video' | 'none'`，三处入口统一调用，避免三份复制粘贴的 markdown 分支。

## 五、插件资源模型重构（2026-07-21 显化，原计划把此步说轻了）

> 这是方案 A 与 bilibili 现状之间**最大的结构落差**，必须在阶段 1 显式处理。

**现状**（C7）：`createImage`（`bilibili_task_handler.go:159-223`）对专栏 cv 的处理是——**每张图 → 一个独立子任务（`TaskCreateChildResponse`）→ 每子任务对应一个独立 Resource，单流 `main`**。即一篇 N 图专栏被拆成 N 个独立 Resource。

**方案 A 要求**：1 篇专栏 = **1 个 Resource**，挂 1 个 `.md`(derived, main/article) + N 个内嵌图(downloaded) store。模型层"1 Resource 挂 N store"已被视频任务验证可行，但插件产出侧需要：

1. cv 不再"每图一独立子任务"，改为**单子任务（或父任务本身）产出多个 StoreSpec**：1 个 `derived .md` + N 个 `downloaded` 图。
2. `parseOpusState` 须额外输出**正文 markdown**（文本段 + 图占位 `![](序号basename)`）与**图列表**，供 `Start` 构造多 StoreSpec。
3. `createImage` 的 article 分支重构时**不得影响 dynamic（图文动态）分支**（R2）——dynamic 当前"每图独立"形态可保留，或一并收敛但须单独验证。

> 视频任务的多 StoreSpec 产出（`createVideo` 的 videoTrack+audioTrack+thumbnail）是现成的"1 任务多 store"参考实现。

## 六、源结构（bilibili cv，阶段 0 待确认）

专栏正文在 opus 页 `__INITIAL_STATE__.detail.modules[MODULE_TYPE_CONTENT].module_content.paragraphs[]`：
- 文本段：`para_type` + `text.nodes[]`（`word.words`/`rich.text`）—— 已能解析（图文动态复用，`parseOpusState`）。
- **内嵌图段**：`paragraphs[]` 中应有图片节点（`para_type` 区分图文）——**结构待用一篇"图多 + 正文长"的真实专栏确认**（R1，之前 cv dump 在 MODULE_TYPE_AUTHOR 截断，未看到 CONTENT 的图片节点）。

> 阶段 0 第一步：取一篇富专栏，复用插件 `getRaw(页面)` + `initialStateRe`（`article.go:10`）抓 `__INITIAL_STATE__`，dump `MODULE_TYPE_CONTENT`，确认内嵌图节点形态（预计类似 `module_top.display.album.pics` 或 paragraph 内 `pic` 字段）。

标题在 `MODULE_TYPE_TITLE.module_title.text`；相册图在 `MODULE_TYPE_TOP.module_top.display.album.pics[].url`；作者在 `MODULE_TYPE_AUTHOR`/`basic.uid`。

## 七、设计选项（类型 1 的存储格式，需拍板 → D1）

| 方案 | main store | 内嵌图 | 渲染 | 取舍 |
|---|---|---|---|---|
| **A. Markdown + 独立图 store**（推荐） | `.md`（derived，插件据结构生成） | 各图为 `downloaded` store，`.md` 按文件名引用 | 前端 md 渲染器 + 图 URL 解析 | 可编辑/可搜索/标准；需 md 渲染器 + 图引用解析 + sanitize |
| B. 自包含 HTML | `.html`（derived，图内联 base64） | 内联 | WebView | 自包含、富格式；base64 膨胀、不可编辑 |
| C. 结构化 JSON | `.json`（derived，AST） | 独立 store | 自定义渲染器 | 结构精确；需专用渲染器、不通用 |

> ~~D. PDF~~：不适合类型 1（不可编辑、丢可搜索性、需渲染管线）；PDF 是**类型 2**的现成文件形态，归延后分支。

**推荐 A（Markdown）**：个人资源库场景下可编辑/可全文搜索/格式标准，图作为独立 store 复用现有下载/存储/备份链路。资源形态：1 个 `main`/`article` = `.md`(derived) + N 图 store(downloaded，`.md` 按序号 basename 引用)，全部挂同一 Resource。

## 八、与 taskManager 执行面的关系（不加深耦合）

**结论**（C3）：本功能实现**不加深** taskManager 执行面/控制面耦合，`.md` 复用既有的 derived 通用路径，执行面零改动。

**理由**（方案 A：`.md` derived + 内嵌图 downloaded）：
- **`.md`（derived）**：插件 `TaskHandler.Start` 返回 `{Role:main/article, Generation:derived, Format:".md", ReadCloser:io.NopCloser(mdBytes)}`，走 `streamController` 的 downloaded/derived 通用 reader→writer 拷贝路径（`taskManager/model.go:298`）。缩略图已是 `derived + NopCloser` 的现有案例，`.md` 与之同款。
- **内嵌图（downloaded）**：走标准下载流（`StoreSpec` 带 ReadCloser 流），与现有图片下载完全一致。
- **执行面改动**：零。只在数据模型层新增一个 `store_type` 常量（复用 `main` 或新增 `article`，开放枚举不改表）。

**衍生认知**：`NopCloser` 是 derived store 的**标准产出方式**（非"伪装/撞墙"），StoreSpec 的 ReadCloser 契约既已包容派生文件产出。本功能不触发 longops 执行面剥离，可安心实现。

## 九、Format 前导点契约（纠偏原 §九，已钉锚 C5）

> 原计划 §九 line 122 称"derived 缩略图不带点，`.md` 是 derived 所以带点"——**因果错置**。真相是按**角色路径**分两套，与 derived/downloaded 无关。

**两套路径**（`doc/plugin-dev-guide.md:291-293` + `taskManager/model.go:1878-1895`）：
- **`resolveStorePath:1878-1886` 按 role 分流**：`thumbnail` 走缩略图路径函数；**其余所有角色（main/videoTrack/audioTrack/未来 article）走 `resolveMainPath:1852`**。
- **`resolveMainPath` 路径**（main/article 等）：用 `normalizeExt:1889-1895` 拼扩展名——`normalizeExt` **强制补前导点**（`if !HasPrefix(ext,".") { ext="."+ext }`），落 `store/resource/[作者/]文件名.ext`。即 Format 带不带点都能正确产出 `.md`，但契约建议**带点 `.md`**。
- **缩略图路径**（thumbnail 专属特例）：`buildThumbnailRelPath:2029-2036` 直接 `"_thumbnail." + thumbFormat` 拼接，**无补点容错**——故 thumbnail 的 Format **必须不带点**（`jpg`），否则产出 `_thumbnail..jpg`，`..` 子串触发静态服务路径穿越拦截 → 404。

**对本功能的结论**：`.md` 作为 main 或新 article 角色 → 走 `resolveMainPath` → **Format 用 `.md`（带点）**，与 generation 无关。插件实现时按此给值即可，勿被"derived 不带点"误导。

## 十、实施阶段（选型确定后）

工作面三块：**插件侧**（bilibili cv 提取 + 资源模型重构）+ **前端侧**（md 渲染 + 公共分发）+ **常量同步**（三处 store_type）。后端阶段作废。

### 阶段 0 · 调研（选型拍板前后均可做）
1. **确认 cv 内嵌图节点**（R1）：dump 一篇富专栏的 `MODULE_TYPE_CONTENT`，确认图片 paragraph/node 形态，定文本/图穿插顺序。
2. **前端 md 渲染器选型 + sanitize 方案**（R3）：评估 markdown-it / marked / md-editor-v3 等，配 sanitize-html 防 XSS（文本节点取自站点）。
3. **用户拍板 D1-D5**（默认 A + 新增 article + 序号 basename + 通用）。

### 阶段 1 · 插件提取（bilibili）
1. 扩展 `parseOpusState`：输出正文 markdown（文本段 + `![](序号basename)` 图占位）与图 URL 列表。
2. **重构 `createImage` 的 article 分支**（§五）：从"每图独立 Resource"改为"1 Resource 挂 `.md`(derived) + N 图(downloaded)"，`Start` 产出多 StoreSpec；**保证 dynamic 分支不回归**（R2）。
3. 图命名约定（D4）：序号 basename（如 `001.jpg`），`.md` 按此引用。
4. 定 `store_type = article`（D2），三处常量同步（C8）。
5. `.md` StoreSpec：`{Role:article, Generation:derived, Format:".md", ReadCloser:io.NopCloser(mdBytes)}`（§九）。

### 阶段 2 · 前端渲染
1. 新装 md 渲染库 + sanitize。
2. 抽 `getResourcePreviewKind(resource)` 公共分发到 `ResourceUtil.ts`（§四）。
3. 三处入口（`WorkCard`/`WorkDialog`/`WorkSetCard`）按 kind 分支：md → md 渲染器，image → 现状，video → 占位。
4. md 内 `![](basename)` → 经 Resource.stores 的 filePath + `buildStoreUrl`（`UrlUtil.ts:5`）解析为图 URL。
5. sanitize 文本节点。

### 阶段 3 · 集成验证
1. 粘一篇富专栏 → 建任务 → 下载 `.md` + 图 → 前端渲染为完整图文文章。
2. 验证 dynamic（图文动态）行为不回归（R2）。

## 十一、验收标准

- 粘贴 bilibili 专栏 URL 可建任务并产出 1 个 Resource（挂 `.md` + N 图 store），非 N 个独立图片资源。
- 前端在卡片/详情弹窗内**直接渲染** markdown 正文 + 内嵌图（图经 store URL 解析显示，非外部打开）。
- `.md` 文件落盘路径符合 `store/resource/[作者/]文件名.md`，扩展名正确（无 `..md`）。
- dynamic（图文动态）现有下载/展示行为不回归。
- 正文文本经 sanitize，无 XSS。

## 十二、关键代码定位（恢复/续作用）

### 当前资源/store 模型（已核验零改动，见 §三）
- `backend/base/model/entity/resource.go`（Resource：属 Work，挂 N store）
- `backend/base/model/entity/resource_store.go:29`（`StoreType string` 开放枚举，无校验）
- `backend/base/model/entity/persistent_store.go`（`file_path`/`width`/`height`）
- 路径/存储规则：`CLAUDE.md` + `.claude/rules/database.md`

### DTO 数据流（前端拿 stores 的路径，已通）
- `backend/base/model/dto/resource_dto.go:67`（`NewResourceFullDTO` → `Stores[]`）
- `backend/base/model/dto/persistent_store_dto.go:14-24`（`FilePath` 等）
- `backend/work/service.go:1181`（Work 详情嵌入 ResourceFullDTO）
- 前端 `frontend/src/utils/ResourceUtil.ts:10-35`（已消费 stores + filePath）
- 非阻塞缺口：`backend/resource/handler.go:69`（无 `GetFullById`，渲染在 Work 上下文不影响）

### 前端渲染入口（§四 已定位）
- `frontend/src/components/common/WorkCard.vue:36-47`、`dialogs/WorkDialog.vue:116-127`、`common/WorkSetCard.vue:34-45`（三处分发挂载点）
- `frontend/src/utils/UrlUtil.ts:5`（`buildStoreUrl` 图 URL 解析复用）
- `frontend/src/constants/sectionCode.ts:1-26`（store 角色常量，需加 `ARTICLE`）
- `frontend/package.json`（需新增 md 渲染依赖）

### bilibili 插件 cv 提取（要扩展 + 重构）
- `library-squirrel-plugin-bilibili/internal/bilibiliapi/article.go:10,14`（`initialStateRe`/`GetArticleImages`）
- `library-squirrel-plugin-bilibili/internal/bilibiliapi/dynamic.go`（`parseOpusState` 解析 `detail.modules`）
- `library-squirrel-plugin-bilibili/bilibili_task_handler.go:159-223`（`createImage`，需按 §五 重构 article 分支）
- `library-squirrel-plugin-bilibili/urls.go`（`ArticlePageURL` 等）

### 插件↔平台 store 契约
- SDK `dto/handler_dto.go:68-77`（`StoreSpec`）
- `doc/plugin-dev-guide.md:274-323`（StoreSpec 字段速查 + Format 前导点契约 + 缩略图约定）

### taskManager 执行面（零改动）
- `backend/taskManager/model.go:298`（`streamController` downloaded/derived 通用）
- `backend/taskManager/model.go:1852-1895`（`resolveMainPath`/`resolveStorePath`/`normalizeExt`，Format 处理）

## 延后分支（类型 2：现成文档文件的多格式查看器）

**派生自本任务的设计反思**（2026-07-21）：识别"富文本资源"时发现类型 2（站点现成 pdf/doc/docx/txt/rtf）存储组织与类型 1 本质不同，不应统一选型。

- **本质**：原文件本身即 cohesive 单位，内嵌内容不拆；存储 = 原文件单 store 做 main（`downloaded` 或 `derived`，现有模型已支持，**后端零改动**）。
- **真正工作**：前端多格式查看器（pdf viewer / docx 渲染 / 纯文本 / rtf）——纯渲染层，不涉及资源模型/插件提取/存储契约。
- **为何延后**：与类型 1 工作面完全不重叠（类型 1 是插件提取 + md 渲染，类型 2 是多格式查看器），硬捆进同一 epic 模糊焦点。等类型 1 交付或用户需要文档查看时再 focus。
- **追踪**：在 `.claude/workflow/active/rich-article-resource/TREE.md` 中作为 `[延后]` 节点 T2。
