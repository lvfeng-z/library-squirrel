<script setup lang="ts">
import { ref, watch } from 'vue'
import { pluginApi } from '@renderer/apis/http'
import { ElMessage } from 'element-plus'
import { PluginStatusDTO } from '@bindings/github.com/library-squirrel/backend/plugin/models'
import { isNotBlank } from '@renderer/utils/StringUtil'

interface Props {
  publicId: string
}

const props = defineProps<Props>()

const loading = ref(false)
const status = ref<PluginStatusDTO | null>(null)

watch(() => props.publicId, (newId) => {
  if (isNotBlank(newId)) {
    loadStatus(newId)
  }
}, { immediate: true })

async function loadStatus(publicId: string) {
  loading.value = true
  try {
    const result = await pluginApi.pluginGetStatus(publicId)
    status.value = result.data
  } catch (e) {
    ElMessage.error(`获取插件状态失败: ${(e as Error).message}`)
  } finally {
    loading.value = false
  }
}

function formatTime(timestamp: number | undefined): string {
  if (!timestamp) return '-'
  return new Date(timestamp).toLocaleString()
}

function formatSize(bytes: number | undefined): string {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
</script>

<template>
  <div
    v-loading="loading"
    class="plugin-status-panel"
  >
    <template v-if="status">
      <!-- 运行时状态 -->
      <el-descriptions
        title="运行时状态"
        :column="1"
        border
        size="small"
      >
        <el-descriptions-item label="状态">
          <el-tag
            :type="status.isRunning ? 'success' : 'info'"
            size="small"
          >
            {{ status.isRunning ? '在线' : '离线' }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item
          v-if="status.pid"
          label="PID"
        >
          {{ status.pid }}
        </el-descriptions-item>
        <el-descriptions-item label="激活时间">
          {{ formatTime(status.activatedAt) }}
        </el-descriptions-item>
      </el-descriptions>

      <!-- 扩展点列表 -->
      <el-descriptions
        title="扩展点"
        :column="1"
        border
        size="small"
      >
        <el-descriptions-item label="TaskHandler">
          <template v-if="status.taskHandlers && status.taskHandlers.length > 0">
            <el-tag
              v-for="th in status.taskHandlers"
              :key="th.id"
              size="small"
              class="status-tag"
            >
              {{ th.name || th.id }}
            </el-tag>
          </template>
          <span
            v-else
            class="text-muted"
          >无</span>
        </el-descriptions-item>
        <el-descriptions-item label="SiteBrowser">
          <template v-if="status.siteBrowsers && status.siteBrowsers.length > 0">
            <el-tag
              v-for="sb in status.siteBrowsers"
              :key="sb.id"
              size="small"
              type="success"
              class="status-tag"
            >
              {{ sb.name || sb.id }}
            </el-tag>
          </template>
          <span
            v-else
            class="text-muted"
          >无</span>
        </el-descriptions-item>
        <el-descriptions-item label="Slot">
          <template v-if="status.slots && status.slots.length > 0">
            <el-tag
              v-for="s in status.slots"
              :key="s.id"
              size="small"
              type="warning"
              class="status-tag"
            >
              {{ s.name || s.id }} ({{ s.slotType }})
            </el-tag>
          </template>
          <span
            v-else
            class="text-muted"
          >无</span>
        </el-descriptions-item>
      </el-descriptions>

      <!-- 存储状态 -->
      <el-descriptions
        title="存储状态"
        :column="1"
        border
        size="small"
      >
        <el-descriptions-item label="PluginData">
          {{ formatSize(status.pluginDataSize) }}
        </el-descriptions-item>
      </el-descriptions>

      <!-- URL 监听规则 -->
      <el-descriptions
        title="URL 监听规则"
        :column="1"
        border
        size="small"
      >
        <el-descriptions-item label="匹配模式">
          <template v-if="status.urlPatterns && status.urlPatterns.length > 0">
            <div
              v-for="pattern in status.urlPatterns"
              :key="pattern"
              class="url-pattern"
            >
              <code>{{ pattern }}</code>
            </div>
          </template>
          <span
            v-else
            class="text-muted"
          >无</span>
        </el-descriptions-item>
      </el-descriptions>
    </template>
  </div>
</template>

<style scoped>
.plugin-status-panel {
  padding: 12px;
}

.plugin-status-panel :deep(.el-descriptions) {
  margin-bottom: 16px;
}

.plugin-status-panel :deep(.el-descriptions__title) {
  font-size: 14px;
  font-weight: 600;
}

.status-tag {
  margin-right: 4px;
  margin-bottom: 4px;
}

.text-muted {
  color: #909399;
  font-size: 12px;
}

.url-pattern {
  margin-bottom: 4px;
}

.url-pattern code {
  background: #f5f7fa;
  padding: 2px 6px;
  border-radius: 3px;
  font-size: 12px;
}
</style>
