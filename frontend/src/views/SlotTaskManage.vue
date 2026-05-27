<script setup lang="ts">
import BaseSubpage from './BaseSubpage.vue'
import {onMounted, onUnmounted, Ref, ref, toRaw, watch} from 'vue'
import SlotSearchTable from '../components/common/SlotSearchTable.vue'
import DialogMode from '../model/util/DialogMode.ts'
import { ElMessage, ElTag } from 'element-plus'
import { arrayIsEmpty, arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { throttle } from 'lodash'
import { TaskStatusEnum } from '../constants/TaskStatusEnum.ts'
import { getNodeByPath } from '@renderer/utils/TreeUtil.ts'
import TaskDialog from '../components/dialogs/TaskDialog.vue'

import { TaskOperationCodeEnum } from '@renderer/constants/TaskOperationCodeEnum.ts'
import TaskOperationBarActive from '@renderer/components/common/TaskOperationBarActiveV1.vue'
import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { useParentTaskStore } from '@renderer/store/UseParentTaskStore.ts'
import NotificationItem from '@renderer/model/util/NotificationItem.ts'
import { useNotificationStore } from '@renderer/store/UseNotificationStore.ts'
import { siteQuerySelectItemPageBySiteName } from '@renderer/apis/http'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { useTourStatesStore } from '@renderer/store/UseTourStatesStore.ts'
import { fileSysUtilApi, taskApi, pluginTaskUrlListenerApi } from '@renderer/apis/http'
import {TaskQueryDTO} from '@bindings/github.com/library-squirrel/backend/task/models'
import {QueryAttribute, SortOrder} from '@bindings/github.com/library-squirrel/backend/base/query/models'
import type { PluginWithContributionVO } from '@renderer/apis/http/wrappers/pluginTaskUrlListener'
import {Page} from "@bindings/github.com/library-squirrel/backend/base/model";
import {TaskProgressTreeDTO} from "@bindings/github.com/library-squirrel/backend/base/model/dto";
import {newPage} from "@renderer/utils/Pager.ts";

// onMounted
onMounted(() => {
  taskManageSearchTable.value.doSearch()
})

// 事件
const emits = defineEmits(['openReplaceResConfirmDialog'])

// 变量
const taskManageSearchTable = ref()
const localImportButton = ref()
const siteDownloadButton = ref()
const dataList: Ref<TaskProgressTreeDTO[]> = ref([])
const page: Ref<Page<TaskProgressTreeDTO>> = ref(newPage<TaskProgressTreeDTO>())
const taskSearchParams: Ref<TaskQueryDTO> = ref(new TaskQueryDTO())
const sort: Ref<{ prop: string; order: 'ascending' | 'descending' | null }> = ref({ prop: '', order: null })
let refreshing: boolean = false
const throttleRefreshTask = throttle(() => refreshTask(), 500, { leading: true, trailing: true })
const dialogData: Ref<TaskProgressTreeDTO> = ref(new TaskProgressTreeDTO())
const taskDialogState: Ref<boolean> = ref(false)
const downloadDialogState: Ref<boolean> = ref(false)
const downloadMode: Ref<boolean> = ref(true)
const downloadInputPlaceholder: Ref<string> = ref('')
const sourceUrl: Ref<string> = ref('')
const supportedPluginListenerList: Ref<PluginWithContributionVO[]> = ref([])
const supportStatus: Ref<string> = ref('')
const taskStore = useTaskStore()
const parentTaskStore = useParentTaskStore()


// 监听 store 变化，同步更新行数据中的 status
const unwatchTaskStore = watch(
  () => taskStore.tasks,
  (tasks) => {
    tasks.forEach((storeObj, taskId) => {
      if (notNullish(storeObj.task?.status)) {
        const row = findRowByTaskId(taskId)
        if (notNullish(row?.taskProgress?.task) && row!.taskProgress!.task!.status !== storeObj.task.status) {
          row!.taskProgress!.task!.status = storeObj.task.status
        }
      }
    })
  },
  { deep: true }
)

const unwatchParentTaskStore = watch(
  () => parentTaskStore.parentTasks,
  (parentTasks) => {
    parentTasks.forEach((storeTask, taskId) => {
      if (notNullish(storeTask.status)) {
        const row = findRowByTaskId(taskId)
        if (notNullish(row?.taskProgress?.task) && row!.taskProgress!.task!.status !== storeTask.status) {
          row!.taskProgress!.task!.status = storeTask.status
        }
      }
    })
  },
  { deep: true }
)

onUnmounted(() => {
  unwatchTaskStore()
  unwatchParentTaskStore()
})

function findRowByTaskId(taskId: number): TaskProgressTreeDTO | undefined {
  return getNodeByPath(
    dataList.value,
    taskId,
    (task) => task.taskProgress?.task?.id,
    (task) => task.children as TaskProgressTreeDTO[]
  )
}

// 状态渲染辅助
const invalidStatus = -1
const statusTagTypeMap: Record<number, 'success' | 'warning' | 'info' | 'primary' | 'danger'> = {
  [invalidStatus]: 'primary',
  [TaskStatusEnum.CREATED]: 'primary',
  [TaskStatusEnum.PROCESSING]: 'warning',
  [TaskStatusEnum.WAITING]: 'warning',
  [TaskStatusEnum.PAUSING]: 'warning',
  [TaskStatusEnum.PAUSED]: 'info',
  [TaskStatusEnum.STOPPING]: 'warning',
  [TaskStatusEnum.FINISHED]: 'success',
  [TaskStatusEnum.PARTLY_FINISHED]: 'success',
  [TaskStatusEnum.FAILED]: 'danger',
  [TaskStatusEnum.WAITING_FOR_INPUT]: 'warning'
}
const statusTextMap: Record<number, string> = {
  [invalidStatus]: '已创建',
  [TaskStatusEnum.CREATED]: '已创建',
  [TaskStatusEnum.PROCESSING]: '进行中',
  [TaskStatusEnum.WAITING]: '等待中',
  [TaskStatusEnum.PAUSING]: '暂停中',
  [TaskStatusEnum.PAUSED]: '已暂停',
  [TaskStatusEnum.STOPPING]: '停止中',
  [TaskStatusEnum.FINISHED]: '完成',
  [TaskStatusEnum.PARTLY_FINISHED]: '部分完成',
  [TaskStatusEnum.FAILED]: '失败',
  [TaskStatusEnum.WAITING_FOR_INPUT]: '等待确认'
}

function formatDatetime(timestamp: number | null | undefined): string {
  if (!timestamp) return '-'
  const d = new Date(timestamp)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function getStatus(row: TaskProgressTreeDTO): number {
  const taskId = row.taskProgress?.task?.id
  const rowStatus = row.taskProgress?.task?.status
  if (isNullish(taskId)) return rowStatus ?? invalidStatus
  const storeStatus = (row.hasChildren ? parentTaskStore : taskStore).getTask(taskId)?.status
  return storeStatus ?? rowStatus ?? invalidStatus
}

// 方法
async function createTaskFromSource() {
  const notificationItem = new NotificationItem()
  notificationItem.title = `正在根据【${sourceUrl.value}】创建任务`
  const notificationStore = useNotificationStore()
  const notificationId = notificationStore.add(notificationItem)
  taskApi.taskCreateByUrl(sourceUrl.value)
    .then((response) => {
      const data = response.data
      taskManageSearchTable.value.doSearch()
      if (data.succeed) {
        useNotificationStore().remove(notificationId, { type: 'success', msg: `成功创建了 ${data.addedQuantity} 个任务` })
      } else {
        useNotificationStore().remove(notificationId, { type: 'error', msg: '创建失败，' + (data.msg || '未知错误') })
      }
    })
    .catch((e: Error) => {
      useNotificationStore().remove(notificationId, { type: 'error', msg: e.message })
    })
  downloadDialogState.value = false
  // 移除url
  sourceUrl.value = ''
  // 移除url支持情况文本
  supportStatus.value = ''
}

async function taskQueryParentPage(page: Page<TaskProgressTreeDTO>): Promise<Page<TaskProgressTreeDTO>> {
  const query = toRaw(taskSearchParams.value)
  if (sort.value.prop && sort.value.order) {
    const orderField = sort.value.prop as keyof TaskQueryDTO
    ;(query as any)[orderField] = {
      value: null,
      order: sort.value.order === 'ascending' ? SortOrder.OrderAsc : SortOrder.OrderDesc,
      priority: -1
    }
  }
  query.createTime = { value: null, order: SortOrder.OrderDesc, priority: 0 }
  query.updateTime = { value: null, order: SortOrder.OrderDesc, priority: 1 }
  try {
    const response = await taskApi.taskQueryParentPage(page, query)
    return response.data
  } catch (e: any) {
    ElMessage.error(e.message)
    throw e
  }
}

async function load(row: TaskProgressTreeDTO): Promise<TaskProgressTreeDTO[]> {
  const parentId = row.taskProgress?.task?.id
  if (isNullish(parentId)) {
    ElMessage.error('加载失败，父任务id不能为空')
    return []
  }
  const query = new TaskQueryDTO({pid: new QueryAttribute({value: parentId})})
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

function getRowTaskId(row: any): number {
  return Number(row?.taskProgress?.task?.id ?? row?.id ?? 0)
}

function rowClassName(data: { row: unknown; rowIndex: number }) {
  const row = data.row as TaskProgressTreeDTO
  const task = row.taskProgress?.task
  if (row.hasChildren || isNullish(task?.pid) || task?.pid === 0) {
    return 'task-manage-search-table-parent-row'
  } else {
    return 'task-manage-search-table-child-row'
  }
}

async function handleOperationButtonClicked(row: TaskProgressTreeDTO, code: TaskOperationCodeEnum) {
  switch (code) {
    case TaskOperationCodeEnum.VIEW:
      dialogData.value = row
      taskDialogState.value = true
      break
    case TaskOperationCodeEnum.START:
      await startTask(row, false)
      await refreshTask()
      break
    case TaskOperationCodeEnum.PAUSE:
      taskApi.taskPauseTree(getRowTaskId(row))
      await refreshTask()
      break
    case TaskOperationCodeEnum.RESUME:
      taskApi.taskResumeTree(getRowTaskId(row))
      await refreshTask()
      break
    case TaskOperationCodeEnum.RETRY:
      await startTask(row, true)
      await refreshTask()
      break
    case TaskOperationCodeEnum.CANCEL:
      taskApi.taskStopTree(getRowTaskId(row))
      break
    case TaskOperationCodeEnum.DELETE:
      await deleteTask(getRowTaskId(row))
      break
    case TaskOperationCodeEnum.CONFIRM_REPLACE_RES:
      emits('openReplaceResConfirmDialog')
      break
    default:
      break
  }
}

async function selectDir(openFile: boolean) {
  try {
    const response = await fileSysUtilApi.fileSysUtilDirSelect(openFile, true)
    const dirSelectResult = response.data as { canceled: boolean; filePaths: string[] }
    if (!dirSelectResult.canceled) {
      for (const dir of dirSelectResult.filePaths) {
        sourceUrl.value = dir
      }
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

async function refreshTask() {
  if (!refreshing) {
    refreshing = true
    const getActiveTaskIds = (): number[] => {
      const visibleRowsId = taskManageSearchTable.value.getVisibleRows(200, 200).map((id: string) => Number(id))
      return visibleRowsId.filter((id: number) => {
        const taskProgressTree = getNodeByPath(dataList.value, id, (task) => task.taskProgress?.task?.id, (task) => (task.children as TaskProgressTreeDTO[]))
        const task = taskProgressTree?.taskProgress?.task
        return (
          notNullish(task) &&
          (task.status === TaskStatusEnum.WAITING ||
            task.status === TaskStatusEnum.PROCESSING ||
            task.status === TaskStatusEnum.PAUSED ||
            task.status === TaskStatusEnum.PAUSING ||
            task.status === TaskStatusEnum.STOPPING ||
            parentTaskStore.hasTask(task.id) ||
            taskStore.hasTask(task.id))
        )
      })
    }

    let activeTaskIds = getActiveTaskIds()

    while (activeTaskIds.length > 0) {
      await refreshRows(activeTaskIds)
      await new Promise((resolve) => setTimeout(resolve, 500))
      if (isNullish(taskManageSearchTable.value)) {
        break
      }
      activeTaskIds = getActiveTaskIds()
    }
    refreshing = false
  }
}

async function refreshRows(ids: number[]) {
  const scheduleMap = new Map<number, { status: number | null | undefined; total: number | null | undefined; finished: number | null | undefined }>()
  const notFoundList: number[] = []
  for (const id of ids) {
    const storeTask = parentTaskStore.getTask(id) ?? taskStore.getTask(id)
    if (notNullish(storeTask)) {
      scheduleMap.set(storeTask.id!, { status: storeTask.status, total: storeTask.total, finished: storeTask.finished })
      continue
    }
    notFoundList.push(id)
  }
  if (arrayNotEmpty(notFoundList)) {
    try {
      const response = await taskApi.taskListStatus(notFoundList)
      if (arrayNotEmpty(response.data)) {
        for (const schedule of response.data) {
          const taskId = schedule.task?.id
          if (notNullish(taskId)) {
            scheduleMap.set(taskId, { status: schedule.task?.status, total: schedule.total, finished: schedule.finished })
          }
        }
      }
    } catch {
      // 查询状态失败，静默处理
    }
  }
  scheduleMap.forEach(({ status, total, finished }, taskId) => {
    const row = getNodeByPath(dataList.value, taskId, (task) => task.taskProgress?.task?.id, (task) => (task.children as TaskProgressTreeDTO[]))
    if (notNullish(row?.taskProgress)) {
      if (notNullish(row.taskProgress.task)) {
        row.taskProgress.task.status = isNullish(status) ? invalidStatus : status
      }
      row.taskProgress.total = total
      row.taskProgress.finished = finished
    }
  })
}

function handleScroll() {
  throttleRefreshTask()
}

async function startTask(row: TaskProgressTreeDTO, retry: boolean): Promise<boolean> {
  const apiCall = retry ? taskApi.taskRetryTree : taskApi.taskStartTree
  try {
    const response = await apiCall(getRowTaskId(row))
    return isNullish(response?.success) ? false : response.success
  } catch (e) {
    return false
  }
  // if (row.taskProgress?.task) {
  //   row.taskProgress.task.status = TaskStatusEnum.WAITING
  // }
  // if (row.taskProgress?.task?.hasChild && notNullish(row.children)) {
  //   row.children.filter(notNullish).forEach((child) => {
  //     if (child.taskProgress?.task) {
  //       child.taskProgress.task.status = TaskStatusEnum.WAITING
  //     }
  //   })
  // }
}

async function deleteTask(id: number) {
  try {
    await taskApi.taskDelete(id)
    await taskManageSearchTable.value.doSearch()
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

async function getUrlMatchedPlugin(url: string): Promise<PluginWithContributionVO[]> {
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
  <base-subpage>
    <div class="task-manage-local-import-button-row">
      <div class="task-manage-local-import-button-col">
        <el-button
          ref="localImportButton"
          size="large"
          type="danger"
          icon="Monitor"
          @click="(event: PointerEvent) => handleDownloadDialog(event, true)"
        >
          从本地导入
        </el-button>
      </div>
      <div class="task-manage-site-import-button-col">
        <el-button
          ref="siteDownloadButton"
          v-model="downloadDialogState"
          size="large"
          type="primary"
          icon="Link"
          @click="(event: PointerEvent) => handleDownloadDialog(event, false)"
        >
          从站点下载
        </el-button>
      </div>
    </div>
    <div class="task-manage-search-table-wrapper">
      <slot-search-table
        ref="taskManageSearchTable"
        v-model:data="dataList"
        v-model:page="page"
        v-model:sort="sort"
        class="task-manage-search-table"
        :selectable="true"
        :search="taskQueryParentPage"
        data-key="taskProgress.task.id"
        :row-class-name="rowClassName"
        :tree-lazy="true"
        :tree-load="load"
        :multi-select="true"
        :page-sizes="[10, 20, 30, 50, 100]"
        :tree-data="true"
        @scroll="handleScroll"
        @sort-change="taskManageSearchTable.doSearch()"
      >
        <template #toolbarMain>
          <el-row class="task-manage-search-bar">
            <el-col :span="14">
              <el-input v-model="taskSearchParams.taskName.value" placeholder="输入任务名称" clearable />
            </el-col>
            <el-col :span="6">
              <auto-load-select
                v-model="taskSearchParams.siteId.value"
                :load="siteQuerySelectItemPageBySiteName"
                placeholder="选择站点"
                remote
                filterable
                clearable
              >
                <template #default="{ list }">
                  <el-option v-for="item in list" :key="item.value" :value="item.value" :label="item.label" />
                </template>
              </auto-load-select>
            </el-col>
            <el-col :span="4">
              <el-select v-model="taskSearchParams.status.value" placeholder="选择状态" clearable>
                <el-option :value="TaskStatusEnum.CREATED" label="已创建"></el-option>
                <el-option :value="TaskStatusEnum.WAITING" label="等待中"></el-option>
                <el-option :value="TaskStatusEnum.PROCESSING" label="进行中"></el-option>
                <el-option :value="TaskStatusEnum.PAUSED" label="暂停"></el-option>
                <el-option :value="TaskStatusEnum.FINISHED" label="完成"></el-option>
                <el-option :value="TaskStatusEnum.PARTLY_FINISHED" label="部分完成"></el-option>
                <el-option :value="TaskStatusEnum.FAILED" label="失败"></el-option>
              </el-select>
            </el-col>
          </el-row>
        </template>
        <!-- 名称 -->
        <el-table-column label="名称" min-width="380" show-overflow-tooltip sortable="custom" align="left">
          <template #header>
            <el-tag>名称</el-tag>
          </template>
          <template #default="{ row }">
            {{ row.taskProgress?.task?.taskName ?? '-' }}
          </template>
        </el-table-column>
        <!-- 站点 -->
        <el-table-column label="站点" width="100" align="center">
          <template #header>
            <el-tag>站点</el-tag>
          </template>
          <template #default="{ row }">
            {{ row.taskProgress?.siteName ?? '-' }}
          </template>
        </el-table-column>
        <!-- 创建时间 -->
        <el-table-column label="创建时间" width="165" show-overflow-tooltip sortable="custom" align="center">
          <template #header>
            <el-tag>创建时间</el-tag>
          </template>
          <template #default="{ row }">
            {{ formatDatetime(row.taskProgress?.task?.createTime) }}
          </template>
        </el-table-column>
        <!-- URL -->
        <el-table-column label="url" width="380" show-overflow-tooltip sortable="custom" align="center">
          <template #header>
            <el-tag>url</el-tag>
          </template>
          <template #default="{ row }">
            {{ row.taskProgress?.task?.url ?? '-' }}
          </template>
        </el-table-column>
        <!-- 状态 -->
        <el-table-column label="状态" width="110" fixed="right" sortable="custom" align="center">
          <template #header>
            <el-tag>状态</el-tag>
          </template>
          <template #default="{ row }">
            <div style="display: flex; align-items: center; justify-content: center">
              <el-tag :type="statusTagTypeMap[getStatus(row)] ?? 'info'">
                {{ statusTextMap[getStatus(row)] ?? '未知' }}
              </el-tag>
            </div>
          </template>
        </el-table-column>
        <!-- 操作列 -->
        <el-table-column fixed="right" :width="163" align="center">
          <template #header>
            <el-tag type="warning">操作</el-tag>
          </template>
          <template #default="{ row }">
            <task-operation-bar-active :row="row" :button-clicked="handleOperationButtonClicked" />
          </template>
        </el-table-column>
      </slot-search-table>
    </div>
    <template #dialog>
      <task-dialog v-model:state="taskDialogState" v-model:form-data="dialogData" :mode="DialogMode.VIEW" width="90%" />
      <el-dialog v-model="downloadDialogState" width="80%">
        <div v-if="downloadMode" class="task-manage-download-dialog-local-button-container">
          <el-button type="primary" icon="FolderOpened" @click="selectDir(false)">选择文件夹导入</el-button>
          <el-button type="primary" icon="Document" @click="selectDir(true)">选择单个文件导入</el-button>
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
            <el-button type="primary" :disabled="arrayIsEmpty(supportedPluginListenerList)" @click="createTaskFromSource">
              创建任务
            </el-button>
            <template #content> 当前输入的url不受支持 </template>
          </el-tooltip>
          <el-button @click="downloadDialogState = false">取消</el-button>
        </template>
      </el-dialog>
      <el-tour v-model="useTourStatesStore().tourStates.taskTour" @finish="useTourStatesStore().tourStates.getCallback('taskTour')">
        <el-tour-step description="这里可以创建和开始任务" />
        <el-tour-step :target="localImportButton.$el" description="在这里从本地创建任务，可以选择目录或单个文件" />
        <el-tour-step
          :target="siteDownloadButton.$el"
          description="在这里输入url从站点创建任务，只能使用受支持的url（可以通过安装插件扩展受支持的url）"
        />
      </el-tour>
    </template>
  </base-subpage>

</template>

<style scoped>
.task-manage-local-import-button-row {
  height: 50px;
  width: 100%;
  display: flex;
  align-items: center;
}
.task-manage-local-import-button-col {
  margin: auto;
}
.task-manage-site-import-button-col {
  margin: auto;
}
.task-manage-search-table-wrapper {
  background: #f4f4f4;
  border-radius: 6px;
  width: calc(100% - 20px);
  height: calc(100% - 20px - 50px);
  padding: 5px;
  margin: 5px;
}
.task-manage-search-table {
  height: 100%;
  width: 100%;
}
:deep(.el-table .task-manage-search-table-parent-row) {
  font-weight: bold;
}
:deep(.el-table .task-manage-search-table-child-row > :nth-child(3) > .cell :nth-child(1)) {
  transform: translateX(7px);
}
.task-manage-search-bar {
  flex-grow: 1;
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
