# 向导创建技能

## 适用场景

新增或修改前端向导（功能引导 / 操作演示）。典型触发词：

- "添加一个向导"、"新建向导"、"加个引导"
- "向导"、"引导"、"tour"
- "首次使用引导"、"操作演示"
- "高亮某个按钮"、"引导用户到 xxx 页面"

## 架构概览

向导采用**集中定义 + 集中渲染 + 分布式注册目标**架构，禁止页面内自行编写 `el-tour`（`TOUR_FRAMEWORK` P1 规则）。

```
definitions.ts（声明式定义）
        │  main.ts 启动时 registerBuiltinTours → store.registerTour
        ▼
UseTourCenterStore（引擎：注册表 / 步进 / 路由跳转 / 就绪协议）
        │ storeToRefs                          ▲ register(key, ref) / reportReady
        ▼                                      │
TourOverlay（全局唯一渲染层，挂 MainLayout）   各业务页面（useTourTargets / useTourReady）
        ▲ 启动 / 重置 / 结束
        │
TourCenterPanel（向导中心 UI，挂 Guide.vue）
```

数据流严格自上而下：`TourDefinition.ts`（类型）→ store（运行态）→ 前端接口。渲染层全项目只有 `TourOverlay` 一处。

## 关键文件

| 角色 | 文件 |
|------|------|
| 类型定义 | `frontend/src/model/tour/TourDefinition.ts` |
| **向导定义（创建入口）** | `frontend/src/tour/definitions.ts` |
| 引擎 + 目标注册表 + 就绪协议 | `frontend/src/store/UseTourCenterStore.ts` |
| 目标元素注册 composable | `frontend/src/composables/useTourTargets.ts` |
| 就绪信号 composable | `frontend/src/composables/useTourReady.ts` |
| 全局渲染层（挂 MainLayout） | `frontend/src/components/tour/TourOverlay.vue` |
| 向导中心 UI（挂 Guide.vue） | `frontend/src/components/tour/TourCenterPanel.vue` |
| 启动注册 | `frontend/src/main.ts`（`registerBuiltinTours` + `loadCompleted`） |

## 核心数据结构

### TourDefinition（向导）

| 字段 | 必填 | 说明 |
|------|------|------|
| `id` | 是 | 唯一标识，用于持久化完成状态，**一经发布不要修改** |
| `name` | 是 | 显示名称（向导中心列表） |
| `description` | 是 | 一句话描述 |
| `steps` | 是 | 步骤列表，按顺序执行 |

### TourStep（单步）

| 字段 | 必填 | 说明 |
|------|------|------|
| `target` | 是 | 目标定位：`{ route, targetKey? }` |
| `description` | 是 | 气泡正文 |
| `title` | 否 | 气泡标题 |
| `placement` | 否 | 气泡位置，不填由 el-tour 默认决定 |
| `data` | 否 | 跨页定位数据，触发就绪协议 |
| `onEnterPage` | 否 | 进入目标页前的钩子（如打开对话框），跳路由前执行 |

### TourStepTarget

| 字段 | 必填 | 说明 |
|------|------|------|
| `route` | 是 | 目标路由 name，**必须与 SlotRegistry 的 viewId 对齐**（如 `settings`、`taskManage`） |
| `targetKey` | 否 | 页面内高亮元素 key；不填则气泡居中显示 |

### TourStepPlacement

`top` / `top-start` / `top-end` / `bottom` / `bottom-start` / `bottom-end` / `left` / `right` / `center`

### TourStepData（跨页定位数据，联合类型）

| kind | 携带字段 |
|------|---------|
| `work` | `workId` |
| `tag` | `tagId`、`scope?: 'local' \| 'site'` |
| `author` | `authorId`、`scope?: 'local' \| 'site'` |
| `task` | `taskId` |
| `none` | 无（表示不需要就绪等待） |

## 创建向导工作流程

### 第 0 步：判断复杂度

| 需求 | 改动范围 |
|------|---------|
| 只展示文字气泡 | 仅改 `definitions.ts` |
| 高亮某页面已有元素 | 仅改 `definitions.ts`（前提：目标 key 已注册） |
| 高亮某页面**尚不存在**的元素 | `definitions.ts` + 目标页面补 `ref` 与 `register` |
| 跨页跳转并定位到某条数据 | `definitions.ts` + 目标页面接 `useTourReady` |

**绝大多数情况下只需改 `definitions.ts` 的 `builtinTours` 数组，无需动 store 或渲染层。**

### 场景 1：纯居中气泡（不依附元素）

```ts
{
  id: 'my-tour',
  name: '我的向导',
  description: '说明这个向导是干嘛的',
  steps: [
    {
      target: { route: 'settings' },   // 不写 targetKey
      title: '欢迎',
      description: '这里是设置页面……',
    },
  ],
}
```

### 场景 2：高亮页面内**已注册**的元素

先确认目标页面已注册同名 key（用 Grep 搜 `registerTourTarget('xxx.yyy'`），再写定义：

```ts
{
  target: { route: 'settings', targetKey: 'settings.workdirInput' },
  title: '工作目录',
  description: '在这里设置资源库的根目录',
  placement: 'bottom',
}
```

### 场景 3：高亮页面内**尚未注册**的元素

需同时改定义和目标页面。

**定义**（`definitions.ts`）：

```ts
{ target: { route: 'taskManage', targetKey: 'taskManage.myButton' }, description: '点这里……' }
```

