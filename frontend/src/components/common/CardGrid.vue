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
const INFO_HEIGHT = 54 // info 区（WorkInfo + AuthorInfo）估算，各卡同值不影响列均衡
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
// 动画窗口标志：容器宽度防抖重排的 0.3s 过渡进行中为真，卡片高度观察者在此期间挂起
let layoutAnimating = false
// 动画窗口关闭计时器：0.3s 过渡 + 50ms 余量的固定计时（不依赖 transitionend，该事件在新旧值相同或元素不可见时不触发）
let animTeardownTimer: ReturnType<typeof setTimeout> | null = null
// 关窗计时的启动帧句柄：大卡量批量样式写入使首帧推迟，计时自下一帧（样式提交、过渡可见起跑）起算，
// 避免在过渡尾声提前拆窗
let animTeardownRaf: ReturnType<typeof requestAnimationFrame> | null = null
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
  // 宽度/坐标一律直写终值：窗口期是否过渡由卡片所挂类决定（全员 transform 过渡、视口内卡
  // 另含宽度过渡），窗口外无类即瞬切；高度不显式写入，由内容随宽度自然撑开，
  // 与宽度过渡逐帧同步、同缓动收尾
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
  // 动画窗口内重排：先完成视口判定读批（可见卡挂宽度过渡类），随后的写入循环才逐卡改样式
  if (layoutAnimating) {
    markViewportCards()
  }
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
  // 动画窗口与数据变化重叠时先拆窗再布局：数据变化路径的定位保持直写、无过渡
  if (layoutAnimating) {
    endAnimWindow()
  }
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

// ===== 宽度重排过渡动画窗口 =====
// 动画窗口只在容器宽度防抖到期时开启：网格根挂 animating 类后全量重排，坐标走 transform
// 过渡（合成器路径近零成本，全员启用）；宽度过渡仅限视口内卡（判定后另挂类）——内容随宽度
// 逐帧重排，宽度、位置与内容高度以同值同缓动自然同步起止，列内几何过渡期保持一致；视口外卡
// 宽度瞬切终值（不可见无需动画，免全量逐帧重排）。首次布局、增量追加、数据变化不经过该入口，
// 定位直写无过渡。

// 视口判定外扩边距：可见带上下各外扩此距离，近缘与即将滚入的卡一并获得宽度过渡
const VIEWPORT_MARGIN = 200
// 视口内卡在窗口期挂的类：命中下方 CSS 的宽度过渡规则
const IN_VIEWPORT_CLASS = 'in-viewport'
// 窗口期内挂过视口类的卡片账目（关窗时逐个摘类）
let widthAnimatedEls: HTMLElement[] = []

// 网格根的最近滚动祖先：自 containerRef 父级向上取 overflowY 为 auto/scroll/overlay 的元素。
// body/html 不参与判定：二者命中时盒矩形覆盖整个文档，无法充当可见带，此情形以视口为准
function findScrollAncestor(): HTMLElement | null {
  let node = containerRef.value?.parentElement ?? null
  while (node) {
    if (node !== document.body && node !== document.documentElement) {
      const overflowY = getComputedStyle(node).overflowY
      if (overflowY === 'auto' || overflowY === 'scroll' || overflowY === 'overlay') {
        return node
      }
    }
    node = node.parentElement
  }
  return null
}

// 当前可见带的纵向区间（与 getBoundingClientRect 同处视口坐标系）：有滚动祖先取其盒矩形，
// 没有则回退浏览器视口（0 至根元素所见高度）；区间无效（根未挂载/高度退化）返回 null
function getViewportBand(): { top: number; bottom: number } | null {
  const scroller = findScrollAncestor()
  if (scroller) {
    const rect = scroller.getBoundingClientRect()
    if (rect.bottom > rect.top) {
      return { top: rect.top, bottom: rect.bottom }
    }
    return null
  }
  return window.innerHeight > 0 ? { top: 0, bottom: window.innerHeight } : null
}

