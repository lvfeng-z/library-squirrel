import { DefineComponent } from 'vue'

export interface DialogSlot {
  slotId: string
  component: () => Promise<DefineComponent>
  props?: Record<string, unknown>
  order?: number
}
