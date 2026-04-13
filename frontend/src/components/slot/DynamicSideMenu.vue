<script setup lang="ts">
import { computed, Ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import SideMenu from '@renderer/components/oneOff/SideMenu.vue'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import type { MenuSlotItem } from '@renderer/store/SlotRegistryStore'

const props = defineProps<{
  width?: string
  foldWidth?: string
}>()

const router = useRouter()
const route = useRoute()
const slotStore = useSlotRegistryStore()

interface MenuItem {
  slotId: string
  index: string
  name: string
  label: string
  icon: unknown
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
    background-color="black"
  >
    <template #default>
      <div v-if="menuItems.length === 0">菜单为空</div>
      <template v-for="item in menuItems" :key="item.slotId">
        <!-- 分组菜单（有子菜单） -->
        <el-sub-menu v-if="item.children.length > 0" :index="item.index">
          <template #title>
            <el-icon><component :is="item.icon" /></el-icon>
            <span>{{ item.label }}</span>
          </template>
          <el-menu-item v-for="child in item.children" :key="child.index" :index="child.index" @click="handleMenuClick(child)">
            <el-icon><component :is="child.icon" /></el-icon>
            <span>{{ child.label }}</span>
          </el-menu-item>
        </el-sub-menu>

        <!-- 单个菜单项 -->
        <el-menu-item v-else :index="item.index" @click="handleMenuClick(item)">
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ item.label }}</span>
        </el-menu-item>
      </template>
    </template>
  </side-menu>
</template>
