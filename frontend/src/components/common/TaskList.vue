<script setup lang="ts">
import SlotSearchTable from './SlotSearchTable.vue'
import StatusTag from './StatusTag.vue'
import TaskOperationBarActive from './TaskOperationBarActiveV1.vue'
import AutoLoadSelect from './AutoLoadSelect.vue'
import { onUnmounted, Ref, ref, toRaw, watch } from 'vue'
import { TaskStatusEnum } from '@renderer/constants/TaskStatusEnum.ts'
import { taskStatusToKey } from '@renderer/constants/StatusRegistry'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { getNodeByPath } from '@renderer/utils/TreeUtil.ts'
import { siteQuerySelectItemPageBySiteName } from '@renderer/apis/http'
import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { useParentTaskStore } from '@renderer/store/UseParentTaskStore.ts'
import { useTaskOperations } from '@renderer/composables/useTaskOperations'
import { TaskProgressTreeDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import { TaskQueryDTO } from '@bindings/github.com/library-squirrel/backend/task/models'
import { SortOrder } from '@bindings/github.com/library-squirrel/backend/base/query/models'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model'
import lodash from 'lodash'

// props
const props = withDefaults(
  defineProps<{
    // 数据来源：调用方提供具体接口调用（主页查父任务分页、dialog 按 pid 查子任务）
    search: (page: Page<TaskProgressTreeDTO>, query: TaskQueryDTO) => Promise<Page<TaskProgressTreeDTO> | undefined>
    treeData?: boolean
    treeLazy?: boolean
    treeLoad?: (row: TaskProgressTreeDTO) => Promise<TaskProgressTreeDTO[]>
    selectable?: boolean
    multiSelect?: boolean
    pageSizes?: number[]
  }>(),
  {
    treeData: false,
    treeLazy: false,
    selectable: true,
    multiSelect: true,
    pageSizes: () => [10, 20, 30, 50, 100]
  }
)

// model
const data = defineModel<TaskProgressTreeDTO[]>('data', { default: () => [], required: false })
const page = defineModel<Page<TaskProgressTreeDTO>>('page', { required: true })

// 事件：仅「查看」操作上抛调用方（主页打开 TaskDialog、dialog 下钻到子任务）
const emits = defineEmits<{
  (e: 'view', row: TaskProgressTreeDTO): void
  (e: 'selectionChange', rows: TaskProgressTreeDTO[]): void
}>()

// 暴露：供调用方在打开/下钻/删除后触发重新查询
const slotSearchTableRef = ref()
function doSearch() {
  slotSearchTableRef.value?.doSearch()
}
defineExpose({ doSearch })

// 变量
const taskSearchParams: Ref<TaskQueryDTO> = ref(new TaskQueryDTO())
const sort: Ref<{ prop: string; order: 'ascending' | 'descending' | null }> = ref({ prop: '', order: null })
const taskStore = useTaskStore()
const parentTaskStore = useParentTaskStore()
const invalidStatus = -1

// 构建 query：搜索参数 + 排序字段 + 创建/更新时间默认降序（深拷贝避免污染搜索参数）
function buildQuery(): TaskQueryDTO {
  const query = lodash.cloneDeep(toRaw(taskSearchParams.value))
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
  return query
}
// 包装搜索：内部构建 query 后交调用方提供的接口
async function wrappedSearch(p: Page<TaskProgressTreeDTO>): Promise<Page<TaskProgressTreeDTO> | undefined> {
  return props.search(p, buildQuery())
}
// 按 taskId 在当前数据树中定位行
function findRowByTaskId(taskId: number): TaskProgressTreeDTO | undefined {
  return getNodeByPath(
    data.value,
    taskId,
    (task) => task.taskProgress?.task?.id,
    (task) => task.children as TaskProgressTreeDTO[]
  )
}
// 时间戳格式化
function formatDatetime(timestamp: number | null | undefined): string {
  if (!timestamp) return '-'
  const d = new Date(timestamp)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}
// 取行状态：优先 store 实时状态，回退行数据自带状态
function getStatus(row: TaskProgressTreeDTO): number {
  const taskId = row.taskProgress?.task?.id
  const rowStatus = row.taskProgress?.task?.status
  if (isNullish(taskId)) return rowStatus ?? invalidStatus
  const storeStatus = (row.hasChildren ? parentTaskStore : taskStore).getTask(taskId)?.task?.status
  return storeStatus ?? rowStatus ?? invalidStatus
}
// 状态别名 key（颜色与文案由 StatusRegistry + 主题令牌驱动）
function getStatusKey(row: TaskProgressTreeDTO): string {
  return taskStatusToKey(getStatus(row))
}
// 行样式：父任务加粗、子任务首列缩进
function rowClassName(rowData: { row: unknown; rowIndex: number }) {
  const row = rowData.row as TaskProgressTreeDTO
  const task = row.taskProgress?.task
  if (row.hasChildren || isNullish(task?.pid) || task?.pid === 0) {
    return 'task-list-parent-row'
  }
  return 'task-list-child-row'
}
// 操作栏分发：「查看」上抛调用方，删除后刷新列表（其余操作由 useTaskOperations 处理）
const { buildOperationHandler } = useTaskOperations()
const handleOperationButtonClicked = buildOperationHandler({
  onView: (row: TaskProgressTreeDTO) => emits('view', row),
  onDeleted: () => doSearch()
})

// 监听子任务 store：把实时 status/total/finished 同步到当前数据树对应行
const unwatchTaskStore = watch(
  () => taskStore.tasks,
  (tasks) => {
    tasks.forEach((storeObj, taskId) => {
      const row = findRowByTaskId(taskId)
      if (notNullish(row?.taskProgress) && notNullish(storeObj.task)) {
        if (notNullish(storeObj.task.task?.status) && row!.taskProgress!.task?.status !== storeObj.task.task.status) {
          row!.taskProgress!.task!.status = storeObj.task.task.status
        }
        if (notNullish(storeObj.task.total)) {
          row!.taskProgress!.total = storeObj.task.total
        }
        if (notNullish(storeObj.task.finished)) {
          row!.taskProgress!.finished = storeObj.task.finished
        }
      }
    })
  },
  { deep: true }
)

// 监听父任务 store：同上，针对父任务行
const unwatchParentTaskStore = watch(
  () => parentTaskStore.parentTasks,
  (parentTasks) => {
    parentTasks.forEach((storeTask, taskId) => {
      const row = findRowByTaskId(taskId)
      if (notNullish(row?.taskProgress)) {
        if (notNullish(storeTask.task?.status) && row!.taskProgress!.task?.status !== storeTask.task.status) {
          row!.taskProgress!.task!.status = storeTask.task.status
        }
        if (notNullish(storeTask.total)) {
          row!.taskProgress!.total = storeTask.total
        }
        if (notNullish(storeTask.finished)) {
          row!.taskProgress!.finished = storeTask.finished
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
</script>

<template>
  <slot-search-table
    ref="slotSearchTableRef"
    v-model:data="data"
    v-model:page="page"
    v-model:sort="sort"
    class="task-list"
    :selectable="selectable as boolean"
    :multi-select="multiSelect as boolean"
    :search="wrappedSearch"
    data-key="taskProgress.task.id"
    :row-class-name="rowClassName"
    :tree-lazy="treeLazy"
    :tree-load="treeLoad"
    :tree-data="treeData"
    :page-sizes="pageSizes"
    @sort-change="doSearch"
    @selection-change="(rows: TaskProgressTreeDTO[]) => emits('selectionChange', rows)"
  >
    <template #toolbarMain>
      <el-row class="task-list-search-bar">
        <el-col :span="14">
          <el-input
            v-model="taskSearchParams.taskName.value"
            placeholder="输入任务名称"
            clearable
          />
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
              <el-option
                v-for="item in list"
                :key="item.value"
                :value="item.value"
                :label="item.label"
              />
            </template>
          </auto-load-select>
        </el-col>
        <el-col :span="4">
          <el-select
            v-model="taskSearchParams.status.value"
            placeholder="选择状态"
            clearable
          >
            <el-option
              :value="TaskStatusEnum.CREATED"
              label="已创建"
            />
            <el-option
              :value="TaskStatusEnum.WAITING"
              label="等待中"
            />
            <el-option
              :value="TaskStatusEnum.PROCESSING"
              label="进行中"
            />
            <el-option
              :value="TaskStatusEnum.PAUSED"
              label="暂停"
            />
            <el-option
              :value="TaskStatusEnum.FINISHED"
              label="完成"
            />
            <el-option
              :value="TaskStatusEnum.PARTLY_FINISHED"
              label="部分完成"
            />
            <el-option
              :value="TaskStatusEnum.FAILED"
              label="失败"
            />
          </el-select>
        </el-col>
      </el-row>
    </template>
    <!-- 名称 -->
    <el-table-column
      label="名称"
      min-width="380"
      show-overflow-tooltip
      sortable="custom"
      align="left"
    >
      <template #header>
        <el-tag>名称</el-tag>
      </template>
      <template #default="{ row }">
        {{ row.taskProgress?.task?.taskName ?? '-' }}
      </template>
    </el-table-column>
    <!-- 站点 -->
    <el-table-column
      label="站点"
      width="100"
      align="center"
    >
      <template #header>
        <el-tag>站点</el-tag>
      </template>
      <template #default="{ row }">
        {{ row.taskProgress?.siteName ?? '-' }}
      </template>
    </el-table-column>
    <!-- 是否可接续 -->
    <el-table-column
      label="是否可接续"
      width="80"
      show-overflow-tooltip
      sortable="custom"
      align="center"
    >
      <template #header>
        <el-tag>是否可接续</el-tag>
      </template>
      <template #default="{ row }">
        {{ row.taskProgress?.task?.continuable ?? '-' }}
      </template>
    </el-table-column>
    <!-- 错误信息 -->
    <el-table-column
      label="错误信息"
      width="380"
      show-overflow-tooltip
      sortable="custom"
      align="center"
    >
      <template #header>
        <el-tag>错误信息</el-tag>
      </template>
      <template #default="{ row }">
        {{ row.taskProgress?.task?.errorMessage ?? '-' }}
      </template>
    </el-table-column>
    <!-- 操作列 -->
    <el-table-column
      fixed="right"
      :width="163"
      align="center"
    >
      <template #header>
        <el-tag type="warning">
          操作
        </el-tag>
      </template>
      <template #default="{ row }">
        <task-operation-bar-active
          :row="row"
          :button-clicked="handleOperationButtonClicked"
        />
      </template>
    </el-table-column>
    <!-- 状态 -->
    <el-table-column
      label="状态"
      width="110"
      fixed="right"
      sortable="custom"
      align="center"
    >
      <template #header>
        <el-tag>状态</el-tag>
      </template>
      <template #default="{ row }">
        <div style="display: flex; align-items: center; justify-content: center">
          <StatusTag :status="getStatusKey(row)" />
        </div>
      </template>
    </el-table-column>
    <!-- 修改时间 -->
    <el-table-column
      fixed="right"
      label="修改时间"
      width="165"
      show-overflow-tooltip
      sortable="custom"
      align="center"
    >
      <template #header>
        <el-tag>修改时间</el-tag>
      </template>
      <template #default="{ row }">
        {{ formatDatetime(row.taskProgress?.task?.updateTime) }}
      </template>
    </el-table-column>
  </slot-search-table>
</template>

<style scoped>
.task-list {
  height: 100%;
  width: 100%;
}
.task-list-search-bar {
  flex-grow: 1;
}
:deep(.el-table .task-list-parent-row) {
  font-weight: bold;
}
:deep(.el-table .task-list-child-row > :nth-child(3) > .cell :nth-child(1)) {
  transform: translateX(7px);
}
</style>
