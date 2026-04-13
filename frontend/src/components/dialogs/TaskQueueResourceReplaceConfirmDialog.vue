<script setup lang="ts">
import { Ref } from 'vue'

// model
const state: Ref<boolean> = defineModel<boolean>('state', { required: true })
const confirmList: Ref<{ taskId: number; msg: string }[]> = defineModel<{ taskId: number; msg: string }[]>('confirmList', {
  required: true
})

// 方法
// 解决所有
function resolveAll(confirm: boolean) {
  const responseConfirmIds = confirmList.value.map((resourceReplaceConfirm) => resourceReplaceConfirm.taskId)
  window.electron.ipcRenderer.send('task-queue-resource-replace-confirm-echo', responseConfirmIds, confirm)
  confirmList.value.splice(0, responseConfirmIds.length)
  state.value = confirmList.value.length !== 0
}
// 解决一个
function resolveOne(confirm: boolean, confirmItem: { taskId: number; msg: string }) {
  window.electron.ipcRenderer.send('task-queue-resource-replace-confirm-echo', [confirmItem.taskId], confirm)
  confirmList.value.splice(confirmList.value.indexOf(confirmItem), 1)
  state.value = confirmList.value.length !== 0
}
</script>

<template>
  <el-dialog v-model="state" :title="`以下任务下载的作品已有可用的资源，是否替换？(共${confirmList.length}个)`" destroy-on-close>
    <div class="task-queue-replace-confirm-list-container">
      <template v-for="(item, index) in confirmList" :key="item.taskId">
        <div class="task-queue-replace-confirm-list-item-container">
          <div class="task-queue-replace-confirm-list-item">
            <span class="task-queue-replace-confirm-list-item-index">{{ index + 1 }}</span>
            <span>{{ item.msg }}</span>
            <el-button-group class="task-queue-replace-confirm-list-item-button">
              <el-button icon="EditPen" type="danger" @click="resolveOne(true, item)" />
              <el-button icon="Remove" type="primary" @click="resolveOne(false, item)" />
            </el-button-group>
          </div>
          <el-divider v-if="index < confirmList.length" class="task-queue-replace-confirm-list-divider" />
        </div>
      </template>
    </div>
    <template #footer>
      <el-button type="danger" @click="resolveAll(true)">替换原有资源</el-button>
      <el-button type="primary" @click="resolveAll(false)">保留原有资源</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.task-queue-replace-confirm-list-container {
  max-height: 200px;
  overflow-x: hidden;
  overflow-y: scroll;
}
.task-queue-replace-confirm-list-item {
  display: flex;
  flex-direction: row;
}
.task-queue-replace-confirm-list-item-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-weight: bold;
  font-size: 20px;
  margin-right: 4px;
  padding: 2px;
  background-color: var(--el-fill-color-dark);
  border-radius: 4px;
}
.task-queue-replace-confirm-list-divider {
  margin: 6px;
}
.task-queue-replace-confirm-list-item-button {
  margin-left: auto;
}
</style>
