<script setup lang="ts">
import { computed, defineAsyncComponent, h, type Component } from 'vue'
import { useRouter } from 'vue-router'
import { useSlotRegistryStore } from '@renderer/store/SlotRegistryStore'
import { Loading } from '@element-plus/icons-vue'

const props = defineProps<{
  showHeader?: boolean
}>()

const router = useRouter()
const store = useSlotRegistryStore()

const activeView = computed(() => store.activeView)

// 加载中组件
const LoadingComponent: Component = {
  render() {
    return h('div', { style: 'display: flex; align-items: center; justify-content: center; height: 100%;' }, [
      h(Loading, { class: 'is-loading', size: 24 }),
      h('span', { style: 'margin-left: 8px;' }, '加载中...')
    ])
  }
}

// 加载失败组件
const ErrorComponent: Component = {
  render() {
    return h(
      'div',
      { style: 'display: flex; align-items: center; justify-content: center; height: 100%; color: var(--el-color-danger);' },
      [h('span', {}, '加载失败')]
    )
  }
}

const activeComponent = computed<Component | null>(() => {
  if (!activeView.value) return null

  return defineAsyncComponent({
    loader: activeView.value.component,
    loadingComponent: LoadingComponent,
    errorComponent: ErrorComponent,
    delay: 200,
    timeout: 10000
  })
})
</script>

<template>
  <div class="view-slot-container">
    <!-- 可选的标题栏 -->
    <div v-if="props.showHeader && activeView" class="view-slot-header">
      <span class="view-slot-title">{{ activeView.name }}</span>
    </div>
    <div class="view-slot-content">
      <component :is="activeComponent" v-if="activeComponent && activeView" v-bind="activeView.props" />
      <div v-else class="view-slot-empty">
        <el-empty description="请选择一个视图">
          <el-button type="primary" @click="router.push('/')">返回主页</el-button>
        </el-empty>
      </div>
    </div>
  </div>
</template>

<style scoped>
.view-slot-container {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.view-slot-header {
  display: flex;
  align-items: center;
  padding: 8px 16px;
  border-bottom: 1px solid var(--el-border-color);
}

.view-slot-title {
  font-size: 16px;
  font-weight: bold;
}

.view-slot-content {
  flex: 1;
  overflow: auto;
}

.view-slot-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}
</style>
