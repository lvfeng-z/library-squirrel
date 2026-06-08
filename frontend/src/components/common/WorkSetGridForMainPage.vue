<script setup lang="ts">
import { Ref, ref } from 'vue'
import WorkSetDialog from '@renderer/components/dialogs/WorkSetDialog.vue'
import WorkSetGrid from '@renderer/components/common/WorkSetGrid.vue'
import { WorkSetWithCoverDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'

// props
const props = defineProps<{
  workSetList: WorkSetWithCoverDTO[]
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
    <work-set-grid :work-set-list="props.workSetList" :checkable="false" @image-clicked="handleImageClicked"></work-set-grid>
    <work-set-dialog v-model:state="workSetDialogState" v-model:current-work-set-id="currentWorkSetId" width="90%" />
  </div>
</template>

<style scoped></style>
