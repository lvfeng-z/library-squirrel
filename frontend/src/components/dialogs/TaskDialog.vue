<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import {computed, h, nextTick, Ref, ref, toRaw, VNode} from 'vue'
import SearchTable from '../common/SearchTable.vue'
import { Thead } from '../../model/util/Thead'
import { TaskStatusEnum } from '../../constants/TaskStatusEnum.ts'
import { ElMessage, ElTag } from 'element-plus'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import {getNode, getNodeByPath} from '@renderer/utils/TreeUtil.ts'
import lodash, { throttle } from 'lodash'
import TaskOperationBarActive from '@renderer/components/common/TaskOperationBarActive.vue'
import { TaskOperationCodeEnum } from '@renderer/constants/TaskOperationCodeEnum.ts'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import { siteQuerySelectItemPageBySiteName } from '@renderer/apis/http'
import AutoLoadSelect from '@renderer/components/common/AutoLoadSelect.vue'
import { TaskProgressTreeDTO, TaskProgressDTO, TaskDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { TaskQueryDTO } from '@bindings/github.com/library-squirrel/backend/task/models'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model'
import { isNotBlank } from '@renderer/utils/StringUtil.ts'
import { taskApi } from '@renderer/apis/http'
import {QueryAttribute} from "@bindings/github.com/library-squirrel/backend/base/query";
import {newPage} from "@renderer/utils/Pager.ts";
import {useTaskStore} from "@renderer/store/UseTaskStore.ts";
import {useParentTaskStore} from "@renderer/store/UseParentTaskStore.ts";

// props
const props = defineProps<{
  mode: DialogMode
}>()

// model
// 表单数据
const formData: Ref<TaskProgressTreeDTO> = defineModel('formData', { type: Object, required: true })
// 弹窗开关
const state = defineModel<boolean>('state', { required: true })

// 变量
// childTaskSearchTable组件的实例
const childTaskSearchTable = ref()
// 下级任务
const children: Ref<TaskProgressTreeDTO[]> = ref([])
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
    dataAlign: 'center',
    showOverflowTooltip: true
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
      }
      const elTag = h(ElTag, { type: tagType }, () => tagText)
      return h('div', { style: { display: 'flex', 'align-items': 'center', 'justify-content': 'center' } }, elTag)
    }
  })
])
// 任务查询的参数
const taskSearchParams: Ref<TaskQueryDTO> = ref(new TaskQueryDTO())
// 任务SearchTable的分页
const page: Ref<Page<TaskProgressTreeDTO>> = ref(newPage<TaskProgressTreeDTO>())
// 改变的行数据
const changedRows: Ref<object[]> = ref([])
// 是否正在刷新数据
let refreshing: boolean = false
// 防抖动refreshTask
const throttleRefreshTask = throttle(() => refreshTask(), 500, { leading: true, trailing: true })
const formTask = computed(() => formData.value.taskProgress?.task ?? new TaskDTO())
// 是否为父任务
const isParent = computed(() => notNullish(formTask.value.isCollection) && Boolean(formTask.value.isCollection))
let parentCache: TaskProgressTreeDTO | undefined = undefined
const taskStore = useTaskStore()
const parentTaskStore = useParentTaskStore()

