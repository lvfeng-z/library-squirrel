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

**已决策（2026-07-24 全部拍板）**：
- **D1. 存储格式**：✅ **定为 A（Markdown）**。正文存 `.md`，图用 `![](序号basename)` 引用、自定义渲染器解析 basename→store URL（见 D4）；可读/可移植/可 sanitize/生态好。B(HTML)/C(JSON) 否决（体积/可编辑性无关/XSS 面/无生态，见 §七）。
- **D2. `store_type`/资源类型**：✅ **已落地**（K done）。`backend/base/model/entity/resource_type.go:98-111` `articleResourceTypeSpec`：Roles=`document`(1~1)+`image`(0~N)+`thumbnail`(0~1)，PrimaryRole=`document`，标准 document=`.md`(derived)、image=`.jpg/.png/.webp`(downloaded)。原 kind→ResourceType。
- **D3. 可编辑性**：✅ **不允许库内编辑**。`.md` 纯 derived 只读展示（与 spec `document=derived` 一致），前端无需编辑器。
- **D4. 图引用命名约定**：✅ **暂定序号 basename**（`001.jpg`/`002.png`，.md 引用名=图 store 落盘名）；渲染器覆盖 md image 规则，basename→`buildStoreUrl(store.filePath)`（复用 `UrlUtil.ts:5`）。**正式命名规约延后**——1:1→1:N 演进遗留的 store 落盘命名设计空缺（见 TREE 节点 N）；R 暂用本约定推进，N 落地后可能回更。
- **D5. 范围**：✅ **通用“图文交织”能力**。抽象成 `getResourcePreviewKind` 分发，不绑 bilibili。

**自曝风险（作者没把握 / 可能错的地方）**：
- **R1. cv 内嵌图节点形态** — ✅ **已实测确认（2026-07-24，`cv48146678`）**：`MODULE_TYPE_CONTENT.paragraphs[]` 按 `para_type` 分文本段(1)/图片段(2)逐段交错，图段 `pic.pics[]` 带 `url/width/height/size`（详见 §六）。附带发现：cv 页 302 跳 opus；匿名访问三态不稳（空壳/超时/全量），真实实现需 SESSDATA + buvid。`.md` 提取的图文穿插顺序 = paragraphs 数组顺序，已落实。
- **R2. `createImage` 重构同时服务 cv 与 dynamic（图文动态），须保证 dynamic 现有"每图独立"行为不回归**。
- **R3. markdown 渲染的 XSS 防护** — ✅ **方案已定（2026-07-24）**：markdown-it `html:false` 转义裸 HTML + DOMPurify 深度防御（ALLOWED_TAGS/ATTR 收紧、`a.href` 限 http/https）；文本取自站点必过 sanitize，图 URL 由 store 解析（可控）。
- **R4. bilibili 插件目录访问** — ✅ 已验证（2026-07-24 实测 `E:/code/lvfeng/library-squirrel-plugin-bilibili` 读写 + `go test` 均正常，仅 gopls 提示未纳入 workspace，不影响编译运行）。

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

**N image reader 的活跃管理（article 衍生）**：article 的 N 个内嵌图在 `Start` 各产一条 downloaded reader，须全部注册到插件 `activeReaders` 供 Pause/Stop/Shutdown 广播。但 `activeReaders` 内层原以 role 为键（video 不同 role 不冲突），N 个同 role(image) reader 会互相覆盖——须改用 `role#seq` 复合键（`storeKey` 辅助函数，seq=store_seq=InlineImages 下标）；Pause/Stop 遍历内层 map 全部 reader 的逻辑不变。此为插件执行面适配（详见 §八），与 taskManager 按 (role,store_seq) 身份化呼应。

## 六、源结构（bilibili cv，阶段 0 已实测确认 · 2026-07-24 `cv48146678`）

