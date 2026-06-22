# 作品网格与作品集网格合并为通用 CardGrid 方案

## Context（背景与现状）

`WorkGrid.vue`（作品网格）与 `WorkSetGrid.vue`（作品集网格）是两个职责同构、代码高度重复的组件：

- **Grid 层重复度 ≈ 90%**：`checkedStates` 状态管理、`initCheckedStates`、三个 `watch`、`updateCheckedState`、`handleImageClicked` 逐字复制；模板结构与 `.work-grid-container` 样式几乎一致。
- **Card 层重复度 ≈ 80%**（`WorkCard` / `WorkSetCard`）：图像加载、单击延迟、双击打开、`error` 插槽、checkmark、`.work-card` / `.work-card-image` / `.work-card-info` 样式逐字复制。

两者的**实质差异**仅在三处：渲染的卡片组件不同（`WorkCard` vs `WorkSetCard`）、数据类型不同（`WorkCardItem[]` vs `WorkSetWithCoverDTO[]`）、ID 提取路径不同（`work.id` vs `workSet.workSet?.id`）；外加 `WorkGrid` 多一套拖拽逻辑、两者布局样式当前不一致（`WorkGrid` 已是瀑布流 `column-count`，`WorkSetGrid` 仍是普通 `grid`）。

### 合并动机

1. **消除重复**：选中态/布局/拖拽逻辑只维护一份，避免一处改、两处漏。
2. **与瀑布流方案（见 `作品网格高低图像展示方案.md`）协同**：A2 瀑布流升级若在两个组件上各写一遍，违背合并初衷；先抽出通用组件，A2 只需在单点实施，作品网格与作品集网格同时受益。
3. **观感对齐**：作品集网格封面高低不同时存在与作品网格相同的参差问题，合并后两者布局自动一致。

## 重构目标

提取一个**泛型通用网格组件 `CardGrid.vue`**，承载布局、选中态、拖拽三项与领域无关的能力；卡片内容通过**作用域插槽**由调用方注入。`WorkGrid` / `WorkSetGrid` 降为该通用组件的**薄封装**（固化各自卡片的 props，提供领域语义），使用方（`WorkGridForMainPage` 等）无需改动。

**不在本次范围**：
- 卡片层（`WorkCard` / `WorkSetCard`）的合并 —— info 区差异大（`WorkInfo`+`AuthorInfo` 组件 vs 纯文本名称），第一步只合并 Grid 层，收益最大、风险最小；卡片层是否进一步合并另开任务评估。
- A2 瀑布流（JS 绝对定位）的落地 —— 本方案只统一到当前 `WorkGrid` 已采用的 CSS `columns` 布局；A2 后续在 `CardGrid` 上实施。

## 通用组件 CardGrid 设计

位置：`frontend/src/components/common/CardGrid.vue`。
技术：Vue 3.3+ 泛型组件（`<script setup lang="ts" generic="T">`），项目 Vite 8 + Vue 3 组合满足。

### Props

```typescript
<script setup lang="ts" generic="T">
const props = defineProps<{
  items: T[]                                  // 数据列表
  checkable: boolean
  checkedIds?: number[]                       // 选中的 id 列表
  draggable?: boolean                         // 是否允许拖拽（作品集网格不传）
  getId: (item: T) => number | undefined      // 从 item 提取 id（领域差异隔离点）
  dragData?: (item: T) => unknown             // 拖拽携带数据
  dragImage?: string                          // 自定义拖拽图标
}>()
```

### Events

```typescript
const emits = defineEmits([
  'imageClicked',     // payload: T
  'checkedChange',    // payload: number[]
  'dragStart',        // payload: { item: T, data: unknown, event: DragEvent }
  'dragEnd',          // payload: { item: T, event: DragEvent }
  'dragOver'          // payload: { item: T, event: DragEvent }
])
```

### Slot

```vue
<template #card="{ item, checked }">
  <!-- 调用方注入具体卡片，自行绑定 checked / checkable / maxHeight 等 -->
</template>
```

插槽作用域只暴露 `item` 与 `checked`（`checkable` 已是 CardGrid 的 prop，卡片可直接读或由调用方透传）。

### 内部逻辑（从 WorkGrid.vue 迁移，把 `work.id` 替换为 `getId(item)`）

1. **选中态管理**：`checkedStates` + `initCheckedStates` + 三个 `watch`（`items` / `checkedIds` / `checkedStates`）+ `updateCheckedState`，逻辑与当前 `WorkGrid.vue:26-97` 完全一致，仅把 `work.id`、`props.workList`、`props.checkedWorkIds` 替换为 `getId(item)`、`props.items`、`props.checkedIds`。
2. **拖拽**：`handleDragStart` / `handleDragEnd` / `handleDragOver`（当前 `WorkGrid.vue:100-137`），通过 `props.draggable` 控制是否启用；作品集网格不传 `draggable`，拖拽相关 DOM 属性与监听不挂载。
3. **布局**：`.work-grid` 用 `column-count: 4; column-gap: 6px`；`.work-grid-container` 用 `width:100%; break-inside:avoid; margin-bottom:6px` + 容器 hover 样式（与当前 `WorkGrid.vue:170-191` 一致）。

