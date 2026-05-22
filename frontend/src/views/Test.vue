<script setup lang="ts">
import { computed, ref } from 'vue'
import BaseSubpage from '@renderer/views/BaseSubpage.vue'
import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { useParentTaskStore } from '@renderer/store/UseParentTaskStore.ts'

const taskStore = useTaskStore()
const parentTaskStore = useParentTaskStore()

const taskEntries = computed(() => [...taskStore.tasks.entries()])
const parentEntries = computed(() => [...parentTaskStore.parentTasks.entries()])

const activeTab = ref('task')
</script>

<template>
  <base-subpage>
    <el-tabs v-model="activeTab">
      <el-tab-pane label="TaskStore" name="task">
        <div style="margin-bottom: 8px; color: #888">共 {{ taskEntries.length }} 条</div>
        <el-table :data="taskEntries" border size="small" style="width: 100%">
          <el-table-column label="ID" prop="0" width="80" />
          <el-table-column label="task" min-width="400">
            <template #default="{ row }">
              <pre style="margin: 0; white-space: pre-wrap; font-size: 12px">{{ JSON.stringify(row[1].task, null, 2) }}</pre>
            </template>
          </el-table-column>
          <el-table-column label="notificationId" width="200">
            <template #default="{ row }">
              {{ row[1].notificationId ?? '-' }}
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
      <el-tab-pane label="ParentTaskStore" name="parent">
        <div style="margin-bottom: 8px; color: #888">共 {{ parentEntries.length }} 条</div>
        <el-table :data="parentEntries" border size="small" style="width: 100%">
          <el-table-column label="ID" prop="0" width="80" />
          <el-table-column label="TaskProgressDTO" min-width="400">
            <template #default="{ row }">
              <pre style="margin: 0; white-space: pre-wrap; font-size: 12px">{{ JSON.stringify(row[1], null, 2) }}</pre>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>
  </base-subpage>
</template>

<style scoped>
</style>
