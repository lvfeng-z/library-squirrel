<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import { computed, h, nextTick, Ref, ref, VNode } from 'vue'
import SearchTable from '../common/SearchTable.vue'
import { Thead } from '../../model/util/Thead'
import { TaskStatusEnum } from '../../constants/TaskStatusEnum.ts'
import { ElTag } from 'element-plus'
import ApiUtil from '@renderer/utils/ApiUtil'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { getNode } from '@renderer/utils/TreeUtil.ts'
import { throttle } from 'lodash'
import TaskOperationBarActive from '@renderer/components/common/TaskOperationBarActive.vue'
import { TaskOperationCodeEnum } from '@renderer/constants/TaskOperationCodeEnum.ts'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import Page from '@renderer/model/util/Page.ts'
import { siteQuerySelectItemPageBySiteName } from '@renderer/apis/SiteApi.ts'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import TaskTreeDTO from '@renderer/model/model/dto/TaskTreeDTO.ts'
import TaskProgressTreeDTO from '@renderer/model/model/dto/TaskProgressTreeDTO.ts'
import { TaskQueryDTO } from '@bindings/github.com/library-squirrel/wails/internal/task/models'
import Task from '@renderer/model/model/entity/Task.ts'
import TaskScheduleDTO from '@renderer/model/model/dto/TaskScheduleDTO.ts'
import { isNotBlank } from '@renderer/utils/StringUtil.ts'
import { taskApi } from '@renderer/apis/http'

// props
const props = defineProps<{
  mode: DialogMode
}>()

