<script setup lang="ts">
import { computed, Ref, type Component } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import SideMenu from '@renderer/components/oneOff/SideMenu.vue'
import AppIcon from '@renderer/components/common/AppIcon.vue'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import type { MenuSlotItem } from '@renderer/store/SlotRegistryStore'
import { useMenuBadgeStore } from '@renderer/store/UseMenuBadgeStore.ts'

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
  icon: Component | string
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

// 当前路由对应的菜单项 index（按 viewId 匹配），用于高亮当前页
// el-menu 仅在点击时内部更新高亮，程序化跳转（如向导）需通过 default-active 响应式驱动
const activeMenuIndex = computed(() => {
  const viewId = route.name as string | undefined
  if (!viewId) return undefined
  return findMenuIndexByViewId(menuItems.value, viewId)
})

// 递归查找 viewId 对应的菜单项 index
function findMenuIndexByViewId(items: MenuItem[], viewId: string): string | undefined {
  for (const item of items) {
    if (item.viewId === viewId) return item.index
    if (item.children?.length) {
      const matched = findMenuIndexByViewId(item.children, viewId)
      if (matched) return matched
    }
  }
  return undefined
}

// 处理菜单点击
function handleMenuClick(item: MenuItem) {
  if (item.viewId) {
    // 通过 viewId 跳转到视图
    router.push({ name: item.viewId })
  }
}
</script>

<template>
  <side-menu
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
</style>
