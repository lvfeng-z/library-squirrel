<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, Ref, watch, type Component, type ComponentPublicInstance } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import SideMenu from '@renderer/components/oneOff/SideMenu.vue'
import AppIcon from '@renderer/components/common/AppIcon.vue'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import type { MenuSlotItem } from '@renderer/store/SlotRegistryStore'
import { useMenuBadgeStore } from '@renderer/store/UseMenuBadgeStore.ts'
import { useMenuOrderStore, reorderMenusByUserOrder } from '@renderer/store/UseMenuOrderStore.ts'
import { isBlank } from '@renderer/utils/StringUtil.ts'
import { isNullish } from '@renderer/utils/CommonUtil.ts'
import { resolveAttentionTarget } from './menuAttention'

const props = defineProps<{
  width?: string
  foldWidth?: string
}>()

const router = useRouter()
const route = useRoute()
const slotStore = useSlotRegistryStore()
const menuBadgeStore = useMenuBadgeStore()
const menuOrderStore = useMenuOrderStore()

interface MenuItem {
  slotId: string
  index: string
  name: string
  label: string
  // 内置菜单为 Element Plus 图标组件对象，插件菜单为图片 URL 字符串
  icon?: Component | string
  order: number
  isGroup: boolean
  children: MenuItem[]
  // 插槽菜单关联的视图ID
  viewId?: string
}

// 从插槽菜单配置生成菜单项
function buildMenuItems(menuSlots: MenuSlotItem[]): MenuItem[] {
  const result: MenuItem[] = []

  menuSlots.forEach((menu) => {
    const item: MenuItem = {
      slotId: menu.slotId,
      index: menu.index,
      name: menu.slotId,
      label: menu.label,
      icon: menu.icon,
      order: menu.order ?? 100,
      isGroup: false,
      viewId: menu.viewId,
      children: []
    }

    // 递归处理子菜单
    if (menu.children?.length) {
      item.children = buildMenuItems(menu.children)
      item.isGroup = item.children.length > 0
    }

    result.push(item)
  })

  return result.sort((a, b) => a.order - b.order)
}

// 只从 menuSlots 生成菜单项：静态排序后再按用户手动排序快照合成一级显示顺序（分组子项保持静态递归序，不参与用户排序）
const menuItems: Ref<MenuItem[]> = computed(() => {
  const slots = slotStore.allMenuSlots
  return reorderMenusByUserOrder(buildMenuItems(slots))
})

const activeIndex = computed(() => route.path)

// 当前路由对应的菜单项 index（按 viewId 匹配），用于高亮当前页：
// 精确命中优先，未中沿路由父链上溯（二级页面驻留保持「所在分区」祖先项选中）。
// el-menu 仅在点击时内部更新高亮，程序化跳转（如向导）需通过 default-active 响应式驱动
const activeMenuIndex = computed(() => {
  const target = resolveAttentionTarget(menuItems.value, route.name as string | undefined, parentRouteNameOf)
  return target?.index
})

// 路由名 → 其父页面路由名（读路由表 meta.parent，仅查询不导航），供定位解析上溯
function parentRouteNameOf(name: string): string | undefined {
  const record = router.getRoutes().find((r) => r.name === name)
  const parent = record?.meta?.parent
  return isBlank(parent) ? undefined : parent
}

// —— 跳转定位提示：程序化跳转落定后，滚动目标菜单项入视并做一次性脉冲 ——

// 菜单项自身点击引发的跳转不提示（目标就在指针下，提示无信息量）
let isSelfNavigation = false

// 侧栏壳实例：取根元素做容器隐藏判定与目标项查询
const sideMenuRef = ref<ComponentPublicInstance | null>(null)

const ATTENTION_PULSE_CLASS = 'menu-attention-pulse'
// 脉冲两闪总时长（约 0.45s/闪）；animationend 丢失时按此加缓冲兜底移除类
const ATTENTION_PULSE_DURATION_MS = 900
const ATTENTION_PULSE_FALLBACK_MS = ATTENTION_PULSE_DURATION_MS + 600

// 以 route.name 变化粒度触发（纯 query 变化不重提示）；初次加载不提示（immediate 缺省 +
// 初次导航的 name 自缺省落定也跳过——外壳挂载先于初始导航落定，该次变化不属跳转）
watch(
  () => route.name,
  (_newName, oldName) => {
    if (oldName === undefined) return
    if (isSelfNavigation) {
      isSelfNavigation = false
      return
    }
    // 时序：路由落定 → nextTick（菜单项 DOM 更新）→ rAF（布局就绪）→ 解析定位
    void nextTick().then(() =>
      requestAnimationFrame(() => {
        drawAttentionToRoute()
      })
    )
  }
)

