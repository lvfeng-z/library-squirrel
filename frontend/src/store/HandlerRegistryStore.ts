import { defineStore } from 'pinia'
import type { ResourceViewerHandler } from '@renderer/model/handler/ResourceViewerHandler'

/**
 * Handler 注册中心（被动响应型扩展）。
 * 与 SlotRegistryStore（主动注入型 slot）平级，统属于前端插件扩展点。
 * 当前仅承载 resourceViewer 一种 handler；未来其他被动响应型扩展（如自定义搜索渲染器）可在此加新桶。
 */
export const useHandlerRegistryStore = defineStore('handlerRegistry', {
  state: () => ({
    // key=slotId；查询时按 resourceType 过滤、order 升序取首（同 resourceType 多声明取 order 最小者，对应决策1）
    resourceViewerHandlers: new Map<string, ResourceViewerHandler>()
  }),

  getters: {
    // 按 resourceType 查渲染器：命中的 order 最小者；未命中返回 null（前端回落内置渲染器）
    resourceViewerByType:
      (state) =>
      (resourceType: string): ResourceViewerHandler | null => {
        const matched = Array.from(state.resourceViewerHandlers.values())
          .filter((h) => h.resourceType === resourceType)
          .sort((a, b) => a.order - b.order)
        return matched.length > 0 ? matched[0] : null
      }
  },

  actions: {
    // 注册资源渲染器（按 slotId 存，同 slotId 覆盖）
    registerResourceViewerHandler(handler: ResourceViewerHandler) {
      this.resourceViewerHandlers.set(handler.slotId, handler)
    },

    // 注销资源渲染器（按 slotId 删；同 resourceType 的其他 handler 因查询时重排序而自动接管）
    unregisterResourceViewerHandler(slotId: string) {
      this.resourceViewerHandlers.delete(slotId)
    }
  }
})
