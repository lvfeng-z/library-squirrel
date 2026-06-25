<script setup lang="ts">
import { computed, defineAsyncComponent, h, type Component } from 'vue'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import { Loading } from '@element-plus/icons-vue'
import type { DialogSlot } from '@renderer/model/slot'

const store = useSlotRegistryStore()

const slots = computed(() => store.allDialogSlots)

const LoadingComponent: Component = {
  render() {
    return h(Loading, { class: 'is-loading', size: 16 })
  }
}

const ErrorComponent: Component = {
  render() {
    return h('span', { style: 'color: var(--app-color-danger); font-size: 12px;' }, '加载失败')
  }
}

const createSlotRenderer = (slot: DialogSlot) => {
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
  <template
    v-for="slot in slots"
    :key="slot.slotId"
  >
    <component
      :is="createSlotRenderer(slot)"
      v-bind="slot.props"
    />
  </template>
</template>
