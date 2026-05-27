<script setup lang="ts">
import BaseSubpage from './BaseSubpage.vue'
import {h, onMounted, Ref, ref, toRaw, VNode} from 'vue'
import SearchTable from '../components/common/SearchTable.vue'
import { Thead } from '../model/util/Thead.ts'
import DialogMode from '../model/util/DialogMode.ts'
import { ElMessage, ElTag } from 'element-plus'
import { arrayIsEmpty, arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { throttle } from 'lodash'
import { TaskStatusEnum } from '../constants/TaskStatusEnum.ts'
import { getNodeByPath } from '@renderer/utils/TreeUtil.ts'
import TaskDialog from '../components/dialogs/TaskDialog.vue'

import { TaskOperationCodeEnum } from '@renderer/constants/TaskOperationCodeEnum.ts'
import TaskOperationBarActive from '@renderer/components/common/TaskOperationBarActive.vue'
import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { useParentTaskStore } from '@renderer/store/UseParentTaskStore.ts'
import NotificationItem from '@renderer/model/util/NotificationItem.ts'
import { useNotificationStore } from '@renderer/store/UseNotificationStore.ts'
import { siteQuerySelectItemPageBySiteName } from '@renderer/apis/http'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { useTourStatesStore } from '@renderer/store/UseTourStatesStore.ts'
import { fileSysUtilApi, taskApi, pluginTaskUrlListenerApi } from '@renderer/apis/http'
import TaskTreeDTO from '@renderer/model/model/dto/TaskTreeDTO.ts'
import {TaskQueryDTO} from '@bindings/github.com/library-squirrel/backend/task/models'
import TaskScheduleDTO from '@renderer/model/model/dto/TaskScheduleDTO.ts'
import {QueryAttribute, SortOrder} from '@bindings/github.com/library-squirrel/backend/base/query/models'
import Plugin from '@renderer/model/model/entity/Plugin.ts'
import {Page} from "@bindings/github.com/library-squirrel/backend/base/model";
import {TaskProgressTreeDTO} from "@bindings/github.com/library-squirrel/backend/base/model/dto";
import {newPage} from "@renderer/utils/Pager.ts";

// onMounted
onMounted(() => {
  taskManageSearchTable.value.doSearch()
})

// 事件


// 变量
// taskManageSearchTable的组件实例
const taskManageSearchTable = ref()
// 本地导入按钮的实例
const localImportButton = ref()
// 站点导入按钮的实例
const siteDownloadButton = ref()
// 任务SearchTable的数据
const dataList: Ref<TaskProgressTreeDTO[]> = ref([])
// 表头
const thead: Ref<Thead<TaskProgressTreeDTO>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'taskProgress.task.taskName',
    title: '名称',
    hide: false,
    minWidth: 380,
    headerAlign: 'center',
    dataAlign: 'left',
    showOverflowTooltip: true,
    sortable: 'custom'
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'taskProgress.siteName',
    title: '站点',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center'
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'taskProgress.task.createTime',
    title: '创建时间',
    hide: false,
    width: 152,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true,
    sortable: 'custom'
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'taskProgress.task.url',
    title: 'url',
    hide: false,
    width: 380,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true,
    sortable: 'custom'
  }),
  new Thead({
    type: 'custom',
    defaultDisabled: true,
    key: 'taskProgress.task.status',
    title: '状态',
    hide: false,
    width: 110,
    headerAlign: 'center',
    dataAlign: 'center',
    fixed: 'right',
    showOverflowTooltip: false,
    sortable: 'custom',
    editMethod: 'replace',
    render: (data: TaskStatusEnum): VNode => {
      let tagType: 'success' | 'warning' | 'info' | 'primary' | 'danger' | undefined
      let tagText: string | undefined
      switch (data) {
        case TaskStatusEnum.CREATED:
          tagType = 'primary'
          tagText = '已创建'
          break
        case TaskStatusEnum.PROCESSING:
          tagType = 'warning'
          tagText = '进行中'
          break
        case TaskStatusEnum.WAITING:
          tagType = 'warning'
          tagText = '等待中'
          break
        case TaskStatusEnum.PAUSING:
          tagType = 'warning'
          tagText = '暂停中'
          break
        case TaskStatusEnum.PAUSED:
          tagType = 'info'
          tagText = '已暂停'
          break
        case TaskStatusEnum.STOPPING:
          tagType = 'warning'
          tagText = '停止中'
          break
        case TaskStatusEnum.FINISHED:
          tagType = 'success'
          tagText = '完成'
          break
        case TaskStatusEnum.PARTLY_FINISHED:
          tagType = 'success'
          tagText = '部分完成'
          break
        case TaskStatusEnum.FAILED:
          tagType = 'danger'
          tagText = '失败'
          break
        case TaskStatusEnum.WAITING_FOR_INPUT:
          tagType = 'warning'
          tagText = '等待确认'
          break
      }
      const elTag = h(ElTag, { type: tagType }, () => tagText)
      return h('div', { style: { display: 'flex', 'align-items': 'center', 'justify-content': 'center' } }, elTag)
    }
  })
])
// 任务SearchTable的分页
const page: Ref<Page<TaskProgressTreeDTO>> = ref(newPage<TaskProgressTreeDTO>())
// 任务查询的参数
const taskSearchParams: Ref<TaskQueryDTO> = ref(new TaskQueryDTO())
// 改变的行数据
const changedRows: Ref<TaskProgressTreeDTO[]> = ref([])
// 排序配置
const sort: Ref<{ prop: string; order: 'ascending' | 'descending' | null }> = ref({ prop: '', order: null })
// 是否正在刷新数据
let refreshing: boolean = false
// 防抖动refreshTask
const throttleRefreshTask = throttle(() => refreshTask(), 500, { leading: true, trailing: true })
// 当前dialog绑定数据
const dialogData: Ref<TaskTreeDTO> = ref(new TaskTreeDTO())
// 任务详情的dialog开关
const taskDialogState: Ref<boolean> = ref(false)
// 下载dialog的开关
const downloadDialogState: Ref<boolean> = ref(false)
// 下载模式
const downloadMode: Ref<boolean> = ref(true)
// 下载dialog输入框占位文本
const downloadInputPlaceholder: Ref<string> = ref('')
// 资源的url或文件路径
const sourceUrl: Ref<string> = ref('')
// 支持当前url的插件列表
const supportedPluginListenerList: Ref<Plugin[]> = ref([])
// 支持状态
const supportStatus: Ref<string> = ref('')


