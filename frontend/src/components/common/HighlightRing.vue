<script setup lang="ts">
import { CSSProperties, onBeforeUnmount, ref, watch } from 'vue'

// 目标区域强调环：teleport 到 body 的 fixed 光环，围绕目标外缘呼吸数轮，供「跳转/定位到达后
// 强调某区块」场景使用。画在 body 层而非目标容器内，不受滚动容器 overflow 裁剪与相邻元素遮挡；
// rAF 逐帧同步目标 rect（scrollIntoView 平滑滚动期间光环跟随目标移动）。

interface Props {
  /** 强调目标元素；空值时不渲染光环（父级以此控制强调起止） */
  target?: HTMLElement | null
  /** 光环框与目标边缘的留白间距（px） */
  gap?: number
  /** 光环色调：状态 tone 键（active/done/fail/warn/pending/idle/source-local/source-site） */
  tone?: string
}

const props = withDefaults(defineProps<Props>(), {
  gap: 8,
  tone: 'warn'
})

const visible = ref(false)
const frameStyle = ref<CSSProperties>({})

let rafId: number | null = null

function syncFrame() {
  const el = props.target
  if (el) {
    const rect = el.getBoundingClientRect()
    frameStyle.value = {
      left: `${rect.left - props.gap}px`,
      top: `${rect.top - props.gap}px`,
      width: `${rect.width + props.gap * 2}px`,
      height: `${rect.height + props.gap * 2}px`
    }
  }
  rafId = requestAnimationFrame(syncFrame)
}

watch(() => props.target, (el) => {
  if (el) {
    visible.value = true
    if (rafId === null) syncFrame()
  } else {
    visible.value = false
    if (rafId !== null) {
      cancelAnimationFrame(rafId)
      rafId = null
    }
  }
}, { immediate: true })

onBeforeUnmount(() => {
  if (rafId !== null) cancelAnimationFrame(rafId)
})
</script>

<template>
  <teleport to="body">
    <div
      v-if="visible"
      class="highlight-ring z-layer-5"
      :style="[frameStyle, {
        '--highlight-ring-text': `var(--app-status-${props.tone}-text)`,
        '--highlight-ring-bg': `var(--app-status-${props.tone}-bg)`
      }]"
    />
  </teleport>
</template>

<style scoped>
/* 多层光环由内到外渐弱；opacity 呼吸动画（非 box-shadow 动画）避免逐帧重绘 */
.highlight-ring {
  position: fixed;
  pointer-events: none;
  border-radius: var(--app-radius);
  box-shadow:
    0 0 0 2px color-mix(in srgb, var(--highlight-ring-text) 80%, transparent),
    0 0 0 7px var(--highlight-ring-bg),
    0 0 22px 2px color-mix(in srgb, var(--highlight-ring-text) 45%, transparent);
  /* 呼吸 1.2s × 3 轮起止全透明；forwards 令播完停留透明态（环保留至父级撤 target 卸载，不跳回不透明基础态） */
  animation: highlight-ring-breathe 1.2s ease-in-out 3 forwards;
}

@keyframes highlight-ring-breathe {
  0%, 100% { opacity: 0; }
  50% { opacity: 1; }
}
</style>
