<script setup lang="ts">
import { Close } from '@element-plus/icons-vue'
import { useReminderStore } from '@renderer/store/UseReminderStore.ts'

const reminderStore = useReminderStore()

/** 聚合卡片内条目列表最多展示条数，超出以「等 N 条」收尾 */
const MAX_LIST_ITEMS = 3
</script>

<template>
  <div class="reminder-stack z-layer-10">
    <transition-group name="reminder">
      <div
        v-for="card in reminderStore.visibleCards"
        :key="card.id"
        :class="['reminder-card', `reminder-level-${card.level}`]"
      >
        <div class="reminder-card-title-row">
          <div class="reminder-card-title-group">
            <span class="reminder-card-title">{{ card.title }}</span>
            <span
              v-if="card.items.length > 1"
              class="reminder-card-count"
            >{{ card.items.length }} 条</span>
          </div>
          <el-icon
            class="reminder-card-close"
            @click="reminderStore.dismiss(card.id)"
          >
            <Close />
          </el-icon>
        </div>
        <div
          v-if="card.items.length === 1"
          class="reminder-card-message"
        >{{ card.items[0] }}</div>
        <ul
          v-else
          class="reminder-card-list"
        >
          <li
            v-for="(message, index) in card.items.slice(0, MAX_LIST_ITEMS)"
            :key="index"
          >
            {{ message }}
          </li>
          <li
            v-if="card.items.length > MAX_LIST_ITEMS"
            class="reminder-card-more"
          >
            等 {{ card.items.length }} 条
          </li>
        </ul>
      </div>
    </transition-group>
    <div
      v-if="reminderStore.overflowCount > 0"
      class="reminder-overflow"
    >
      还有 {{ reminderStore.overflowCount }} 条提醒排队中
    </div>
  </div>
</template>

<style scoped>
.reminder-stack {
  position: fixed;
  top: 16px;
  right: 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 300px;
  /* 容器本身不拦截点击，仅卡片与排队提示可交互 */
  pointer-events: none;
}
.reminder-card {
  pointer-events: auto;
  background-color: var(--app-bg-surface);
  border-radius: var(--app-radius-lg);
  border-left: 3px solid transparent;
  box-shadow: var(--app-shadow);
  padding: 8px 10px;
}
.reminder-card-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}
.reminder-card-title-group {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 6px;
}
.reminder-card-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--app-text-primary);
  font-size: var(--el-font-size-medium);
  font-weight: bold;
}
.reminder-card-count {
  flex-shrink: 0;
  border-radius: var(--app-radius);
  background-color: var(--app-fill-color);
  padding: 0 6px;
  color: var(--app-text-secondary);
  font-size: var(--el-font-size-small);
  line-height: 18px;
}
.reminder-card-close {
  flex-shrink: 0;
  cursor: pointer;
  color: var(--app-text-secondary);
}
.reminder-card-close:hover {
  color: var(--app-text-primary);
}
.reminder-card-message {
  margin-top: 4px;
  color: var(--app-text-regular);
  font-size: var(--el-font-size-small);
  word-break: break-all;
}
.reminder-card-list {
  margin: 4px 0 0 0;
  padding-left: 18px;
}
.reminder-card-list li {
  color: var(--app-text-regular);
  font-size: var(--el-font-size-small);
  list-style: disc;
  word-break: break-all;
}
.reminder-card-more {
  color: var(--app-text-secondary);
}
.reminder-overflow {
  pointer-events: auto;
  align-self: center;
  color: var(--app-text-secondary);
  background-color: color-mix(in srgb, var(--app-bg-surface-variant) 90%, transparent);
  border-radius: var(--app-radius);
  font-size: var(--el-font-size-small);
  padding: 2px 10px;
}
.reminder-level-info {
  border-left-color: var(--app-status-pending-text);
}
.reminder-level-success {
  border-left-color: var(--app-status-done-text);
}
.reminder-level-warning {
  border-left-color: var(--app-status-warn-text);
}
.reminder-level-error {
  border-left-color: var(--app-status-fail-text);
}
/* 过渡：右侧滑入/滑出 + 位移动画 */
.reminder-enter-active,
.reminder-leave-active,
.reminder-move {
  transition: all 0.3s ease;
}
.reminder-enter-from,
.reminder-leave-to {
  opacity: 0;
  transform: translateX(30px);
}
</style>
