<script setup lang="ts">
import { ref, onErrorCaptured, type Component } from 'vue'
import { WarningFilled } from '@element-plus/icons-vue'

/**
 * 插件故障隔离边界
 *
 * 捕获「由本边界直接渲染的子组件」在渲染/更新/生命周期中抛出的错误，
 * 阻断其向 <App> 冒泡，仅将出错子树降级为 fallback，保证主程序与其他插件不受影响。
 *
 * 可靠性关键：边界通过自身的 <component :is="component"> 直接挂载子组件，
 * 因此边界是该子组件真正的 parent，onErrorCaptured 能稳定触发。
 * （相比 <slot/> 模式，slot 内容归属于提供方，边界不在其 parent 链上，捕获不可靠。）
 */
const props = defineProps<{
  /** 出错子树标识（插件名/slotId/位置），用于 fallback 文案与日志定位 */
  name?: string
  /** 直接渲染的子组件定义（组件对象 / 异步组件） */
  component?: Component
  /** 透传给子组件的 props */
  componentProps?: Record<string, unknown>
}>()

const error = ref<unknown>(null)
// 重试计数：变化时强制重新挂载子组件
const retryKey = ref(0)

onErrorCaptured((err, _instance, info) => {
  error.value = err
  const e = err as Error | undefined
  // 经 setupConsoleForward 转发到 backend frontend.log（仅 dev）
  console.error('[PluginBoundary] 插件渲染失败', { name: props.name ?? '', info, msg: e?.message, stack: e?.stack })
  // 返回 false 阻止错误继续向父级冒泡，避免拖垮主程序
  return false
})

function handleRetry() {
  error.value = null
  retryKey.value++
}
</script>

<template>
  <!-- 出错：降级为 fallback，主程序继续运行 -->
  <div
    v-if="error"
    class="plugin-boundary-fallback"
    :key="`fallback-${retryKey}`"
  >
    <el-icon class="plugin-boundary-fallback-icon"><WarningFilled /></el-icon>
    <span class="plugin-boundary-fallback-text">
      插件渲染失败{{ name ? `：${name}` : '' }}
    </span>
    <el-link
      type="primary"
      underline="never"
      class="plugin-boundary-fallback-retry"
      @click="handleRetry"
    >
      重试
    </el-link>
  </div>
  <!-- 健康：直接渲染子组件（无包装节点，边界即其 parent，onErrorCaptured 稳定生效） -->
  <component
    :is="component"
    v-bind="componentProps"
    v-else
    :key="`ok-${retryKey}`"
  />
</template>

<style scoped>
.plugin-boundary-fallback {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 16px 8px;
  width: 100%;
  height: 100%;
  min-height: 64px;
  color: var(--app-text-secondary);
  font-size: 12px;
  text-align: center;
}

.plugin-boundary-fallback-icon {
  color: var(--app-color-danger);
  font-size: 22px;
}

.plugin-boundary-fallback-text {
  line-height: 1.4;
  word-break: break-all;
}

.plugin-boundary-fallback-retry {
  font-size: 12px;
}
</style>
