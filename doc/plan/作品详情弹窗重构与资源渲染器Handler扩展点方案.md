# 作品详情弹窗重构与资源渲染器 Handler 扩展点方案

## 审查摘要

**关键声明**（抽查锚点）：
- 声明1：现有 WorkDialog 渲染仅两路分发——article 走 MarkdownView，image/video/document/unknown 全走 el-image 占位（非图触发 error 后外部打开）— `frontend/src/components/dialogs/WorkDialog.vue:124-129`、`:431-451`
- 声明2：video 当前无 `<video>` 内联播放，仅能外部打开（frontend/src 下 `<video` grep 零命中）；video 的"合并"按钮仅做音视频轨合并，合并后仍外部打开 — `frontend/src/components/dialogs/WorkDialog.vue:163-181`、`:683-691`
- 声明3：slot 下发链路对 SlotType 完全透明——后端 `SlotRegistry.Register/GetSlotConfigs` 不校验 type 值（`backend/plugin/extension/slot_registry.go:38-58`、`:146-155`），`parseSlotContent` 的 switch 无 default 分支、未知 type 原样透传（`app.go:466-535`，SlotType 是 string 别名、`app.go:387` 纯强转无校验），前端 `registerSlotByType` 无 else 分支、未知 type 静默丢弃（`frontend/src/composables/useSlotSyncListener.ts:428-442`）
- 声明4：ResourceType 是封闭枚举 5 种（image/video/article/document/unknown），后端严格识别拒绝自定义值（`ValidateResourceType` 写入路径抛错）— `doc/resource-type-spec.md` 第四、七节；前端常量 `frontend/src/constants/sectionCode.ts:32-38`
- 声明5：PluginBoundary 用 component-prop 模式隔离插件渲染错误（onErrorCaptured 返回 false 阻断冒泡，仅降级出错子树）— `frontend/src/components/common/PluginBoundary.vue:28-35`、`:63-69`
- 声明6：项目已大量使用 `extension` 作扩展点统称词（ExtensionMetadata/Extension[T]/proto RegisterExtensionRequest/目录 backend/plugin/extension/）— `doc/plan/插件扩展点命名统一方案-contribution转extension.md:9`
- 声明7：WorkDialog 当前调用方仅两处（WorkGridForMainPage、WorkGridForWorkSet）— `frontend/src/components/common/WorkGridForMainPage.vue:63`、`frontend/src/components/common/WorkGridForWorkSet.vue:156`
- 声明8：MarkdownView props 为 `{markdown, imageStores}`，内嵌图按 stores 数组顺序（后端按 store_seq 升序）做位置绑定 — `frontend/src/components/common/MarkdownView.vue:12-37`

**已定决策**（用户 2026-07-27 确认）：
- 决策1：多插件声明同一 resourceType 渲染器时取 order 最小者生效（互斥）
- 决策2：HandlerRegistry 通用命名，当前仅实现 resourceViewer 一种 handler（YAGNI）
- 决策3：插件渲染器组件 props 契约为 `{resource: ResourceFullDTO, work: WorkFullDTO}`；stores 已含于 `resource.stores`（`frontend/bindings/github.com/lvfeng-z/library-squirrel-sdk/dto/models.ts:608`），插件可遍历到任意 role，无需单独传 stores
- 决策4：旧 WorkDialog.vue 保留不删，仅替换调用方（用户手动移除）。保留期间旧组件不得再被任何 import / 路由引用（避免被意外打包或残留全局引用）；本次调用方迁移完成后，旧 WorkDialog 应处于"无引用、仅文件留存"状态

**自曝风险**：
- 风险1：后端 T3（插件自定义 ResourceType 注册表）延后，resourceViewer 的 resourceType 字段当前仅对 5 种枚举有意义；前端按"任意 resourceType 值"设计但实际只跑 5 种，T3 真落地时预留口形态可能需返工
- 风险2：本方案引入的"前端 SlotRegistry/HandlerRegistry 分桶"与待执行的「contribution→extension 统一计划」存在命名交叉，二者执行顺序需协调（见『与既有计划的关系』节）
- 风险3：video 内联播放的大文件性能、merged（合并产物）vs videoTrack（视频轨）选择策略、浏览器 codec 兼容需实测
- 风险4：`parseSlotContent` 加 resourceViewer case 是后端唯一逻辑改动点，必须做 resourceType 必填校验，否则前端拿到空 resourceType 无法路由渲染器。该路径不调用 `ValidateResourceType`，故插件可声明任意 resourceType 字符串（如 "interactive-novel"）通过管道；但 `resource.resource_type` 严格是 5 种封闭枚举（声明4），非法值永远不会命中真实资源——即"声明侧任意值 + 命中侧严格 5 种"，T3 落地前任意自定义类型串都是死键（与风险1 互补：框架按任意值设计，但实际只有 5 种能生效）

