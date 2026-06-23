<script setup lang="ts">
import { computed, Ref, ref } from 'vue'
import CollapsePanel from '@renderer/components/common/CollapsePanel.vue'
import { useNotificationStore } from '@renderer/store/UseNotificationStore.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import NotificationItem from '@renderer/model/util/NotificationItem.ts'
import { isNotBlank } from '@renderer/utils/StringUtil.ts'

// model
// 开关状态
const state = defineModel<boolean>('state', { required: true })

// 变量
const pageNumber: Ref<number> = ref(1)
const pageSize: Ref<number> = ref(10)
const currentPage: Ref<NotificationItem[]> = computed(() => {
  const start = (pageNumber.value - 1) * pageSize.value
  return useNotificationStore().getRange(start, start + pageSize.value)
})
</script>

<template>
  <collapse-panel
    v-model:state="state"
    :destroy-on-close="true"
    :enable-badge="useNotificationStore().count !== 0"
    :badge-value="useNotificationStore().count"
    :badge-max="999"
    border-radios="10px"
    position="right"
  >
    <div class="notification-list-container">
      <el-scrollbar class="notification-list-container-scrollbar">
        <template
          v-for="item in currentPage"
          :key="item.title"
        >
          <div class="notification-list-item">
            <span
              class="notification-list-item-title"
              :title="item.title"
            >{{ item.title }}</span>
            <span
              v-if="isNotBlank(item.description)"
              class="notification-list-item-description"
            >
              {{ item.description }}
            </span>
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
          :total="useNotificationStore().count"
        />
      </div>
    </div>
  </collapse-panel>
</template>

<style scoped>
.notification-list-container {
  height: 100%;
  width: 300px;
  background-color: rgb(255, 255, 255, 0.9);
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
  background-color: var(--app-fill-color);
  padding: 5px;
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
}
.notification-list-item-description {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 1;
  white-space: normal;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--app-text-regular);
  font-size: var(--el-font-size-small);
}
.notification-list-pagination-wrapper {
  display: flex;
  justify-content: center;
  height: 40px;
}
</style>
