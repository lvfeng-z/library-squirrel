---
description: "前端架构与编码规则，适用于修改 frontend/ 目录下的代码时加载"
globs:
  - "frontend/**"
---

# 前端架构与规则

## 前端目录结构
```
frontend/src/
  views/              — 页面组件（15 个视图）
  components/
    common/           — 通用组件（DataTable、SearchTable、WorkCard、CardGrid、TagBox）
    dialogs/          — 对话框组件
    slot/             — 插件插槽渲染器
    tour/             — 向导组件（TourOverlay、TourCenterPanel）
  composables/        — 组合式函数（useTourTargets、useTourReady、useBuiltinMenus 等）
  store/              — Pinia 状态（SlotRegistry、Notification、Task、TourCenter、Theme 等）
  theme/              — 主题元信息清单（themes.ts：主题 id/名称/预览色板）
  tour/               — 向导定义集中文件（definitions.ts）
  styles/             — 全局样式（z-axis-layers、rounded-borders、scroll-text 等）
  styles/theme/       — 主题令牌体系（tokens.css 令牌定义、ep-bridge.css EP 桥接、theme-*.css 各主题配色）
  apis/http/wrappers/ — 按模块封装 Wails bindings 的 API wrapper
  utils/              — 通用工具函数（UrlUtil、CommonUtil、ImageDimension 等）
  model/tour/         — 向导类型定义（TourDefinition.ts）
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
- **DIALOG_USE_BINDING_DTO** (P1): Dialog `formData` 使用 Wails 绑定层 DTO 类型。组合 DTO 初始化时预创建嵌套对象，模板直接绑定，禁止使用 `computed` 中间层。
- **ID_TYPE_NUMBER** (P2): ID 统一使用 `number`，从 `SelectItem.value`（string）取出时 `Number()` 转换。
- **方法命名**: 禁止与 prop 同名遮蔽。使用前缀：`handleXxx`、`doXxx`、`buildXxx`、`loadXxx`、`checkXxx`。
- **日期时间**: 统一使用 Unix 时间戳（毫秒），前端进行本地化格式转换。
- **THEME_TOKEN_USAGE** (P1): 样式统一使用 `--app-*` 主题令牌（清单见 `frontend/src/styles/theme/tokens.css`：颜色/背景/文字/边框/填充/标签/圆角/阴影），禁止硬编码颜色值、禁止直接使用 Element Plus 的 `var(--el-*)`（`--el-font-size-*` 等非颜色变量除外）。主题切换由 `<html data-theme="<id>">` + `useThemeStore`（`frontend/src/store/UseThemeStore.ts`）控制，业务代码通过令牌自动跟随，无需感知当前主题。插件样式契约见 `doc/plugin-theme-tokens.md`。
- **TOUR_FRAMEWORK** (P1): 向导统一由 `useTourCenterStore` 控制，向导定义集中在 `frontend/src/tour/definitions.ts`，渲染统一由 `TourOverlay`（挂载于 `MainLayout`）完成。禁止在各页面内自行编写 `el-tour`。需被高亮的元素通过 `useTourTargets().register(key, ref)` 注册，`targetKey` 命名约定为 `{viewId}.{element}`（如 `settings.workdirInput`）。跨页面或需定位数据的步骤携带 `TourStepData`，目标页面通过 `useTourReady(onLocate)` 据 `ctx.data` 定位后报告就绪，引擎收到就绪信号后才显示该步气泡。
