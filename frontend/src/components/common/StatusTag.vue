<script setup lang="ts">
import { computed, CSSProperties } from 'vue'
import { getStatusLabel } from '@renderer/constants/StatusRegistry.ts'

// props
const props = withDefaults(
  defineProps<{
    /** 状态 key，须与 StatusRegistry 及 tokens.css 的 --app-status-{key}-* 令牌一致 */
    status: string
    size?: 'small' | 'default'
  }>(),
  {
    size: 'default'
  }
)

// 变量
const label = computed(() => getStatusLabel(props.status))
// 通过 CSS 变量引用令牌：status key 即令牌后缀，组件无需为每个状态写专用 class
const tagStyle = computed<CSSProperties>(() => ({
  backgroundColor: `var(--app-status-${props.status}-bg)`,
  color: `var(--app-status-${props.status}-text)`,
  borderColor: `var(--app-status-${props.status}-border)`
}))
</script>

<template>
  <span
    class="status-tag"
    :class="{ 'status-tag--small': size === 'small' }"
    :style="tagStyle"
  >
    <slot>{{ label }}</slot>
  </span>
</template>

<style scoped>
.status-tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  height: 22px;
  padding: 0 8px;
  border: 1px solid transparent;
  border-radius: var(--app-radius-sm);
  font-size: 12px;
  line-height: 1;
  white-space: nowrap;
}
.status-tag--small {
  height: 18px;
  padding: 0 6px;
  font-size: 11px;
}
</style>
