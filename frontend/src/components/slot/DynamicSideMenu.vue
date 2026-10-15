<script setup lang="ts">
import { computed, nextTick, ref, Ref, watch, type Component, type ComponentPublicInstance } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import SideMenu from '@renderer/components/oneOff/SideMenu.vue'
import AppIcon from '@renderer/components/common/AppIcon.vue'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import type { MenuSlotItem } from '@renderer/store/SlotRegistryStore'
import { useMenuBadgeStore } from '@renderer/store/UseMenuBadgeStore.ts'
import { isBlank } from '@renderer/utils/StringUtil.ts'
import { resolveAttentionTarget } from './menuAttention'

const props = defineProps<{
  width?: string
  foldWidth?: string
}>()

const router = useRouter()
const route = useRoute()
const slotStore = useSlotRegistryStore()
const menuBadgeStore = useMenuBadgeStore()

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

// 只从 menuSlots 生成菜单项
const menuItems: Ref<MenuItem[]> = computed(() => {
  const slots = slotStore.allMenuSlots
  return buildMenuItems(slots)
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
      <template
        v-for="item in menuItems"
        :key="item.slotId"
      >
        <!-- 分组菜单（有子菜单） -->
        <el-sub-menu
          v-if="item.children.length > 0"
          :index="item.index"
          :data-menu-slot="item.slotId"
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

        <!-- 单个菜单项 -->
        <el-menu-item
          v-else
          :index="item.index"
          :data-menu-slot="item.slotId"
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
    </template>
  </side-menu>
</template>

<style scoped>
/* 菜单红点（UseMenuBadgeStore 注册表驱动，任意菜单项可注册）：色取 fail tone（失败语义色槽），随主题调色变化 */
.menu-badge :deep(.el-badge__content) {
  background-color: var(--app-status-fail-text);
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
