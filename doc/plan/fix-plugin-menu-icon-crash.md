# 插件故障隔离 — 阻止插件问题向主程序传播

> 对应待办:`doc/todo.md#L29` —「插件前端提供错误的菜单按钮图标时,会导致前端页面无法加载」
> 定性:这不是「渲染对图标」的问题,而是**插件错误击穿了主程序渲染边界**。图标崩溃只是表象,本质要建立插件→主程序的故障隔离。
>
> ✅ **已实施并验证(2026-06-30)**。最终实现见文末「六、实施结果」。关键演进:`PluginBoundary` 从原计划的 `<slot/>` 模式改为 **component-prop 模式**(slot 模式下 `onErrorCaptured` 不触发);菜单改用 `AppIcon` 数据边界、不套 `PluginBoundary`。下文第一~五节为「计划原文」,以第六节为准。

## 一、重新定性

一个插件的数据或组件出错,绝不允许拖垮整个主程序 UI。当前四个「插件内容注入主程序」的入口,隔离能力参差,全都存在逃逸路径:

| 入口 | 渲染处 | 现有隔离 | 缺口 |
|---|---|---|---|
| menu | `DynamicSideMenu.vue:115/124/135` | **无** | 插件图标字符串直接喂 `<component :is>`,Vue 把非法字符串当标签名 `createElement('/plugin/.../Link')` 抛 `InvalidCharacterError`,逃逸到 `<App>`/`<RouterView>` 未捕获 → 整页白屏(**本次崩溃**) |
| embed | `EmbedSlotRenderer.vue` | `defineAsyncComponent.errorComponent` | 只兜**加载阶段**(loader reject),**渲染阶段**抛错照样逃逸 |
| dialog | `DialogSlotRenderer.vue` | 同上 | 同上 |
| view/replaceView | `useSlotSyncListener.ts` → 路由 `component: () => loadPluginComponent(...)` | 同 loader 模式 | 同上,只兜加载 |

主程序目前**没有任何 Vue 错误边界**,也没有全局 `app.config.errorHandler`。

**Vue 机制澄清**:`defineAsyncComponent` 的 `errorComponent` 只在 loader Promise reject 时显示;组件**加载成功后在 render/setup/生命周期抛出的错误**不会被它捕获,会继续冒泡。能阻断冒泡、让出错子树降级而非炸掉整页的,是 `onErrorCaptured`(错误边界)。

## 二、设计原则

1. **数据边界**:不把插件来源的数据直接喂给「可抛异常的渲染 API」。`<component :is>` 拿到非法字符串会 `createElement` 抛错——插件数据必须先归一化/校验,或改走天然容错的渲染方式(如 `<el-image>`)。
2. **组件边界**:所有插件注入的组件子树,必须包裹在错误边界里;边界 `onErrorCaptured` 返回 `false` 阻断冒泡,渲染降级 UI(「插件渲染失败」+ 可重试),**仅出错插件降级,主程序与其它插件不受影响**。
3. **最后防线**:全局 `app.config.errorHandler` 兜底,保证任何漏网的渲染错误不再整页白屏,并落日志。

三条原则是「纵深防御」:数据边界消除已知抛错点,组件边界隔离未知渲染错误,全局兜底防漏网。

## 三、改动清单

### 1. 新增 `components/common/PluginBoundary.vue`(组件边界,核心)

- 用 `onErrorCaptured((err, instance, info) => { 记日志; error.value = true; return false })`:`return false` 阻断错误向 `<App>` 冒泡。
- 正常时渲染 `<slot />`;捕获错误后渲染 fallback(占位图标 + 「插件渲染失败」文案 + 可选「重试」按钮,重试即重置 `error` 重新挂载)。
- 落日志:经现有 `frontendLog` 通道写 `frontend.log`(含插件 slotId、错误堆栈、`info`),便于定位是哪个插件炸了。
- 样式遵循 `THEME_TOKEN_USAGE`(`--app-*` 令牌)。

### 2. 新增 `components/common/AppIcon.vue`(数据边界,针对图标)

图标属于「主程序代码渲染插件数据」(非插件组件),逐个图标包错误边界太重,正确做法是**归一化数据**:
- `icon` 为字符串(插件图片 URL)→ `<el-image :src>` + `#error` 兜底占位(非法 URL 如 `/plugin/.../Link` 只显示占位,**绝不抛异常**)。
- `icon` 为组件对象(内置菜单 `markRaw(...)`)→ `<component :is>`。
- `icon` 为空 → 默认占位图标。
- 这样 `<component :is>` 永远拿不到非法字符串,从源头消除本次崩溃。

### 3. 应用到各入口

- **`DynamicSideMenu.vue`**:三处图标改 `<AppIcon :icon="item.icon" />`(消除崩溃);插件菜单子树外层包 `<PluginBoundary>`(防未来其他插件数据问题)。
- **`EmbedSlotRenderer.vue` / `DialogSlotRenderer.vue`**:保留现有 `defineAsyncComponent`(处理加载态/loading/超时),把 `<component :is>` 包进 `<PluginBoundary>`(处理渲染态错误)。
- **view/replaceView**:在插件路由组件注册处用 `PluginBoundary` 包裹(loader 解析出的组件外面套一层边界),或在 `<RouterView>` 渲染处按 `isPlugin` 包裹——保证插件页 render 抛错只降级该页。