// 开窗读批：全部卡片矩形一次读完，视口内（含外扩边距）者随后统一挂视口类并入账，
// 读写分离避免逐卡读写交错强拍布局；可见带无效时保守回退为全员挂类——
// 宁可全局宽度过渡，也不静默丢动画
function markViewportCards() {
  const cards = getCardElements()
  const band = getViewportBand()
  const visibleFlags = cards.map((el) => {
    if (!band) {
      return true
    }
    const rect = el.getBoundingClientRect()
    return rect.bottom >= band.top - VIEWPORT_MARGIN && rect.top <= band.bottom + VIEWPORT_MARGIN
  })
  cards.forEach((el, index) => {
    // 窗口内重排重复判定：已挂类的卡不重复入账
    if (visibleFlags[index] && !el.classList.contains(IN_VIEWPORT_CLASS)) {
      el.classList.add(IN_VIEWPORT_CLASS)
      widthAnimatedEls.push(el)
    }
  })
}

function beginAnimWindow() {
  layoutAnimating = true
  containerRef.value?.classList.add('animating')
  // 清旧计新：窗口自最后一次可见起跑后 350ms 关闭，过渡向最新目标平滑重定向；
  // 计时启动挂在下一帧（样式提交后），首帧因批量写入推迟时不再提前剪断过渡
  if (animTeardownRaf !== null) {
    cancelAnimationFrame(animTeardownRaf)
  }
  if (animTeardownTimer) {
    clearTimeout(animTeardownTimer)
  }
  animTeardownRaf = requestAnimationFrame(() => {
    animTeardownRaf = null
    if (!layoutAnimating) {
      return
    }
    animTeardownTimer = setTimeout(endAnimWindow, 350)
  })
  // 开窗后紧接全量重排，窗前排队的高度修正批次作废
  if (itemResizeRaf !== null) {
    cancelAnimationFrame(itemResizeRaf)
    itemResizeRaf = null
  }
  dirtyIndices.clear()
}

function endAnimWindow() {
  layoutAnimating = false
  containerRef.value?.classList.remove('animating')
  // 摘除视口卡的宽度过渡类并清账：关窗后宽度写入回归瞬切，残留类会让下一窗口的
  // 视口外卡错误地带上宽度过渡
  widthAnimatedEls.forEach((el) => el.classList.remove(IN_VIEWPORT_CLASS))
  widthAnimatedEls = []
  if (animTeardownRaf !== null) {
    cancelAnimationFrame(animTeardownRaf)
    animTeardownRaf = null
  }
  if (animTeardownTimer) {
    clearTimeout(animTeardownTimer)
    animTeardownTimer = null
  }
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
      containerResizeTimer = setTimeout(() => {
        beginAnimWindow()
        relayoutAll()
      }, 100)
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
    // 动画窗口内视口卡的宽度逐帧过渡、高度随之逐帧变化，属过渡中间态，不做同列重排
    if (layoutAnimating) {
      return
    }
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
  if (animTeardownRaf !== null) {
    cancelAnimationFrame(animTeardownRaf)
    animTeardownRaf = null
  }
  if (animTeardownTimer) {
    clearTimeout(animTeardownTimer)
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
  /* width / transform / visibility 由 JS 设置，高度恒由内容随宽度自然撑开 */
  box-sizing: border-box;
  overflow: hidden;
  padding: 4px;
  border-radius: var(--app-radius-lg);
  background-color: var(--app-bg-surface);
  visibility: hidden;
  /* 常态仅过渡背景与滤镜，transform/width 直写瞬移；宽度重排的过渡由下方动画窗口规则提供 */
  transition: background-color 0.3s ease, filter 0.3s ease;
}
.card-grid-container:hover {
  background-color: rgb(166.2, 168.6, 173.4, 30%);
  filter: drop-shadow(var(--app-shadow-card));
}
/* 动画窗口（容器宽度防抖重排期间，网格根挂 animating 类）：全员卡位置走 transform 过渡
    （合成器路径近零成本）；时值缓动与侧边栏过渡一致；超出目标盒高的内容以 overflow hidden
    裁切渐显；窗口外该规则不命中，首次布局/追加/数据刷新的定位无过渡 */
.card-grid.animating .card-grid-container {
  transition: transform 0.3s ease,
    background-color 0.3s ease, filter 0.3s ease;
}
/* 视口内卡（开窗时 JS 判定后挂 in-viewport 类）追加宽度过渡：宽度渐变驱动内容逐帧重排，
    宽度、位置与内容高度以同值同缓动同步起止，列内间距过渡期保持一致；视口外卡不命中，
    宽度瞬切终值（不可见无需动画，免全量卡量逐帧重排） */
.card-grid.animating .card-grid-container.in-viewport {
  transition: transform 0.3s ease,
    width 0.3s ease, background-color 0.3s ease, filter 0.3s ease;
}
</style>
