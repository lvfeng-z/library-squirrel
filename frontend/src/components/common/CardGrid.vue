<script setup lang="ts" generic="T">
import { ref, watch, nextTick, onMounted, onUnmounted } from 'vue'

// props
const props = defineProps<{
  items: T[] // 数据列表
  checkable: boolean
  checkedIds?: number[] // 选中的 id 列表
  draggable?: boolean // 是否允许拖拽
  getId: (item: T) => number | undefined | null // 从 item 提取 id（领域差异隔离点）
  getDimension?: (item: T) => { width: number; height: number } | undefined | null // 从 item 提取图像宽高（首屏精准布局）
  dragData?: (item: T) => unknown // 拖拽携带数据
  dragImage?: string // 自定义拖拽图标
}>()

// 事件
const emits = defineEmits([
  'checkedChange', // payload: number[]
  'dragStart', // payload: { item: T, data: unknown, event: DragEvent }
  'dragEnd', // payload: { item: T, event: DragEvent }
  'dragOver' // payload: { item: T, event: DragEvent }
])

// ===== 选中态 =====
const checkedStates = ref<Record<number, boolean>>({})
// 是否正在初始化，用于避免初始化时触发 checkedChange 事件
const isInitializing = ref(false)

const initCheckedStates = () => {
  isInitializing.value = true
  const states: Record<number, boolean> = {}
  props.items.forEach((item) => {
    const id = props.getId(item)
    if (id) {
      states[id] = props.checkedIds?.includes(id) || false
    }
  })

  const current = checkedStates.value
  if (JSON.stringify(current) !== JSON.stringify(states)) {
    checkedStates.value = states
  }

  nextTick(() => {
    isInitializing.value = false
  })
}

watch(
  () => props.items,
  () => {
    initCheckedStates()
  },
  { immediate: true }
)
watch(
  () => props.checkedIds,
  () => {
    initCheckedStates()
  },
  { deep: true }
)
watch(
  checkedStates,
  (newStates) => {
    if (isInitializing.value) {
      return
    }
    const checkedIdList = Object.entries(newStates)
      .filter(([, checked]) => checked)
      .map(([id]) => parseInt(id))
    emits('checkedChange', checkedIdList)
  },
  { deep: true }
)

function updateCheckedState(id: number, value: boolean) {
  checkedStates.value[id] = value
}

// ===== 拖拽 =====
function handleDragStart(event: DragEvent, item: T) {
  const id = props.getId(item)
  if (!props.draggable || !id) {
    return
  }

  if (props.dragData) {
    const data = props.dragData(item)
    event.dataTransfer?.setData('application/json', JSON.stringify(data))
  }

  if (props.dragImage) {
    const img = new Image()
    img.src = props.dragImage
    img.onload = () => {
      event.dataTransfer?.setDragImage(img, img.width / 2, img.height / 2)
    }
  }

  emits('dragStart', {
    item,
    data: props.dragData ? props.dragData(item) : undefined,
    event
  })
}

function handleDragEnd(event: DragEvent, item: T) {
  emits('dragEnd', { item, event })
}

function handleDragOver(event: DragEvent, item: T) {
  event.preventDefault() // 允许 drop
  emits('dragOver', { item, event })
}

// ===== 瀑布流布局（A2：绝对定位 + JS 计算） =====
const COLUMN_COUNT = 4
const GAP = 6
// 卡片高度预估常量（与 WorkCard 样式对齐）
const CARD_PADDING = 8 // .card-grid-container padding 4*2
const MAX_IMG_HEIGHT = 400 // WorkCard .work-card-image max-height = calc(500-100)
const INFO_HEIGHT = 60 // info 区（WorkInfo + AuthorInfo）估算，各卡同值不影响列均衡
const MIN_REAL_HEIGHT = 100 // offsetHeight ≥ 此值视为图片已 load
const DEFAULT_CARD_HEIGHT = 300 // 无宽高且未 load 时的默认估值

const containerRef = ref<HTMLElement>()
// 每列累计高度（下一个卡片在该列的 Y 坐标）
const columnHeights: number[] = new Array(COLUMN_COUNT).fill(0)
// 每个卡片的布局信息：所在列与 Y 坐标
const itemLayouts: { col: number; y: number }[] = []
// 每列包含的卡片索引（按 Y 顺序），用于单卡高度变化后重排同列后续
const columnItemIndices: number[][] = Array.from({ length: COLUMN_COUNT }, () => [])
// 上次布局完成时的 id 序列，用于判定增量追加
let lastItemIds: (number | undefined | null)[] = []
// 每个卡片上次记录的高度，用于判定真实高度变化
const itemLastHeights: number[] = []

