---
description: "前端架构与编码规则，适用于修改 frontend/ 目录下的代码时加载"
globs:
  - "frontend/**"
---

# 前端架构与规则

## 前端目录结构
```
frontend/src/
  views/              — 页面组件（17 个，含 BaseView 外壳）
  components/
    common/           — 通用组件 + 业务复用组件（DataTable/SearchTable 等通用容器；TaskList 业务复用列表；WorkCard/CardGrid/TagBox）
    dialogs/          — 对话框组件
    resource/         — 作品资源渲染（ResourceViewer 分发 + 内置渲染器 image/video/article/document/audio/unknown）
    slot/             — 插件插槽渲染器
    tour/             — 向导组件（TourOverlay、TourCenterPanel）
  composables/        — 组合式函数（useTourTargets、useTourReady、useBuiltinMenus 等）
  store/              — Pinia 状态（SlotRegistry 主动注入型扩展、HandlerRegistry 被动响应型扩展、Notification、Task、TourCenter、Theme 等）；UseMenuBadgeStore 为菜单红点注册表——任意菜单项按 slotId 写入计数即显示红点（0 隐藏），消费侧 DynamicSideMenu 不感知业务来源，插件插槽菜单的复合键同样可注册
  theme/              — 主题元信息清单（themes.ts：主题 id/名称/预览色板）
  tour/               — 向导定义集中文件（definitions.ts）
  styles/             — 全局样式（z-axis-layers、rounded-borders、scroll-text 等）
  styles/theme/       — 主题令牌体系（tokens.css 令牌定义、ep-bridge.css EP 桥接、theme-*.css 各主题配色）
  apis/http/wrappers/ — 按模块封装 Wails bindings 的 API wrapper
  utils/              — 通用工具函数（UrlUtil、CommonUtil、ImageDimension 等）
  model/tour/         — 向导类型定义（TourDefinition.ts）
  model/handler/      — 被动响应型扩展（Handler）类型（如 ResourceViewerHandler）
  model/model/        — 与 Go DTO/实体对应的 TypeScript 类型（已存在于"frontend/bindings/" 中的类型需逐步废弃并迁移到 "frontend/bindings/"）
frontend/bindings/    — 自动生成的 Wails TypeScript bindings（禁止手动编辑）
```

## 文件与命名规范

| 元素         | 规则                          | 示例                                  |
| ------------ | ----------------------------- | ------------------------------------- |
| Vue 组件     | PascalCase + `.vue`           | `WorkCard.vue`、`MainLayout.vue`      |
| TS 类型文件  | PascalCase + `.ts`            | `ApiResponse.ts`、`Page.ts`           |
| 工具函数文件 | PascalCase + `.ts`            | `treeUtil.ts`、`apiUtil.ts`           |
| 常量文件     | camelCase + `.ts`             | `httpStatus.ts`                       |
| 配置文件     | kebab-case + `.json`/`.yml`   | `tsconfig.json`                       |
| 布尔变量     | `is`/`has`/`can` 前缀         | `isLoading`、`hasError`、`canEdit`    |

## TypeScript 规范

- 模块解析：使用 `bundler`
- 路径别名：`@renderer/*` → `frontend/src/*`（前端专用），`@shared/*` → `frontend/src/model/*`（共用类型）
- 类型定义：优先使用 `interface` 定义对象结构，`type` 用于联合类型或工具类型
- 严格模式：启用 `strict: true`

## Vue 组件规范

- 语法：`<script setup lang="ts">` 组合式 API
- Props 接口使用 `Props` 后缀（如 `WorkCardProps`）
- Emits 使用 `defineEmits` 和 TypeScript 字面量类型
- 样式优先使用类选择器（`.class-name`），避免元素选择器和 ID 选择器

## 前端编码规则

