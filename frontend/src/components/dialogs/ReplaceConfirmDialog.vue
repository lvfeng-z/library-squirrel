<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { taskManagerConfirmReplace, taskManagerConfirmReplaceBatch } from '@renderer/apis/http/wrappers/task'
import { useReplaceConfirmStore } from '@renderer/store/UseReplaceConfirmStore'
import type { DuplicateInfo } from '@renderer/store/UseReplaceConfirmStore'

const store = useReplaceConfirmStore()

async function handleReplace(item: DuplicateInfo) {
  store.setLoading(item.taskId, true)
  try {
    await taskManagerConfirmReplace(item.taskId, 'replace')
    store.remove(item.taskId)
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    store.setLoading(item.taskId, false)
  }
}

async function handleSkip(item: DuplicateInfo) {
  store.setLoading(item.taskId, true)
  try {
    await taskManagerConfirmReplace(item.taskId, 'skip')
    store.remove(item.taskId)
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    store.setLoading(item.taskId, false)
  }
}

async function handleReplaceAll() {
  const taskIds = store.list.map((item) => item.taskId)
  store.list.forEach((item) => store.setLoading(item.taskId, true))
  try {
    await taskManagerConfirmReplaceBatch(taskIds, 'replace')
    store.clear()
  } catch (e: any) {
    ElMessage.error(e.message)
    store.list.forEach((item) => store.setLoading(item.taskId, false))
  }
}

async function handleSkipAll() {
  const taskIds = store.list.map((item) => item.taskId)
  store.list.forEach((item) => store.setLoading(item.taskId, true))
  try {
    await taskManagerConfirmReplaceBatch(taskIds, 'skip')
    store.clear()
  } catch (e: any) {
    ElMessage.error(e.message)
    store.list.forEach((item) => store.setLoading(item.taskId, false))
  }
}
</script>

<template>
  <el-dialog
    :model-value="store.visible"
    :title="`以下任务对应的作品已存在，是否替换？(共${store.totalCount}个)`"
    width="500px"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
  >
    <div class="replace-confirm-list">
      <div v-for="(item, index) in store.list" :key="item.taskId" class="replace-confirm-item">
        <div class="replace-confirm-item-content">
          <span class="replace-confirm-item-index">{{ index + 1 }}</span>
          <div class="replace-confirm-item-info">
            <span class="replace-confirm-item-name">{{ item.taskName }}</span>
            <span class="replace-confirm-item-existing">已有作品：{{ item.existingWorkName }}</span>
          </div>
        </div>
        <el-button-group class="replace-confirm-item-actions">
          <el-button
            size="small"
            type="danger"
            :loading="store.isLoading(item.taskId)"
            @click="handleReplace(item)"
          >
            替换
          </el-button>
          <el-button
            size="small"
            :loading="store.isLoading(item.taskId)"
            @click="handleSkip(item)"
          >
            跳过
          </el-button>
        </el-button-group>
      </div>
    </div>
    <template #footer>
      <el-button type="danger" @click="handleReplaceAll">全部替换</el-button>
      <el-button @click="handleSkipAll">全部跳过</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.replace-confirm-list {
  max-height: 300px;
  overflow-x: hidden;
  overflow-y: auto;
}

.replace-confirm-item {
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  padding: 8px 0;
}

.replace-confirm-item + .replace-confirm-item {
  border-top: 1px solid var(--el-border-color-lighter);
}

.replace-confirm-item-content {
  display: flex;
  flex-direction: row;
  align-items: center;
  flex: 1;
  min-width: 0;
}

.replace-confirm-item-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-weight: bold;
  font-size: 16px;
  width: 28px;
  height: 28px;
  margin-right: 10px;
  background-color: var(--el-fill-color-dark);
  border-radius: 4px;
  flex-shrink: 0;
}

.replace-confirm-item-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.replace-confirm-item-name {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.replace-confirm-item-existing {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.replace-confirm-item-actions {
  margin-left: 12px;
  flex-shrink: 0;
}
</style>