---

## 一、背景与目标

现有作品详情弹窗（WorkDialog.vue）存在三方面不足：

1. **空间利用低效**：图片/正文展示区与作品信息（作者/简介/标签/站点/作品集）挤在同一横向布局，且右侧锚点导航占用空间；非图片资源（video/document）在展示区只能显示占位图，需外部打开。
2. **渲染能力单一**：只支持 article（MarkdownView）与 image（el-image）两路，video/document/unknown 无内联展示；插件无法为某类资源提供自定义渲染器。
3. **插件自定义渲染无扩展点**：用户预期的"图文紧密结合资源"等场景，插件可能需要比内置 MarkdownView 更强的专栏渲染器，但当前前端插件体系（6 种 slotType 均为"主动注入型"）没有"被动响应型"的扩展通道。

**目标**：

- 重构弹窗：展示区占绝大部分空间，功能按钮移至右侧栏，元数据移入 el-drawer。
- 抽取独立的 ResourceViewer 组件，按 ResourceType 分发内置渲染器（image/video/article/document/unknown），其中 video 新增 `<video>` 内联播放。
- 新建前端插件"被动响应型"扩展通道——HandlerRegistry，resourceViewer 是其首个应用；插件可为某个 ResourceType 提供自定义渲染器，覆盖内置渲染。

## 二、范围

**本次做**：

- 弹窗重构（新 WorkDetailDialog 替代 WorkDialog）。
- ResourceViewer 组件 + 5 种内置渲染器（image/video/article/document/unknown），video 内联播放。
- 前端 HandlerRegistry（通用命名，仅实现 resourceViewer 一种 handler）。
- 后端 slotType 扩展：新增 `resourceViewer`（复用现有下发管道，见声明3）。
- 调用方迁移（WorkGridForMainPage/WorkGridForWorkSet 改用 WorkDetailDialog）。旧 WorkDialog.vue 保留，由用户手动移除。

**本次不做（延后）**：

- 后端 T3：插件自定义 ResourceType 注册表。本次 resourceViewer 的 resourceType 字段仅承载现有 5 种枚举；插件暂不能声明全新 ResourceType（如 interactive-novel），后端严格识别会拒绝（见声明4）。前端框架按"任意 resourceType 值"设计，T3 落地后无需改前端即可支持。
- contribution→extension 全量重命名（独立计划，见『与既有计划的关系』节）。
- 现有 6 种 slotType 的任何改动。

## 三、架构设计

### 3.1 前端插件扩展点二分框架

确立两种正交的扩展模式（用户已确认）：

| 模式 | 含义 | 触发方 | 现有承载 | 本次 |
|---|---|---|---|---|
| 主动注入型（Slot） | 插件声明"组件+占据位置"，主程序在固定位置渲染 | 主程序在固定位置渲染 | SlotRegistryStore（6 种 slotType） | 不动 |
| 被动响应型（Handler） | 插件声明"我能处理某 key"，主程序遇该 key 时调用 | 主程序遇特定情境按 key 调用 | **无** | 新建 HandlerRegistry，resourceViewer 首例 |

统称词沿用既定的 `extension`（见声明6）；二分类别为 `Slot` 与 `Handler`。

**关键约束**：resourceViewer 不进 SlotRegistryStore，而是路由到独立的 HandlerRegistryStore——前端 slot 体系保持纯粹（6 种主动注入型不变），resourceViewer 是"被动响应型"独立通道，不是新 slot。后端 slotType 枚举 +1（`resourceViewer`）仅是复用下发管道的数据层 type 标签（声明3 已证管道对 type 透明）。

### 3.2 弹窗布局

```
┌──────────────────────────────┬────┐
│                              │删除│  ← 破坏性操作(danger + tone-fail)
│                              │合并│  ← 仅 mergeable 资源显示
│        作品展示区             │上一│  ← 切换作品(← →键)
│      ResourceViewer          │下一│
│  (内置渲染器 / 插件渲染器)    │作品集│ ← dropdown
│                              │详情│  ← 触发 el-drawer
│                              │    │
└──────────────────────────────┴────┘
        el-drawer（右侧滑出）
  作者 / 简介 / 本地标签 / 站点标签 / 站点 / 作品集
  + 标签编辑（沿用现有 ExchangeBox）
```

- 主体：ResourceViewer，占满剩余空间。
- 右侧栏：垂直功能按钮列（图标按钮为主，破坏性操作遵循 `tone-fail` 规约）。
- el-drawer：元数据 + 标签编辑。由右侧栏"详情"按钮触发。drawer 内保留现有标签编辑的 ExchangeBox 逻辑（从 WorkDialog 迁移）。