**目标页面**（如 `TaskManage.vue`）仿照现有写法：

```vue
<script setup lang="ts">
import { useTourTargets } from '@renderer/composables/useTourTargets'

const myButton = ref()
const { register: registerTourTarget } = useTourTargets()
registerTourTarget('taskManage.myButton', myButton)   // key 必须与定义里的 targetKey 完全一致
</script>

<template>
  <el-button ref="myButton">我的按钮</el-button>
</template>
```

> `useTourTargets` 兼容组件实例（取 `$el`）与原生元素，组件卸载时自动注销。

### 场景 4：跨页面 + 定位到具体数据

需同时改定义和目标页面，配对使用 `data` + `useTourReady`。

**定义**（`definitions.ts`）：

```ts
{
  target: { route: 'localTagManage', targetKey: 'localTagManage.table' },
  description: '正在为您定位标签……',
  data: { kind: 'tag', tagId: 123, scope: 'local' },
  // 可选：进入页面前的钩子，如预先打开某个筛选条件
  // onEnterPage: (ctx) => { ... }
}
```

**目标页面**（如 `LocalTagManage.vue`）：

```ts
import { useTourReady } from '@renderer/composables/useTourReady'

useTourReady(async (ctx) => {
  // 据 ctx.data 定位：查询并滚动到目标数据
  if (ctx.data?.kind === 'tag') {
    // ...定位逻辑
  }
  // 回调结束后 composable 自动调用 reportReady()
})
```

引擎只在收到 `reportReady` 后才显示该步气泡（带 3s 超时兜底）。

## 步骤编排时序（store.resolveStep）

每进入一步，引擎按以下顺序执行，理解时序有助于排查"气泡不出现 / 出现太早"：

```
1. 构建 context { tourId, stepIndex, data, payload }
2. 若有 onEnterPage → 执行（跳路由前，可打开对话框/设状态）
3. 若 data 存在且 kind !== 'none' → beginReadyWait()（注册就绪等待，必须在跳转前）
4. router.push(target.route)（若与当前路由不同）+ nextTick
5. 若有 targetKey → waitTargetEl：50ms 轮询，3s 超时后降级为居中显示
6. 若 needReady → awaitReady：等 reportReady，3s 超时兜底
7. stepResolved = true → TourOverlay 显示气泡
```

## 命名约定

- **targetKey**：`{viewId}.{element}`，如 `settings.workdirInput`、`taskManage.localImportButton`。
- **id**：kebab-case，如 `first-time`、`locate-tag`。发布后勿改（关联 `settings.tour.completed` 持久化）。
- **route**：与 `SlotRegistry` 的 viewId 完全一致。

## 验证清单

- [ ] `cd frontend && yarn build` 编译通过
- [ ] 每个 `targetKey` 都在对应页面通过 `useTourTargets().register(...)` 注册（Grep 确认）
- [ ] 每个 `route` 与 `SlotRegistry` 的 viewId 对齐
- [ ] 用了 `data` 的步骤，目标页面已接入 `useTourReady`
- [ ] 手动启动向导走查：每步气泡定位正确、下一步/上一步/跳过/完成均正常
- [ ] 已修改过的向导，在向导中心点「重置」清除旧完成状态后再测

## 常见陷阱

### 陷阱 1：targetKey 未注册导致气泡居中错位

`targetKey` 在目标页面没注册时，引擎不会报错，而是 3s 超时后让气泡居中显示——表现为"气泡飘在屏幕中间"。**新增 targetKey 必须同步在页面注册。**

### 陷阱 2：route 与 viewId 不一致

`route` 取的是路由 name，必须等于该页面的 viewId（`SlotRegistry`）。写错会跳转失败或跳错页面。不确定时先 Grep 该页面的路由注册。

### 陷阱 3：在页面里自己写 el-tour

违反 `TOUR_FRAMEWORK`（P1）。所有渲染集中在 `TourOverlay`，页面只负责"注册目标元素"和"报告就绪"，不要引入 `el-tour`。

### 陷阱 4：依赖 el-tour 内置导航按钮

`TourOverlay` 用全局样式 `display:none` 隐藏了 `.el-tour__footer`（el-tour 自带的进度点 + 上一步/Finish 按钮），改由 `el-tour-step` 默认插槽内**自绘按钮**接管（跳过 / 上一步 / 下一步 / 完成）。因此向导的导航交互是固定的，不要试图通过 el-tour 配置改导航按钮。

### 陷阱 5：修改已发布向导的 id

完成状态以 `id` 为 key 存在 `settings.tour.completed`。改 `id` 会让旧记录变成孤儿，用户需手动「重置」才能重看。调整步骤内容可以，**不要改 id**。

### 陷阱 6：data 与 useTourReady 未配对

定义里写了 `data`（非 `none`），但目标页面没接 `useTourReady` → 引擎会等满 3s 超时才显示气泡，体感卡顿。两者必须成对出现。

### 陷阱 7：误以为上一步会重新跳路由

`prev()` 仅回退气泡索引，**不重新跳路由、不重新执行 onEnterPage**。若向导需要"回退重定位"，应重新设计步骤划分，不要依赖 prev 复现定位。

### 陷阱 8：点遮罩关闭 = 跳过

`TourOverlay` 的 `visible` 写入 `false`（用户点遮罩）时等同于 `skip()`，会标记向导完成并结束。这是有意设计，告知用户即可。
