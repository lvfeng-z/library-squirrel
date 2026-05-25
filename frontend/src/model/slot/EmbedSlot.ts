import { DefineComponent } from 'vue'

export interface EmbedSlot {
  slotId: string
  position: 'topbar' | 'statusbar' | 'toolbar' | 'dialog'
  component: () => Promise<DefineComponent>
  props?: Record<string, unknown>
  order?: number
}
