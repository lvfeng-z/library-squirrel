import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ViewSlot, EmbedSlot, PanelSlot } from '@renderer/model/slot'
import type { RouteRecordRaw } from 'vue-router'
import type { Router } from 'vue-router'

// 菜单项类型
export interface MenuSlotItem {
  slotId: string
  index: string
  icon?: unknown
  label: string
  order?: number
  children?: MenuSlotItem[]
  // 如果是叶子菜单项，指向对应的视图
  viewId?: string
  // 如果是叶子菜单项，指向对应的页面状态
  pageStateKey?: string
}

// 站点浏览器列表插槽项类型
export interface SiteBrowserListSlotItem {
  slotId: string
  pluginId: number
  pluginPublicId: string
  name: string
  order?: number
  contributionId: string
  imagePath: string
}

// Router 实例管理
let routerInstance: Router | null = null

/**
 * 设置 Router 实例
 * 在应用启动时调用
 */
export function setRouterInstance(router: Router) {
  routerInstance = router
}

/**
 * 获取当前的 Router 实例
 */
export function getRouterInstance(): Router | null {
  return routerInstance
}

export const useSlotRegistryStore = defineStore('slotRegistry', {
  state: () => ({
    viewSlots: new Map<string, ViewSlot>(),
    embedSlots: new Map<string, EmbedSlot>(),
    panelSlots: new Map<string, PanelSlot>(),
    menuSlots: new Map<string, MenuSlotItem>(),
    siteBrowserSlots: new Map<string, SiteBrowserListSlotItem>(),
    activeViewId: ref<string | null>(null),
    // 视图替换状态
    replacedViewId: ref<string | null>(null)
  }),

  getters: {
    allViewSlots: (state): ViewSlot[] => {
      return Array.from(state.viewSlots.values()).sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
    },

    activeView: (state): ViewSlot | null => {
      if (!state.activeViewId) return null
      return state.viewSlots.get(state.activeViewId) || null
    },

    embedSlotsByPosition:
      (state) =>
      (position: string): EmbedSlot[] => {
        return Array.from(state.embedSlots.values()).filter((slot) => slot.position === position)
      },

    panelSlotsByPosition:
      (state) =>
      (position: string): PanelSlot[] => {
        return Array.from(state.panelSlots.values()).filter((slot) => slot.position === position)
      },

    // 获取所有菜单项（已排序）
    allMenuSlots: (state): MenuSlotItem[] => {
      return Array.from(state.menuSlots.values()).sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
    },

    // 获取所有站点浏览器列表插槽（已排序）
    allSiteBrowserSlots: (state): SiteBrowserListSlotItem[] => {
      return Array.from(state.siteBrowserSlots.values()).sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
    },

    // 获取用于菜单的路由配置
    routeConfigs(): RouteRecordRaw[] {
      const routes: RouteRecordRaw[] = []

      // 从 viewSlots 生成路由
      this.viewSlots.forEach((slot) => {
        routes.push({
          path: `/${slot.slotId}`,
          name: slot.slotId,
          component: slot.component,
          meta: {
            title: slot.name,
            order: slot.order ?? 100
          }
        })
      })

      return routes.sort((a, b) => ((a.meta?.order as number) ?? 100) - ((b.meta?.order as number) ?? 100))
    }
  },

  actions: {
    // 注册视图插槽
    registerViewSlot(slot: ViewSlot) {
      this.viewSlots.set(slot.slotId, slot)

      // 如果是插件视图且 router 可用，自动添加路由
      if (slot.isPlugin && routerInstance) {
        routerInstance.addRoute('MainLayout', {
          path: slot.slotId,
          name: slot.slotId,
          component: slot.component,
          meta: { title: slot.name, order: slot.order ?? 100, isPlugin: true }
        })
      }
    },

    // 注册视图插槽并同步到路由
    registerViewSlotWithRoute(slot: ViewSlot) {
      this.registerViewSlot(slot)

      // 如果提供了 router 实例，添加路由
      if (routerInstance) {
        routerInstance.addRoute('MainLayout', {
          path: slot.slotId,
          name: slot.slotId,
          component: slot.component,
          meta: { title: slot.name, order: slot.order ?? 100 }
        })
      }
    },

    // 取消注册视图插槽
    unregisterViewSlot(id: string) {
      const slot = this.viewSlots.get(id)
      // 如果是插件视图且 router 可用，自动移除路由
      if (slot?.isPlugin && routerInstance) {
        routerInstance.removeRoute(id)
      }

      if (this.activeViewId === id) {
        this.activeViewId = null
      }
      this.viewSlots.delete(id)
    },

    // 注册嵌入插槽
    // 注册嵌入插槽
    registerEmbedSlot(slot: EmbedSlot) {
      this.embedSlots.set(slot.slotId, slot)
    },

    // 取消注册嵌入插槽
    unregisterEmbedSlot(id: string) {
      this.embedSlots.delete(id)
    },

    // 注册面板插槽
    registerPanelSlot(slot: PanelSlot) {
      this.panelSlots.set(slot.slotId, slot)
    },

    // 取消注册面板插槽
    unregisterPanelSlot(id: string) {
      this.panelSlots.delete(id)
    },

    // 切换视图
    switchView(viewId: string): boolean {
      const slot = this.viewSlots.get(viewId)
      if (slot) {
        this.activeViewId = viewId
        return true
      }
      return false
    },

    // 清除当前视图
    clearActiveView() {
      this.activeViewId = null
    },

    // 替换视图 (面板插槽替换主程序页面)
    replaceView(panelSlotId: string, originalViewId: string) {
      // 记录被替换的视图ID
      this.replacedViewId = originalViewId
      // 切换到面板对应的视图
      this.switchView(panelSlotId)
    },

    // 恢复被替换的视图
    restoreView() {
      if (this.replacedViewId) {
        this.switchView(this.replacedViewId)
        this.replacedViewId = null
      }
    },

    // 注册菜单插槽
    registerMenuSlot(item: MenuSlotItem) {
      this.menuSlots.set(item.slotId, item)
    },

    // 批量注册菜单插槽
    registerMenuSlots(items: MenuSlotItem[]) {
      items.forEach((item) => {
        this.menuSlots.set(item.slotId, item)
      })
    },

    // 取消注册菜单插槽
    unregisterMenuSlot(id: string) {
      this.menuSlots.delete(id)
    },

    // 注册站点浏览器列表插槽
    registerSiteBrowserSlot(item: SiteBrowserListSlotItem) {
      this.siteBrowserSlots.set(item.slotId, item)
    },

    // 批量注册站点浏览器列表插槽
    registerSiteBrowserSlots(items: SiteBrowserListSlotItem[]) {
      items.forEach((item) => {
        this.siteBrowserSlots.set(item.slotId, item)
      })
    },

    // 取消注册站点浏览器列表插槽
    unregisterSiteBrowserSlot(id: string) {
      this.siteBrowserSlots.delete(id)
    },

    // 重置所有注册（用于测试或清理）
    reset() {
      this.viewSlots.clear()
      this.embedSlots.clear()
      this.panelSlots.clear()
      this.menuSlots.clear()
      this.siteBrowserSlots.clear()
      this.activeViewId = null
      this.replacedViewId = null
    }
  }
})