let containerResizeObserver: ResizeObserver | null = null
let itemResizeObserver: ResizeObserver | null = null
let lastContainerWidth = 0
let containerResizeTimer: ReturnType<typeof setTimeout> | null = null
let layoutScheduled = false
// 卡片高度变化的待重排索引集合（rAF 批次内合并，避免每帧逐条重排）
const dirtyIndices = new Set<number>()
// 卡片高度变化的重排调度句柄（rAF 合并到下一帧，切断 item→container 跨 observer 同帧反馈）
let itemResizeRaf: ReturnType<typeof requestAnimationFrame> | null = null

function getColumnWidth(): number {
  if (!containerRef.value) {
    return 0
  }
  const containerWidth = containerRef.value.clientWidth
  return (containerWidth - (COLUMN_COUNT - 1) * GAP) / COLUMN_COUNT
}

function findMinColumn(): number {
  let minCol = 0
  for (let i = 1; i < COLUMN_COUNT; i++) {
    if (columnHeights[i] < columnHeights[minCol]) {
      minCol = i
    }
  }
  return minCol
}

function getCardElements(): HTMLElement[] {
  if (!containerRef.value) {
    return []
  }
  return Array.from(containerRef.value.children) as HTMLElement[]
}

// 预估卡片高度：优先用图像宽高按列宽等比计算（首屏精准），否则 fallback 到真实/默认高度
function estimateCardHeight(el: HTMLElement, index: number, colWidth: number): number {
  const dim = props.getDimension?.(props.items[index])
  if (dim && dim.width > 0 && dim.height > 0) {
    const innerWidth = colWidth - CARD_PADDING
    let imgHeight = (innerWidth * dim.height) / dim.width
    if (imgHeight > MAX_IMG_HEIGHT) {
      imgHeight = MAX_IMG_HEIGHT
    }
    return imgHeight + INFO_HEIGHT
  }
  return el.offsetHeight >= MIN_REAL_HEIGHT ? el.offsetHeight : DEFAULT_CARD_HEIGHT
}

// 定位单个卡片到最短列
function placeItem(el: HTMLElement, index: number, colWidth: number) {
  const col = findMinColumn()
  const x = col * (colWidth + GAP)
  const y = columnHeights[col]
  el.style.width = colWidth + 'px'
  el.style.transform = `translate(${x}px, ${y}px)`
  el.style.visibility = 'visible'
  itemLayouts[index] = { col, y }
  if (!columnItemIndices[col].includes(index)) {
    columnItemIndices[col].push(index)
  }
  const h = estimateCardHeight(el, index, colWidth)
  itemLastHeights[index] = h
  columnHeights[col] = y + h + GAP
}

// 全量重排：清空布局状态后重新定位所有卡片
function relayoutAll() {
  columnHeights.fill(0)
  columnItemIndices.forEach((arr) => {
    arr.length = 0
  })
  itemLayouts.length = 0
  const colWidth = getColumnWidth()
  const cards = getCardElements()
  cards.forEach((el, index) => placeItem(el, index, colWidth))
  observeAllItems()
  updateContainerHeight()
}

// 增量追加：仅定位新增卡片，已有卡片坐标不动（追加零闪烁核心）
function appendItems(startIdx: number) {
  const colWidth = getColumnWidth()
  const cards = getCardElements()
  for (let i = startIdx; i < cards.length; i++) {
    placeItem(cards[i], i, colWidth)
  }
  observeAllItems()
  updateContainerHeight()
}

// 单卡高度变化后，重排其所在列的后续卡片（单向往下推移）
function relayoutColumnFrom(index: number) {
  const layout = itemLayouts[index]
  if (!layout) {
    return
  }
  const col = layout.col
  const colIndices = columnItemIndices[col]
  const pos = colIndices.indexOf(index)
  if (pos === -1) {
    return
  }
  const colWidth = getColumnWidth()
  const x = col * (colWidth + GAP)
  const cards = getCardElements()
  for (let i = pos; i < colIndices.length; i++) {
    const itemIdx = colIndices[i]
    const el = cards[itemIdx]
    if (!el) {
      continue
    }
    const prevIdx = colIndices[i - 1]
    const y = i === 0 ? 0 : itemLayouts[prevIdx].y + (cards[prevIdx]?.offsetHeight ?? 0) + GAP
    el.style.transform = `translate(${x}px, ${y}px)`
    itemLayouts[itemIdx].y = y
    itemLastHeights[itemIdx] = el.offsetHeight
  }
  const lastIdx = colIndices[colIndices.length - 1]
  columnHeights[col] = itemLayouts[lastIdx].y + (cards[lastIdx]?.offsetHeight ?? 0) + GAP
  updateContainerHeight()
}

// 更新容器高度，撑开滚动区域
function updateContainerHeight() {
  if (!containerRef.value) {
    return
  }
  const maxHeight = Math.max(...columnHeights)
  containerRef.value.style.height = maxHeight + 'px'
}