专栏 cv 页会 **302 重定向到 opus 页**（`cv48146678` → `opus/1194849825756020768`）；正文在 opus 页 `__INITIAL_STATE__.detail.modules[MODULE_TYPE_CONTENT].module_content.paragraphs[]`，**每段 `para_type` 二值、按阅读顺序逐段交错**：

- **文本段 `para_type=1`**：`text.nodes[]`，节点 `type=TEXT_NODE_TYPE_WORD` 取 `word.words`（另有 `TEXT_NODE_TYPE_RICH` 取 `rich.text`，现有 `parseOpusState` 已建模）；`pic=null`。
- **图片段 `para_type=2`**：`pic.pics[]`，每张带 `url/width/height/size`（一段可多图）；`text=null`。
- 段对象另有 `heading/list/blockquote/code/line/link_card/format` 等富文本块字段——**本次 dump 的漫画汉化专栏全为 null**（22 文本段 + 58 图片段，仅 WORD 节点）。**核心图文穿插机制已确认**；heading/list/code 等富文本块未在本文触发，提取设计仍须为它们留位（按需映射 md 的 `#`/`-`/` ``` `/`>`），后续可再 dump 一篇技术类专栏补证。

> 封面在 `MODULE_TYPE_TOP.module_top.display.album.pics[].url`（https）；内嵌图在 CONTENT 图段（本次为 `http://i0.hdslb.com/...`，下载时须升级 https 或跟随重定向）。标题在 `MODULE_TYPE_TITLE.module_title.text`；作者在 `MODULE_TYPE_AUTHOR`/`basic.uid`。
>
> **访问稳定性（实测）**：匿名访问三态不稳——同一请求时而返回 opus 全量态（68KB，含 `__INITIAL_STATE__`）、时而返回 read 页 SPA 空壳（3.3KB，无 state）、时而拖连接超时。须带 buvid + 长超时重试，**真实实现大概率需 SESSDATA 登录态保证稳定**（见 R1 解除备注）。dump 工具：`library-squirrel-plugin-bilibili/internal/bilibiliapi/article_dump_test.go`（`DUMP_CV=cv... [DUMP_SESSDATA=...] go test`）。

## 七、设计选项（类型 1 的存储格式，需拍板 → D1）

| 方案 | main store | 内嵌图 | 渲染 | 取舍 |
|---|---|---|---|---|
| **A. Markdown + 独立图 store**（推荐） | `.md`（derived，插件据结构生成） | 各图为 `downloaded` store，`.md` 按文件名引用 | 前端 md 渲染器 + 图 URL 解析 | 可编辑/可搜索/标准；需 md 渲染器 + 图引用解析 + sanitize |
| B. 自包含 HTML | `.html`（derived，图内联 base64） | 内联 | WebView | 自包含、富格式；base64 膨胀、不可编辑 |
| C. 结构化 JSON | `.json`（derived，AST） | 独立 store | 自定义渲染器 | 结构精确；需专用渲染器、不通用 |

> ~~D. PDF~~：不适合类型 1（不可编辑、丢可搜索性、需渲染管线）；PDF 是**类型 2**的现成文件形态，归延后分支。

**推荐 A（Markdown）**：个人资源库场景下可编辑/可全文搜索/格式标准，图作为独立 store 复用现有下载/存储/备份链路。资源形态：1 个 `main`/`article` = `.md`(derived) + N 图 store(downloaded，`.md` 按序号 basename 引用)，全部挂同一 Resource。

## 八、与 taskManager 执行面的关系（不加深耦合）

**结论**（C3）：本功能实现**不加深** taskManager 执行面/控制面耦合，`.md` 复用既有的 derived 通用路径，R 阶段1/2 执行面零改动。**M 节点已完成 taskManager N-同 role 多 store 路径消歧 + resume 按 (role,store_seq) 身份化 + SDK `StreamOffsets` structured（见 `doc/plan/taskmanager-multi-same-role-store-plan.md`，2026-07-25 四端发布 + video 实测无回归），R 直接复用，不再改后端执行面**。