// model
// 表单数据
const formData: Ref<TaskTreeDTO> = defineModel('formData', { type: Object, required: true })
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// 变量
// 接口
const apis = {
  taskStartTask: (taskId: number) => taskApi.taskStartTree(taskId),
  taskRetryTask: (taskId: number) => taskApi.taskRetryTree(taskId),
  taskDeleteTask: (id: number) => taskApi.taskDelete(id),
  taskListSchedule: (ids: number[]) => taskApi.taskListStatus(ids),
  taskQueryChildrenTaskPage: (pid: number, pageNumber: number, pageSize: number, query?: Record<string, unknown>) =>
    taskApi.taskQueryChildrenTaskPage(pid, pageNumber, pageSize, query),
  taskPauseTaskTree: (taskId: number) => taskApi.taskPauseTree(taskId),
  taskResumeTaskTree: (taskId: number) => taskApi.taskResumeTree(taskId)
}
// childTaskSearchTable组件的实例
const childTaskSearchTable = ref()
// 下级任务
const children: Ref<TaskTreeDTO[]> = ref([])
// 表头
const thead: Ref<Thead<TaskProgressTreeDTO>[]> = ref([
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'taskName',
    title: '名称',
    hide: false,
    width: 380,
    headerAlign: 'center',
    dataAlign: 'left',
    showOverflowTooltip: true
  }),
  new Thead({
    type: 'text',
    defaultDisabled: true,
    key: 'siteName',
    title: '站点',
    hide: false,
    width: 100,
    headerAlign: 'center',
    dataAlign: 'center',
    showOverflowTooltip: true
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
    showOverflowTooltip: true
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
    showOverflowTooltip: true
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
// 任务查询的参数
const taskSearchParams: Ref<TaskQueryDTO> = ref(new TaskQueryDTO())
// 任务SearchTable的分页
const page: Ref<Page<TaskQueryDTO, Task>> = ref(new Page<TaskQueryDTO, Task>())
// 改变的行数据
const changedRows: Ref<object[]> = ref([])
// 是否正在刷新数据
let refreshing: boolean = false
// 防抖动refreshTask
const throttleRefreshTask = throttle(() => refreshTask(), 500, { leading: true, trailing: true })
// 是否为父任务
const isParent = computed(() => notNullish(formData.value.isCollection) && Boolean(formData.value.isCollection))
let parentCache: TaskTreeDTO | undefined = undefined

// 方法
// 分页查询子任务的函数
async function taskQueryChildrenTaskPage(page: Page<TaskQueryDTO, object>): Promise<Page<TaskQueryDTO, object> | undefined> {
  if (isNullish(page.query)) {
    page.query = new TaskQueryDTO()
  }
  const pid = formData.value.id
  if (isNullish(pid)) {
    return undefined
  }
  page.query.pid = pid
  const response = await apis.taskQueryChildrenTaskPage(pid, page.pageNumber, page.pageSize, page.query as Record<string, unknown>)
  if (ApiUtil.check(response)) {
    return ApiUtil.data(response) as Page<TaskQueryDTO, object>
  } else {
    ApiUtil.msg(response)
    return undefined
  }
}
// 更新进度的数据加载函数
async function updateLoad(ids: (number | string)[]): Promise<TaskScheduleDTO[] | undefined> {
  const response = await apis.taskListSchedule(ids)
  if (ApiUtil.check(response)) {
    const scheduleList = ApiUtil.data(response) as TaskScheduleDTO[]
    return arrayNotEmpty(scheduleList) ? scheduleList : undefined
  } else {
    return undefined
  }
}
// 开关dialog
function handleOpen() {
  nextTick(() => {
    if (notNullish(childTaskSearchTable.value)) {
      childTaskSearchTable.value.doSearch()
    }
  })
}
// 刷新任务进度和状态
async function refreshTask() {
  if (!refreshing) {
    refreshing = true
    // 获取需要刷新的任务
    const getRefreshTasks = (): number[] => {
      // 获取可视区域及附近的行id
      const visibleRowsId = childTaskSearchTable.value.getVisibleRows(200, 200).map((id: string) => Number(id))
      // 利用树形工具找到所有id对应的数据，判断是否需要刷新
      const tempRoot = new TaskTreeDTO()
      tempRoot.children = children.value
      return visibleRowsId.filter((id: number) => {
        const task = getNode<TaskTreeDTO>(tempRoot, id)
        return (
          notNullish(task) &&
          (task.status === TaskStatusEnum.WAITING || task.status === TaskStatusEnum.PROCESSING || task.status === TaskStatusEnum.PAUSE)
        )
      })
    }

    let refreshTasks: number[] = getRefreshTasks()

    while (refreshTasks.length > 0) {
      await childTaskSearchTable.value.refreshData(refreshTasks, false)
      await new Promise((resolve) => setTimeout(resolve, 500))
      if (isNullish(childTaskSearchTable.value)) {
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
// 处理操作栏按钮点击事件
function handleOperationButtonClicked(row: TaskTreeDTO, code: TaskOperationCodeEnum) {
  switch (code) {
    case TaskOperationCodeEnum.VIEW:
      parentCache = formData.value
      formData.value = row
      children.value = []
      break
    case TaskOperationCodeEnum.START:
      startTask(row, false)
      throttleRefreshTask()
      break
    case TaskOperationCodeEnum.PAUSE:
      apis.taskPauseTaskTree(row.id)
      throttleRefreshTask()
      break
    case TaskOperationCodeEnum.RESUME:
      apis.taskResumeTaskTree(row.id)
      throttleRefreshTask()
      break
    case TaskOperationCodeEnum.RETRY:
      startTask(row, true)
      throttleRefreshTask()
      break
    case TaskOperationCodeEnum.CANCEL:
      break
    case TaskOperationCodeEnum.DELETE:
      deleteTask(row.id as number)
      break
    default:
      break
  }
}
// 获取表示任务状态的ElTag的render函数
function getTaskStatusElTag(data: TaskStatusEnum): VNode {
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
  }
  const elTag = h(ElTag, { type: tagType }, () => tagText)
  return h('div', { style: { display: 'flex', 'align-items': 'center', 'justify-content': 'center' } }, elTag)
}
// 开始任务
function startTask(row: TaskTreeDTO, retry: boolean) {
  if (retry) {
    apis.taskRetryTask(row.id)
  } else {
    apis.taskStartTask(row.id)
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
    await childTaskSearchTable.value.doSearch()
  }
}
// 转到父任务
function toParent() {
  if (notNullish(parentCache)) formData.value = parentCache
  nextTick(() => childTaskSearchTable.value.doSearch())
}
</script>

<template>
  <form-dialog v-model:form-data="formData" v-model:state="state" :mode="props.mode" @open="handleOpen">
    <template #header>
      <el-button v-show="!isParent" icon="ArrowLeftBold" type="primary" @click="toParent">查看任务集</el-button>
    </template>
    <template #form>
      <div>
        <el-row>
          <el-col>
            <el-form-item label="名称">
              <el-input v-model="formData.taskName"></el-input>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row>
          <el-col>
            <el-form-item label="来源">
              <el-input v-model="formData.url"></el-input>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row>
          <el-col :span="7">
            <el-form-item label="站点">
              <el-input v-model="formData.siteId"></el-input>
            </el-form-item>
          </el-col>
          <el-col v-if="!isParent" :span="17">
            <el-form-item label="站点作品id">
              <el-input v-model="formData.siteWorkId" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row>
          <el-col :span="3">
            <el-form-item label="状态">
              <component :is="getTaskStatusElTag(formData.status as TaskStatusEnum)" />
            </el-form-item>
          </el-col>
          <el-col :span="7">
            <el-form-item label="创建时间">
              <el-date-picker v-model="formData.createTime" type="datetime"></el-date-picker>
            </el-form-item>
          </el-col>
          <el-col :span="7">
            <el-form-item label="修改时间">
              <el-date-picker v-model="formData.updateTime" type="datetime"></el-date-picker>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row v-if="isNotBlank(formData.errorMessage)">
          <el-col>
            <el-form-item label="异常信息">
              <el-input v-model="formData.errorMessage" type="textarea" autosize />
            </el-form-item>
          </el-col>
        </el-row>
      </div>
    </template>
    <template #afterForm>
      <search-table
        v-show="isParent"
        ref="childTaskSearchTable"
        v-model:page="page"
        v-model:toolbar-params="taskSearchParams"
        v-model:data="children"
        class="task-dialog-search-table"
        :selectable="true"
        :thead="thead as Thead<object>[]"
        :search="taskQueryChildrenTaskPage"
        :update-load="updateLoad"
        :update-properties="['schedule', 'status']"
        :multi-select="true"
        :changed-rows="changedRows"
        :custom-operation-button="true"
        :operation-width="163"
        data-key="id"
        @scroll="handleScroll"
      >
        <template #toolbarMain>
          <el-row class="task-dialog-search-bar">
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
    </template>
  </form-dialog>
</template>

<style scoped>
.task-dialog-search-table {
  height: calc(90vh - 80px);
}
.task-dialog-search-bar {
  flex-grow: 1;
}
</style>
