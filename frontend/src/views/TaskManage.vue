<script setup lang="ts">
import BaseSubpage from './BaseSubpage.vue'
import { h, onMounted, Ref, ref, VNode } from 'vue'
import ApiUtil from '../utils/ApiUtil.ts'
import SearchTable from '../components/common/SearchTable.vue'
import { Thead } from '../model/util/Thead.ts'
import DialogMode from '../model/util/DialogMode.ts'
import { ElMessage, ElTag } from 'element-plus'
import { arrayIsEmpty, arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { throttle } from 'lodash'
import { TaskStatusEnum } from '../constants/TaskStatusEnum.ts'
import { getNode } from '@renderer/utils/TreeUtil.ts'
import TaskDialog from '../components/dialogs/TaskDialog.vue'
import Page from '../model/util/Page.ts'
import ApiResponse from '@renderer/model/util/ApiResponse.ts'
import { TaskOperationCodeEnum } from '@renderer/constants/TaskOperationCodeEnum.ts'
import TaskOperationBarActive from '@renderer/components/common/TaskOperationBarActive.vue'
import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { useParentTaskStore } from '@renderer/store/UseParentTaskStore.ts'
import NotificationItem from '@renderer/model/util/NotificationItem.ts'
import { useNotificationStore } from '@renderer/store/UseNotificationStore.ts'
import { siteQuerySelectItemPageBySiteName } from '@renderer/apis/SiteApi.ts'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { useTourStatesStore } from '@renderer/store/UseTourStatesStore.ts'
import { fileSysUtilApi, taskApi, siteApi, pluginTaskUrlListenerApi } from '@renderer/apis/http'
import TaskTreeDTO from '@renderer/model/model/dto/TaskTreeDTO.ts'
import TaskProgressTreeDTO from '@renderer/model/model/dto/TaskProgressTreeDTO.ts'
import Task from '@renderer/model/model/entity/Task.ts'
import TaskQueryDTO from '@renderer/model/model/queryDTO/TaskQueryDTO.ts'
import Plugin from '@renderer/model/model/entity/Plugin.ts'
import TaskScheduleDTO from '@renderer/model/model/dto/TaskScheduleDTO.ts'
import type { QuerySortOption } from '@renderer/model/util/QuerySortOption.ts'

// onMounted
onMounted(() => {
  taskManageSearchTable.value.doSearch()
})

// 事件
const emits = defineEmits(['openReplaceResConfirmDialog'])

// 变量
// 接口 - 使用 HTTP API
const apis = {
  pluginTaskUrlListenerManagerListListener: pluginTaskUrlListenerApi.listListener,
  siteQuerySelectItemPage: siteApi.siteQuerySelectItemPage,
  taskCreateTask: (url: string) => taskApi.taskCreateByUrl(url),
  taskListStatus: (ids: number[]) => taskApi.taskListStatus(ids),
  taskStartTask: (taskId: number) => taskApi.taskStartTree(taskId),
  taskRetryTask: (taskId: number) => taskApi.taskRetryTree(taskId),
  taskDeleteTask: (id: number) => taskApi.taskDelete(id),
  taskQueryParentPage: (page: Page<TaskQueryDTO, object>) =>
    taskApi.taskQueryParentPage({
      pageNumber: page.pageNumber,
      pageSize: page.pageSize,
      query: page.query as Record<string, unknown>
    }),
  taskQueryChildrenTaskPage: (page: Page<TaskQueryDTO, object>) => {
    const query = page.query as Record<string, unknown>
    return taskApi.taskQueryChildrenTaskPage(query.pid as number, page.pageNumber, page.pageSize, query)
  },
  taskPauseTaskTree: (taskId: number) => taskApi.taskPauseTree(taskId),
  taskStopTaskTree: (taskId: number) => taskApi.taskStopTree(taskId),
  taskResumeTaskTree: (taskId: number) => taskApi.taskResumeTree(taskId),
  dirSelect: fileSysUtilApi.dirSelect
}
// taskManageSearchTable的组件实例
const taskManageSearchTable = ref()
// 本地导入按钮的实例
const localImportButton = ref()
// 站点导入按钮的实例
const siteDownloadButton = ref()
// 任务SearchTable的数据
const dataList: Ref<TaskTreeDTO[]> = ref([])
// 表头
const thead: Ref<Thead<TaskProgressTreeDTO>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'taskName',
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
    key: 'siteName',
    title: '站点',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center'
  }),
  new Thead({
    type: 'datetime',
    defaultDisabled: true,
    key: 'createTime',
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
    key: 'url',
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
    key: 'status',
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
        case TaskStatusEnum.PAUSE:
          tagType = 'info'
          tagText = '已暂停'
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
        case TaskStatusEnum.WAITING_USER_INPUT:
          tagType = 'warning'
          tagText = '等待用户操作'
          break
      }
      const elTag = h(ElTag, { type: tagType }, () => tagText)
      return h('div', { style: { display: 'flex', 'align-items': 'center', 'justify-content': 'center' } }, elTag)
    }
  })
])
// 任务SearchTable的分页
const page: Ref<Page<TaskQueryDTO, Task>> = ref(new Page<TaskQueryDTO, Task>())
// 任务查询的参数
const taskSearchParams: Ref<TaskQueryDTO> = ref(new TaskQueryDTO())
// 改变的行数据
const changedRows: Ref<object[]> = ref([])
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
  apis.taskCreateTask(sourceUrl.value).then((response: ApiResponse) => handleCreatTaskResponse(response, notificationId))
  downloadDialogState.value = false
  sourceUrl.value = ''
}
// 处理任务创建的响应
function handleCreatTaskResponse(response: ApiResponse, notificationId: string) {
  let type: 'error' | 'primary' | 'success' | 'warning' | 'info'
  let msg: string
  if (ApiUtil.check(response)) {
    const data = ApiUtil.data<{ succeed: boolean; addedQuantity: number; msg?: string }>(response)
    if (notNullish(data)) {
      // 刷新一次列表
      taskManageSearchTable.value.doSearch()
      if (data.succeed) {
        type = 'success'
        msg = `成功创建了 ${data.addedQuantity} 个任务`
      } else {
        type = 'error'
        msg = '创建失败，' + (data.msg || '未知错误')
      }
    } else {
      type = 'error'
      msg = '创建失败'
    }
  } else {
    type = 'error'
    msg = '创建失败'
  }
  useNotificationStore().remove(notificationId, {
    type: type,
    msg: msg
  })
}
// 分页查询父任务的函数
async function taskQueryParentPage(page: Page<TaskQueryDTO, object>): Promise<Page<TaskQueryDTO, object> | undefined> {
  if (isNullish(page.query)) {
    page.query = new TaskQueryDTO()
  }
  // 根据用户选择的排序构建排序配置
  const userSort: QuerySortOption[] = []
  // 添加用户选择的排序（如果存在）
  if (sort.value.prop && sort.value.order) {
    userSort.push({
      key: sort.value.prop,
      asc: sort.value.order === 'ascending'
    })
  }
  // 添加默认排序（按创建时间和更新时间降序）
  if (arrayNotEmpty(page.query.sort)) {
    page.query.sort = [
      ...userSort,
      ...page.query.sort,
      ...[
        { key: 'createTime', asc: false },
        { key: 'updateTime', asc: false }
      ]
    ]
  } else {
    page.query.sort = [...userSort, { key: 'createTime', asc: false }, { key: 'updateTime', asc: false }]
  }
  const response = await apis.taskQueryParentPage(page)
  if (ApiUtil.check(response)) {
    return ApiUtil.data<Page<TaskQueryDTO, object>>(response)
  } else {
    ApiUtil.msg(response)
    return undefined
  }
}
// 懒加载处理函数
async function load(row: unknown): Promise<TaskTreeDTO[]> {
  // 配置分页参数
  const pageCondition: Page<TaskQueryDTO, object> = new Page()
  pageCondition.pageSize = 100
  pageCondition.pageNumber = 1
  // 配置查询参数
  pageCondition.query = { ...new TaskQueryDTO(), ...{ pid: (row as TaskTreeDTO).id } }

  return apis.taskQueryChildrenTaskPage(pageCondition).then((response: ApiResponse) => {
    if (ApiUtil.check(response)) {
      const page = ApiUtil.data(response) as Page<TaskQueryDTO, object>
      const data = (page.data === undefined ? [] : page.data) as TaskTreeDTO[]
      // 子任务列表赋值给对应的父任务的children
      const parent = dataList.value.find((task) => (row as TaskTreeDTO).id === task.id)
      if (notNullish(parent)) {
        parent.children = data
      }
      return data
    } else {
      ApiUtil.msg(response)
      return []
    }
  })
}
// 更新进度的数据加载函数
async function updateLoad(ids: (number | string)[]): Promise<TaskScheduleDTO[] | undefined> {
  const scheduleList: TaskScheduleDTO[] = []
  const notFoundList: (number | string)[] = []
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
    notFoundList.push(id)
  }
  if (arrayNotEmpty(notFoundList)) {
    const response = await apis.taskListStatus(notFoundList)
    if (ApiUtil.check(response)) {
      const responseScheduleList = ApiUtil.data<TaskScheduleDTO[]>(response)
      if (arrayNotEmpty(responseScheduleList)) {
        scheduleList.push(...responseScheduleList)
      }
    }
  }
  return arrayNotEmpty(scheduleList) ? scheduleList : undefined
}
// 给行添加选择器，用于区分父任务和子任务
function rowClassName(data: { row: unknown; rowIndex: number }) {
  const row = data.row as TaskTreeDTO
  if (row.hasChildren || isNullish(row.pid) || row.pid === 0) {
    return 'task-manage-search-table-parent-row'
  } else {
    return 'task-manage-search-table-child-row'
  }
}
// 处理操作栏按钮点击事件
function handleOperationButtonClicked(row: TaskTreeDTO, code: TaskOperationCodeEnum) {
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
      apis.taskPauseTaskTree(row.id)
      refreshTask()
      break
    case TaskOperationCodeEnum.RESUME:
      apis.taskResumeTaskTree(row.id)
      refreshTask()
      break
    case TaskOperationCodeEnum.RETRY:
      startTask(row, true)
      refreshTask()
      break
    case TaskOperationCodeEnum.CANCEL:
      apis.taskStopTaskTree(row.id)
      break
    case TaskOperationCodeEnum.DELETE:
      deleteTask(row.id as number)
      break
    case TaskOperationCodeEnum.CONFIRM_REPLACE_RES:
      emits('openReplaceResConfirmDialog')
      break
    default:
      break
  }
}
// 选择目录
async function selectDir(openFile: boolean) {
  const response = await apis.dirSelect(openFile)
  if (ApiUtil.check(response)) {
    const dirSelectResult = ApiUtil.data(response) as { canceled: boolean; filePaths: string[] }
    if (!dirSelectResult.canceled) {
      for (const dir of dirSelectResult.filePaths) {
        sourceUrl.value = dir
      }
    }
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
      const tempRoot = new TaskTreeDTO()
      tempRoot.children = dataList.value
      return visibleRowsId.filter((id: number) => {
        const task = getNode<TaskTreeDTO>(tempRoot, id)
        return (
          notNullish(task) &&
          (task.status === TaskStatusEnum.WAITING ||
            task.status === TaskStatusEnum.PROCESSING ||
            task.status === TaskStatusEnum.PAUSE ||
            useParentTaskStore().hasTask(task.id as number) ||
            useTaskStore().hasTask(task.id as number))
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
function startTask(row: TaskTreeDTO, retry: boolean) {
  if (retry) {
    apis.taskRetryTask(row.id).then((response: ApiResponse) => {
      if (!ApiUtil.check(response)) {
        ApiUtil.failedMsg(response)
      }
    })
  } else {
    apis.taskStartTask(row.id).then((response: ApiResponse) => {
      if (!ApiUtil.check(response)) {
        ApiUtil.failedMsg(response)
      }
    })
  }
  row.status = TaskStatusEnum.WAITING
  if (row.isCollection && notNullish(row.children)) {
    row.children.forEach((child) => (child.status = TaskStatusEnum.WAITING))
  }
}
// 删除任务
async function deleteTask(id: number) {
  const response = await apis.taskDeleteTask(id)
  ApiUtil.msg(response)
  if (ApiUtil.check(response)) {
    await taskManageSearchTable.value.doSearch()
  }
}
// 获取url匹配的插件
async function getUrlMatchedPlugin(url: string): Promise<Plugin[]> {
  const response = await apis.pluginTaskUrlListenerManagerListListener(url)
  if (ApiUtil.check(response)) {
    const plugins = ApiUtil.data<Plugin[]>(response)
    if (notNullish(plugins)) {
      return plugins
    }
  }
  return []
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
        v-model:toolbar-params="taskSearchParams"
        v-model:changed-rows="changedRows"
        v-model:sort="sort"
        class="task-manage-search-table"
        :selectable="true"
        :thead="thead as Thead<object>[]"
        :search="taskQueryParentPage"
        :update-load="updateLoad"
        :update-properties="['status']"
        data-key="id"
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
              <el-input v-model="taskSearchParams.taskName" placeholder="输入任务名称" clearable />
            </el-col>
            <el-col :span="6">
              <auto-load-select
                v-model="taskSearchParams.siteId"
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
              <el-select v-model="taskSearchParams.status" placeholder="选择状态" clearable>
                <el-option :value="TaskStatusEnum.CREATED" label="已创建"></el-option>
                <el-option :value="TaskStatusEnum.WAITING" label="等待中"></el-option>
                <el-option :value="TaskStatusEnum.PROCESSING" label="进行中"></el-option>
                <el-option :value="TaskStatusEnum.PAUSE" label="暂停"></el-option>
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
