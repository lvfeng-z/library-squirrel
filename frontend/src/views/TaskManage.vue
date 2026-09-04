<script setup lang="ts">
import BaseView from './BaseView.vue'
import { onMounted, Ref, ref } from 'vue'
import DialogMode from '../model/util/DialogMode.ts'
import { ElMessage } from 'element-plus'
import { arrayIsEmpty, arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import TaskDialog from '../components/dialogs/TaskDialog.vue'
import TaskList from '../components/common/TaskList.vue'

import { useNotificationStore } from '@renderer/store/UseNotificationStore.ts'
import { useTourTargets } from '@renderer/composables/useTourTargets'
import { fileSysUtilApi, taskApi, pluginTaskUrlListenerApi } from '@renderer/apis/http'
import { TaskQueryDTO } from '@bindings/github.com/library-squirrel/backend/task/models'
import { QueryAttribute } from '@bindings/github.com/library-squirrel/backend/base/query/models'
import type { PluginWithExtensionVO } from '@renderer/apis/http/wrappers/pluginTaskUrlListener'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model'
import { TaskProgressTreeDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { newPage } from '@renderer/utils/Pager.ts'

// onMounted
onMounted(() => {
  taskListRef.value.doSearch()
})

// 变量
const taskListRef = ref()
const localImportButton = ref()
const siteDownloadButton = ref()
// 向导目标注册
const { register: registerTourTarget } = useTourTargets()
registerTourTarget('taskManage.localImportButton', localImportButton)
registerTourTarget('taskManage.siteDownloadButton', siteDownloadButton)
const dataList: Ref<TaskProgressTreeDTO[]> = ref([])
const page: Ref<Page<TaskProgressTreeDTO>> = ref(newPage<TaskProgressTreeDTO>())
const dialogData: Ref<TaskProgressTreeDTO> = ref(new TaskProgressTreeDTO())
const taskDialogState: Ref<boolean> = ref(false)
const downloadDialogState: Ref<boolean> = ref(false)
const downloadMode: Ref<boolean> = ref(true)
const downloadInputPlaceholder: Ref<string> = ref('')
const sourceUrl: Ref<string> = ref('')
const supportedPluginListenerList: Ref<PluginWithExtensionVO[]> = ref([])
const supportStatus: Ref<string> = ref('')

// 方法
// 查看行：打开任务详情弹窗
function onViewRow(row: TaskProgressTreeDTO) {
  dialogData.value = row
  taskDialogState.value = true
}

// 父任务分页查询（排序与默认排序由 TaskList 内部构建 query）
async function taskQueryParentPage(p: Page<TaskProgressTreeDTO>, query: TaskQueryDTO): Promise<Page<TaskProgressTreeDTO> | undefined> {
  try {
    const response = await taskApi.taskQueryParentPage(p, query)
    return response.data
  } catch (e: any) {
    ElMessage.error(e.message)
    throw e
  }
}

// 树形懒加载：展开父任务时拉取其子任务并挂回数据树
async function load(row: TaskProgressTreeDTO): Promise<TaskProgressTreeDTO[]> {
  const parentId = row.taskProgress?.task?.id
  if (isNullish(parentId)) {
    ElMessage.error('加载失败，父任务id不能为空')
    return []
  }
  const query = new TaskQueryDTO({ pid: new QueryAttribute({ value: parentId }) })
  const tempPage = newPage<TaskProgressTreeDTO>()

  const response = await taskApi.taskQueryChildrenTaskPage(tempPage, query)
  try {
    const resultPage = response.data
    const data = (resultPage.data ?? []).filter((d): d is TaskProgressTreeDTO => d !== null)
    const parent = dataList.value.find((task) => parentId === task.taskProgress?.task?.id)
    if (notNullish(parent)) {
      parent.children = data
    }
    return data
  } catch (e: any) {
    ElMessage.error(e.message)
    return []
  }
}

async function createTaskFromSource() {
  const notificationId = useNotificationStore().add({
    level: 'info',
    category: 'task',
    title: `正在根据【${sourceUrl.value}】创建任务`,
    statusText: '创建中',
    route: { name: 'taskManage' }
  })
  taskApi.taskCreateByUrl(sourceUrl.value)
    .then((response) => {
      const data = response.data
      taskListRef.value.doSearch()
      if (data.succeed) {
        useNotificationStore().update(notificationId, { terminal: true, level: 'success', statusText: data.msg || '创建成功' })
        ElMessage.success(data.msg || '创建成功')
      } else {
        useNotificationStore().update(notificationId, { terminal: true, level: 'error', statusText: '创建失败', exception: data.msg || '未知错误' })
        ElMessage.error('创建失败，' + (data.msg || '未知错误'))
      }
    })
    .catch((e: Error) => {
      useNotificationStore().update(notificationId, { terminal: true, level: 'error', statusText: '创建失败', exception: e.message })
      ElMessage.error(e.message)
    })
  downloadDialogState.value = false
  // 移除url
  sourceUrl.value = ''
  // 移除url支持情况文本
  supportStatus.value = ''
}

// 选择文件夹导入
async function selectDirectory() {
  try {
    const response = await fileSysUtilApi.fileSysUtilSelectDirectory('选择文件夹')
    const dirSelectResult = response.data as { canceled: boolean; filePaths: string[] } | undefined
    if (dirSelectResult && !dirSelectResult.canceled && arrayNotEmpty(dirSelectResult.filePaths)) {
      sourceUrl.value = dirSelectResult.filePaths[0]
      await handleSourceUrlInput()
    }
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}
// 选择文件导入
async function selectFile() {
  try {
    const response = await fileSysUtilApi.fileSysUtilSelectFile('选择文件')
    const dirSelectResult = response.data as { canceled: boolean; filePaths: string[] } | undefined
    if (dirSelectResult && !dirSelectResult.canceled && arrayNotEmpty(dirSelectResult.filePaths)) {
      sourceUrl.value = dirSelectResult.filePaths[0]
      await handleSourceUrlInput()
    }
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

function handleDownloadDialog(_event: PointerEvent, isLocal: boolean, newState?: boolean) {
  downloadMode.value = isLocal
  if (isLocal) {
    downloadInputPlaceholder.value = '输入文件路径'
  } else {
    downloadInputPlaceholder.value = '输入url'
  }
  if (notNullish(newState)) {
    downloadDialogState.value = newState
  } else {
    downloadDialogState.value = !downloadDialogState.value
  }
}

async function getUrlMatchedPlugin(url: string): Promise<PluginWithExtensionVO[]> {
  try {
    const response = await pluginTaskUrlListenerApi.listListener(url)
    return response.data ?? []
  } catch {
    return []
  }
}

async function getSupportedText() {
  const supportedPlugins = await getUrlMatchedPlugin(sourceUrl.value)
  if (arrayIsEmpty(supportedPlugins)) {
    return ''
  } else {
    const pluginNumber = supportedPlugins.length
    if (pluginNumber > 5) {
      const texts = supportedPlugins
        .slice(0, 5)
        .map((supportedPlugin) => `${supportedPlugin.author}-${supportedPlugin.name}-${supportedPlugin.version}`)
      return texts.join('、') + `等${pluginNumber}个插件支持这个url`
    } else {
      const texts = supportedPlugins.map(
        (supportedPlugin) => `${supportedPlugin.author}-${supportedPlugin.name}-${supportedPlugin.version}`
      )
      return texts.join('、') + ` 共${pluginNumber}个插件支持这个url`
    }
  }
}

async function handleSourceUrlInput() {
  supportedPluginListenerList.value = await getUrlMatchedPlugin(sourceUrl.value)
  supportStatus.value = await getSupportedText()
}
</script>

<template>
  <base-view>
    <div class="task-manage-search-table-wrapper">
      <task-list
        ref="taskListRef"
        v-model:data="dataList"
        v-model:page="page"
        class="task-manage-search-table"
        toolbar-radius="var(--app-radius)"
        data-radius="var(--app-radius)"
        :search="taskQueryParentPage"
        :selectable="true"
        :multi-select="true"
        :tree-data="true"
        :tree-lazy="true"
        :tree-load="load"
        @view="onViewRow"
      >
        <template #toolbarPrefix>
          <!-- 第一行：入口大按钮（全宽元素强制分行，保持 large 尺寸） -->
          <div class="task-manage-toolbar-action-row">
            <el-button
              ref="localImportButton"
              size="large"
              type="primary"
              icon="Monitor"
              @click="(event: PointerEvent) => handleDownloadDialog(event, true)"
            >
              从本地导入
            </el-button>
            <el-button
              ref="siteDownloadButton"
              class="source-site-button"
              size="large"
              type="primary"
              icon="Link"
              @click="(event: PointerEvent) => handleDownloadDialog(event, false)"
            >
              从站点下载
            </el-button>
          </div>
        </template>
      </task-list>
    </div>
    <template #dialog>
      <task-dialog
        v-model:state="taskDialogState"
        v-model:form-data="dialogData"
        :mode="DialogMode.VIEW"
        width="90%"
      />
      <el-dialog
        v-model="downloadDialogState"
        width="80%"
      >
        <div
          v-if="downloadMode"
          class="task-manage-download-dialog-local-button-container"
        >
          <el-button
            type="primary"
            icon="FolderOpened"
            @click="selectDirectory()"
          >
            选择文件夹导入
          </el-button>
          <el-button
            type="primary"
            icon="Document"
            @click="selectFile()"
          >
            选择单个文件导入
          </el-button>
        </div>
        <el-input
          v-model="sourceUrl"
          type="textarea"
          :rows="6"
          :placeholder="downloadInputPlaceholder"
          @input="handleSourceUrlInput"
        />
        <span class="task-manage-download-dialog-supported-tips"> {{ supportStatus }} </span>
        <template #footer>
          <el-tooltip :disabled="arrayNotEmpty(supportedPluginListenerList)">
            <el-button
              type="primary"
              :disabled="arrayIsEmpty(supportedPluginListenerList)"
              @click="createTaskFromSource"
            >
              创建任务
            </el-button>
            <template #content>
              当前输入的url不受支持
            </template>
          </el-tooltip>
          <el-button @click="downloadDialogState = false">
            取消
          </el-button>
        </template>
      </el-dialog>
    </template>
  </base-view>
</template>

<style scoped>
/* 工具栏第一行：入口大按钮（全宽强制分行，筛选区落第二行；居中排布、按钮间留间隔） */
.task-manage-toolbar-action-row {
  width: 100%;
  display: flex;
  justify-content: center;
  gap: 30%;
}
/* 从站点下载按钮：走"浅底深字"（区别于本地导入按钮的 primary 深底白字）——
   底用 source-site-bg（淡主题色）、字用 source-site-text（primary 深字），体现站点来源"较浅"。
   本地导入按钮用 type="primary" 即天然等于 source-local（深底白字）。
   经 EP 按钮 CSS 变量覆盖各态，hover/active 底色逐级加深、字色保持深色。 */
.source-site-button {
  --el-button-bg-color: var(--app-status-source-site-bg);
  --el-button-border-color: var(--app-status-source-site-border);
  --el-button-text-color: var(--app-status-source-site-text);
  --el-button-hover-bg-color: color-mix(in srgb, var(--app-color-primary) 16%, white);
  --el-button-hover-border-color: color-mix(in srgb, var(--app-color-primary) 22%, white);
  --el-button-hover-text-color: var(--app-status-source-site-text);
  --el-button-active-bg-color: color-mix(in srgb, var(--app-color-primary) 22%, white);
  --el-button-active-border-color: color-mix(in srgb, var(--app-color-primary) 28%, white);
  --el-button-active-text-color: var(--app-status-source-site-text);
}
.task-manage-search-table-wrapper {
  /* 容器不带底色：按钮行与任务列表各自成 surface 卡；间距纯 margin（总边距 10px 不变） */
  display: flex;
  flex-direction: column;
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  margin: 10px;
}
.task-manage-search-table {
  /* 高度由 flex 分配（原 calc(100% - 50px) 按按钮行硬补偿，含卡片间距后改为 flex 自适应） */
  flex: 1;
  min-height: 0;
  width: 100%;
}
.task-manage-download-dialog-local-button-container {
  display: flex;
  justify-content: center;
  margin-bottom: 10px;
}
.task-manage-download-dialog-supported-tips {
  color: rgb(245, 108, 108);
}
</style>
