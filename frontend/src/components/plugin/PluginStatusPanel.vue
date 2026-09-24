<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { pluginApi } from '@renderer/apis/http'
import { ElMessage } from 'element-plus'
import StatusTag from '@renderer/components/common/StatusTag.vue'
import { PluginStatusDTO, ParticipationEntryState } from '@bindings/github.com/library-squirrel/backend/plugin/models'
import { PluginDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { isBlank, isNotBlank } from '@renderer/utils/StringUtil'

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

// 派生面展示名与展示序（point 值与后端参与度条目的 point 字段对应）
const POINT_LABELS: ReadonlyArray<{ point: string; label: string }> = [
  { point: 'workFetch', label: '作品拉取' },
  { point: 'siteAuthorFetch', label: '作者拉取' },
  { point: 'siteBrowsers', label: '站点浏览器' },
  { point: 'resourceTypes', label: '资源类型' },
  { point: 'frontendExtensions', label: '前端扩展' }
]

// 声明条目按派生面分组（声明 = 清单条目、状态 = 真相层参与度；仅含有条目的面，序取 POINT_LABELS）
const participationGroups = computed(() => {
  const entries = status.value?.participation?.entries
  if (!entries || entries.length === 0) {
    return []
  }
  const byPoint = new Map<string, ParticipationEntryState[]>()
  for (const entry of entries) {
    const list = byPoint.get(entry.point)
    if (list) {
      list.push(entry)
    } else {
      byPoint.set(entry.point, [entry])
    }
  }
  return POINT_LABELS.flatMap(({ point, label }) => {
    const groupEntries = byPoint.get(point)
    return groupEntries ? [{ label, entries: groupEntries }] : []
  })
})

// resolver 失败分类 → 人读标签（后端求值编排层与运行器的分类全集；未知值回退原值）
const FAILURE_KIND_LABELS: Record<string, string> = {
  syntax: '语法错误',
  timeout: '执行超时',
  runtime: '运行异常',
  invalid_output: '输出不合法',
  script_too_large: '脚本超体积',
  script_load: '脚本装载失败',
  settings_read: '设置读取失败',
  canceled: '已取消',
  internal: '内部错误'
}

// resolver 降级态标注文案：最近一次求值失败（超时/异常等）时显著标注，参与度展示可能滞后
const resolverDegradedText = computed(() => {
  const evalStatus = status.value?.participation?.status
  if (!evalStatus || isBlank(evalStatus.lastFailure)) {
    return null
  }
  const kindLabel = FAILURE_KIND_LABELS[evalStatus.lastFailure] ?? evalStatus.lastFailure
  const msg = isNotBlank(evalStatus.lastFailureMsg) ? evalStatus.lastFailureMsg : '无详细信息'
  return `${kindLabel}：${msg}。参与度展示可能滞后于最新设置，重新保存该插件的设置项或重启后恢复。`
})

// 最近一次求值的单条拒收提示（软降级：被拒条目保持之前状态，整体覆盖表仍生效）
const rejectedNoteText = computed(() => {
  const evalStatus = status.value?.participation?.status
  if (!evalStatus || evalStatus.lastRejected <= 0) {
    return null
  }
  return `最近一次求值有 ${evalStatus.lastRejected} 条输出被拒收，对应意愿未生效`
})

// 最近求值完成时间（降级标注与拒收提示的附带信息；0 = 从未求值）
const lastEvalText = computed(() => {
  const evalStatus = status.value?.participation?.status
  if (!evalStatus || !evalStatus.lastEvalAt) {
    return null
  }
  return `最近求值：${formatTime(evalStatus.lastEvalAt)}`
})

// 覆盖停用条目的悬浮理由（resolver 给出；无理由停用不悬浮）
function disableReasonTooltip(entry: ParticipationEntryState): string | null {
  if (entry.active || isBlank(entry.reason)) {
    return null
  }
  return `停用理由：${entry.reason}`
}

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
        <el-descriptions-item label="作品拉取">
          <template v-if="status.workFetch && status.workFetch.length > 0">
            <el-tag
              v-for="wf in status.workFetch"
              :key="wf.id"
              size="small"
              class="status-tag"
            >
              {{ wf.name || wf.id }}
            </el-tag>
          </template>
          <span
            v-else
            class="text-muted"
          >无</span>
        </el-descriptions-item>
        <el-descriptions-item label="站点浏览器">
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
        <el-descriptions-item label="前端扩展">
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

      <!-- 声明条目参与度（声明 = 清单条目、状态 = 真相层参与度；行内徽标，被覆盖停用的条目悬浮展示
           resolver 给出的理由；插件未激活无会话时后端不下发该节） -->
      <el-descriptions
        v-if="status.participation"
        title="声明条目参与度"
        :column="1"
        border
        size="small"
      >
        <!-- resolver 降级态显著标注（超时/失败：展示可能滞后于最新设置） -->
        <el-descriptions-item label="求值状态">
          <div
            v-if="resolverDegradedText"
            class="resolver-degraded"
            data-testid="resolver-degraded"
          >
            <span class="resolver-degraded__title">求值降级</span>
            <span class="resolver-degraded__text">{{ resolverDegradedText }}</span>
          </div>
          <template v-else>
            <span
              v-if="status.participation.status.hasResolver"
              class="text-muted"
            >正常</span>
            <span
              v-else
              class="text-muted"
            >未声明设置解析器，条目全部基线参与</span>
          </template>
          <div
            v-if="lastEvalText"
            class="eval-meta"
          >
            {{ lastEvalText }}
          </div>
          <div
            v-if="rejectedNoteText"
            class="eval-meta eval-meta--warn"
            data-testid="rejected-note"
          >
            {{ rejectedNoteText }}
          </div>
        </el-descriptions-item>
        <el-descriptions-item
          v-for="group in participationGroups"
          :key="group.label"
          :label="group.label"
        >
          <span
            v-for="entry in group.entries"
            :key="entry.id"
            class="entry-item"
          >
            <el-tooltip
              v-if="disableReasonTooltip(entry)"
              :content="disableReasonTooltip(entry)"
              placement="top"
            >
              <span class="entry-item__inner">
                <code class="entry-item__id">{{ entry.id }}</code>
                <StatusTag
                  status="plugin-entry-disabled"
                  size="small"
                />
              </span>
            </el-tooltip>
            <span
              v-else
              class="entry-item__inner"
            >
              <code class="entry-item__id">{{ entry.id }}</code>
              <StatusTag
                :status="entry.active ? 'plugin-entry-active' : 'plugin-entry-disabled'"
                size="small"
              />
            </span>
          </span>
        </el-descriptions-item>
        <el-descriptions-item
          v-if="participationGroups.length === 0"
          label="声明条目"
        >
          <span class="text-muted">无</span>
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

/* 声明条目参与度：条目行内徽标（id + 参与态） */
.entry-item {
  display: inline-flex;
  margin-right: 8px;
  margin-bottom: 4px;
}

.entry-item__inner {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.entry-item__id {
  font-size: 12px;
}

/* resolver 降级态显著标注（warn tone：超时/失败时参与度展示可能滞后） */
.resolver-degraded {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 6px 10px;
  border: 1px solid var(--app-status-warn-border);
  border-radius: var(--app-radius-sm);
  background: var(--app-status-warn-bg);
}

.resolver-degraded__title {
  flex-shrink: 0;
  font-weight: 600;
  color: var(--app-status-warn-text);
}

.resolver-degraded__text {
  color: var(--app-status-warn-text);
  font-size: 12px;
  word-break: break-all;
}

/* 求值附注（最近求值时间 / 单条拒收提示） */
.eval-meta {
  margin-top: 4px;
  color: var(--app-status-idle-text);
  font-size: 12px;
}

.eval-meta--warn {
  color: var(--app-status-warn-text);
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
