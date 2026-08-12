<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { useChangeConfirmStore, CHANGE_KIND } from '@renderer/store/UseChangeConfirmStore'
import { fsmonitorConfirmChange } from '@renderer/apis/http/wrappers/fsmonitor'
import type { ChangeInfo } from '@renderer/store/UseChangeConfirmStore'

const store = useChangeConfirmStore()

/** 是否为"同步路径"类变更（Move/DirMove → sync；Delete → ack） */
function isSyncKind(kind: number): boolean {
  return kind === CHANGE_KIND.Move || kind === CHANGE_KIND.DirMove
}

/** 接受现状：Move/DirMove→sync(同步路径)，Delete→ack(标记失效) */
async function handleAccept(item: ChangeInfo) {
  store.setLoading(item.id, true)
  try {
    const action = isSyncKind(item.kind) ? 'sync' : 'ack'
    await fsmonitorConfirmChange(item.id, action)
    store.remove(item.id)
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    store.setLoading(item.id, false)
  }
}

/** 复原：Move/DirMove→restore(文件/目录移回旧路径) */
async function handleRestore(item: ChangeInfo) {
  store.setLoading(item.id, true)
  try {
    await fsmonitorConfirmChange(item.id, 'restore')
    store.remove(item.id)
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    store.setLoading(item.id, false)
  }
}

/** 批量：全部接受现状（逐条调，Move/DirMove→sync, Delete→ack） */
async function handleAcceptAll() {
  const items = [...store.list]
  store.clear()
  for (const item of items) {
    try {
      const action = isSyncKind(item.kind) ? 'sync' : 'ack'
      await fsmonitorConfirmChange(item.id, action)
    } catch (e: any) {
      ElMessage.error(e.message)
    }
  }
}

/** 变更描述（路径对比） */
function describe(item: ChangeInfo): string {
  if (isSyncKind(item.kind)) {
    return `${item.fromPath} → ${item.toPath}`
  }
  return item.fromPath
}
</script>

<template>
  <el-dialog
    :model-value="store.visible"
    title="检测到工作目录文件变更，请处理"
    width="640px"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
  >
    <div class="change-list">
      <div
        v-for="item in store.list"
        :key="item.id"
        class="change-item"
      >
        <div class="change-item-content">
          <span class="change-item-kind">{{ item.kindName }}</span>
          <span class="change-item-path">{{ describe(item) }}</span>
        </div>
        <el-button-group class="change-item-actions">
          <el-button
            size="small"
            type="primary"
            :loading="store.isLoading(item.id)"
            @click="handleAccept(item)"
          >
            {{ isSyncKind(item.kind) ? '同步路径' : '确认删除' }}
          </el-button>
          <el-button
            v-if="isSyncKind(item.kind)"
            size="small"
            :loading="store.isLoading(item.id)"
            @click="handleRestore(item)"
          >
            复原
          </el-button>
        </el-button-group>
      </div>
    </div>
    <template #footer>
      <el-button type="primary" @click="handleAcceptAll">全部接受现状</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.change-list {
  max-height: 360px;
  overflow-x: hidden;
  overflow-y: auto;
}

.change-item {
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  padding: 8px 0;
}

.change-item + .change-item {
  border-top: 1px solid var(--app-border-color-lighter);
}

.change-item-content {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  margin-right: 12px;
}

.change-item-kind {
  font-weight: 500;
  color: var(--app-text-primary);
  margin-bottom: 2px;
}

.change-item-path {
  font-size: 12px;
  color: var(--app-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.change-item-actions {
  flex-shrink: 0;
}
</style>
