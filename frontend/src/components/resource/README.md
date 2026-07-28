# resource — 作品资源渲染

作品详情弹窗主体区的资源展示模块：按 ResourceType 分发到对应内置渲染器，并支持插件经 HandlerRegistry（被动响应型扩展）提供自定义渲染器覆盖内置。

## 对外接口

- `ResourceViewer.vue`：唯一入口组件。
  - props：`{ resource: ResourceFullDTO, work: WorkFullDTO }`
  - 渲染流程：
    1. `getResourcePreviewType(resource)`（含 unknown 降级嗅探）得到展示类型 rt
    2. 查 `HandlerRegistryStore.resourceViewerByType(rt)`：命中则渲染插件组件（`PluginBoundary` 包裹，透传 `{resource, work}`）
    3. 未命中则按 rt 分发到内置渲染器

## 内置渲染器（`renderers/`）

| 渲染器 | ResourceType | 行为 |
|---|---|---|
| ImageRenderer | image | el-image 展示原图（workStore）；双模式切换（全部展示 contain / 较短边占满 fill）；点击 `appLauncherOpenImage` |
| VideoRenderer | video | `<video controls>` 内联播放，优先 videoMain（可播放主体）否则 videoTrack |
| ArticleRenderer | article | MarkdownView 渲染正文 .md（document store fetch）+ 内嵌图（image stores 按 store_seq 顺序位置绑定） |
| DocumentRenderer | document | 无内联查看器，文件名占位 + 外部打开按钮（`appLauncherOpen`） |
| UnknownRenderer | unknown | 嗅探后仍无法分类的兜底，占位 + 外部打开 |

## 插件覆盖（Handler 通道）

插件在 `plugin.json` 声明 `slotType: "resourceViewer"` + `content.resourceType`，前端 `useSlotSyncListener` 路由到 `HandlerRegistryStore`（不进 SlotRegistryStore）。同 resourceType 多插件取 `order` 最小者。插件渲染器组件接收 `{resource, work}` props（运行时注入）。声明契约见 `.claude/rules/plugin.md`，二分框架（Slot 主动注入 vs Handler 被动响应）见同文件。

## 追加新内置渲染器

1. 在 `renderers/` 加 `XxxRenderer.vue`（props `{resource, work}`）
2. 在 `ResourceViewer.vue` 的 `v-else-if` 链按 ResourceType 加分支

## 依赖

- `frontend/src/store/HandlerRegistryStore.ts`（插件渲染器查询）
- `frontend/src/utils/ResourceUtil.ts`（`getResourcePreviewType`）
- `frontend/src/components/common/PluginBoundary.vue`（插件渲染器故障隔离）
- `frontend/src/components/common/MarkdownView.vue`（article 正文）
- `frontend/src/apis/http/wrappers/appLauncher`（图片/文档外部打开）

## 关键实现细节

- **ImageRenderer 双模式动画**：用 CSS 实验性 `interpolate-size: allow-keywords` 让 `width`/`height` 在 `100%` ↔ `auto` 之间双向过渡。关键约束：el-image 容器（`.image-renderer`）固定 `100%×100%` 不参与过渡——否则 img 的 `100%` 目标值会依赖正在过渡的容器尺寸（嵌套 auto + 百分比），导致切换瞬间图片先坍缩到极小再放大。仅 img 一层切换尺寸。WebView2（Chromium 129+）支持，不支持则回退单向过渡（渐进增强）。
