<script setup lang="ts">
import WorkDialog from '../dialogs/WorkDialog.vue'
import { computed, Ref, ref } from 'vue'
import WorkSetDialog from '@renderer/components/dialogs/WorkSetDialog.vue'
import WorkGrid from '@renderer/components/common/WorkGrid.vue'
import {WorkFullDTO} from "@bindings/github.com//lvfeng-z/library-squirrel-plugin-sdk/dto";
import WorkCardItem from '@renderer/model/model/dto/WorkCardItem.ts'

// props
const props = defineProps<{
  workList: WorkFullDTO[]
}>()

// model
const currentWorkIndex = defineModel<number>('currentWorkIndex', { required: true })

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
// 打开作品集dialog
async function openWorkSetDialog(workSetId: number) {
  currentWorkSetId.value = workSetId
  workDialogState.value = false
  workSetDialogState.value = true
}
</script>

<template>
  <div>
    <work-grid :work-list="workCardItemList" :checkable="false" @image-clicked="handleImageClicked"></work-grid>
    <work-dialog
      v-model:state="workDialogState"
      v-model:current-work-index="currentWorkIndex"
      :work="props.workList"
      width="90%"
      @open-work-set="openWorkSetDialog"
    />
    <work-set-dialog v-model:state="workSetDialogState" v-model:current-work-set-id="currentWorkSetId" width="90%" />
  </div>
</template>

<style scoped></style>