// 方法
// 分页查询子任务的函数
async function taskQueryChildrenTaskPage(page: Page<object>): Promise<Page<object> | undefined> {
  const query = toRaw(taskSearchParams.value)
  const pid = formTask.value.id
  if (isNullish(pid)) {
    return undefined
  }
  query.pid = new QueryAttribute({value: pid})
  const tempPage = toRaw(page)
  try {
    const response = await taskApi.taskQueryChildrenTaskPage(tempPage, query)
    return response.data as unknown as Page<object>
  } catch (e: any) {
    ElMessage.error(e.message)
    return undefined
  }
}
// 更新进度的数据加载函数
async function updateLoad(ids: (number | string)[]): Promise<TaskProgressDTO[] | undefined> {
  try {
    const response = await taskApi.taskListStatus(ids as number[])
    return arrayNotEmpty(response.data) ? response.data : undefined
  } catch {
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
    const getActiveTaskIds = (): number[] => {
      const visibleRowsId = childTaskSearchTable.value.getVisibleRows(200, 200).map((id: string) => Number(id))
      return visibleRowsId.filter((id: number) => {
        const taskProgressTree = getNodeByPath(children.value, id, (task) => task.taskProgress?.task?.id, (task) => (task.children as TaskProgressTreeDTO[]))
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

    let refreshTasks: number[] = getActiveTaskIds()

    while (refreshTasks.length > 0) {
      await childTaskSearchTable.value.refreshData(refreshTasks, false)
      await new Promise((resolve) => setTimeout(resolve, 500))
      if (isNullish(childTaskSearchTable.value)) {
        break
      }
      refreshTasks = getActiveTaskIds()
    }
    refreshing = false
  }
}
// 滚动事件处理函数
function handleScroll() {
  throttleRefreshTask()
}
// 处理操作栏按钮点击事件
function handleOperationButtonClicked(row: TaskProgressTreeDTO, code: TaskOperationCodeEnum) {
  const rowTaskId = row.taskProgress?.task?.id
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
      if (notNullish(rowTaskId)) taskApi.taskPauseTree(rowTaskId)
      throttleRefreshTask()
      break
    case TaskOperationCodeEnum.RESUME:
      if (notNullish(rowTaskId)) taskApi.taskResumeTree(rowTaskId)
      throttleRefreshTask()
      break
    case TaskOperationCodeEnum.RETRY:
      startTask(row, true)
      throttleRefreshTask()
      break
    case TaskOperationCodeEnum.CANCEL:
      break
    case TaskOperationCodeEnum.DELETE:
      if (notNullish(rowTaskId)) deleteTask(rowTaskId)
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
  }
  const elTag = h(ElTag, { type: tagType }, () => tagText)
  return h('div', { style: { display: 'flex', 'align-items': 'center', 'justify-content': 'center' } }, elTag)
}
// 开始任务
function startTask(row: TaskProgressTreeDTO, retry: boolean) {
  const rowTaskId = row.taskProgress?.task?.id
  if (isNullish(rowTaskId)) return
  const apiCall = retry ? taskApi.taskRetryTree : taskApi.taskStartTree
  apiCall(rowTaskId).catch((e: Error) => {
    ElMessage.error(e.message)
  })
  if (row.taskProgress?.task) {
    row.taskProgress.task.status = TaskStatusEnum.WAITING
  }
  if (row.taskProgress?.task?.isCollection && notNullish(row.children)) {
    row.children.filter(notNullish).forEach((child) => {
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
    await childTaskSearchTable.value.doSearch()
  } catch (e: any) {
    ElMessage.error(e.message)
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
              <el-input v-model="formTask.taskName"></el-input>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row>
          <el-col>
            <el-form-item label="来源">
              <el-input v-model="formTask.url"></el-input>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row>
          <el-col :span="7">
            <el-form-item label="站点">
              <el-input v-model="formTask.siteId"></el-input>
            </el-form-item>
          </el-col>
          <el-col v-if="!isParent" :span="17">
            <el-form-item label="站点作品id">
              <el-input v-model="formTask.siteWorkId" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row>
          <el-col :span="3">
            <el-form-item label="状态">
              <component :is="getTaskStatusElTag(formTask.status as TaskStatusEnum)" />
            </el-form-item>
          </el-col>
          <el-col :span="7">
            <el-form-item label="创建时间">
              <el-date-picker v-model="formTask.createTime" type="datetime"></el-date-picker>
            </el-form-item>
          </el-col>
          <el-col :span="7">
            <el-form-item label="修改时间">
              <el-date-picker v-model="formTask.updateTime" type="datetime"></el-date-picker>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row v-if="isNotBlank(formTask.errorMessage)">
          <el-col>
            <el-form-item label="异常信息">
              <el-input v-model="formTask.errorMessage" type="textarea" autosize />
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
        v-model:data="children"
        class="task-dialog-search-table"
        :selectable="true"
        :thead="thead"
        :search="taskQueryChildrenTaskPage"
        :update-load="updateLoad"
        :update-properties="['schedule', 'status']"
        :multi-select="true"
        :changed-rows="changedRows"
        :custom-operation-button="true"
        :operation-width="163"
        data-key="taskProgress.task.id"
        @scroll="handleScroll"
      >
        <template #toolbarMain>
          <el-row class="task-dialog-search-bar">
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
