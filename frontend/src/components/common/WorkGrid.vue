<script setup lang="ts">
import WorkCard from './WorkCard.vue'
import { ref, watch, nextTick } from 'vue'
import WorkCardItem from '@renderer/model/model/dto/WorkCardItem.ts'

// props
const props = defineProps<{
  workList: WorkCardItem[]
  checkable: boolean
  checkedWorkIds?: number[] // 选中的作品id列表
  draggable?: boolean // 是否允许拖拽
  dragData?: (work: WorkCardItem) => unknown // 自定义拖拽携带的数据
  dragImage?: string // 自定义拖拽图标（图片URL）
}>()

// 事件
const emits = defineEmits([
  'imageClicked',
  'checkedChange',
  'dragStart', // 拖拽开始，payload: { work: WorkCardItem, data: unknown, event: DragEvent }
  'dragEnd', // 拖拽结束，payload: { work: WorkCardItem, event: DragEvent }
  'dragOver' // 拖拽到某个元素上，payload: { work: WorkCardItem, event: DragEvent }
])

// 响应式状态：存储每个作品的checked状态
const checkedStates = ref<Record<number, boolean>>({})
// 是否正在初始化，用于避免初始化时触发checkedChange事件
const isInitializing = ref(false)

// 初始化checked状态

// 初始化checkedStates
const initCheckedStates = () => {
  isInitializing.value = true
  const states: Record<number, boolean> = {}
  props.workList.forEach((work) => {
    if (work.id) {
      states[work.id] = props.checkedWorkIds?.includes(work.id) || false
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

// 监听props.workList变化
watch(
  () => props.workList,
  () => {
    initCheckedStates()
  },
  { immediate: true }
)

// 监听props.checkedWorkIds变化
watch(
  () => props.checkedWorkIds,
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
function handleImageClicked(workCardItem: WorkCardItem) {
  emits('imageClicked', workCardItem)
}

function updateCheckedState(id: number, value: boolean) {
  checkedStates.value[id] = value
}

// 拖拽相关方法
function handleDragStart(event: DragEvent, work: WorkCardItem) {
  console.log('[WorkGrid] dragStart called', { draggable: props.draggable, workId: work.id })
  if (!props.draggable || !work.id) {
    console.log('[WorkGrid] dragStart blocked', { draggable: props.draggable, workId: work.id })
    return
  }

  // 设置拖拽数据
  if (props.dragData) {
    const data = props.dragData(work)
    event.dataTransfer?.setData('application/json', JSON.stringify(data))
  }

  // 设置自定义拖拽图标
  if (props.dragImage) {
    const img = new Image()
    img.src = props.dragImage
    img.onload = () => {
      event.dataTransfer?.setDragImage(img, img.width / 2, img.height / 2)
    }
  }

  // 发出拖拽开始事件
  emits('dragStart', {
    work,
    data: props.dragData ? props.dragData(work) : undefined,
    event
  })
}

function handleDragEnd(event: DragEvent, work: WorkCardItem) {
  emits('dragEnd', { work, event })
}

function handleDragOver(event: DragEvent, work: WorkCardItem) {
  event.preventDefault() // 允许 drop
  emits('dragOver', { work, event })
}
</script>

<template>
  <div class="work-grid">
    <template v-for="work in props.workList" :key="work.id ? work.id : Math.random()">
      <div
        class="work-grid-container"
        :draggable="draggable && !!work.id"
        @dragstart="(event) => handleDragStart(event, work)"
        @dragend="(event) => handleDragEnd(event, work)"
        @dragover="(event) => handleDragOver(event, work)"
      >
        <work-card
          :checked="work.id ? checkedStates[work.id] : false"
          class="work-grid-work-card"
          :work="work"
          :max-height="500"
          :max-width="500"
          :checkable="checkable"
          work-info-popper-width="380px"
          author-info-popper-width="380px"
          @update:checked="(value) => work.id && updateCheckedState(work.id, value)"
          @image-clicked="handleImageClicked(work)"
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
