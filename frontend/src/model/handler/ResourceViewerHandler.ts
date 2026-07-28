import { DefineComponent } from 'vue'

/**
 * 资源渲染器 handler：插件为某 resourceType 提供的自定义渲染器（被动响应型扩展）。
 * 与 EmbedSlot（主动注入型 slot）正交——主程序渲染该类型资源时按 resourceType 查找命中后调用，
 * 插件组件不占任何固定位置，而是被动被选中渲染。
 */
export interface ResourceViewerHandler {
  slotId: string
  resourceType: string // 资源类型查找键（前端按 resource.resourceType 匹配）
  component: () => Promise<DefineComponent> // 组件 loader（复用 loadCompiledComponent）
  props?: Record<string, unknown>
  order: number
}