- **WRAPPER_REQUIRE_RESPONSE** (P0): Wrapper 函数使用 `requireResponse<T>()`。查询调用默认 `requireData=true`，变更调用传 `requireData=false`。`requireResponse` 内部已处理 null 检查和错误校验，禁止重复手写同类检查。
- **UNIFIED_ERROR_THROW** (P0): 失败时 `throw new Error(msg)`，禁止返回 `undefined` 或静默空页面。
- **空值检查**: 使用 `NotNullish`/`IsNullish`、`ArrayNotEmpty`/`ArrayIsEmpty`（`@renderer/util/CommonUtil.ts`），`isNotBlank`/`isBlank`（`@renderer/util/StringUtil.ts`）。避免 `!value`。例外：布尔值取反（`!isLoading`）、判断 falsy 值时可使用 `!`。
- **PAGE_TYPE_UNIFICATION** (P1): 使用 Wails binding 的 `Page<T>`（`@bindings/...`），`copyPage<T>()`/`newPage<T>()` 转换/创建。禁止自定义 Page 模型。
- **QUERY_ATTRIBUTE_VALUE_BINDING** (P1): `QueryAttribute` 通过 `.value` 读写实际值，`v-model` 绑定 `xxx.value`，`@clear` 重置为 `null`，模糊搜索设置 `operator: Operator.OpLike`。
- **COMPOSITION_DTO_NESTED_PATH** (P1): 后端组合 DTO 后，DataTable thead 的 key 使用点号嵌套路径访问子 DTO 字段（如 `taskProgress.task.taskName`）。
- **BINDING_TYPE_REUSE** (P1): 已有 Wails 生成的 bindings 时，禁止创建或使用与其等价的自定义类型。前后端数据契约类型统一从 `frontend/bindings/`（`@bindings/...`）引用，禁止在 `model/model/`、组件内重复定义同义的 DTO/实体；`model/model/` 中等价的历史类型需逐步迁移至 bindings，不得新增。SDK 的 gRPC 契约类型（`TaskDTO`/`WorkDTO`/`WorkSetDTO`/`SiteDTO`/`LocalAuthorDTO`/`LocalTagDTO` 等 A 类）只从 `@bindings/.../library-squirrel-sdk/dto` 引用，其同级 `gen/` 目录是 proto 生成、供别名 re-export 的内部依赖（Wails 对 `type X=gen.Y` 别名固有跨包引用，非手写 API 面），禁止手写代码直接 import `gen/`。插件前端渲染契约类型（`render.Context`）从 `@bindings/.../library-squirrel-sdk/dto/render` 引用——它是独立于主程序展示 DTO 的断链契约（初始对齐 WorkFullDTO，此后独立演进，受 contractVersion 保护），禁止用主程序 `WorkFullDTO`/`ResourceFullDTO` 等展示 DTO 替代作插件渲染器 props。
- **ID_TYPE_NUMBER** (P2): ID 统一使用 `number`，从 `SelectItem.value`（string）取出时 `Number()` 转换。
- **方法命名**: 禁止与 prop 同名遮蔽。使用前缀：`handleXxx`、`doXxx`、`buildXxx`、`loadXxx`、`checkXxx`。
- **日期时间**: 统一使用 Unix 时间戳（毫秒），前端进行本地化格式转换。
- **THEME_TOKEN_USAGE** (P1): 样式统一使用 `--app-*` 主题令牌（清单见 `frontend/src/styles/theme/tokens.css`：颜色/背景/文字/边框/填充/标签/圆角/阴影），禁止硬编码颜色值、禁止直接使用 Element Plus 的 `var(--el-*)`（`--el-font-size-*` 等非颜色变量除外）。主题切换由 `<html data-theme="<id>">` + `useThemeStore`（`frontend/src/store/UseThemeStore.ts`）控制，业务代码通过令牌自动跟随，无需感知当前主题。插件样式契约见 `doc/plugin-theme-tokens.md`。
- **STATUS_TOKEN_USAGE** (P1): 状态展示（任务/来源/开关/资源等广义状态）的颜色用**语义 tone 色板**：`tokens.css` 定义 8 个 tone（`active`/`done`/`fail`/`warn`/`pending`/`idle` + `source-local`/`source-site` 专属），每个 tone 含 text/bg/border 三分量（bg/border 由 text 与白色 `color-mix` 派生）；`:root` 给出 default 主题默认色（text 沿用 Element Plus 经典色，如 done=#67c23a 绿、fail=#f56c6c 红），forest/ocean/sakura 在各自 `theme-*.css` 独立覆盖这些 tone 的值——**状态色随主题变化**。状态槽位独立于 `--app-color-{success,warning,...}` EP 组件色族（后者经 ep-bridge 驱动 el-button/el-tag，两条轨道默认互不引用），主题填色时自行保证槽位色相两两分散、避免与主色撞色。`source-local` 例外：跟随 `--app-color-primary`（本地数据用品牌色标识），`source-site` 固定紫。状态别名 `--app-status-{类目}-{语义}-{bg|text|border}` 引用对应 tone（如 `task-completed`→`done`、`task-processing`→`active`），类目 `task`/`source`/`toggle`/`resource`。渲染用 `StatusTag`（`frontend/src/components/common/StatusTag.vue`，传 `status` key），状态 key 与文案集中登记在 `frontend/src/constants/StatusRegistry.ts`。禁止用硬编码 rgb 或 EP `el-tag` type 表达业务状态（EP `el-tag` 仅限 success/warning 等纯色族二值场景）。新增状态：复用现有 tone 只需在 `tokens.css` 加别名引用并在 `StatusRegistry.ts` 登记；需新色相则先在 `tokens.css` :root 加 tone 三分量（text hex + `color-mix` bg/border，作为 default 默认色）并让各 `theme-*.css` 跟随覆盖。状态色板可在「状态色板」测试页（路由 `#/statusPalette`）一屏查看校准；调色操作详见 `doc/status-color-tuning.md`。破坏性操作按钮（删除/卸载/重置/替换等）用 `el-button type="danger"` + `tone-fail` class：色源走 fail tone（`frontend/src/styles/tone-button.css` 把 `--el-color-danger*` 档位重定向到 fail-text 派生），与失败/损坏状态语义同源、随 fail 逐主题调色变化——是「两条轨道默认互不引用」的例外。
- **TOUR_FRAMEWORK** (P1): 向导统一由 `useTourCenterStore` 控制，向导定义集中在 `frontend/src/tour/definitions.ts`，渲染统一由 `TourOverlay`（挂载于 `MainLayout`）完成。禁止在各页面内自行编写 `el-tour`。需被高亮的元素通过 `useTourTargets().register(key, ref)` 注册，`targetKey` 命名约定为 `{viewId}.{element}`（如 `settings.workdirInput`）。跨页面或需定位数据的步骤携带 `TourStepData`，目标页面通过 `useTourReady(onLocate)` 据 `ctx.data` 定位后报告就绪，引擎收到就绪信号后才显示该步气泡。
- **BUSINESS_LIST_REUSE** (P2): 业务列表**同时满足**「至少两处复用」与「含易随演进漂移的控制逻辑（操作栏、实时状态/进度渲染、刷新机制）」时，才提取为业务可复用组件——范例 `TaskList`（`frontend/src/components/common/TaskList.vue`）：封装 `SlotSearchTable` + 固定列 + 操作栏 + store 状态同步，仅以 `search(page, query)` 暴露数据来源、`view(row)` 上抛详情行为、`defineExpose({ doSearch })` 暴露刷新入口。**复用是手段而非目标**：单处使用的列表、或仅做简单展示/CRUD 的表格保持页面内联，不强求抽象，避免过度设计徒增间接层；两个判据同时成立才提取。