**理由**（方案 A：`.md` derived + 内嵌图 downloaded）：
- **`.md`（derived）**：插件 `TaskHandler.Start` 返回 `{Role:main/article, Generation:derived, Format:".md", ReadCloser:io.NopCloser(mdBytes)}`，走 `streamController` 的 downloaded/derived 通用 reader→writer 拷贝路径（`taskManager/model.go:298`）。缩略图已是 `derived + NopCloser` 的现有案例，`.md` 与之同款。
- **内嵌图（downloaded）**：走标准下载流（`StoreSpec` 带 ReadCloser 流），与现有图片下载完全一致。
- **taskManager 执行面改动**：零。taskManager 已由 M 节点铺好 N-同 role 多 store 路径消歧 + resume 按 (role,store_seq) 身份化，R 直接复用。

**插件侧 reader 管理（非 taskManager）**：article 的 N 个同 role(image) downloaded reader 须全部注册到插件 `activeReaders`（taskID→map[storeKey]reader）供 Pause/Stop/Shutdown 广播。原内层以 role 为键（video 不同 role 不冲突），N 个同 role reader 会互相覆盖，故改用 `role#seq` 复合键（`storeKey`，seq=store_seq=InlineImages 下标），Pause/Stop 遍历全部 reader 的逻辑不变。此为插件执行面适配，不改 taskManager。

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
1. ~~**确认 cv 内嵌图节点**（R1）~~ — ✅ 已确认（2026-07-24 `cv48146678`，见 §六）：paragraphs 按 `para_type` 1=文本段/2=图片段交错，图段 `pic.pics[]` 带 url/宽高；附带发现 cv→opus 302、匿名不稳。
2. ~~**前端 md 渲染器选型 + sanitize 方案**（R3）~~ — ✅ 已选（2026-07-24）：**markdown-it@14 + dompurify@3**（见阶段 2 蓝图）。否决 md-editor-v3（CodeMirror 编辑器，D3 不可编辑→大材小用）、sanitize-html（含 postcss 偏重，客户端用 DOMPurify）；marked（0 依赖）备选。
3. **用户拍板 D1-D5**（默认 A + 新增 article + 序号 basename + 通用）。

### 阶段 1 · 插件提取（bilibili）
1. ~~扩展 `parseOpusState`~~ — ✅ done(2026-07-24):改返回 `opusParseResult`(Markdown+InlineImages+TopPics+Description+Author);cv48146678 实测 40 段(11文本+29图)→ 29 内嵌图 basename 001-029 按序、图文穿插正确。加 `extFromURL` 取扩展名;dump 测试验证。
2. ~~重构 `createImage` 的 article 分支~~ — ✅ done(2026-07-25):`createImage` 拆为分发层 + `createDynamic`(原每图一子任务逻辑,R2 不变) + `createArticle`(单任务无 children:1 篇专栏=1 Resource,PluginData 存 Markdown+InlineImages+TopPics[0]→ThumbnailURL,InvolvedRoles=[document,image,thumbnail]);`GetArticleImages`→`GetArticleDetail` 返回完整 `*OpusParseResult`(原只返回 TopPics,丢弃正文);N image reader 的 `activeReaders` 内层 key 改 `role#seq`(见 §五/§八)。
3. ~~图命名约定（D4）~~ — ⚠ **由 M D3-B 取代**(2026-07-25):M 决定同 role 多 store 落盘名=模板+`_%03d(store_seq)`,**不复用 SuggestName**(模板模式忽略);`.md` 的 `001.jpg` 退化为位置占位,前端按第 k 个 `![]()` → 第 k 个 image store(R 阶段2 位置绑定),非 basename 匹配。step2 实现遵循此决策(image store 不设 SuggestName)。
4. ~~ResourceType=`article` 声明~~ — ✅ done(随 step2):`createArticle` 声明 ResourceType=article + InvolvedRoles=[document,image,thumbnail];正文 Role=document(.md)、图 Role=image(.jpg/.png/.webp)、缩略图 thumbnail(见 `resource_type.go:98-111`)——复用既有 store_type,无需新增常量(C8 不触发)。
5. ~~`.md` StoreSpec~~ — ✅ done(2026-07-25,在 `startArticle`):`{Role:document, Generation:derived, Format:".md", ReadCloser:io.NopCloser(mdBytes), Continuable:false}`,document 路径 Format 带 `.md`(§九/C5)。
6. ~~`resumeArticle`~~ — ✅ done(2026-07-25):遍历 `param.StreamOffsets`,`store_seq`→`InlineImages[seq]` 取图 URL,产 image StoreSpec 带 `ResumeWriteOffset`(磁盘偏移);越界保护(seq≥len 报错);`.md`(derived) 不处理——主程序 Start([document]) 整轨重产(`model.go:1548-1560`);store_seq 顺序=InlineImages 下标(与 Start 一致,M R5)。