### 3.3 ResourceViewer 分发逻辑

```
输入: resource: ResourceFullDTO, work: WorkFullDTO
1. rt = getResourcePreviewType(resource)   // 含 unknown 降级嗅探
2. 查 HandlerRegistryStore.resourceViewerByType(rt):
     命中 → <PluginBoundary :component="handler.component" :componentProps="{resource, work}"/>
     未命中 → 按 rt 分发内置渲染器:
       image    → ImageRenderer
       video    → VideoRenderer
       article  → ArticleRenderer
       document → DocumentRenderer
       unknown  → UnknownRenderer
```

插件渲染器优先于内置（插件可覆盖任意现有类型的渲染）。

## 四、详细设计

### 4.1 后端改动（选项1 标准版，纯声明式透传）

| 文件 | 改动 |
|---|---|
| `backend/base/slot.go:14-25` | SlotType 枚举新增 `SlotTypeResourceViewer SlotType = "resourceViewer"`（枚举始于 :14，:12-13 为上文注释） |
| `backend/base/slot.go:42-56` | SlotConfig 新增 `ResourceType string` 字段（resourceViewer: 资源类型，查找键） |
| `backend/base/model/dto/plugin_types.go` | 新增 `ResourceViewerSlotContent` 结构（仿 EmbedSlotContent `:138-144`）：含 `ContentType/Source/ResourceType/Props` |
| `app.go:466-535`（parseSlotContent） | switch 新增 `case SlotTypeResourceViewer`：解析 ResourceViewerSlotContent → 填 SlotConfig.ResourceType/Content/ContentType；**resourceType 必填校验**（空则 return error，阻止注册，对应风险4） |
| `backend/plugin/extension/slot_handler.go:11-29` | SlotResponse 新增 `ResourceType string` 字段 |
| `backend/plugin/extension/slot_handler.go:52-84` | SlotConfigToResponse 透传 ResourceType（+1 行） |

下发管道（SlotRegistry.Register/Unregister/GetSlotConfigs、WailsSlotPusher、GetAllSlots）**零改动**（声明3）。SDK/proto **零改动**（slot 走主程序 HTTP/IPC，不经插件 gRPC）。

### 4.2 前端改动

**A. HandlerRegistry（新 store）** — `frontend/src/store/HandlerRegistryStore.ts`

通用命名、当前仅实现 resourceViewer 一种 handler（YAGNI，决策2）：

```ts
interface ResourceViewerHandler {
  slotId: string
  resourceType: string                  // 查找键（互斥：同 resourceType 仅留 order 最小者，决策1）
  component: () => Promise<DefineComponent>  // 复用 loadCompiledComponent 的 loader
  props?: Record<string, unknown>
  order: number
}
state: { resourceViewerHandlers: Map<string, ResourceViewerHandler> }  // key=resourceType
getters: { resourceViewerByType: (rt) => handler ?? null }
actions: { registerResourceViewerHandler / unregisterResourceViewerHandler }
```

未来新增其他 handler 类型（如自定义搜索渲染器）只需加新桶 + 新 getter，不影响现有。

**B. 同步层扩展** — `frontend/src/composables/useSlotSyncListener.ts`

- `registerSlotByType`（:428-442）新增 `else if (slot.type === 'resourceViewer')` 分支：路由到 HandlerRegistryStore（**不进** SlotRegistryStore）。
- `unregisterSlotByType`（:447-461）同样新增分支。
- 新增 `convertToResourceViewerHandler(slot)`：解析 SlotResponse，组装 component loader（复用现有 `loadPluginComponent`）+ resourceType。

**C. ResourceViewer 组件** — `frontend/src/components/resource/ResourceViewer.vue`

props: `{ resource: ResourceFullDTO, work: WorkFullDTO }`。实现第三节的分发逻辑。复用现有 `getResourcePreviewType`（`frontend/src/utils/ResourceUtil.ts:34-47`）。

**D. 内置渲染器** — `frontend/src/components/resource/renderers/`

| 渲染器 | 实现 | 数据来源 |
|---|---|---|
| ImageRenderer.vue | el-image（fit=contain），点击 appLauncherOpenImage | 复用 WorkDialog imagePath 逻辑（thumbnail 优先，否则 workStore）`WorkDialog.vue:110-121` |
| VideoRenderer.vue | `<video controls>`，优先 merged 否则 videoTrack；无则降级占位+外部打开 | stores.find(merged) ?? stores.find(videoTrack)；buildStoreUrl |
| ArticleRenderer.vue | MarkdownView，复用现有 article 逻辑 | document store fetch .md + image stores 过滤 `WorkDialog.vue:131-155` |
| DocumentRenderer.vue | 占位（文件图标+文件名）+ "外部打开"按钮（appLauncherOpen） | workStore.filePath |
| UnknownRenderer.vue | 占位 + "外部打开"按钮 | 同上 |

