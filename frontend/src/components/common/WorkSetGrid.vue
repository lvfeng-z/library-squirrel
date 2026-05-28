<script setup lang="ts">
import WorkSetCard from './WorkSetCard.vue'
import { ref, watch, nextTick } from 'vue'
import { WorkSetWithCoverDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-plugin-sdk/dto'

// props
const props = defineProps<{
  workSetList: WorkSetWithCoverDTO[]
  checkable: boolean
  checkedWorkSetIds?: number[] // 选中的作品集id列表
}>()

// 事件
const emits = defineEmits(['imageClicked', 'checkedChange'])

// 响应式状态：存储每个作品集的checked状态
const checkedStates = ref<Record<number, boolean>>({})
// 是否正在初始化，用于避免初始化时触发checkedChange事件
const isInitializing = ref(false)

// 初始化checkedStates
const initCheckedStates = () => {
  isInitializing.value = true
  const states: Record<number, boolean> = {}
  props.workSetList.forEach((workSet) => {
    if (workSet.workSet?.id) {
      states[workSet.workSet.id] = props.checkedWorkSetIds?.includes(workSet.workSet.id) || false
    }
  })

  // 只在状态实际变化时更新，避免不必要的触发
  const current = checkedStates.value
  const currentStr = JSON.stringify(current)
  const newStr = JSON.stringify(states)
  if (currentStr !== newStr) {
    checkedStates.value = states
  }

  // 使用nextTick确保在下一个事件循环中重置isInitializing
  nextTick(() => {
    isInitializing.value = false
  })
}

// 监听props.workSetList变化
watch(
  () => props.workSetList,
  () => {
    initCheckedStates()
  },
  { immediate: true }
)

// 监听props.checkedWorkSetIds变化
watch(
  () => props.checkedWorkSetIds,
  () => {
    initCheckedStates()
  },
  { deep: true }
)

// 监听checkedStates变化，发出事件
watch(
  checkedStates,
  (newStates) => {
    // 如果正在初始化，不发出事件
    if (isInitializing.value) {
      return
    }
    const checkedIds = Object.entries(newStates)
      .filter(([, checked]) => checked)
      .map(([id]) => parseInt(id))
    emits('checkedChange', checkedIds)
  },
  { deep: true }
)

// 方法
function handleImageClicked(workSetCoverDTO: WorkSetWithCoverDTO) {
  emits('imageClicked', workSetCoverDTO)
}

function updateCheckedState(id: number, value: boolean) {
  checkedStates.value[id] = value
}
</script>

<template>
  <div class="work-grid">
    <template v-for="workSet in props.workSetList" :key="workSet.workSet?.id ?? Math.random()">
      <div class="work-grid-container">
        <work-set-card
          :checked="workSet.workSet?.id ? checkedStates[workSet.workSet.id] : false"
          class="work-grid-work-card"
          :work-set="workSet"
          :max-height="500"
          :max-width="500"
          :checkable="checkable"
          @update:checked="(value) => workSet.workSet?.id && updateCheckedState(workSet.workSet.id, value)"
          @image-clicked="handleImageClicked(workSet)"
        />
      </div>
    </template>
  </div>
</template>

<style scoped>
.work-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 6px;
}
.work-grid-container {
  width: 100%;
  box-sizing: border-box;
  overflow: hidden;
  margin: 5px 0 0 0;
  padding: 4px;
  border-radius: 10px;
  background-color: rgb(166.2, 168.6, 173.4, 10%);
  transition-duration: 0.3s;
}
.work-grid-container:hover {
  background-color: rgb(166.2, 168.6, 173.4, 30%);
  filter: drop-shadow(0 0 10px rgba(0, 0, 0, 0.2));
}
.work-grid-work-card {
  height: 100%;
}
</style>
