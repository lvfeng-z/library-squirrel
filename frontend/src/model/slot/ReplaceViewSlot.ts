import { DefineComponent } from 'vue'

export interface ReplaceViewSlot {
  slotId: string
  target: string // 主程序路由 name（覆盖目标）
  component: () => Promise<DefineComponent>
  props?: Record<string, unknown>
}