### 阶段 2 · 前端渲染
1. ~~新装 markdown-it + dompurify + 封装 MarkdownView.vue~~ — ✅ done(2026-07-25):装 markdown-it@14.3.0 + dompurify@3.4.12(均自带 TS 类型);`frontend/src/components/common/MarkdownView.vue`:`md.render(src, env)` → `DOMPurify.sanitize` → `v-html`。
2. ~~抽 getResourcePreviewKind 公共分发~~ — ✅ done(简化,2026-07-25):`ResourceUtil.getResourcePreviewType` 已返回 ResourceType(含 ARTICLE),WorkDialog `previewKind` computed 直接据此判断('article'|'image'),不另抽 getResourcePreviewKind(无额外价值)。
3. ~~三处入口按 kind 分支~~ — ✅ done(2026-07-25,设计调整):**WorkDialog**(详情弹窗)加 article 分支(el-image 区 v-if:article→MarkdownView 渲染正文+内嵌图,否则现状 el-image);**WorkCard/WorkSetCard**(卡片)现状兼容——article 经 thumbnailStore 显示封面(article spec 含 thumbnail),卡片不渲染正文(点击弹窗才看),无需改。
4. ~~图绑定~~ — ✅ done(2026-07-25,**位置绑定,M D3-B 取代 basename 匹配**):MarkdownView 覆盖 markdown-it `renderer.rules.image`,用 env 传 imageStores + 计数器,第 k 个 image token → 第 k 个 image store(stores[] filter image,数组顺序=store_seq 升序)→ `buildStoreUrl(filePath)`;原 plan 的 imageMap{basename→url} 作废(basename 退化为占位)。
5. ~~sanitize~~ — ✅ done(2026-07-25):markdown-it `html:false` 转义裸 HTML + DOMPurify.sanitize(ALLOWED_TAGS/ATTR 收紧白名单;DOMPurify 默认已禁 javascript: URI)。文本取自站点必过。

### 阶段 3 · 集成验证（2026-07-25 完成）

实测 cv48146678 / 多个 opus / resume，首轮暴露一系列"N-同 role 多 store 首次端到端实战"缺陷（M 完成时缺此类集成测试），逐一修复后全场景通过。修复链（bilibili 插件为主，主程序/SDK 各一）：

