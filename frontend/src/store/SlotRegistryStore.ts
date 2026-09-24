import { defineStore } from 'pinia'
import { markRaw, ref } from 'vue'
import type { Component } from 'vue'
import type { ViewSlot, EmbedSlot, DialogSlot, ReplaceViewSlot } from '@renderer/model/slot'
import type { RouteRecordRaw } from 'vue-router'
import type { Router } from 'vue-router'

// 菜单项类型
export interface MenuSlotItem {
  slotId: string
  index: string
  // 内置菜单为 Element Plus 图标组件对象，插件菜单为图片 URL 字符串
  icon?: Component | string
  label: string
  order?: number
  children?: MenuSlotItem[]
  // 如果是叶子菜单项，指向对应视图的路由名：内置菜单为裸路由名（如 'mainPage'），
  // 插件菜单在摄入层（useSlotSyncListener.convertToMenuSlot）已解析为复合路由名 publicId/viewExtId
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
  extensionId: string
  icon: string
}

// Router 实例管理
let routerInstance: Router | null = null

/**
 * 设置 Router 实例
 * 在应用启动时调用；接受 null 供测试在独立路由器上执行后清理模块级实例
 */
export function setRouterInstance(router: Router | null) {
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
    dialogSlots: new Map<string, DialogSlot>(),
    replaceViewSlots: new Map<string, ReplaceViewSlot>(),
    menuSlots: new Map<string, MenuSlotItem>(),
    siteBrowserSlots: new Map<string, SiteBrowserListSlotItem>(),
    activeViewId: ref<string | null>(null),
    // replaceView 覆盖前的原始路由（component + meta，供卸载恢复）
    originalRouteRecords: new Map<string, { component: NonNullable<RouteRecordRaw['component']>; meta?: RouteRecordRaw['meta'] }>(),
    // replaceViewSlot 注册次序（slotId → 单调递增序号）：同 target 多插件时的接管基准——
    // 最近启用（注册）者胜；某覆盖者注销后由剩余覆盖者中序号最大者接管
    replaceViewOrder: new Map<string, number>(),
    replaceViewSeq: 0
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
        return Array.from(state.embedSlots.values())
          .filter((slot) => slot.position === position)
          .sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
      },

    allDialogSlots: (state): DialogSlot[] => {
      return Array.from(state.dialogSlots.values()).sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
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
        // 如果当前在被删的路由，导航到首页强制清除已渲染页面
        if (routerInstance.currentRoute.value.name === id) {
          routerInstance.push('/')
        }
        routerInstance.removeRoute(id)
      }

      if (this.activeViewId === id) {
        this.activeViewId = null
      }
      this.viewSlots.delete(id)
    },

    // 注册嵌入插槽
    registerEmbedSlot(slot: EmbedSlot) {
      this.embedSlots.set(slot.slotId, slot)
    },

    // 取消注册嵌入插槽
    unregisterEmbedSlot(id: string) {
      this.embedSlots.delete(id)
    },

    // 注册弹窗插槽
    registerDialogSlot(slot: DialogSlot) {
      this.dialogSlots.set(slot.slotId, slot)
    },

    // 取消注册弹窗插槽
    unregisterDialogSlot(id: string) {
      this.dialogSlots.delete(id)
    },

    // 注册替换视图插槽（覆盖主程序已有路由）
    registerReplaceViewSlot(slot: ReplaceViewSlot) {
      this.replaceViewSlots.set(slot.slotId, slot)
      this.replaceViewOrder.set(slot.slotId, ++this.replaceViewSeq)
      this.applyReplaceViewToRoute(slot)
    },

    // 取消注册替换视图插槽：同 target 其余覆盖者中最近注册者接管；无剩余覆盖者时恢复
    // 主程序原路由（component 与 meta 均复原）
    unregisterReplaceViewSlot(slotId: string) {
      const slot = this.replaceViewSlots.get(slotId)
      this.replaceViewSlots.delete(slotId)
      this.replaceViewOrder.delete(slotId)
      if (!slot) return

      if (routerInstance) {
        // 剩余同 target 覆盖者按注册序取最近者
        const successor = Array.from(this.replaceViewSlots.entries())
          .filter(([, s]) => s.target === slot.target)
          .sort((a, b) => (this.replaceViewOrder.get(b[0]) ?? 0) - (this.replaceViewOrder.get(a[0]) ?? 0))[0]
        if (successor) {
          this.applyReplaceViewToRoute(successor[1])
        } else {
          const original = this.originalRouteRecords.get(slot.target)
          if (original) {
            // 恢复为 MainLayout children（与覆盖前一致，保留侧边菜单布局）
            routerInstance.addRoute('MainLayout', {
              name: slot.target,
              path: slot.target,
              component: original.component,
              meta: original.meta ? { ...original.meta } : undefined
            })
          }
          this.originalRouteRecords.delete(slot.target)
        }
        // 如果当前在被替换的路由，导航到首页强制刷新（清除插件组件缓存）
        if (routerInstance.currentRoute.value.name === slot.target) {
          routerInstance.push('/')
        }
      }
    },

    // 将替换视图写入路由表：覆盖 target 路由的 component（作为 MainLayout children，保留
    // 侧边菜单布局）；首次覆盖该 target 时记录原始路由供卸载恢复，后续覆盖者不覆盖该记录
    applyReplaceViewToRoute(slot: ReplaceViewSlot) {
      if (!routerInstance) return
      const existing = routerInstance.getRoutes().find((r) => r.name === slot.target)
      const existingComponent = existing?.components?.default
      if (!this.originalRouteRecords.has(slot.target) && existingComponent) {
        this.originalRouteRecords.set(slot.target, {
          // markRaw：组件对象不进响应式系统——pinia 深层代理会令恢复时 addRoute 拿到
          // 代理组件（引用同一性破坏，路由表持有代理也有无谓的开销）
          component: markRaw(existingComponent),
          meta: existing?.meta ? { ...existing.meta } : undefined
        })
      }
      routerInstance.addRoute('MainLayout', {
        name: slot.target,
        path: slot.target,
        component: slot.component,
        meta: { ...(existing?.meta || {}), isPlugin: true, replaced: true }
      })
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
      this.dialogSlots.clear()
      this.replaceViewSlots.clear()
      this.menuSlots.clear()
      this.siteBrowserSlots.clear()
      this.activeViewId = null
      this.originalRouteRecords.clear()
      this.replaceViewOrder.clear()
      this.replaceViewSeq = 0
    }
  }
})
