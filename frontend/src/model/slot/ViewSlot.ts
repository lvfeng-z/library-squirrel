import { AsyncComponentLoader, DefineComponent } from 'vue'

export interface ViewSlot {
  slotId: string // 唯一标识，如 "plugin-xxx-dashboard"
  name: string // 显示名称，如 "数据大屏"
  component: AsyncComponentLoader | (() => Promise<DefineComponent>) // 动态导入组件
  order?: number // 排序权重
  props?: Record<string, unknown> // 传递给组件的额外属性
  // 新增：是否为内置视图（区分插件视图）
  isBuiltin?: boolean
  // 新增：是否为插件视图
  isPlugin?: boolean
  // 新增：关闭时的回调
  onClose?: () => void | Promise<void>
}
