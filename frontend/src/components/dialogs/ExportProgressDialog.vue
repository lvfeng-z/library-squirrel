<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { ElLink, ElMessage } from 'element-plus'
import { exportStartExport } from '@renderer/apis/http/wrappers/export'
import { fileSysUtilSelectDirectory } from '@renderer/apis/http/wrappers/fileSysUtil'
import { settingsGetSettings } from '@renderer/apis/http/wrappers/settings'
import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { gotoPage } from '@renderer/utils/PageUtil'
import { PageEnum } from '@renderer/model/constant/PageEnum.ts'
import { arrayNotEmpty } from '@renderer/utils/CommonUtil'

// model
// 弹窗开关（MainView 控制：打开后确认输出目录再创建导出任务）
const state = defineModel<boolean>('state', { required: true })

// props
// 选中作品/作品集 id 列表（决策5：前端把选中 id 列表传给后端收集）
const props = defineProps<{
  workIds: number[]
  workSetIds: number[]
}>()

// StartExport IPC 进行中（防重复触发，按钮 loading）
const starting = ref(false)
// StartExport 同步失败信息（空选择/后端前置错误）
const startError = ref('')
// 本次导出的输出目录（'' = 使用工作目录）；初始值来自设置页配置的导出默认目录（exportSettings.outputDir），
// 弹窗内浏览选择为仅本次生效的临时覆盖，不写回设置
const outputDir = ref('')

// 是否还有可消费的选中（空选择时后端会报错，前端也可先行提示）
const hasSelection = computed(() => arrayNotEmpty(props.workIds) || arrayNotEmpty(props.workSetIds))

// 创建导出任务：成功后关闭弹窗并提示跳转入口（进度与终态由导出任务视图承载）
async function startExport() {
  if (starting.value) return
  starting.value = true
  startError.value = ''
  try {
    const result = await exportStartExport(props.workIds, props.workSetIds, outputDir.value)
    // 登记任务类型：任务通知条目按类型路由到导出视图
    useTaskStore().setTaskType(result.data.taskId, 'export')
    closeDialog()
    ElMessage({
      type: 'success',
      duration: 6000,
      message: h('span', null, [
        '已创建导出任务，进度见任务面板 ',
        h(
          ElLink,
          {
            type: 'primary',
            style: 'vertical-align: baseline; font-size: 13px;',
            onClick: () => {
              void gotoPage(PageEnum.ExportTaskManage)
            }
          },
          { default: () => '前往查看' }
        )
      ])
    })
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

function closeDialog() {
  startError.value = ''
  state.value = false
}

// 打开弹窗：复位本次导出状态并载入设置页配置的导出默认目录，等待用户点 [开始导出]
watch(state, (open) => {
  if (!open) return
  startError.value = ''
  outputDir.value = ''
  void loadDefaultOutputDir()
})
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
    <div
      v-if="startError"
      class="export-dialog-body"
    >
      <div class="export-dialog-status export-dialog-status-fail">
        导出任务创建失败
      </div>
      <div class="export-dialog-msg">
        {{ startError }}
      </div>
      <div class="export-dialog-footer">
        <el-button
          type="primary"
          @click="closeDialog"
        >
          关闭
        </el-button>
      </div>
    </div>

    <div
      v-else-if="!hasSelection"
      class="export-dialog-body"
    >
      <div class="export-dialog-status export-dialog-status-fail">
        未选择任何作品或作品集
      </div>
      <div class="export-dialog-footer">
        <el-button
          type="primary"
          @click="closeDialog"
        >
          关闭
        </el-button>
      </div>
    </div>

    <div
      v-else
      class="export-dialog-body"
    >
      <div class="export-dialog-config-label">
        输出目录
      </div>
      <div class="export-dialog-config-row">
        <el-input
          :model-value="outputDir"
          placeholder="默认：工作目录"
          readonly
          clearable
          @clear="resetOutputDir"
        />
        <el-button @click="pickOutputDir">
          浏览…
        </el-button>
      </div>
      <div class="export-dialog-hint">
        导出产物 library-squirrel-export-*.zip 将写入所选目录；未选择时写入工作目录。
      </div>
      <div class="export-dialog-footer">
        <el-button @click="closeDialog">
          取消
        </el-button>
        <el-button
          type="primary"
          :loading="starting"
          @click="startExport"
        >
          开始导出
        </el-button>
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

.export-dialog-status-fail {
  color: var(--app-status-task-failed-text);
}

.export-dialog-msg {
  font-size: 13px;
  color: var(--app-status-task-failed-text);
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
