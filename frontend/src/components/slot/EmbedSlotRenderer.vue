<script setup lang="ts">
import { computed, defineAsyncComponent, h, type Component } from 'vue'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import { Loading } from '@element-plus/icons-vue'
import type { EmbedSlot } from '@renderer/model/slot'

const props = defineProps<{
  position: 'topbar' | 'statusbar' | 'toolbar'
}>()

const store = useSlotRegistryStore()

const slots = computed(() => store.embedSlotsByPosition(props.position))

// 加载中组件
const LoadingComponent: Component = {
  render() {
    return h(Loading, { class: 'is-loading', size: 16 })
  }
}

// 加载失败组件
const ErrorComponent: Component = {
  render() {
    return h('span', { style: 'color: var(--el-color-danger); font-size: 12px;' }, '加载失败')
  }
}

// 创建异步组件渲染器
const createSlotRenderer = (slot: EmbedSlot) => {
  return defineAsyncComponent({
    loader: () => slot.component(),
    loadingComponent: LoadingComponent,
    errorComponent: ErrorComponent,
    delay: 100,
    timeout: 5000
  })
}
</script>

<template>
  <div class="embed-slot-container">
    <template v-for="slot in slots" :key="slot.slotId">
      <component :is="createSlotRenderer(slot)" v-bind="slot.props" />
    </template>
  </div>
</template>

<style scoped>
.embed-slot-container {
  display: flex;
  align-items: center;
  height: 100%;
  gap: 4px;
}
</style>
