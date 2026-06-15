# LibrarySquirrel 向导功能规格文档

本文档描述向导（Tour）功能的架构、数据模型、运行时序和扩展规范。向导用于在真实功能页面上分步引导用户，支持跨页面编排和"跳转到指定页面的指定数据"。

## 关键文件

| 组件 | 文件路径 |
|------|---------|
| 向导类型定义（TourStep/TourDefinition/TourContext 等） | `frontend/src/model/tour/TourDefinition.ts` |
| 向导控制中心 Store（状态机 + 跨页面引擎 + 持久化） | `frontend/src/store/UseTourCenterStore.ts` |
| 全局统一渲染层（el-tour 单步托管 + 自绘导航按钮） | `frontend/src/components/tour/TourOverlay.vue` |
| 控制中心 UI 面板（向导列表 / 状态 / 启动 / 重置） | `frontend/src/components/tour/TourCenterPanel.vue` |
| 页面侧：注册可高亮元素 | `frontend/src/composables/useTourTargets.ts` |
| 页面侧：就绪协议（据 data 定位后报告） | `frontend/src/composables/useTourReady.ts` |
| 内置向导定义集中文件 | `frontend/src/tour/definitions.ts` |
| 向导挂载入口（TourOverlay） | `frontend/src/MainLayout.vue` |
| 控制中心挂载入口（左控制中心 + 右说明） | `frontend/src/views/Guide.vue` |
| 启动注册 + 加载完成状态 | `frontend/src/main.ts` |
| 后端持久化结构（TourSettings.Completed） | `backend/settings/model.go`、`backend/settings/service.go` |
| IPC 跳转入口（goto-page → 启动向导） | `frontend/src/utils/PageUtil.ts`、`frontend/src/MainIpcListener.ts` |

---

## 设计原则

1. **声明式定义**：向导 = 一组有序 step 的数据描述（写在 `definitions.ts`），引擎消费它；新增向导无需改 store 或页面。
2. **集中渲染**：全局只有一个 `TourOverlay`（挂于 `MainLayout`）渲染当前 step，**禁止在各页面内自行编写 `el-tour`**。
3. **页面无关的元素定位**：step 用"路由 + targetKey"声明要高亮的元素，目标页面通过 composable 注册 targetKey→DOM。
4. **就绪协议**：跨页面切换后，引擎等待目标页面"加载完数据 + 挂载完元素"再显示 step。
5. **数据通道标准化**：step 携带强类型的 `TourStepData`，目标页面据此定位。
6. **持久化按向导 id**：用 `Map<tourId, completed>` 记录完成状态，替代固定 boolean 字段。

---

## 数据模型

### TourDefinition（向导定义）

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 向导唯一标识（如 `'first-time'`） |
| `name` | string | 显示名称 |
| `description` | string | 描述（控制中心展示） |
| `steps` | TourStep[] | 步骤列表，按顺序执行 |

### TourStep（单步）

| 字段 | 类型 | 说明 |
|------|------|------|
| `target` | TourStepTarget | 目标定位（路由 + 可选元素 key） |
| `title` | string? | 标题（渲染于气泡 header） |
| `description` | string | 描述（**必须**，渲染于气泡 body） |
| `placement` | TourStepPlacement? | 气泡相对目标的位置（top/bottom/left/right/center 等） |
| `data` | TourStepData? | 该步需要的目标数据（引擎写入 context，目标页面读取并定位） |
| `onEnterPage` | (ctx) => void \| Promise\<void\>? | 进入该步前、在目标页面侧执行的钩子 |

### TourStepTarget（目标定位）

| 字段 | 类型 | 说明 |
|------|------|------|
| `route` | string | 目标路由 name（= SlotRegistry 的 viewId，如 `'settings'`、`'taskManage'`） |
| `targetKey` | string? | 页面内可被高亮的元素 key；不填则气泡居中显示、不依附元素 |

### TourStepData（目标数据，可辨识联合）

```ts
| { kind: 'work'; workId: number }
| { kind: 'tag'; tagId: number; scope?: 'local' | 'site' }
| { kind: 'author'; authorId: number; scope?: 'local' | 'site' }
| { kind: 'task'; taskId: number }
| { kind: 'none' }
```

### TourContext（运行期上下文，控制中心 → 目标页面）

| 字段 | 类型 | 说明 |
|------|------|------|
| `tourId` | string | 当前向导 ID |
| `stepIndex` | number | 当前步骤索引 |
| `data` | TourStepData? | 该步的目标数据 |
| `payload` | Record\<string, unknown\>? | 调用方自定义载荷 |

### TourStatus（运行状态）

`'idle'` → `'running'` → `'finished'`

---

## 架构与数据流

