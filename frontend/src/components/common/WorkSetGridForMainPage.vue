<script setup lang="ts">
import { Ref, ref } from 'vue'
import WorkSetDialog from '@renderer/components/dialogs/WorkSetDialog.vue'
import CardGrid from '@renderer/components/common/CardGrid.vue'
import WorkSetCard from '@renderer/components/common/WorkSetCard.vue'
import { getWorkSetCardDimension } from '@renderer/utils/ImageDimension.ts'
import { WorkSetWithCoverDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'

// props
const props = withDefaults(
  defineProps<{
    workSetList: WorkSetWithCoverDTO[]
    /** 是否可选中（主页接线由 MainView 传 true；false 时保持原网格纯浏览形态） */
    checkable?: boolean
    /** 选中的作品集 id 列表（跨页保持，由上层选择集 store 提供，作勾选态初始化种子） */
    checkedWorkSetIds?: number[]
  }>(),
  {
    checkable: false,
    checkedWorkSetIds: () => []
  }
)

// emits
// workSetDeleted: 作品集已软删除（转发弹窗事件，供上层刷新作品集列表）
// checkedChange: 勾选变化（当前网格已加载项中的勾选 id 列表，跨页保持由上层选择集 store 合并）
const emits = defineEmits<{
  workSetDeleted: [id: number]
  checkedChange: [workSetIds: number[]]
}>()

// model
const currentWorkSetIndex = defineModel<number>('current-work-set-index', { required: true })

// 变量
// workSetDialog开关
const workSetDialogState: Ref<boolean> = ref(false)
// 当前作品集id
const currentWorkSetId: Ref<number> = ref(-1)

// 方法
function handleImageClicked(workSet: WorkSetWithCoverDTO) {
  currentWorkSetIndex.value = props.workSetList.findIndex((ws) => ws.workSet?.id === workSet.workSet?.id)
  if (workSet.workSet?.id) {
    currentWorkSetId.value = workSet.workSet.id
    workSetDialogState.value = true
  }
}
// 勾选变化上抛（跨页保持由上层选择集 store 合并）
function handleCheckedChange(workSetIds: number[]): void {
  emits('checkedChange', workSetIds)
}
</script>

<template>
  <div>
    <card-grid
      :items="props.workSetList"
      :checkable="checkable"
      :checked-ids="checkedWorkSetIds"
      :get-id="(workSet: WorkSetWithCoverDTO) => workSet.workSet?.id"
      :get-dimension="getWorkSetCardDimension"
      @checked-change="handleCheckedChange"
    >
      <template #card="{ item, checked, onUpdateChecked }">
        <work-set-card
          :checked="checked"
          :work-set="item"
          :max-height="500"
          :max-width="500"
          :checkable="checkable"
          @update:checked="onUpdateChecked"
          @image-clicked="handleImageClicked"
        />
      </template>
    </card-grid>
    <work-set-dialog
      v-model:state="workSetDialogState"
      v-model:current-work-set-id="currentWorkSetId"
      width="90%"
      @work-set-deleted="emits('workSetDeleted', $event)"
    />
  </div>
</template>

<style scoped></style>
