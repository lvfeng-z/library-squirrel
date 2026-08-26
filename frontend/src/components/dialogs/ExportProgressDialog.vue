<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { exportCancelExport, exportStartExport } from '@renderer/apis/http/wrappers/export'
import { appLauncherOpenAbsolutePath } from '@renderer/apis/http/wrappers/appLauncher'
import { fileSysUtilSelectDirectory } from '@renderer/apis/http/wrappers/fileSysUtil'
import { settingsGetSettings } from '@renderer/apis/http/wrappers/settings'
import { clearExportState, getExportState, markExportStarted } from '@renderer/composables/useExportProgress'
import { arrayNotEmpty } from '@renderer/utils/CommonUtil'

// model
// 弹窗开关（MainView 控制：打开即触发导出启动）
const state = defineModel<boolean>('state', { required: true })

// props
// 选中作品/作品集 id 列表（决策5：前端把选中 id 列表传给后端收集）
const props = defineProps<{
  workIds: number[]
  workSetIds: number[]
}>()

// emits
// closed: 弹窗关闭（含完成/失败后手动关闭）
const emits = defineEmits<{
  closed: []
}>()

// 本次导出的 exportID（null=尚未发起）
const exportId = ref<string | null>(null)
// StartExport IPC 进行中（防重复触发）
const starting = ref(false)
// StartExport 同步失败信息（空选择/后端前置错误）
const startError = ref('')
// 本次导出的输出目录（'' = 使用工作目录）；初始值来自设置页配置的导出默认目录（exportSettings.outputDir），
// 弹窗内浏览选择为仅本次生效的临时覆盖，不写回设置
const outputDir = ref('')

// 是否还有可消费的选中（空选择时后端会报错，前端也可先行提示）
const hasSelection = computed(() => arrayNotEmpty(props.workIds) || arrayNotEmpty(props.workSetIds))

// 是否处于输出目录配置阶段（有选择、未发起、无错误）
const inConfig = computed(() => hasSelection.value && !exportId.value && !starting.value && !startError.value)

// 进度状态（由 export-events 驱动）
const progress = computed(() => getExportState(exportId.value ?? undefined))

// 取消按钮可用性：导出运行中且未到终态
const canCancel = computed(() => exportId.value !== null && progress.value?.status === 'running')

async function startExport() {
  if (starting.value) return
  if (exportId.value) return // 已发起过
  starting.value = true
  startError.value = ''
  try {
    const result = await exportStartExport(props.workIds, props.workSetIds, outputDir.value)
    exportId.value = result.data
    markExportStarted(result.data)
  } catch (e) {
    startError.value = e instanceof Error ? e.message : String(e)
  } finally {
    starting.value = false
  }
}

// 读取设置页配置的导出默认目录（settings exportSettings.outputDir；读取失败保持默认，不阻塞导出）
async function loadDefaultOutputDir(): Promise<void> {
  try {
    const res = await settingsGetSettings()
    if (!res.success || !res.data) return
    const es = res.data.exportSettings as { outputDir?: string } | undefined
    outputDir.value = es?.outputDir ?? ''
  } catch (e) {
    console.warn('读取导出默认目录失败', e)
  }
}

// 浏览选择输出目录（本次导出临时覆盖；不写回设置——默认路径由设置页显式配置）
async function pickOutputDir(): Promise<void> {
  try {
    const res = await fileSysUtilSelectDirectory('选择导出目录')
    if (res.success && res.data?.filePaths?.length) {
      outputDir.value = res.data.filePaths[0]
    }
  } catch (e) {
    // 选择器被取消或失败：保持当前值
  }
}

// 恢复默认（本次导出使用工作目录；不写回设置）
function resetOutputDir(): void {
  outputDir.value = ''
}

async function handleCancel() {
  if (!exportId.value) return
  try {
    await exportCancelExport(exportId.value)
  } catch (e) {
    // 取消失败（导出可能已结束）：静默，等待终态事件收尾
  }
}

// 打开导出产物文件（系统默认应用）
async function openExportFile(): Promise<void> {
  const target = progress.value?.targetPath
  if (!target) return
  const res = await appLauncherOpenAbsolutePath(target)
  if (!res.success) {
    ElMessage.error(res.msg || '打开文件失败')
  }
}

// 打开导出产物所在目录
async function openExportDir(): Promise<void> {
  const target = progress.value?.targetPath
  if (!target) return
  const res = await appLauncherOpenAbsolutePath(parentDirOf(target))
  if (!res.success) {
    ElMessage.error(res.msg || '打开目录失败')
  }
}

