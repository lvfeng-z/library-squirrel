<script setup lang="ts">
import { Ref, ref } from 'vue'
import WorkSetDialog from '@renderer/components/dialogs/WorkSetDialog.vue'
import WorkSetGrid from '@renderer/components/common/WorkSetGrid.vue'
import WorkSetCoverDTO from '@renderer/model/model/dto/WorkSetCoverDTO.ts'

// props
const props = defineProps<{
  workSetList: WorkSetCoverDTO[]
}>()

// model
const currentWorkSetIndex = defineModel<number>('current-work-set-index', { required: true })

// 变量
// workSetDialog开关
const workSetDialogState: Ref<boolean> = ref(false)
// 当前作品集id
const currentWorkSetId: Ref<number> = ref(-1)

// 方法
function handleImageClicked(workSet: WorkSetCoverDTO) {
  currentWorkSetIndex.value = props.workSetList.findIndex((ws) => ws.id === workSet.id)
  if (workSet.id) {
    currentWorkSetId.value = workSet.id
    workSetDialogState.value = true
  }
}
</script>

<template>
  <div>
    <work-set-grid :work-set-list="props.workSetList" :checkable="false" @image-clicked="handleImageClicked"></work-set-grid>
    <work-set-dialog v-model:state="workSetDialogState" v-model:current-work-set-id="currentWorkSetId" width="90%" />
  </div>
</template>

<style scoped></style>
