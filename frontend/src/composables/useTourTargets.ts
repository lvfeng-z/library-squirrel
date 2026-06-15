import { onBeforeUnmount, type Ref } from 'vue'
import { registerTourTarget, unregisterTourTarget, resolveTourTarget } from '@renderer/store/UseTourCenterStore'

/**
 * 向导目标元素注册。
 *
 * 在页面 setup 中调用 register(key, ref) 注册可被高亮的元素；
 * 组件卸载时自动注销。
 *
 * @param key targetKey，命名约定为 {viewId}.{element}（如 'settings.workdirInput'）
 */
export function useTourTargets() {
  function register<T>(key: string, targetRef: Ref<T | undefined>) {
    const resolver = (): Element | null => {
      const v = targetRef.value as unknown as { $el?: Element } | Element | undefined
      if (!v) return null
      // 兼容组件实例（取 $el）与原生元素
      return ('$el' in v ? v.$el : v) ?? null
    }
    registerTourTarget(key, resolver)
    onBeforeUnmount(() => unregisterTourTarget(key))
  }

  return { register, resolve: resolveTourTarget }
}
