import { DefineComponent } from 'vue'

export interface PanelSlot {
  slotId: string
  position: 'left-sidebar' | 'right-sidebar' | 'bottom'
  width?: number // 面板宽度（仅 sidebar 生效）
  height?: number // 面板高度（仅 bottom 生效）
  component: () => Promise<DefineComponent>
  props?: Record<string, unknown>
  order?: number
}