### 模板骨架

```vue
<template>
  <div class="card-grid">
    <template
      v-for="(item, index) in props.items"
      :key="getId(item) ?? index"
    >
      <div
        class="card-grid-container"
        :draggable="draggable && !!getId(item)"
        @dragstart="(event) => handleDragStart(event, item)"
        @dragend="(event) => handleDragEnd(event, item)"
        @dragover="(event) => handleDragOver(event, item)"
      >
        <slot
          name="card"
          :item="item"
          :checked="getId(item) ? checkedStates[getId(item)!] : false"
        />
      </div>
    </template>
  </div>
</template>
```

> 类名由 `work-grid` / `work-grid-container` 改为 `card-grid` / `card-grid-container`，体现通用语义。

## 迁移方案

`WorkGrid` / `WorkSetGrid` 改为 `CardGrid` 的薄封装，固化各自卡片 props，保持领域语义与对外 API 不变。

### WorkGrid.vue（作品网格薄封装）

```vue
<script setup lang="ts">
import CardGrid from './CardGrid.vue'
import { WorkCardItem } from '@renderer/model/dto/WorkCardItem.ts'

const props = defineProps<{
  workList: WorkCardItem[]
  checkable: boolean
  checkedWorkIds?: number[]
  draggable?: boolean
  dragData?: (work: WorkCardItem) => unknown
  dragImage?: string
}>()
const emits = defineEmits(['imageClicked', 'checkedChange', 'dragStart', 'dragEnd', 'dragOver'])
</script>

<template>
  <card-grid
    :items="props.workList"
    :checkable="props.checkable"
    :checked-ids="props.checkedWorkIds"
    :draggable="props.draggable"
    :get-id="(w: WorkCardItem) => w.id"
    :drag-data="props.dragData"
    :drag-image="props.dragImage"
    @image-clicked="(w) => emits('imageClicked', w)"
    @checked-change="(ids) => emits('checkedChange', ids)"
    @drag-start="(p) => emits('dragStart', p)"
    @drag-end="(p) => emits('dragEnd', p)"
    @drag-over="(p) => emits('dragOver', p)"
  >
    <template #card="{ item, checked }">
      <work-card
        :checked="checked"
        :work="item"
        :max-height="500"
        :max-width="500"
        :checkable="props.checkable"
        work-info-popper-width="380px"
        author-info-popper-width="380px"
      />
    </template>
  </card-grid>
</template>
```

> 注：原 `WorkGrid` 内部直接监听 `WorkCard` 的 `@update:checked` / `@image-clicked`；合并后选中态由 `CardGrid` 通过插槽 `checked` 下发，`WorkCard` 的 `v-model:checked` 改由 `CardGrid` 在插槽层用 `update:checked` 事件回写——见下方「选中态回流」说明。

### WorkSetGrid.vue（作品集网格薄封装）

```vue
<card-grid
  :items="props.workSetList"
  :checkable="props.checkable"
  :checked-ids="props.checkedWorkSetIds"
  :get-id="(ws: WorkSetWithCoverDTO) => ws.workSet?.id"
  @image-clicked="(ws) => emits('imageClicked', ws)"
  @checked-change="(ids) => emits('checkedChange', ids)"
>
  <template #card="{ item, checked }">
    <work-set-card
      :checked="checked"
      :work-set="item"
      :max-height="500"
      :max-width="500"
      :checkable="props.checkable"
    />
  </template>
</card-grid>
```

> 作品集网格不传 `draggable`，拖拽自动关闭，DOM 上不挂 `draggable` 属性与拖拽监听。

### 选中态回流（关键设计点）

当前 `WorkCard` / `WorkSetCard` 通过 `defineModel<boolean>('checked')` 双向绑定。合并后，选中态的**唯一真源**是 `CardGrid` 内部的 `checkedStates`。插槽下发的是 `checked`（只读值），卡片内部 `@click` 改变 `checked` 时，需通过 `update:checked` 事件回写到 `CardGrid`。

两种实现方式（择一）：

- **方式 a（推荐，零额外 API）**：插槽同时下发 `checked` 与一个 `onUpdateChecked` 回调；卡片用 `:checked` + `@update:checked="onUpdateChecked"` 绑定。
  ```vue
  <slot
    name="card"
    :item="item"
    :checked="..."
    :on-update-checked="(v: boolean) => updateCheckedState(getId(item)!, v)"
  />
  ```
  薄封装里：`<work-card :checked="checked" @update:checked="onUpdateChecked" />`。