// 方法
// 根据url或文件路径创建任务
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
  sourceUrl.value = ''
}
// 分页查询父任务的函数
async function taskQueryParentPage(page: Page<TaskProgressTreeDTO>): Promise<Page<TaskProgressTreeDTO>> {
  const query = new TaskQueryDTO()
  // 用户选择的排序优先级最高（priority=-1）
  if (sort.value.prop && sort.value.order) {
    const orderField = sort.value.prop as keyof TaskQueryDTO
    ;(query as any)[orderField] = {
      value: null,
      order: sort.value.order === 'ascending' ? SortOrder.OrderAsc : SortOrder.OrderDesc,
      priority: -1
    }
  }
  // 设置默认排序（用户选择优先级最高，createTime 次之，updateTime 再次）
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
// 懒加载处理函数
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
    // 子任务列表赋值给对应的父任务的children
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
// 更新进度的数据加载函数
async function updateLoad(ids: (number | string)[]): Promise<object[] | undefined> {
  const scheduleList: TaskScheduleDTO[] = []
  const notFoundList: number[] = []
  for (const id of ids) {
    let tempStatus = useParentTaskStore().getTask(Number(id))
    if (notNullish(tempStatus)) {
      scheduleList.push(tempStatus)
      continue
    }
    tempStatus = useTaskStore().getTask(Number(id))
    if (notNullish(tempStatus)) {
      scheduleList.push(tempStatus)
      continue
    }
    notFoundList.push(typeof id === 'string' ? Number(id) : id)
  }
  if (arrayNotEmpty(notFoundList)) {
    try {
      const response = await taskApi.taskListStatus(notFoundList)
      const responseScheduleList = response.data?.map((d: any) => new TaskScheduleDTO(d))
      if (arrayNotEmpty(responseScheduleList)) {
        scheduleList.push(...responseScheduleList)
      }
    } catch {
      // 查询状态失败，静默处理
    }
  }
  if (arrayNotEmpty(scheduleList)) {
    return scheduleList.map((schedule) => ({
      taskProgress: {
        task: { id: schedule.id, status: schedule.status },
        total: schedule.total,
        finished: schedule.finished
      }
    }))
  }
  return undefined
}
// 从行数据中提取任务 ID（兼容 Wails 绑定格式和 DTO 格式）
function getRowTaskId(row: any): number {
  return Number(row?.taskProgress?.task?.id ?? row?.id ?? 0)
}
// 给行添加选择器，用于区分父任务和子任务
function rowClassName(data: { row: unknown; rowIndex: number }) {
  const row = data.row as TaskProgressTreeDTO
  const task = row.taskProgress?.task
  if (row.hasChildren || isNullish(task?.pid) || task?.pid === 0) {
    return 'task-manage-search-table-parent-row'
  } else {
    return 'task-manage-search-table-child-row'
  }
}
// 处理操作栏按钮点击事件
function handleOperationButtonClicked(row: TaskProgressTreeDTO, code: TaskOperationCodeEnum) {
  switch (code) {
    case TaskOperationCodeEnum.VIEW:
      dialogData.value = row
      taskDialogState.value = true
      break
    case TaskOperationCodeEnum.START:
      startTask(row, false)
      refreshTask()
      break
    case TaskOperationCodeEnum.PAUSE:
      taskApi.taskPauseTree(getRowTaskId(row))
      refreshTask()
      break
    case TaskOperationCodeEnum.RESUME:
      taskApi.taskResumeTree(getRowTaskId(row))
      refreshTask()
      break
    case TaskOperationCodeEnum.RETRY:
      startTask(row, true)
      refreshTask()
      break
    case TaskOperationCodeEnum.CANCEL:
      taskApi.taskStopTree(getRowTaskId(row))
      break
    case TaskOperationCodeEnum.DELETE:
      deleteTask(getRowTaskId(row))
      break
    case TaskOperationCodeEnum.CONFIRM_REPLACE_RES:
      emits('openReplaceResConfirmDialog')
      break
    default:
      break
  }
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
// 打开下载dialog
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
// 刷新任务进度和状态
async function refreshTask() {
  if (!refreshing) {
    refreshing = true
    // 获取需要刷新的任务
    const getRefreshTasks = (): number[] => {
      // 获取可视区域及附近的行id
      const visibleRowsId = taskManageSearchTable.value.getVisibleRows(200, 200).map((id: string) => Number(id))
      // 利用树形工具找到所有id对应的数据，判断是否需要刷新
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
            useParentTaskStore().hasTask(task.id) ||
            useTaskStore().hasTask(task.id))
        )
      })
    }

    let refreshTasks: number[] = getRefreshTasks()

    while (refreshTasks.length > 0) {
      await taskManageSearchTable.value.refreshData(refreshTasks, false)
      await new Promise((resolve) => setTimeout(resolve, 500))
      if (isNullish(taskManageSearchTable.value)) {
        break
      }
      refreshTasks = getRefreshTasks()
    }
    refreshing = false
  }
}
// 滚动事件处理函数
function handleScroll() {
  throttleRefreshTask()
}
// 开始任务
function startTask(row: TaskProgressTreeDTO, retry: boolean) {
  const apiCall = retry ? taskApi.taskRetryTree : taskApi.taskStartTree
  apiCall(getRowTaskId(row)).catch((e: Error) => {
    ElMessage.error(e.message)
  })
  if (row.taskProgress?.task) {
    row.taskProgress.task.status = TaskStatusEnum.WAITING
  }
  if (row.taskProgress?.task?.hasChild && notNullish(row.children)) {
    row.children.forEach((child) => {
      if (child.taskProgress?.task) {
        child.taskProgress.task.status = TaskStatusEnum.WAITING
      }
    })
  }
}
// 删除任务
async function deleteTask(id: number) {
  try {
    await taskApi.taskDelete(id)
    await taskManageSearchTable.value.doSearch()
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}
// 获取url匹配的插件
async function getUrlMatchedPlugin(url: string): Promise<Plugin[]> {
  try {
    const response = await pluginTaskUrlListenerApi.listListener(url)
    return response.data as unknown as Plugin[]
  } catch {
    return []
  }
}
// 获取受支持提示文本
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
//
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
      <search-table
        ref="taskManageSearchTable"
        v-model:data="dataList"
        v-model:page="page"
        v-model:changed-rows="changedRows"
        v-model:sort="sort"
        class="task-manage-search-table"
        :selectable="true"
        :thead="thead"
        :search="taskQueryParentPage"
        :update-load="updateLoad"
        :update-properties="['taskProgress.task.status', 'taskProgress.total', 'taskProgress.finished']"
        data-key="taskProgress.task.id"
        :row-class-name="rowClassName"
        :tree-lazy="true"
        :tree-load="load"
        :multi-select="true"
        :default-page-size="10"
        :custom-operation-button="true"
        :operation-width="163"
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
        <template #customOperations="{ row }">
          <task-operation-bar-active :row="row" :button-clicked="handleOperationButtonClicked" />
        </template>
      </search-table>
    </div>
    <template #dialog>
      <task-dialog v-model:state="taskDialogState" v-model:form-data="dialogData" :mode="DialogMode.VIEW" width="90%" />
      <el-dialog v-model="downloadDialogState" width="80%">
        <div v-if="downloadMode" class="task-manage-download-dialog-local-button-container">
          <el-button type="primary" icon="FolderOpened" @click="selectDirectory()">选择文件夹导入</el-button>
          <el-button type="primary" icon="Document" @click="selectFile()">选择单个文件导入</el-button>
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