// 判定增量/全量并执行布局
function doLayout() {
  const currentIds = props.items.map((item) => props.getId(item))
  let isAppend = false
  if (currentIds.length > lastItemIds.length) {
    isAppend = true
    for (let i = 0; i < lastItemIds.length; i++) {
      if (currentIds[i] !== lastItemIds[i]) {
        isAppend = false
        break
      }
    }
  }
  if (isAppend) {
    appendItems(lastItemIds.length)
  } else {
    relayoutAll()
  }
  lastItemIds = currentIds
}

// 合并多次触发到一次 nextTick 布局
function scheduleLayout() {
  if (layoutScheduled) {
    return
  }
  layoutScheduled = true
  nextTick(() => {
    layoutScheduled = false
    doLayout()
  })
}

// 容器宽度变化 → 全量重排（仅监听 width，避免 JS 设置 height 触发循环）
function setupContainerObserver() {
  if (!containerRef.value) {
    return
  }
  lastContainerWidth = containerRef.value.clientWidth
  containerResizeObserver = new ResizeObserver((entries) => {
    const width = entries[0]?.contentRect.width ?? 0
    if (width !== lastContainerWidth) {
      lastContainerWidth = width
      if (containerResizeTimer) {
        clearTimeout(containerResizeTimer)
      }
      containerResizeTimer = setTimeout(() => relayoutAll(), 100)
    }
  })
  containerResizeObserver.observe(containerRef.value)
}

// 卡片高度变化（图片 load / 文本换行）→ 同列后续重排
// 注意：回调内不得同步修改被 containerResizeObserver 观察的容器高度，否则 item→container
// 跨 observer 会形成同帧反馈环触发 ResizeObserver loop。故先收集脏索引，用 rAF 合并到下一帧执行
// （rAF 保证落至下一帧；nextTick 为 microtask 仍可能落在本帧渲染周期内，无法切断反馈）
function setupItemObserver() {
  itemResizeObserver = new ResizeObserver((entries) => {
    for (const entry of entries) {
      const el = entry.target as HTMLElement
      const index = Number(el.dataset.index)
      const newHeight = el.offsetHeight
      if (itemLastHeights[index] !== undefined && Math.abs(newHeight - itemLastHeights[index]) > 1) {
        dirtyIndices.add(index)
        if (itemResizeRaf === null) {
          itemResizeRaf = requestAnimationFrame(() => {
            itemResizeRaf = null
            dirtyIndices.forEach((idx) => relayoutColumnFrom(idx))
            dirtyIndices.clear()
          })
        }
      }
    }
  })
}

function observeAllItems() {
  if (!itemResizeObserver) {
    setupItemObserver()
  }
  const cards = getCardElements()
  cards.forEach((el) => itemResizeObserver!.observe(el))
}

// 监听 items 变化：引用（computed 新数组 / 整体替换）+ 长度（原地 push）
watch(
  () => props.items,
  () => scheduleLayout()
)
watch(
  () => props.items.length,
  () => scheduleLayout()
)

onMounted(() => {
  setupContainerObserver()
  nextTick(() => doLayout())
})

onUnmounted(() => {
  containerResizeObserver?.disconnect()
  itemResizeObserver?.disconnect()
  if (containerResizeTimer) {
    clearTimeout(containerResizeTimer)
  }
  if (itemResizeRaf !== null) {
    cancelAnimationFrame(itemResizeRaf)
    itemResizeRaf = null
  }
})
</script>

<template>
  <div
    ref="containerRef"
    class="card-grid"
  >
    <template
      v-for="(item, index) in props.items"
      :key="props.getId(item) ?? index"
    >
      <div
        class="card-grid-container"
        :data-index="index"
        :draggable="draggable && !!props.getId(item)"
        @dragstart="(event) => handleDragStart(event, item)"
        @dragend="(event) => handleDragEnd(event, item)"
        @dragover="(event) => handleDragOver(event, item)"
      >
        <slot
          name="card"
          :item="item"
          :checked="props.getId(item) ? checkedStates[props.getId(item) as number] : false"
          :on-update-checked="(value: boolean) => props.getId(item) && updateCheckedState(props.getId(item) as number, value)"
        />
      </div>
    </template>
  </div>
</template>

<style scoped>
.card-grid {
  position: relative;
  width: 100%;
}
.card-grid-container {
  position: absolute;
  top: 0;
  left: 0;
  /* width / transform / visibility 由 JS 设置 */
  box-sizing: border-box;
  overflow: hidden;
  padding: 4px;
  border-radius: var(--app-radius-lg);
  background-color: var(--app-bg-surface);
  visibility: hidden;
  /* 仅过渡背景与滤镜，不过渡 transform（避免首次/全量重排时卡片从原点飞散） */
  transition: background-color 0.3s ease, filter 0.3s ease;
}
.card-grid-container:hover {
  background-color: rgb(166.2, 168.6, 173.4, 30%);
  filter: drop-shadow(var(--app-shadow-card));
}
</style>
