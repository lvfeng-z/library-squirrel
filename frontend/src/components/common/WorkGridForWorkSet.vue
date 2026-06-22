<script setup lang="ts">
import WorkDialog from '../dialogs/WorkDialog.vue'
import {computed, Ref, ref, watch} from 'vue'
import CardGrid from '@renderer/components/common/CardGrid.vue'
import WorkCard from '@renderer/components/common/WorkCard.vue'
import {WorkFullDTO} from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import WorkCardItem from '@renderer/model/dto/WorkCardItem.ts'
import { getWorkCardDimension } from '@renderer/utils/ImageDimension.ts'
import {reWorkWorkSetUpdateSortOrders} from '@renderer/apis/http/wrappers/reWorkWorkSet'

// props
const props = defineProps<{
  workList: WorkFullDTO[]
  checkable?: boolean
  checkedWorkIds?: number[]
}>()

// 事件
const emits = defineEmits(['checkedChange', 'workListSorted'])

// model
const currentWorkIndex = defineModel<number>('currentWorkIndex', { required: true })
// 当前作品集id
const currentWorkSetId = defineModel<number>('currentWorkSetId', { required: true })

// 变量
// workDialog开关
const workDialogState: Ref<boolean> = ref(false)
// 本地作品列表（用于拖拽排序）
const localWorkList: Ref<WorkFullDTO[]> = ref([...props.workList] as WorkFullDTO[])
// 监听 props.workList 变化，同步到本地
watch(
  () => props.workList,
  (newList) => {
    localWorkList.value = [...newList]
  },
  { deep: true }
)
// 用于作品网格组件的WorkCardItem数组
const workCardItemList: Ref<WorkCardItem[]> = computed(() => localWorkList.value.map((work) => new WorkCardItem(work)))

// 拖拽相关状态
// 被拖拽的元素索引
const draggedIndex = ref<number | null>(null)
// 拖拽目标元素索引
const dragOverIndex = ref<number | null>(null)

// 方法
function handleImageClicked(workCardItem: WorkCardItem) {
  // 如果在管理模式(checkable为true)，则切换勾选状态，不打开详情
  if (props.checkable) {
    const workId = workCardItem.id
    if (workId) {
      const currentChecked = props.checkedWorkIds?.includes(workId) || false
      // 触发选中状态变化
      const newCheckedIds = currentChecked
        ? props.checkedWorkIds?.filter((id) => id !== workId) || []
        : [...(props.checkedWorkIds || []), workId]
      emits('checkedChange', newCheckedIds)
    }
    return
  }
  // 非管理模式，打开作品详情
  currentWorkIndex.value = workCardItemList.value.indexOf(workCardItem)
  workDialogState.value = true
}
// 打开作品集dialog
async function openWorkSetDialog(workSetId: number) {
  currentWorkSetId.value = workSetId
  workDialogState.value = false
}

// 处理选中状态变化
function handleCheckedChange(checkedIds: number[]) {
  emits('checkedChange', checkedIds)
}

// 拖拽排序相关方法
function handleDragStart(payload: { item: WorkCardItem; data: unknown; event: DragEvent }) {
  const index = localWorkList.value.findIndex((w) => w.work?.id === payload.item.id)
  draggedIndex.value = index
  console.log('[WorkGridForWorkSet] dragStart', { index, work: payload.item })
}

function handleDragOver(payload: { item: WorkCardItem; event: DragEvent }) {
  dragOverIndex.value = localWorkList.value.findIndex((w) => w.work?.id === payload.item.id)
}

async function handleDragEnd() {
  console.log('[WorkGridForWorkSet] dragEnd', {
    draggedIndex: draggedIndex.value,
    dragOverIndex: dragOverIndex.value
  })
  if (draggedIndex.value === null || dragOverIndex.value === null) {
    resetDragState()
    return
  }
  // 如果位置发生变化，执行排序
  if (draggedIndex.value !== dragOverIndex.value) {
    const newList = [...localWorkList.value]
    const [removed] = newList.splice(draggedIndex.value, 1)
    newList.splice(dragOverIndex.value, 0, removed)
    localWorkList.value = newList
    emits('workListSorted', newList)

    // 保存排序到数据库
    const workSetId = currentWorkSetId.value
    if (workSetId) {
      const workIds = newList.map((work) => work.work?.id).filter((id): id is number => id !== undefined)
      try {
        await reWorkWorkSetUpdateSortOrders(workSetId, workIds)
        console.log('[WorkGridForWorkSet] sort order saved', { workSetId, workIds })
      } catch (error) {
        console.error('[WorkGridForWorkSet] failed to save sort order', error)
      }
    }
    console.log('[WorkGridForWorkSet] sorted', { newList })
  }
  resetDragState()
}

function resetDragState() {
  draggedIndex.value = null
  dragOverIndex.value = null
}
</script>

<template>
  <div>
    <card-grid
      :items="workCardItemList"
      :checkable="props.checkable ?? false"
      :checked-ids="props.checkedWorkIds"
      :draggable="true"
      :get-id="(work: WorkCardItem) => work.id"
      :get-dimension="getWorkCardDimension"
      @checked-change="handleCheckedChange"
      @drag-start="handleDragStart"
      @drag-end="handleDragEnd"
      @drag-over="handleDragOver"
    >
      <template #card="{ item, checked, onUpdateChecked }">
        <work-card
          :checked="checked"
          :work="item"
          :max-height="500"
          :max-width="500"
          :checkable="props.checkable ?? false"
          work-info-popper-width="380px"
          author-info-popper-width="380px"
          @update:checked="onUpdateChecked"
          @image-clicked="handleImageClicked"
        />
      </template>
    </card-grid>
    <work-dialog
      v-model:state="workDialogState"
      v-model:current-work-index="currentWorkIndex"
      :work="props.workList"
      width="90%"
      @open-work-set="openWorkSetDialog"
    />
  </div>
</template>

<style scoped></style>
