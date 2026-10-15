import { defineStore } from 'pinia'
import { markRaw, ref } from 'vue'
import type { Component } from 'vue'
import type { ViewSlot, EmbedSlot, DialogSlot, ReplaceViewSlot } from '@renderer/model/slot'
import type { RouteRecordRaw } from 'vue-router'
import type { Router } from 'vue-router'
import { isBlank } from '@renderer/utils/StringUtil.ts'

// 菜单项类型
export interface MenuSlotItem {
  slotId: string
  index: string
  // 内置菜单为 Element Plus 图标组件对象；插件菜单为图片 URL 字符串，
  // 或 ep: 宿主图标名引用（摄入层 resolvePluginIcon 已解析为组件）
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

// meta.parent 链上溯步数上限：宿主侧 routes.ts 静态声明成环属开发期笔误，截断防死循环
const MAX_PARENT_CHAIN_HOPS = 32

/**
 * 从复合 slotId（publicId/extensionId）取插件 publicId。
 * 两段均不含斜杠（publicId 为反向域名格式、extensionId 为 manifest 裸条目 id），
 * 按最后一个斜杠切分即得 publicId；无斜杠返回 undefined
 */
function pluginPublicIdOfSlotId(slotId: string): string | undefined {
  const separatorIndex = slotId.lastIndexOf('/')
  return separatorIndex > 0 ? slotId.slice(0, separatorIndex) : undefined
}

/**
 * 解析 view 插槽的 parent 声明值为最终路由名（纯查询、不告警）。
 * 两形态按确定顺序尝试：先同插件复合名（publicId + 裸 extensionId，与菜单 viewId 解析同约定，
 * 同名重叠时同插件指向胜出为确定行为），后宿主路由名；都不中返回 undefined
 */
function tryResolveParentRouteName(router: Router, slot: ViewSlot): string | undefined {
  const raw = slot.parent
  if (isBlank(raw)) return undefined
  const publicId = pluginPublicIdOfSlotId(slot.slotId)
  const compositeCandidate = publicId !== undefined ? `${publicId}/${raw}` : raw
  if (compositeCandidate !== raw && router.hasRoute(compositeCandidate)) return compositeCandidate
  return router.hasRoute(raw) ? raw : undefined
}

/**
 * 沿 meta.parent 链自 startName 向上收集祖先路由名序列（紧邻父级在前，不含 startName 自身）。
 * 消亡路由的记录移除后无法再沿其 meta 上溯，注销级联须在移除前先快照整条链再挑落点。
 * 环与超长链由 seen 集与步数上限截断，开发态告警
 */
function collectParentChain(router: Router, startName: string): string[] {
  const chain: string[] = []
  const seen = new Set<string>([startName])
  let cursor: string | undefined = startName
  while (chain.length < MAX_PARENT_CHAIN_HOPS) {
    const record = router.getRoutes().find((r) => r.name === cursor)
    const parentName = record?.meta?.parent
    if (isBlank(parentName)) break
    if (seen.has(parentName)) {
      console.warn(`[SlotRegistry] 页面 ${startName} 的 meta.parent 链在 "${parentName}" 处闭环，上溯已截断`)
      break
    }
    seen.add(parentName)
    chain.push(parentName)
    cursor = parentName
  }
  return chain
}

/**
 * 注销级联落点：祖先链上第一个不在本批消亡集且路由仍在场的名字；全灭返回 null（调用方落 '/'）。
 * 同批消亡祖先跳过：朴素跳父会跳进垂死路由（同批到达顺序不定，仅凭当下路由表
 * 会把同批稍后移除的祖先误判为幸存），宿主父页面必然存活
 */
function pickSurvivingAncestor(router: Router, chain: string[], doomed: Set<string>): string | null {
  for (const name of chain) {
    if (!doomed.has(name) && router.hasRoute(name)) return name
  }
  return null
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
    // 注册视图插槽。isSettingsDialog：弹窗呈现的设置页条目只入注册表不挂路由——
    // 宿主设置入口按复合键从注册表取组件加载器挂弹窗；侧栏菜单只从 menuSlots
    // 生成，无路由可跳不影响菜单
    registerViewSlot(slot: ViewSlot, isSettingsDialog = false) {
      this.viewSlots.set(slot.slotId, slot)

      // 如果是插件视图且 router 可用，自动添加路由
      if (slot.isPlugin && routerInstance && !isSettingsDialog) {
        const parentName = tryResolveParentRouteName(routerInstance, slot)
        if (!isBlank(slot.parent) && parentName === undefined) {
          // 层级保真度让位于页面可用性：页面照常注册，仅丢失层级
          console.warn(`[SlotRegistry] 插件页面 ${slot.slotId} 的父页面声明 "${slot.parent}" 无法解析（同插件复合名与宿主路由名均未命中），已按无层级页面注册`)
        }
        routerInstance.addRoute('MainLayout', {
          path: slot.slotId,
          name: slot.slotId,
          component: slot.component,
          meta: {
            title: slot.name,
            order: slot.order ?? 100,
            isPlugin: true,
            ...(parentName !== undefined ? { parent: parentName } : {})
          }
        })
        // 注册顺序不定（后端按 map 遍历推送扩展），同插件父页面可能晚于子页面到达：
        // 为注册时无法解析的同插件子页补检一次，命中即补写其路由 meta.parent
        this.reconcilePendingPluginParents(slot)
      }
    },

    // 同插件父页面晚到补写：注册时两形态均未命中、此刻同插件父页面已在场的子页面，
    // 将解析结果补写进其路由记录的 meta.parent（路由记录 meta 为可变对象，就地补写）
    reconcilePendingPluginParents(newlyRegistered: ViewSlot) {
      const router = routerInstance
      if (!router) return
      const publicId = pluginPublicIdOfSlotId(newlyRegistered.slotId)
      if (publicId === undefined) return
      this.viewSlots.forEach((slot) => {
        if (isBlank(slot.parent) || slot.slotId === newlyRegistered.slotId) return
        if (!slot.slotId.startsWith(`${publicId}/`)) return
        if (!router.hasRoute(slot.slotId)) return
        const record = router.getRoutes().find((r) => r.name === slot.slotId)
        if (record === undefined || record.meta?.parent !== undefined) return
        const resolved = tryResolveParentRouteName(router, slot)
        if (resolved !== undefined) {
          record.meta.parent = resolved
        }
      })
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

    // 取消注册视图插槽（单条）
    unregisterViewSlot(id: string) {
      return this.unregisterViewSlots([id])
    },

    // 批量取消注册视图插槽（插件停用/卸载时其后端注册中心整批注销，前端对应批量卸载事件）。
    // 正停在被删路由时沿 meta.parent 链上溯第一个仍存活的祖先（同批消亡祖先跳过），
    // 全灭落 '/'。祖先链在移除任何路由前快照：消亡路由记录移除后无法再沿其 meta 上溯。
    // 返回级联导航的 promise（未触发导航时为 undefined）——调用方在 reload 前须等它落定
    unregisterViewSlots(ids: string[]) {
      if (ids.length === 0) return
      const router = routerInstance
      // 仅插件视图持有动态路由；不存在或非插件的条目只做桶清理
      const routeIds = ids.filter((id) => this.viewSlots.get(id)?.isPlugin === true)
      const currentName = router?.currentRoute.value.name
      const isCurrentDying = router !== null && typeof currentName === 'string' && routeIds.includes(currentName)
      const chain = isCurrentDying && router !== null ? collectParentChain(router, currentName) : []
      routeIds.forEach((id) => {
        // 按在场判定移除：弹窗呈现条目注册时未挂路由，对不存在的路由 removeRoute 会在 dev 告警
        if (router?.hasRoute(id)) {
          router.removeRoute(id)
        }
      })
      let cascadeNav: ReturnType<Router['push']> | undefined
      if (isCurrentDying && router !== null) {
        const survivor = pickSurvivingAncestor(router, chain, new Set(routeIds))
        cascadeNav = router.push(survivor !== null ? { name: survivor } : '/')
      }
      ids.forEach((id) => {
        if (this.activeViewId === id) {
          this.activeViewId = null
        }
        this.viewSlots.delete(id)
      })
      return cascadeNav
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
