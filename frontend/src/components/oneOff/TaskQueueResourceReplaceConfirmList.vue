<script setup lang="ts">
const props = defineProps<{ confirmList: { taskId: number; msg: string }[] }>()
</script>

<template>
  <div class="task-queue-replace-confirm-list-container">
    <template v-for="(item, index) in props.confirmList" :key="item.taskId">
      <div class="task-queue-replace-confirm-list-item-container">
        <div class="task-queue-replace-confirm-list-item">
          <span class="task-queue-replace-confirm-list-item-index">{{ index + 1 }}</span>
          <span>{{ item.msg }}</span>
        </div>
        <el-divider v-if="index < props.confirmList.length" class="task-queue-replace-confirm-list-divider" />
      </div>
    </template>
  </div>
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
</style>
