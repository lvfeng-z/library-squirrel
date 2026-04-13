import { getCurrentInstance } from 'vue'
import type { Router, RouteRecordRaw } from 'vue-router'
import type { ViewSlot } from '@renderer/model/slot'

/**
 * 获取 Router 实例
 */
function getRouter(): Router | null {
  const instance = getCurrentInstance()
  if (instance?.appContext.config.globalProperties.$router) {
    return instance.appContext.config.globalProperties.$router
  }
  return (window as any).__vueRouter__
}

/**
 * 为插件添加路由
 */
export function addPluginRoute(viewSlot: ViewSlot): boolean {
  const router = getRouter()
  if (!router) {
    console.warn('[PluginRouter] Router not available')
    return false
  }

  // 检查路由是否已存在
  const exists = router.getRoutes().some((r) => r.name === viewSlot.slotId)
  if (exists) {
    console.warn(`[PluginRouter] Route ${viewSlot.slotId} already exists`)
    return false
  }

  // 添加路由到 MainLayout
  router.addRoute('MainLayout', {
    path: viewSlot.slotId,
    name: viewSlot.slotId,
    component: viewSlot.component,
    meta: {
      title: viewSlot.name,
      order: viewSlot.order ?? 100,
      isPlugin: true,
      pluginId: viewSlot.slotId
    }
  })

  console.log(`[PluginRouter] Added route: ${viewSlot.slotId}`)
  return true
}

/**
 * 移除插件路由
 */
export function removePluginRoute(viewSlotId: string): boolean {
  const router = getRouter()
  if (!router) return false

  const route = router.getRoutes().find((r) => r.name === viewSlotId)
  if (!route) return false

  // 检查是否是插件路由
  if (!route.meta?.isPlugin) {
    console.warn(`[PluginRouter] Route ${viewSlotId} is not a plugin route`)
    return false
  }

  router.removeRoute(viewSlotId)
  console.log(`[PluginRouter] Removed route: ${viewSlotId}`)
  return true
}

/**
 * 获取所有插件路由
 */
export function getPluginRoutes(): RouteRecordRaw[] {
  const router = getRouter()
  if (!router) return []

  return router.getRoutes().filter((r) => r.meta?.isPlugin)
}

/**
 * 检查路由是否为插件路由
 */
export function isPluginRoute(routeName: string): boolean {
  const router = getRouter()
  if (!router) return false

  const route = router.getRoutes().find((r) => r.name === routeName)
  return !!route?.meta?.isPlugin
}