// 取绝对路径的父目录（兼容 / 与 \ 分隔符；targetPath 恒为后端 filepath 产出的绝对路径）
function parentDirOf(path: string): string {
  const idx = Math.max(path.lastIndexOf('\\'), path.lastIndexOf('/'))
  return idx > 0 ? path.slice(0, idx) : path
}

function handleClose() {
  if (exportId.value) {
    clearExportState(exportId.value)
  }
  exportId.value = null
  startError.value = ''
  state.value = false
  emits('closed')
}

// 打开弹窗：复位本次导出状态并载入设置页配置的导出默认目录，等待用户点 [开始导出]（不自动启动）
watch(state, (open) => {
  if (!open) return
  exportId.value = null
  startError.value = ''
  outputDir.value = ''
  void loadDefaultOutputDir()
})

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** i).toFixed(1)} ${units[i]}`
}
</script>

<template>
  <el-dialog
    v-model="state"
    title="导出"
    width="460px"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
    append-to-body
  >
    <div v-if="starting" class="export-dialog-body">
      <el-progress :percentage="0" :indeterminate="true" :duration="2" />
      <div class="export-dialog-hint">正在准备导出…</div>
    </div>

    <div v-else-if="startError" class="export-dialog-body">
      <div class="export-dialog-status export-dialog-status-fail">
        导出启动失败
      </div>
      <div class="export-dialog-msg">{{ startError }}</div>
      <div class="export-dialog-footer">
        <el-button type="primary" @click="handleClose">关闭</el-button>
      </div>
    </div>

    <div v-else-if="progress" class="export-dialog-body">
      <el-progress
        :percentage="progress.percent"
        :status="progress.status === 'done' ? 'success' : progress.status === 'failed' ? 'exception' : undefined"
      />
      <div class="export-dialog-status" :class="{
        'export-dialog-status-done': progress.status === 'done',
        'export-dialog-status-fail': progress.status === 'failed'
      }">
        <template v-if="progress.status === 'running'">正在导出…</template>
        <template v-else-if="progress.status === 'done'">导出完成</template>
        <template v-else>导出失败</template>
      </div>
      <div class="export-dialog-detail">
        <template v-if="progress.status === 'running'">
          已处理 {{ progress.processedFiles }} / {{ progress.totalFiles }} 个文件 · 已写入
          {{ formatBytes(progress.processedBytes) }}
        </template>
        <template v-else-if="progress.status === 'failed'">
          {{ progress.errMsg || '导出失败' }}
        </template>
        <template v-else-if="progress.status === 'done'">
          导出文件：<span class="export-dialog-path">{{ progress.targetPath }}</span>
        </template>
      </div>
      <div class="export-dialog-footer">
        <template v-if="canCancel">
          <el-button @click="handleCancel">取消</el-button>
        </template>
        <template v-else-if="progress.status === 'done'">
          <el-button @click="openExportFile">打开文件</el-button>
          <el-button @click="openExportDir">打开目录</el-button>
          <el-button type="primary" @click="handleClose">关闭</el-button>
        </template>
        <template v-else>
          <el-button type="primary" @click="handleClose">关闭</el-button>
        </template>
      </div>
    </div>

    <div v-else-if="inConfig" class="export-dialog-body">
      <div class="export-dialog-config-label">输出目录</div>
      <div class="export-dialog-config-row">
        <el-input
          :model-value="outputDir"
          placeholder="默认：工作目录"
          readonly
          clearable
          @clear="resetOutputDir"
        />
        <el-button @click="pickOutputDir">浏览…</el-button>
      </div>
      <div class="export-dialog-hint">
        导出产物 library-squirrel-export-*.zip 将写入所选目录；未选择时写入工作目录。
      </div>
      <div class="export-dialog-footer">
        <el-button @click="handleClose">取消</el-button>
        <el-button type="primary" @click="startExport">开始导出</el-button>
      </div>
    </div>

    <div v-else-if="!hasSelection" class="export-dialog-body">
      <div class="export-dialog-status export-dialog-status-fail">未选择任何作品或作品集</div>
      <div class="export-dialog-footer">
        <el-button type="primary" @click="handleClose">关闭</el-button>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped>
.export-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.export-dialog-status {
  font-size: 15px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.export-dialog-status-done {
  color: var(--app-status-task-completed-text);
}

.export-dialog-status-fail {
  color: var(--app-status-task-failed-text);
}

.export-dialog-detail {
  font-size: 13px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.export-dialog-msg {
  font-size: 13px;
  color: var(--app-status-task-failed-text);
}

.export-dialog-path {
  color: var(--app-text-primary);
  font-family: monospace;
  word-break: break-all;
}

.export-dialog-hint {
  font-size: 13px;
  color: var(--app-text-secondary);
}

.export-dialog-config-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.export-dialog-config-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.export-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 4px;
}
</style>