```
┌──────────────────────────────────────────────────────────────┐
│                useTourCenterStore（向导控制中心）              │
│  registry: Map<tourId, TourDefinition>                        │
│  activeTourId / activeStepIndex / status / stepResolved       │
│  context: TourContext                                         │
│  completed: Map<tourId, boolean>（持久化到 Settings.tour）     │
│                                                              │
│  start(id, payload?) → resolveStep → next() → ... → finish() │
└───────────────┬──────────────────────────────┬───────────────┘
                │ 读 activeStep                  │ 注册/解析
   ┌────────────▼────────────┐      ┌───────────▼────────────┐
   │  TourOverlay（全局组件）  │      │ 模块级目标注册表          │
   │  挂于 MainLayout          │      │ registerTourTarget(key,  │
   │  渲染当前 step 气泡        │      │   resolver) / resolve     │
   │  自管导航按钮              │      │ 模块级就绪信号            │
   └──────────────────────────┘      │ beginReadyWait/await     │
                                     │   reportTourReady()      │
                                     └───────────┬────────────┘
                                                 │
                ┌────────────────────────────────▼───────────┐
                │ 页面侧 composable                            │
                │ useTourTargets.register(key, ref)            │
                │ useTourReady(onLocate)：据 ctx.data 定位后     │
                │   调用 reportTourReady() 报告就绪              │
                └─────────────────────────────────────────────┘
```

**目标元素注册表与就绪信号位于 store 模块作用域**（非 Pinia state），通过 `registerTourTarget`/`unregisterTourTarget`/`resolveTourTarget` 和 `reportTourReady` 导出函数访问。这样 `TourOverlay` 和页面 composable 都能读写同一份全局注册表。

---

## 运行时序

### 单步解析流程（resolveStep，核心）

引擎推进到每一步时执行：

```
start(tourId, payload?) / next()
  → activeStepIndex 指向 step
  → stepResolved = false
  → 构建 context = { tourId, stepIndex, data: step.data, payload }
  → 若 step.onEnterPage → 执行钩子（如打开某个对话框）
  → 若 step.data 且 kind !== 'none' → beginReadyWait()（注册就绪 Promise，必须在跳转前）
  → 跳转目标路由（router.push({ name: step.target.route })，已在同路由则跳过）
  → nextTick()
  → 若 step.target.targetKey → 轮询等待该 targetKey 的 DOM 挂载
      （50ms 间隔，3s 超时降级：气泡居中显示）
  → 若需要就绪 → await awaitReady()（等待 reportTourReady，同样 3s 超时兜底）
  → stepResolved = true → TourOverlay 显示该步气泡
```

**关键时序**：`beginReadyWait()` 必须在 `router.push` **之前**调用。因为目标页面 `onMounted` 时会立即调用 `reportTourReady`，若等待 Promise 在跳转后才创建，会错过信号导致永久等待（最终靠超时降级）。

### 步进控制

| 动作 | 行为 |
|------|------|
| `next()` | activeStepIndex++ → resolveStep；最后一步则 finish() |
| `prev()` | activeStepIndex--，**不重新跳路由**（仅回退气泡） |
| `skip()` | 等同 finish()（不标记完成） |
| `finish()` | markCompleted(activeTourId) → reset() |
| `reset()` | 清空 active 状态、context、就绪信号 |

### TourOverlay 渲染逻辑

```
visible = (status === 'running') && stepResolved && activeStep 存在
targetEl = stepResolved 后，据 activeStep.target.targetKey 解析的 DOM（无 targetKey 则 undefined → 居中）
```

- el-tour 的 `v-model="visible"`，用户关闭气泡（点遮罩/ESC）→ set false → 触发 `store.skip()`
- el-tour-step 用 `:key="stepKey"`（`${tourId}-${stepIndex}`）确保 target 变化时重新定位
- 导航按钮（跳过 / 上一步 / 下一步·完成）调用 store 的 `skip/prev/next`

---

## el-tour 适配要点（踩坑记录）

### 1. 内置 footer 必须全局隐藏

el-tour 的 footer 硬编码渲染**进度点**（`.el-tour__indicators`）和**上一步/Finish 按钮**（`.el-tour__buttons`），**官方无 prop、无插槽可单独控制**（API 表只有 `show-close`、`type`、`next/prev-button-props`，插槽只有 `default`/`header`/`indicators`）。

- `:show-indicators="false"` 是**不存在的属性**，无效
- el-tour 内容通过 `Teleport` 渲染到 `body`，scoped 的 `:deep()` **无法穿透**到 Teleport 目标
- **正确做法**：TourOverlay 用非 scoped 的全局 `<style>` 块 `display:none` 整个 `.el-tour__footer`，改由 `el-tour-step` 默认插槽内自绘按钮。本项目仅此一处用 el-tour，全局隐藏无副作用

### 2. description 不能用 prop，必须写进插槽

