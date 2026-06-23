<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useTourCenterStore } from '@renderer/store/UseTourCenterStore'

const store = useTourCenterStore()
const { tourList, status, activeTourId } = storeToRefs(store)

function isActive(id: string): boolean {
  return status.value === 'running' && activeTourId.value === id
}

function handleStart(id: string) {
  void store.start(id)
}

function handleReset(id: string) {
  store.resetCompleted(id)
}

function handleStop() {
  void store.skip()
}
</script>

<template>
  <div class="tour-center-panel">
    <div class="tour-center-header">
      <span class="tour-center-title">向导中心</span>
      <span class="tour-center-hint">点击「启动」开始对应向导，可随时重置已完成向导重新查看</span>
    </div>
    <el-scrollbar class="tour-center-list">
      <div
        v-for="tour in tourList"
        :key="tour.id"
        class="tour-center-item"
      >
        <div class="tour-center-item-info">
          <div class="tour-center-item-name">
            <span>{{ tour.name }}</span>
            <el-tag
              v-if="isActive(tour.id)"
              size="small"
              type="warning"
            >
              进行中
            </el-tag>
            <el-tag
              v-else-if="store.isCompleted(tour.id)"
              size="small"
              type="success"
            >
              已完成
            </el-tag>
          </div>
          <div class="tour-center-item-desc">{{ tour.description }}</div>
        </div>
        <div class="tour-center-item-actions">
          <el-button
            v-if="isActive(tour.id)"
            size="small"
            @click="handleStop"
          >
            结束
          </el-button>
          <el-button
            v-else
            size="small"
            type="primary"
            :disabled="status === 'running'"
            @click="handleStart(tour.id)"
          >
            启动
          </el-button>
          <el-button
            v-if="store.isCompleted(tour.id)"
            size="small"
            text
            @click="handleReset(tour.id)"
          >
            重置
          </el-button>
        </div>
      </div>
      <el-empty
        v-if="tourList.length === 0"
        description="暂无可用向导"
      />
    </el-scrollbar>
  </div>
</template>

<style scoped>
.tour-center-panel {
  display: flex;
  flex-direction: column;
  background: var(--app-bg-surface-variant);
  border-radius: var(--app-radius);
  padding: 10px;
  height: 100%;
  box-sizing: border-box;
}

.tour-center-header {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-bottom: 8px;
  border-bottom: 1px solid #e0e0e0;
  margin-bottom: 8px;
}

.tour-center-title {
  font-size: 15px;
  font-weight: 600;
}

.tour-center-hint {
  font-size: 12px;
  color: #909399;
}

.tour-center-list {
  flex: 1;
}

.tour-center-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: var(--app-bg-surface);
  border-radius: var(--app-radius);
  padding: 10px;
  margin-bottom: 8px;
}

.tour-center-item-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-right: 10px;
  flex: 1;
  min-width: 0;
}

.tour-center-item-name {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 500;
}

.tour-center-item-desc {
  font-size: 12px;
  color: #909399;
}

.tour-center-item-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}
</style>
