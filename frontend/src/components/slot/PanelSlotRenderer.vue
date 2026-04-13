<script setup lang="ts">
import { computed, defineAsyncComponent, h, type Component } from 'vue'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import { Loading } from '@element-plus/icons-vue'
import type { PanelSlot } from '@renderer/model/slot'

const props = defineProps<{
  position: 'left-sidebar' | 'right-sidebar' | 'bottom'
}>()

const store = useSlotRegistryStore()

const slots = computed(() => store.panelSlotsByPosition(props.position))

// 加载中组件
const LoadingComponent: Component = {
  render() {
    return h('div', { style: 'display: flex; align-items: center; justify-content: center; height: 100%;' }, [
      h(Loading, { class: 'is-loading', size: 24 }),
      h('span', { style: 'margin-left: 8px; font-size: 12px;' }, '加载中...')
    ])
  }
}

// 加载失败组件
const ErrorComponent: Component = {
  render() {
    return h(
      'div',
      {
        style:
          'display: flex; align-items: center; justify-content: center; height: 100%; color: var(--el-color-danger); font-size: 12px;'
      },
      [h('span', {}, '加载失败')]
    )
  }
}

// 创建异步组件渲染器
const createSlotRenderer = (slot: PanelSlot) => {
  return defineAsyncComponent({
    loader: () => slot.component(),
    loadingComponent: LoadingComponent,
    errorComponent: ErrorComponent,
    delay: 100,
    timeout: 10000
  })
}

// 获取槽位样式
function getSlotStyle(slot: PanelSlot): Record<string, string> {
  const style: Record<string, string> = {}
  if (slot.width && (props.position === 'left-sidebar' || props.position === 'right-sidebar')) {
    style.width = `${slot.width}px`
  }
  if (slot.height && props.position === 'bottom') {
    style.height = `${slot.height}px`
  }
  return style
}
</script>

<template>
  <div class="panel-slot-container" :class="`panel-slot-${position}`">
    <template v-for="slot in slots" :key="slot.slotId">
      <div class="panel-slot-item" :style="getSlotStyle(slot)">
        <component :is="createSlotRenderer(slot)" v-bind="slot.props" />
      </div>
    </template>
  </div>
</template>

<style scoped>
.panel-slot-container {
  display: flex;
  overflow: hidden;
}

.panel-slot-left-sidebar {
  flex-direction: column;
  border-right: 1px solid var(--el-border-color);
}

.panel-slot-right-sidebar {
  flex-direction: column;
  border-left: 1px solid var(--el-border-color);
}

.panel-slot-bottom {
  flex-direction: row;
  border-top: 1px solid var(--el-border-color);
}

.panel-slot-item {
  overflow: auto;
  flex-shrink: 0;
}
</style>