- **方式 b**：`CardGrid` 在插槽外层包一层 `v-model:checked` 代理组件——过度设计，不采用。

采用方式 a，插槽作用域增加 `onUpdateChecked`。

## 布局统一

`WorkSetGrid.vue` 当前的 `display: grid; grid-template-columns: repeat(4, 1fr)` 与容器默认背景色 `rgb(166.2, 168.6, 173.4, 10%)` 会被丢弃，统一到 `CardGrid` 的 `column-count: 4` 瀑布流布局与 `WorkGrid` 现有容器样式（白色背景 `#ffffff`，最新 `perf: 优化样式` 提交的版本）。

- **效果**：作品集封面高低不同时也获得瀑布流紧凑堆叠，与作品网格观感一致。
- **风险**：作品集卡片当前容器有浅灰背景，统一为白色后视觉略变；若产品上需要区分，可后续给 `CardGrid` 增加容器背景 prop。本次按「统一」处理。

## 影响面

| 文件 | 改造 |
|------|------|
| `CardGrid.vue`（新增） | 承载布局/选中/拖拽/插槽 |
| `WorkGrid.vue` | 重写为 `CardGrid` 薄封装，对外 props/events 不变 |
| `WorkSetGrid.vue` | 重写为 `CardGrid` 薄封装，对外 props/events 不变；布局由 grid 切到 columns |
| `WorkGridForMainPage.vue` / `WorkGridForWorkSet.vue` / `WorkSetGridForMainPage.vue` | **无需改动**（依赖的 props/events 不变） |
| `WorkCard.vue` / `WorkSetCard.vue` | **无需改动** |
| `MainView.vue` / `WorkQueryView.vue` | **无需改动** |

## 落地步骤

1. 新建 `frontend/src/components/common/CardGrid.vue`：迁移 `WorkGrid.vue` 的 `<script setup>`（替换为泛型 + `getId` + `items`/`checkedIds` 命名）与 `<style>`（类名改 `card-grid`），模板改为 `v-for + slot`。
2. 改造 `WorkGrid.vue` 为薄封装（见上文骨架），`task dev` 验证主页 / 查询视图 / 作品集详情的拖拽、勾选、点击、瀑布流均正常。
3. 改造 `WorkSetGrid.vue` 为薄封装，验证主页作品集网格选中、点击、瀑布流均正常。
4. 删除 `WorkGrid.vue` / `WorkSetGrid.vue` 中已迁移的冗余代码（`checkedStates` 等），确认两文件仅保留薄封装逻辑。
5. 全局回归：`MainView`（滚动加载）、`WorkQueryView`（加载更多）、`WorkSetDialog`（作品集内作品网格拖拽排序）三处场景目视通过。

## 风险与待决策

1. **选中态回流方式**：采用方式 a（插槽下发 `onUpdateChecked` 回调）。需确认 `WorkCard` / `WorkSetCard` 的 `defineModel('checked')` 在「父组件用 `:checked` + `@update:checked`」而非 `v-model:checked` 绑定时仍正常工作（`defineModel` 本质即这两个，等价，确认即可）。
2. **作品集容器背景色变更**：从浅灰统一为白色（与作品网格一致）。若希望保留区分，需在 `CardGrid` 增加容器背景 prop——本次按统一处理，**待确认**。
3. **泛型组件兼容性**：`<script setup generic="T">` 需 Vue 3.3+。落地前确认项目 Vue 版本（`frontend/package.json`）；若低于 3.3，回退为弱类型（`items: any[]` + `getId: (item: any) => number`），不阻塞合并。
4. **与 A2 瀑布流的衔接**：本方案落地后，A2 的 JS 瀑布流升级直接在 `CardGrid.vue` 内实施，作品网格与作品集网格同步获得「追加零闪烁」能力。A2 作为独立后续任务，不阻塞本方案。
5. **拖拽事件 payload 命名**：`CardGrid` 对外 emit 的拖拽 payload 字段由 `work` 改为 `item`（领域中立）。使用方 `WorkGridForWorkSet.vue:77-85` 中 `payload.work` 需相应改为 `payload.item`——属本方案连带改动，已在「影响面」之外的**连带改动**中识别，落地时一并修改。

### 连带改动（本方案必然触发）

- `WorkGridForWorkSet.vue`：拖拽回调 `payload.work` → `payload.item`（`WorkGridForWorkSet.vue:77、80、84` 等处）。因 `WorkGrid` 对外事件签名不变（仍 emit `dragStart` 等），仅 payload 内字段名从 `work` 改 `item`。
