<script setup lang="ts">
import { Ref, ref } from 'vue'
import WorkSetDialog from '@renderer/components/dialogs/WorkSetDialog.vue'
import CardGrid from '@renderer/components/common/CardGrid.vue'
import WorkSetCard from '@renderer/components/common/WorkSetCard.vue'
import { getWorkSetCardDimension } from '@renderer/utils/ImageDimension.ts'
import { WorkSetWithCoverDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'

// props
const props = defineProps<{
  workSetList: WorkSetWithCoverDTO[]
}>()

// emits
// workSetDeleted: 作品集已软删除（转发弹窗事件，供上层刷新作品集列表）
const emits = defineEmits<{
  workSetDeleted: [id: number]
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
</script>

<template>
  <div>
    <card-grid
      :items="props.workSetList"
      :checkable="false"
      :get-id="(workSet: WorkSetWithCoverDTO) => workSet.workSet?.id"
      :get-dimension="getWorkSetCardDimension"
    >
      <template #card="{ item, checked }">
        <work-set-card
          :checked="checked"
          :work-set="item"
          :max-height="500"
          :max-width="500"
          :checkable="false"
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
