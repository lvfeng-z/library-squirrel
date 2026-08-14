<script setup lang="ts">
import { computed, Ref, ref } from 'vue'
import { useRouter } from 'vue-router'
import CollapsePanel from '@renderer/components/common/CollapsePanel.vue'
import { useNotificationStore } from '@renderer/store/UseNotificationStore.ts'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { type NotificationItem } from '@renderer/model/util/NotificationItem.ts'
import { isNotBlank } from '@renderer/utils/StringUtil.ts'

// model
// 开关状态
const state = defineModel<boolean>('state', { required: true })

const router = useRouter()
const notificationStore = useNotificationStore()

// 变量
const pageNumber: Ref<number> = ref(1)
const pageSize: Ref<number> = ref(10)
const currentPage: Ref<NotificationItem[]> = computed(() => {
  const start = (pageNumber.value - 1) * pageSize.value
  return notificationStore.getRange(start, start + pageSize.value)
})

// 仅当进度可解析为百分比时渲染进度条（total>0 或显式 percent）
function hasProgress(item: NotificationItem): boolean {
  const p = item.progress
  if (isNullish(p)) return false
  if (notNullish(p.percent)) return true
  return notNullish(p.total) && p.total > 0 && notNullish(p.current)
}
function progressPercent(item: NotificationItem): number {
  const p = item.progress
  if (isNullish(p)) return 0
  if (notNullish(p.percent)) return p.percent
  if (notNullish(p.total) && p.total > 0 && notNullish(p.current)) {
    return Math.round((p.current / p.total) * 100)
  }
  return 0
}
// level → 状态 tone 令牌色（el-progress color 用，四色分离：info→蓝/success→绿/warning→橙/error→红）
function levelColor(level: NotificationItem['level']): string {
  switch (level) {
    case 'success':
      return 'var(--app-status-done-text)'
    case 'warning':
      return 'var(--app-status-warn-text)'
    case 'error':
      return 'var(--app-status-fail-text)'
    default:
      return 'var(--app-status-pending-text)'
  }
}
// 点击跳转到关联页面（route 存在才跳）
function handleClick(item: NotificationItem): void {
  if (notNullish(item.route)) {
    router.push(item.route)
  }
}
</script>

<template>
  <collapse-panel
    v-model:state="state"
    :destroy-on-close="true"
    :enable-badge="notificationStore.activeCount !== 0"
    :badge-value="notificationStore.activeCount"
    :badge-max="999"
    border-radios="10px"
    position="right"
  >
    <div class="notification-list-container">
      <el-scrollbar class="notification-list-container-scrollbar">
        <template
          v-for="item in currentPage"
          :key="item.id"
        >
          <div
            :class="[
              'notification-list-item',
              `notification-level-${item.level}`,
              {
                'notification-list-item-terminal': item.terminal,
                'notification-list-item-clickable': notNullish(item.route)
              }
            ]"
            @click="handleClick(item)"
          >
            <div class="notification-list-item-header">
              <span
                class="notification-list-item-title"
                :title="item.title"
              >{{ item.title }}</span>
              <span
                v-if="isNotBlank(item.statusText)"
                class="notification-list-item-status"
              >{{ item.statusText }}</span>
            </div>
            <el-progress
              v-if="hasProgress(item)"
              :percentage="progressPercent(item)"
              :color="levelColor(item.level)"
              :stroke-width="6"
              :show-text="true"
            />
            <span
              v-if="isNotBlank(item.exception)"
              class="notification-list-item-exception"
            >{{ item.exception }}</span>
            <component
              :is="item.render()"
              v-if="notNullish(item.render)"
            />
          </div>
        </template>
      </el-scrollbar>
      <div class="notification-list-pagination-wrapper">
        <el-pagination
          v-model:current-page="pageNumber"
          v-model:page-size="pageSize"
          layout="prev, pager, next"
          :default-page-size="10"
          :pager-count="5"
          :total="notificationStore.count"
        />
      </div>
    </div>
  </collapse-panel>
</template>

<style scoped>
.notification-list-container {
  height: 100%;
  width: 300px;
  background-color: color-mix(in srgb, var(--app-bg-surface) 90%, transparent);
  padding: 5px;
}
.notification-list-container-scrollbar {
  height: calc(100% - 40px);
}
.notification-list-item {
  display: flex;
  flex-direction: column;
  margin: 5px;
  max-height: 160px;
  border-radius: var(--app-radius-lg);
  border-left: 3px solid transparent;
  background-color: var(--app-fill-color);
  padding: 5px;
}
.notification-list-item-clickable {
  cursor: pointer;
}
.notification-list-item-clickable:hover {
  background-color: var(--app-fill-color-dark);
}
.notification-list-item-terminal {
  opacity: 0.6;
}
.notification-list-item-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}
.notification-list-item-title {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 1;
  white-space: normal;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--app-text-primary);
  font-size: var(--el-font-size-medium);
  flex: 1;
}
.notification-list-item-status {
  flex-shrink: 0;
  color: var(--app-text-secondary);
  font-size: var(--el-font-size-small);
}
.notification-list-item-exception {
  color: var(--app-status-fail-text);
  font-size: var(--el-font-size-small);
  word-break: break-all;
}
.notification-level-info {
  border-left-color: var(--app-status-pending-text);
}
.notification-level-success {
  border-left-color: var(--app-status-done-text);
}
.notification-level-warning {
  border-left-color: var(--app-status-warn-text);
}
.notification-level-error {
  border-left-color: var(--app-status-fail-text);
}
.notification-list-pagination-wrapper {
  display: flex;
  justify-content: center;
  height: 40px;
}
</style>
