<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { pluginApi } from '@renderer/apis/http'
import { ElMessage } from 'element-plus'
import StatusTag from '@renderer/components/common/StatusTag.vue'
import { PluginStatusDTO } from '@bindings/github.com/library-squirrel/backend/plugin/models'
import { PluginDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { isNotBlank } from '@renderer/utils/StringUtil'

interface Props {
  publicId: string
}

const props = defineProps<Props>()

const loading = ref(false)
const status = ref<PluginStatusDTO | null>(null)
const plugin = ref<PluginDTO | null>(null)

watch(() => props.publicId, (newId) => {
  if (isNotBlank(newId)) {
    loadStatus(newId)
  }
}, { immediate: true })

async function loadStatus(publicId: string) {
  loading.value = true
  try {
    const statusResult = await pluginApi.pluginGetStatus(publicId)
    status.value = statusResult.data
    const pluginResult = await pluginApi.pluginGetByPublicId(publicId)
    plugin.value = pluginResult.data
  } catch (e) {
    ElMessage.error(`获取插件状态失败: ${(e as Error).message}`)
  } finally {
    loading.value = false
  }
}

// 官方身份（内容摘要命中主程序携带的官方指纹名单；仅 true 视为官方，false/NULL 一律非官方——保守方向）
const official = computed(() => plugin.value?.official === true)
// 安装渠道状态 key（source 枚举值直接拼 plugin-{source}，与列表渠道维度同 key）
const sourceStatusKey = computed(() => `plugin-${plugin.value?.source ?? ''}`)
// 信任状态 key（仅 true 视为已信任，false/NULL 一律未信任——保守方向）
const trustedStatusKey = computed(() =>
  plugin.value?.trusted === true ? 'plugin-trusted' : 'plugin-unverified'
)
// 生命周期状态 key（后端 lifecycleState 值直接拼 plugin-{state}，与渠道维度同拼法；空值兜底未激活）
const lifecycleStatusKey = computed(() => {
  const state = status.value?.lifecycleState
  return isNotBlank(state) ? `plugin-${state}` : 'plugin-inactive'
})

function formatTime(timestamp: number | undefined): string {
  if (!timestamp) return '-'
  return new Date(timestamp).toLocaleString()
}
</script>

<template>
  <div
    v-loading="loading"
    class="plugin-status-panel"
  >
    <template v-if="status">
      <!-- 来源与信任 -->
      <el-descriptions
        v-if="plugin"
        title="来源与信任"
        :column="1"
        border
        size="small"
      >
        <el-descriptions-item label="官方身份">
          <StatusTag
            v-if="official"
            size="small"
            status="plugin-official"
          />
          <span
            v-else
            class="text-unofficial"
          >非官方</span>
        </el-descriptions-item>
        <el-descriptions-item label="安装渠道">
          <StatusTag
            size="small"
            :status="sourceStatusKey"
          />
        </el-descriptions-item>
        <el-descriptions-item label="信任状态">
          <StatusTag
            size="small"
            :status="trustedStatusKey"
          />
        </el-descriptions-item>
      </el-descriptions>

      <!-- 激活失败告警（最近一次激活失败原因，重试成功后自动消失） -->
      <div
        v-if="isNotBlank(status.activateError)"
        class="activate-error"
      >
        <span class="activate-error__title">激活失败</span>
        <span class="activate-error__text">{{ status.activateError }}</span>
      </div>

      <!-- 运行时状态 -->
      <el-descriptions
        title="运行时状态"
        :column="1"
        border
        size="small"
      >
        <el-descriptions-item label="生命周期">
          <StatusTag
            size="small"
            :status="lifecycleStatusKey"
          />
        </el-descriptions-item>
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
          <template v-if="status.frontendExtensions && status.frontendExtensions.length > 0">
            <el-tag
              v-for="s in status.frontendExtensions"
              :key="s.id"
              size="small"
              type="warning"
              class="status-tag"
            >
              {{ s.name || s.id }} ({{ s.kind }})
            </el-tag>
          </template>
          <span
            v-else
            class="text-muted"
          >无</span>
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

/* 激活失败告警行（fail tone：与失败状态语义同源，随主题逐主题调色） */
.activate-error {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 16px;
  padding: 8px 12px;
  border: 1px solid var(--app-status-fail-border);
  border-radius: var(--app-radius-sm);
  background: var(--app-status-fail-bg);
}

.activate-error__title {
  flex-shrink: 0;
  font-weight: 600;
  color: var(--app-status-fail-text);
}

.activate-error__text {
  color: var(--app-status-fail-text);
  font-size: 12px;
  word-break: break-all;
}

.text-muted {
  color: #909399;
  font-size: 12px;
}

/* 官方身份行非官方时的中性文案（与渠道 tag 的灰同走 idle tone，随主题变化） */
.text-unofficial {
  color: var(--app-status-idle-text);
  font-size: 12px;
}

.url-pattern {
  margin-bottom: 4px;
}

.url-pattern code {
  background: var(--app-bg-surface-variant);
  padding: 2px 6px;
  border-radius: 3px;
  font-size: 12px;
}
</style>