el-tour-step 源码：`renderSlot("default", {}, () => <span>{description}</span>)`。`description` prop 只是 **default 插槽的 fallback**——一旦提供 default 插槽内容（自绘按钮），description 会被整体覆盖而消失。

- **正确做法**：不传 `:description` prop，在 default 插槽内手动渲染 `<span>{{ description }}</span>`，再跟自绘按钮

---

## 持久化

### 后端结构

`backend/settings/model.go`：

```go
type TourSettings struct {
    Completed map[string]bool `json:"completed" koanf:"completed"`
}
```

- 按 tourId 记录完成状态，旧固定字段（firstTimeTourPassed/workdirTour/taskTour）已废弃
- **不迁移历史完成状态**：升级后以空 `completed` 启动，老用户可能再次被引导（已确认可接受）

### 前端读写

- `useTourCenterStore.loadCompleted()`：启动时从 `settingsGetSettings()` 读取 `tour.completed`，填入 store 的 `completed` Map
- `useTourCenterStore.persist()`：通过 `settingsSaveSettings([{ path: 'tour.completed', value: Object.fromEntries(completed) }])` 写回（点号路径，整个 map 替换）
- `main.ts` 启动时执行 `registerBuiltinTours` + `loadCompleted`

---

## 页面接入规范

### 注册可高亮元素

```ts
// 在页面 setup 中
const workdirInput = ref()
const { register } = useTourTargets()
register('settings.workdirInput', workdirInput)  // 自动 onBeforeUnmount 注销
```

**targetKey 命名约定**：`{viewId}.{element}`，如 `settings.workdirInput`、`taskManage.localImportButton`。

`useTourTargets` 兼容组件实例（取 `$el`）与原生元素。

### 接入就绪协议（需定位数据时）

```ts
// 在页面 setup 中
useTourReady(async (ctx) => {
  if (ctx.data?.kind === 'tag') {
    await loadAndLocateTag(ctx.data.tagId)  // 业务定位：查询、滚动、选中、打开详情
  }
})
```

- `useTourReady` 在 `onMounted` 时，仅当向导运行中且 context 存在才触发
- 执行完 `onLocate` 后自动调用 `reportTourReady()`，引擎收到信号才显示气泡
- 不带 step.data 的简单向导（纯高亮）无需接入 `useTourReady`

---

## 控制中心与启动入口

### 控制中心 UI（TourCenterPanel）

挂于 `Guide.vue`（左控制中心 + 右功能说明）。列出 `tourList`，每行显示：
- 名称 + 状态标签（进行中 / 已完成）
- 「启动」按钮（向导运行时禁用其他向导的启动，避免并发）
- 「重置」按钮（仅已完成向导显示）
- 进行中向导显示「结束」按钮

### 程序化启动

- 控制中心点「启动」→ `store.start(tourId)`
- IPC `goto-page` 到 Settings 且 extraData 为真 → `store.start('first-time')`（见 `PageUtil.askGotoPage`）

---

## 扩展：新增一个向导

1. 在 `frontend/src/tour/definitions.ts` 的 `builtinTours` 数组添加一个 `TourDefinition`
2. 若该向导需要高亮**新元素**，在目标页面通过 `useTourTargets().register(key, ref)` 注册对应 targetKey
3. 若该向导需要**定位数据**，在目标页面接入 `useTourReady`，并在 step 携带 `data`

无需改动 store、TourOverlay 或任何渲染层。

### 示例（定位指定数据）

```ts
{
  id: 'locate-tag',
  name: '定位指定标签',
  description: '演示跳转到本地标签页并定位到某条数据',
  steps: [
    {
      target: { route: 'localTagManage', targetKey: 'localTagManage.table' },
      description: '正在为您定位标签…',
      data: { kind: 'tag', tagId: 123, scope: 'local' },
    },
  ],
}
```

> 注：该示例当前以注释形式保留在 `definitions.ts`，启用需先在 `LocalTagManage` 页接入 `useTourTargets`/`useTourReady`。

---

## 编码规则（摘自 `.claude/rules/frontend.md`）

- **TOUR_FRAMEWORK (P1)**：向导统一由 `useTourCenterStore` 控制，定义集中在 `tour/definitions.ts`，渲染统一由 `TourOverlay`（挂载于 `MainLayout`）完成。**禁止在各页面内自行编写 `el-tour`**。需被高亮的元素通过 `useTourTargets().register(key, ref)` 注册，`targetKey` 命名约定为 `{viewId}.{element}`。跨页面或需定位数据的步骤携带 `TourStepData`，目标页面通过 `useTourReady(onLocate)` 据 `ctx.data` 定位后报告就绪，引擎收到就绪信号后才显示该步气泡。

---

## 更新记录

### 2026-06-15
- [新增] 创建向导功能规格文档，覆盖数据模型、跨页面引擎时序、el-tour 适配要点、持久化与扩展规范