1. ~~cv 下载+渲染~~ ✅（任务46）：cv48146678 → 1 document.md + 29 image，前端渲染正文+图。
2. ~~opus 路由~~ ✅：`urlparser` 把 `opus/` 归 ContentDynamic（走 createDynamic 取 pics=0 失败）→ 改 opus→ContentArticle + 加 `GetOpusDetail` + `createArticle` 据 url 分派 cv/opus。
3. ~~createArticle 任务结构~~ ✅：`handleCreateTaskArray` 跳过无 children 的 parent → createArticle 返回 parent+1 child（主程序单 child 分支创建 child 为 article 任务）。
4. ~~activeReaders 同 role 多 reader~~ ✅：内层 key 从 role 改 `role#seq`（N image reader 不再互相覆盖）。
5. ~~needsSuffix relPath（M 遗漏①）~~ ✅：`resolveStorePath` needsSuffix 只改 fileName 漏改 relPath（StoreStream 用 relPath 创建文件）→ 同步重建 relPath（见 M plan §六）。
6. ~~SDK serveSpecsPull 同 role 覆盖（M 遗漏②）~~ ✅：`readers[role]` 覆盖只留 1 reader → 改 `role#specIndex` 编码 + readers slice 按 specIndex 定位（见 M plan §六）。
7. ~~parseOpusState 标题 fallback~~ ✅：动态类 opus MODULE_TYPE_TITLE 空 → 正文首行 → 作者兜底。
8. ~~parseOpusState 图集 fallback~~ ✅：图在 TOP 相册（非 CONTENT 图段）时 InlineImages 空 → TOP 图转 InlineImages + Markdown 末尾追加引用；cv（CONTENT 有图）不触发。

辅助修复：`backupStore` 容错（解脏数据删作品阻塞）、`RetryReader` 提前 EOF 重试（防御，本次未触发但保留）、`CreateTaskByURL` 诊断日志。

验证：cv（任务46）/ opus 图文（任务50/56）/ opus 图在 TOP（任务57）/ resume 多次暂停恢复（任务48）全通过。R2 `/dynamic/` 回归降级代码审查（bilibili 现网动态都 opus，`/dynamic/` 入口不存在，改动对 createDynamic 非侵入）。

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

### taskManager 执行面（R 零改动;M 已完成多 store + resume 身份化基建）
- R 不改 taskManager:M 节点(`doc/plan/taskmanager-multi-same-role-store-plan.md`)已完成 N-同 role 多 store 路径消歧(`resolveStorePath` 同 role 加 `_%03d` 后缀) + resume 按 (role,store_seq) 身份化(`findStoreRowByIdentity`/`findResumeOffset` + 全量重挂) + SDK `StreamOffsets` structured。R 阶段1 仅插件侧产多 StoreSpec(image N 个同 role)+ `resumeArticle`,taskManager 自动处理。
- `backend/taskManager/model.go`(`streamController` downloaded/derived 通用、`resolveStorePath`/`resolveMainPath`/`normalizeExt` Format 处理、`findStoreRowByIdentity`/`findResumeOffset` 身份匹配)
- `backend/base/model/entity/resource_type.go`(`articleResourceTypeSpec`:document 1~1 + image 0~N(Max=0 不限) + thumbnail 0~1;PrimaryRoles=[document])
- **StoreSeq 语义**:同 role 内 0-based 序号(M 决策);article 的 image store_seq = InlineImages 文档顺序,前端 .md 位置绑定(R 阶段2)据此

## 延后分支（类型 2：现成文档文件的多格式查看器）

**派生自本任务的设计反思**（2026-07-21）：识别"富文本资源"时发现类型 2（站点现成 pdf/doc/docx/txt/rtf）存储组织与类型 1 本质不同，不应统一选型。

- **本质**：原文件本身即 cohesive 单位，内嵌内容不拆；存储 = 原文件单 store 做 main（`downloaded` 或 `derived`，现有模型已支持，**后端零改动**）。
- **真正工作**：前端多格式查看器（pdf viewer / docx 渲染 / 纯文本 / rtf）——纯渲染层，不涉及资源模型/插件提取/存储契约。
- **为何延后**：与类型 1 工作面完全不重叠（类型 1 是插件提取 + md 渲染，类型 2 是多格式查看器），硬捆进同一 epic 模糊焦点。等类型 1 交付或用户需要文档查看时再 focus。
- **追踪**：在 `.claude/workflow/active/rich-article-resource/TREE.md` 中作为 `[延后]` 节点 T2。
