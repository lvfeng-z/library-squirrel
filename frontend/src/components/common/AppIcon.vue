<script setup lang="ts">
import { computed, type Component } from 'vue'
import { Picture } from '@element-plus/icons-vue'

/**
 * 自适应图标渲染（数据边界）
 *
 * 消除「插件数据直接喂给可抛异常的渲染 API」的隐患：
 * - 字符串 → 视为图片 URL，走 <el-image>，非法/加载失败的 URL 只显示兜底图标，绝不抛 createElement 异常
 * - 组件对象 → 走 <component :is>（内置菜单的 Element Plus 图标）
 *
 * 这样 <component :is> 永远拿不到脏字符串，从源头避免 InvalidCharacterError。
 */
const props = defineProps<{
  icon?: Component | string
}>()

const isImage = computed(() => typeof props.icon === 'string')
</script>

<template>
  <el-image
    v-if="isImage && icon"
    :src="icon as string"
    fit="contain"
    class="app-icon"
  >
    <template #error>
      <el-icon class="app-icon-fallback"><Picture /></el-icon>
    </template>
  </el-image>
  <component
    v-else-if="icon"
    :is="icon as Component"
  />
</template>

<style scoped>
/* 跟随外层 el-icon 的 font-size（1em），与 Element Plus 图标尺寸一致 */
.app-icon {
  width: 1em;
  height: 1em;
}

:deep(.app-icon img) {
  object-fit: contain;
}

.app-icon-fallback {
  width: 100%;
  height: 100%;
  color: var(--app-text-secondary);
}
</style>