### 4. 全局兜底

- app 初始化(`main.ts` 或等价处)设 `app.config.errorHandler`:记日志,避免最后白屏。这是「最后防线」,不替代上述边界。

### 5. 类型与文档

- `store/SlotRegistryStore.ts:11` `MenuSlotItem.icon`:`unknown` → `Component | string`。
- `DynamicSideMenu.vue:22` `MenuItem.icon` 同步收窄。
- `model/interface/SlotConfigs.ts:46` 注释「图标 (Element Plus 图标名)」更正为:插件为图片 URL(后端 `resolveIconURL` 包装为 `/plugin/...`),内置菜单为 Element Plus 图标组件对象。

> 后端不在本次范围:`app.go` `resolveIconURL` 无条件包装 icon 是合理的(它无法判断插件意图);前端必须自身容错。可选地后端在 icon 非法时打 `WARN` 日志,作为辅助定位,非必需。

## 四、验证

1. `cd frontend && yarn build` 通过(类型收窄无报错)。
2. **复现 menu 崩溃**:构造 icon 为非法值(如 `Link`)的插件 menu slot → 主程序**正常加载**,该项图标显示占位,控制台无 `InvalidCharacterError`。
3. **渲染期抛错隔离**:构造一个 render 时主动抛错的插件组件,分别验证 embed / dialog / view 三种入口 → **仅该插件位显示降级 UI**,主程序与其它插件正常,`frontend.log` 记录到对应插件。
4. 内置菜单/视图图标与渲染不受影响。
5. 站点浏览器列表图标(若沿用 `AppIcon`)回归正常。

## 五、影响面与里程碑建议

- 仅前端;新增 2 个通用组件(`PluginBoundary`、`AppIcon`),改动 4 个渲染入口 + app 初始化 + 类型。
- 不改 IPC/DTO/后端契约。
- 建立可复用的插件隔离基础设施,后续新 slot 类型默认沿用 `PluginBoundary`。
- **可分两步交付**:
  - 第一步(止血,最小):改动 2(`AppIcon`)+ 改动 3 的 `DynamicSideMenu` 图标部分 + 类型收窄 → 解决本次 todo#29 崩溃。
  - 第二步(根治,系统性):改动 1(`PluginBoundary`)+ 改动 3 的 embed/dialog/view 包裹 + 改动 4 全局兜底 → 彻底阻断任何插件问题向主程序传播。

## 六、实施结果（最终实现，2026-06-30 完成）

已实施并通过运行时验证。最终实现与原计划的差异记录如下,**以本节为准**。

### 最终机制

- **数据边界 `AppIcon.vue`**:`DynamicSideMenu` 三处图标改用 `<AppIcon>`——字符串走 `<el-image>`+`#error`,组件对象走 `<component :is>`。`MenuSlotItem.icon`/`MenuItem.icon` 收窄为 `Component | string`。→ 解决 todo#29 的菜单图标白屏。
- **组件边界 `PluginBoundary.vue`**:`onErrorCaptured` + `return false` + fallback + 重试,落 `frontend.log`(含 `msg`+`stack`)。应用于 embed/dialog(`:component`/`:component-props`)、view/replaceView(`useSlotSyncListener.ts` 的 `wrapWithBoundary` → `h(PluginBoundary, { name, component: child })`)。
- **全局兜底**:`main.ts` 的 `app.config.errorHandler`(仅记录)。

### 关键演进:slot 模式 → component-prop

原计划用 `<plugin-boundary><child/></plugin-boundary>`(slot 模式)包裹。**实测失败**:菜单图标错误仍白屏,`onErrorCaptured` 未触发。

根因(Vue 3.5 源码确认):slot 内容的 `instance.parent` 归属「提供方」(如 `EmbedSlotRenderer`),`handleError`(runtime-core:226)沿 `instance.parent` 链上行时**绕过**渲染 `<slot/>` 的边界。改用 component-prop——边界自身 `<component :is="component">` 渲染子组件,成为其真正的 parent,`onErrorCaptured` 稳定触发。

### 菜单不走 PluginBoundary

菜单项不是插件组件,而是主程序用插件**数据**渲染。故菜单仅用 `AppIcon`(数据边界),不套 `PluginBoundary`。**插件数据错误靠预防(AppIcon)、插件组件错误靠隔离(PluginBoundary),两层并行。**

### 验证结果

- 构造 render 抛错的 embed(`ThrowingPluginTest.vue`,验证后已删)→ `[PluginBoundary]` 捕获 `info:"render function"`,应用不崩,无 `[GlobalErrorHandler]`、无白屏。✅
- 菜单图标:当前会话 `createElement InvalidCharacterError` 消失。✅
- `yarn build` 通过。
- 关键结论已写入记忆 `plugin-fault-isolation.md`。