// 解析当前路由的定位目标并滚动 + 脉冲；无命中、容器不在场或隐藏时静默
function drawAttentionToRoute() {
  const container = sideMenuRef.value?.$el
  if (!container || !(container instanceof HTMLElement)) return
  if (isMenuContainerHidden(container)) return
  const target = resolveAttentionTarget(menuItems.value, route.name as string | undefined, parentRouteNameOf)
  if (!target) return
  const element = container.querySelector<HTMLElement>(`[data-menu-slot="${escapeAttributeValue(target.slotId)}"]`)
  if (!element) return
  element.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  pulseOnce(element)
}

// 侧栏隐藏收起把容器宽度压到 0（可见式隐藏，checkVisibility 探不到），以 offsetWidth 探宽；
// 无布局引擎的环境（checkVisibility 缺失）不据尺寸判隐藏
function isMenuContainerHidden(container: HTMLElement): boolean {
  if (typeof container.checkVisibility !== 'function') return false
  if (container.offsetWidth === 0) return true
  return !container.checkVisibility()
}

// attribute 选择器值转义：slotId（含点/斜杠的复合键）只需引号与反斜杠字面转义
function escapeAttributeValue(value: string): string {
  return value.replace(/["\\]/g, '\\$&')
}

// 一次性脉冲：animationend 即移除类，不残留状态；动画事件不触发（如无动画引擎环境）按定时兜底移除。
// 新脉冲开始前先终止上一次未完的收尾，避免旧定时器中途拆除新一轮脉冲的类
const pulseFinishers = new WeakMap<HTMLElement, () => void>()

function pulseOnce(element: HTMLElement) {
  pulseFinishers.get(element)?.()
  element.classList.add(ATTENTION_PULSE_CLASS)
  let done = false
  const finish = () => {
    if (done) return
    done = true
    window.clearTimeout(fallbackTimer)
    element.removeEventListener('animationend', finish)
    element.classList.remove(ATTENTION_PULSE_CLASS)
    pulseFinishers.delete(element)
  }
  const fallbackTimer = window.setTimeout(finish, ATTENTION_PULSE_FALLBACK_MS)
  element.addEventListener('animationend', finish, { once: true })
  pulseFinishers.set(element, finish)
}

// 处理菜单点击
function handleMenuClick(item: MenuItem) {
  if (item.viewId) {
    // 通过 viewId 跳转到视图
    isSelfNavigation = true
    // 同名路由重复导航（点当前页菜单项）或导航失败时 route.name 不变、watch 不触发，
    // push 落定后无条件清标志，避免残留误抑制下一次程序化跳转
    void router.push({ name: item.viewId }).finally(() => {
      isSelfNavigation = false
    })
  }
}

// —— 菜单一级条目手动排序（HTML5 拖放，展开与折叠（仅图标）态共用同一条链路）——

// 拖拽临时态：源项 slotId（半透明反馈）与悬停目标的插入位（目标项上/下半区）
const draggingSlotId = ref<string | null>(null)
const dropIndicator = ref<{ slotId: string; before: boolean } | null>(null)

// —— 拖拽中边缘自动滚动 ——
// HTML5 原生拖拽进行中滚轮事件被拖拽循环吞掉，el-scrollbar 也不会随指针自动滚动；
// 指针处于侧栏滚动容器上/下边缘带内时由 rAF 循环按方向持续滚动。滚动步进按
// 最后已知指针位计算，不依赖 dragover 连发驱动（Chromium 拖拽中约每 350ms 才补发一次）。

// 边缘带宽：指针距滚动视口上/下缘不超过该距离即开始滚动（px）
const DRAG_AUTOSCROLL_EDGE_PX = 36
// 贴缘最深处的单帧滚动步进（px）；带内随深入程度线性衰减，最小 1px/帧
const DRAG_AUTOSCROLL_STEP_PX = 8

let dragScrollHost: HTMLElement | null = null
let dragPointerX: number | null = null
let dragPointerY: number | null = null
let dragScrollRAFId = 0

function clearDragState() {
  draggingSlotId.value = null
  dropIndicator.value = null
  stopDragAutoScroll()
}

// window 级 dragover 冒泡捕获指针最新位置：条目间隙、侧栏以外等不触发条目 dragover 的区域也持续刷新
function trackDragPointer(event: DragEvent) {
  dragPointerX = event.clientX
  dragPointerY = event.clientY
}

// 从源条目上溯侧栏滚动容器（el-scrollbar 的滚动承载元素）并启动滚动循环；未命中容器则每帧空转、不产生滚动
function startDragAutoScroll(event: DragEvent) {
  const host = event.currentTarget
  dragScrollHost = host instanceof HTMLElement ? host.closest('.el-scrollbar__wrap') : null
  dragPointerX = event.clientX
  dragPointerY = event.clientY
  window.addEventListener('dragover', trackDragPointer)
  if (dragScrollRAFId !== 0) return
  const tick = () => {
    dragScrollRAFId = requestAnimationFrame(tick)
    stepDragAutoScroll()
  }
  dragScrollRAFId = requestAnimationFrame(tick)
}

function stepDragAutoScroll() {
  const host = dragScrollHost
  if (isNullish(host) || isNullish(dragPointerX) || isNullish(dragPointerY)) return
  const rect = host.getBoundingClientRect()
  // 指针横向离开侧栏滚动区（悬于内容区等）时不滚动，仅纵向对齐边缘带不足以触发
  if (dragPointerX < rect.left - DRAG_AUTOSCROLL_EDGE_PX || dragPointerX > rect.right + DRAG_AUTOSCROLL_EDGE_PX) {
    return
  }
  // 指针深入上/下边缘带的程度：正值即在带内（含越过容器缘），越大滚得越快
  const depthTop = rect.top + DRAG_AUTOSCROLL_EDGE_PX - dragPointerY
  const depthBottom = dragPointerY - (rect.bottom - DRAG_AUTOSCROLL_EDGE_PX)
  if (depthTop > 0) {
    host.scrollTop -= autoScrollStep(depthTop)
  } else if (depthBottom > 0) {
    host.scrollTop += autoScrollStep(depthBottom)
  }
}

function autoScrollStep(depth: number): number {
  const ratio = Math.min(1, depth / DRAG_AUTOSCROLL_EDGE_PX)
  return Math.max(1, Math.round(DRAG_AUTOSCROLL_STEP_PX * ratio))
}

// 拖拽结束（drop/dragend）与组件卸载两条中断路径都经此终止滚动循环与指针监听
function stopDragAutoScroll() {
  window.removeEventListener('dragover', trackDragPointer)
  if (dragScrollRAFId !== 0) {
    cancelAnimationFrame(dragScrollRAFId)
    dragScrollRAFId = 0
  }
  dragScrollHost = null
  dragPointerX = null
  dragPointerY = null
}

onBeforeUnmount(stopDragAutoScroll)

// 一级条目拖拽视觉反馈类：源项半透明；悬停目标按插入方向画指示线
function topLevelDragClass(item: MenuItem): string[] {
  const classes: string[] = []
  if (draggingSlotId.value === item.slotId) classes.push('menu-drag-source')
  const indicator = dropIndicator.value
  if (indicator?.slotId === item.slotId) {
    classes.push(indicator.before ? 'menu-drop-before' : 'menu-drop-after')
  }
  return classes
}

// 指针是否落在条目上半区（上半=插到目标之前，下半=插到目标之后）
function isPointerAboveMiddle(element: HTMLElement, event: DragEvent): boolean {
  const rect = element.getBoundingClientRect()
  return event.clientY < rect.top + rect.height / 2
}

function handleDragStart(item: MenuItem, event: DragEvent) {
  draggingSlotId.value = item.slotId
  dropIndicator.value = null
  // 写入拖拽数据与效果类型：部分浏览器（如 Firefox）要求 dragstart 内写入数据才启动拖动
  event.dataTransfer?.setData('text/plain', item.slotId)
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
  startDragAutoScroll(event)
}

function handleDragOver(item: MenuItem, event: DragEvent) {
  // 只响应本菜单发起的排序拖动（外部内容拖入不拦截）；悬停源项自身不设落点
  if (isNullish(draggingSlotId.value) || draggingSlotId.value === item.slotId) return
  // 允许落点（不 preventDefault 的目标浏览器按不可放置处理，drop 不触发）
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  const host = event.currentTarget
  if (host instanceof HTMLElement) {
    dropIndicator.value = { slotId: item.slotId, before: isPointerAboveMiddle(host, event) }
  }
}

async function handleDrop(item: MenuItem, event: DragEvent) {
  event.preventDefault()
  const fromSlotId = draggingSlotId.value
  clearDragState()
  if (isNullish(fromSlotId) || fromSlotId === item.slotId) return
  const host = event.currentTarget
  if (!(host instanceof HTMLElement)) return
  // 落定基线取当前显示顺序的全量一级 slotId 表
  const currentIds = menuItems.value.map((entry) => entry.slotId)
  const fromIndex = currentIds.indexOf(fromSlotId)
  const targetIndex = currentIds.indexOf(item.slotId)
  if (fromIndex < 0 || targetIndex < 0) return
  // 插入位折算成落定语义：先按目标前/后取移除前下标，源项原本在插入位之前时补偿移除造成的位移
  const insertAt = targetIndex + (isPointerAboveMiddle(host, event) ? 0 : 1)
  const toIndex = fromIndex < insertAt ? insertAt - 1 : insertAt
  if (toIndex === fromIndex) return
  try {
    // 本地快照同步写入（列表随即按新序渲染并播放落定位移动画）；持久化失败抛错由这里提示，会话内序保留
    await menuOrderStore.commitMove(currentIds, fromIndex, toIndex)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '菜单顺序保存失败')
  }
}

function handleDragEnd() {
  clearDragState()
}
</script>

<template>
  <side-menu
    ref="sideMenuRef"
    :width="props.width || '160px'"
    :fold-width="props.foldWidth || '64px'"
    :default-active="[activeIndex]"
    :active-index="activeMenuIndex"
    background-color="black"
  >
    <template #default>
      <transition-group name="menu-order-flip">
        <template
          v-for="item in menuItems"
          :key="item.slotId"
        >
          <!-- 分组菜单（有子菜单）：一级层参与手动排序拖拽 -->
          <el-sub-menu
            v-if="item.children.length > 0"
            :key="item.slotId"
            :index="item.index"
            :data-menu-slot="item.slotId"
            :draggable="true"
            :class="topLevelDragClass(item)"
            @dragstart="handleDragStart(item, $event)"
            @dragover="handleDragOver(item, $event)"
            @drop="handleDrop(item, $event)"
            @dragend="handleDragEnd"
          >
            <template #title>
              <el-badge
                :value="menuBadgeStore.badgeOf(item.slotId)"
                :max="99"
                :offset="[-2, 20]"
                :hidden="menuBadgeStore.badgeOf(item.slotId) === 0"
                class="menu-badge"
              >
                <el-icon><AppIcon :icon="item.icon" /></el-icon>
              </el-badge>
              <span>{{ item.label }}</span>
            </template>
            <el-menu-item
              v-for="child in item.children"
              :key="child.index"
              :index="child.index"
              :data-menu-slot="child.slotId"
              @click="handleMenuClick(child)"
            >
              <el-badge
                :value="menuBadgeStore.badgeOf(child.slotId)"
                :max="99"
                :offset="[-2, 20]"
                :hidden="menuBadgeStore.badgeOf(child.slotId) === 0"
                class="menu-badge"
              >
                <el-icon><AppIcon :icon="child.icon" /></el-icon>
              </el-badge>
              <span>{{ child.label }}</span>
            </el-menu-item>
          </el-sub-menu>

          <!-- 单个菜单项：一级层参与手动排序拖拽 -->
          <el-menu-item
            v-else
            :key="item.slotId"
            :index="item.index"
            :data-menu-slot="item.slotId"
            :draggable="true"
            :class="topLevelDragClass(item)"
            @dragstart="handleDragStart(item, $event)"
            @dragover="handleDragOver(item, $event)"
            @drop="handleDrop(item, $event)"
            @dragend="handleDragEnd"
            @click="handleMenuClick(item)"
          >
            <el-badge
              :value="menuBadgeStore.badgeOf(item.slotId)"
              :max="99"
              :offset="[-2, 20]"
              :hidden="menuBadgeStore.badgeOf(item.slotId) === 0"
              class="menu-badge"
            >
              <el-icon><AppIcon :icon="item.icon" /></el-icon>
            </el-badge>
            <span>{{ item.label }}</span>
          </el-menu-item>
        </template>
      </transition-group>
    </template>
  </side-menu>
</template>

<style scoped>
/* 菜单红点（UseMenuBadgeStore 注册表驱动，任意菜单项可注册）：色取 fail tone（失败语义色槽），随主题调色变化 */
.menu-badge :deep(.el-badge__content) {
  background-color: var(--app-status-fail-text);
}

/* —— 一级条目手动排序反馈 —— */
/* 拖动源项半透明 */
.menu-drag-source {
  opacity: 0.4;
}
/* 悬停目标插入位指示线：内阴影画线不挤占条目布局，色随主题主色 */
.menu-drop-before {
  box-shadow: inset 0 2px 0 0 var(--app-color-primary);
}
.menu-drop-after {
  box-shadow: inset 0 -2px 0 0 var(--app-color-primary);
}
/* 折叠态（el-menu--collapse）条目窄、仅图标：插入指示线加粗一档保持醒目 */
.el-menu--collapse .menu-drop-before {
  box-shadow: inset 0 3px 0 0 var(--app-color-primary);
}
.el-menu--collapse .menu-drop-after {
  box-shadow: inset 0 -3px 0 0 var(--app-color-primary);
}
/* 拖拽落定后一级条目的 FLIP 位移动画（TransitionGroup move 类） */
.menu-order-flip-move {
  transition: transform 0.3s ease;
}

/* 跳转定位脉冲：背景色在透明与主题主色浅档间两闪（约 0.45s/闪），动画结束自动移除类不残留 */
.menu-attention-pulse {
  animation: menu-attention-flash 0.45s ease-in-out 2;
}
@keyframes menu-attention-flash {
  0%,
  100% {
    background-color: transparent;
  }
  50% {
    background-color: var(--app-color-primary-light-8);
  }
}
</style>