**E. 新弹窗** — `frontend/src/components/dialogs/WorkDetailDialog.vue`

- 外壳：评估复用 AutoHeightDialog（header/scrollbar/footer 三段）或新建外壳（主体不需滚动、drawer 浮层）。倾向复用 AutoHeightDialog，主体区固定高度、内部由各渲染器自行处理滚动。
- 布局：第三节的"主体 + 右侧栏 + drawer"。
- 迁移 WorkDialog 的全部业务逻辑：getWorkInfo/refreshTags/refreshWorkSets/handleTagExchangeConfirm/setCurrentWork/handleKeydown/删除/合并/作品集等。
- 元数据区（el-descriptions）+ 标签编辑（ExchangeBox）整体迁入 el-drawer。

**F. 调用方迁移** — `frontend/src/components/common/WorkGridForMainPage.vue:63`、`WorkGridForWorkSet.vue:156`

WorkDialog → WorkDetailDialog（props/emits 契约保持：work/state/currentWorkIndex/openWorkSet）。旧 WorkDialog.vue 保留不删（决策4，用户手动移除）。

### 4.3 plugin.json 声明样例（插件侧）

```jsonc
{
  "slotType": "resourceViewer",
  "id": "bilibili-article-viewer",
  "name": "Bilibili专栏增强渲染器",
  "content": {
    "resourceType": "article",
    "contentType": "precompiled",
    "source": { "js": "views/article-viewer.js", "css": "views/article-viewer.css" },
    "props": {}
  }
}
```

插件渲染器组件接收 `{resource, work}` props（决策3），用 `var(--app-*)` 主题令牌（遵循 THEME_TOKEN_CONFORMANCE）。

## 五、实施顺序

1. **后端 slotType 扩展**：slot.go + plugin_types.go + app.go parseSlotContent（含 resourceType 必填校验）+ slot_handler.go。
2. **前端 HandlerRegistry + 同步层分支**：HandlerRegistryStore + useSlotSyncListener 分支 + convertToResourceViewerHandler。
3. **ResourceViewer + 内置渲染器**：先 ImageRenderer/ArticleRenderer（迁现有逻辑），再 VideoRenderer（内联播放），再 DocumentRenderer/UnknownRenderer（占位）。
4. **WorkDetailDialog**：外壳 + 布局 + 迁移业务逻辑 + drawer。
5. **调用方迁移**（WorkGridForMainPage/WorkGridForWorkSet → WorkDetailDialog；旧 WorkDialog.vue 保留，用户手动移除）。
6. **bindings 重生成 + 文档同步**（plugin.md/plugin-dev-guide.md 补 resourceViewer 声明契约 + resource-type-spec 第六节消费者表补"前端·渲染器查 HandlerRegistry"）。
7. **编译 + 验证**。

## 六、验证

- **内置渲染器**：分别打开 image/video/article/document/unknown 资源，展示正确；video 内联可播放；document/unknown 占位 + 外部打开可用。
- **插件渲染器**：测试插件声明 resourceViewer(article) → article 资源用插件渲染器；卸载插件 → 回落内置 ArticleRenderer；多插件同 resourceType → 取 order 最小（决策1）。
- **弹窗布局**：主体占大空间、右侧栏功能齐全、drawer 触发与标签编辑正常、键盘 ← → 切换作品。
- **故障隔离**：插件渲染器抛错 → PluginBoundary 降级为 fallback，不影响主程序与其他插件（声明5）。
- **回归**：WorkGridForMainPage/WorkGridForWorkSet 打开详情、openWorkSet 跳转正常。

## 七、与既有计划的关系

- **「插件slotType重构方案.md」**：定义了当前 6 种 slotType 体系。本方案不改动这 6 种，仅在 slotType 枚举新增第 7 种 `resourceViewer`，并引入"被动响应"语义。二者无冲突。
- **「插件扩展点命名统一方案-contribution转extension.md」**：待执行，方向与本方案一致（都用 extension 统称）。本方案**不执行** contribution→extension 全量重命名，仅在新引入的 HandlerRegistry 部分使用 extension 语义；contribution 残留由那个独立计划处理。**建议执行顺序**：本方案先落地（新增部分直接用 extension 命名），contribution→extension 统一后做（清理历史残留），避免两计划交叉重命名冲突（风险2）。
- **「resource-type-spec.md」/ T3**：本次仅覆盖现有 5 种 ResourceType。T3（插件自定义 ResourceType 后端注册表）延后；前端 HandlerRegistry 按"任意 resourceType 值"设计，T3 落地后前端无需改动即可支持自定义类型（风险1）。
