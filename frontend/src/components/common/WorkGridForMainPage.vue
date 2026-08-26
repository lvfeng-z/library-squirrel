<script setup lang="ts">
import WorkDetailDialog from '../dialogs/WorkDetailDialog.vue'
import { computed, Ref, ref } from 'vue'
import WorkSetDialog from '@renderer/components/dialogs/WorkSetDialog.vue'
import CardGrid from '@renderer/components/common/CardGrid.vue'
import WorkCard from '@renderer/components/common/WorkCard.vue'
import {WorkFullDTO} from "@bindings/github.com/library-squirrel/backend/base/model/dto";
import WorkCardItem from '@renderer/model/dto/WorkCardItem.ts'
import { getWorkCardDimension } from '@renderer/utils/ImageDimension.ts'

// props
const props = withDefaults(
  defineProps<{
    workList: WorkFullDTO[]
    /** 是否可选中（主页接线由 MainView 传 true；false 时保持原网格纯浏览形态） */
    checkable?: boolean
    /** 选中的作品 id 列表（跨页保持，由上层选择集 store 提供，作勾选态初始化种子） */
    checkedWorkIds?: number[]
  }>(),
  {
    checkable: false,
    checkedWorkIds: () => []
  }
)

// model
const currentWorkIndex = defineModel<number>('currentWorkIndex', { required: true })

// emits
// workSetDeleted: 作品集已软删除（转发弹窗事件，供上层刷新作品集列表）
// checkedChange: 勾选变化（当前网格已加载项中的勾选 id 列表，跨页保持由上层选择集 store 合并）
const emits = defineEmits<{
  workSetDeleted: [id: number]
  checkedChange: [workIds: number[]]
}>()

// 变量
// workDialog开关
const workDialogState: Ref<boolean> = ref(false)
// workSetDialogState开关
const workSetDialogState: Ref<boolean> = ref(false)
// 当前作品集id
const currentWorkSetId: Ref<number> = ref(-1)
// 用于作品网格组件的WorkCardItem数组
const workCardItemList: Ref<WorkCardItem[]> = computed(() => props.workList.map((work) => new WorkCardItem(work)))

// 方法
function handleImageClicked(work: WorkCardItem) {
  currentWorkIndex.value = workCardItemList.value.indexOf(work)
  workDialogState.value = true
}
// 勾选变化上抛（跨页保持由上层选择集 store 合并）
function handleCheckedChange(workIds: number[]): void {
  emits('checkedChange', workIds)
}
// 打开作品集dialog
async function openWorkSetDialog(workSetId: number) {
  currentWorkSetId.value = workSetId
  workDialogState.value = false
  workSetDialogState.value = true
}
</script>

<template>
  <div>
    <card-grid
      :items="workCardItemList"
      :checkable="checkable"
      :checked-ids="checkedWorkIds"
      :get-id="(work: WorkCardItem) => work.id"
      :get-dimension="getWorkCardDimension"
      @checked-change="handleCheckedChange"
    >
      <template #card="{ item, checked, onUpdateChecked }">
        <work-card
          :checked="checked"
          :work="item"
          :max-height="500"
          :max-width="500"
          :checkable="checkable"
          work-info-popper-width="380px"
          author-info-popper-width="380px"
          @update:checked="onUpdateChecked"
          @image-clicked="handleImageClicked"
        />
      </template>
    </card-grid>
    <work-detail-dialog
      v-model:state="workDialogState"
      v-model:current-work-index="currentWorkIndex"
      :work="props.workList"
      width="90%"
      @open-work-set="openWorkSetDialog"
    />
    <work-set-dialog
      v-model:state="workSetDialogState"
      v-model:current-work-set-id="currentWorkSetId"
      width="90%"
      @work-set-deleted="emits('workSetDeleted', $event)"
    />
  </div>
</template>

<style scoped></style>
