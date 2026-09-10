<script setup lang="ts">
import BaseView from './BaseView.vue'
import { onMounted, Ref, ref } from 'vue'
import { ElMessage } from 'element-plus'
import DialogMode from '../model/util/DialogMode.ts'
import TaskDialog from '../components/dialogs/TaskDialog.vue'
import TaskList from '../components/common/TaskList.vue'
import { taskApi } from '@renderer/apis/http'
import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { arrayIsEmpty, notNullish } from '@renderer/utils/CommonUtil.ts'
import { TaskQueryDTO } from '@bindings/github.com/library-squirrel/backend/task/models'
import { Operator, QueryAttribute } from '@bindings/github.com/library-squirrel/backend/base/query/models'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model'
import { TaskProgressTreeDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { newPage } from '@renderer/utils/Pager.ts'

// onMounted
onMounted(() => {
  taskListRef.value.doSearch()
})

// 变量
const taskListRef = ref()
const dataList: Ref<TaskProgressTreeDTO[]> = ref([])
const page: Ref<Page<TaskProgressTreeDTO>> = ref(newPage<TaskProgressTreeDTO>())
const dialogData: Ref<TaskProgressTreeDTO> = ref(new TaskProgressTreeDTO())
const taskDialogState: Ref<boolean> = ref(false)

// 方法
// 查看行：打开任务详情弹窗
function onViewRow(row: TaskProgressTreeDTO) {
  dialogData.value = row
  taskDialogState.value = true
}

// 导出任务分页查询（排序与默认排序由 TaskList 内部构建 query）；
// 注入 taskType eq 过滤圈定导出任务——本视图无创建入口，导出经主页多选发起
async function exportTaskQueryParentPage(p: Page<TaskProgressTreeDTO>, query: TaskQueryDTO): Promise<Page<TaskProgressTreeDTO> | undefined> {
  query.taskType = new QueryAttribute({ value: 'export', operator: Operator.OpEq })
  try {
    const response = await taskApi.taskQueryParentPage(p, query)
    const rows = (response.data?.data ?? []).filter((d): d is TaskProgressTreeDTO => d !== null)
    registerRowTaskTypes(rows)
    return response.data
  } catch (e: any) {
    ElMessage.error(e.message)
    throw e
  }
}

// 行数据登记任务类型：任务事件与快照载荷不含 task_type，任务通知条目按类型路由依赖此登记
function registerRowTaskTypes(rows: TaskProgressTreeDTO[]): void {
  const taskStore = useTaskStore()
  rows.forEach((row) => {
    const id = row.taskProgress?.task?.id
    const taskType = row.taskProgress?.task?.taskType
    if (notNullish(id) && notNullish(taskType)) {
      taskStore.setTaskType(id, taskType)
    }
  })
}
</script>

<template>
  <base-view>
    <div class="export-task-manage-wrapper">
      <task-list
        ref="taskListRef"
        v-model:data="dataList"
        v-model:page="page"
        class="export-task-manage-search-table"
        toolbar-radius="var(--app-radius)"
        data-radius="var(--app-radius)"
        :search="exportTaskQueryParentPage"
        :selectable="false"
        @view="onViewRow"
      >
        <template #toolbarPrefix>
          <div
            v-if="arrayIsEmpty(dataList)"
            class="export-task-empty-hint"
          >
            暂无导出任务——在主页勾选作品或作品集后点「导出」发起
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
    </template>
  </base-view>
</template>

<style scoped>
.export-task-manage-wrapper {
  display: flex;
  flex-direction: column;
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  margin: 10px;
}
.export-task-manage-search-table {
  flex: 1;
  min-height: 0;
  width: 100%;
}
.export-task-empty-hint {
  width: 100%;
  text-align: center;
  color: var(--app-text-secondary);
  font-size: 13px;
  padding: 6px 0;
}
</style>
