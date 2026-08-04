<script setup lang="ts">
import DialogMode from '../../model/util/DialogMode'
import { computed, h, nextTick, Ref, ref, VNode } from 'vue'
import TaskList from '../common/TaskList.vue'
import TaskControlBar from '../common/TaskControlBar.vue'
import { TaskStatusEnum } from '../../constants/TaskStatusEnum.ts'
import { ElMessage, ElMessageBox, ElTag } from 'element-plus'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import FormDialog from '@renderer/components/dialogs/FormDialog.vue'
import { TaskDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import { TaskProgressTreeDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { TaskQueryDTO } from '@bindings/github.com/library-squirrel/backend/task/models'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model'
import { isNotBlank } from '@renderer/utils/StringUtil.ts'
import { taskApi } from '@renderer/apis/http'
import { QueryAttribute } from '@bindings/github.com/library-squirrel/backend/base/query/models'
import { newPage } from '@renderer/utils/Pager.ts'
import { useTaskOperations } from '@renderer/composables/useTaskOperations'
import { TaskOperationCodeEnum } from '../../constants/TaskOperationCodeEnum.ts'

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
// TaskList 组件实例
const taskListRef = ref()
// 下级任务
const children: Ref<TaskProgressTreeDTO[]> = ref([])
// 任务SearchTable的分页
const page: Ref<Page<TaskProgressTreeDTO>> = ref(newPage<TaskProgressTreeDTO>())
const formTask = computed(() => formData.value.taskProgress?.task ?? new TaskDTO())
// 是否为父任务
const isParent = computed(() => formTask.value.hasChild === true)
let parentCache: TaskProgressTreeDTO | undefined = undefined
// 任务信息折叠面板：父任务模式下可收起表单，把空间让给子任务列表
const formCollapseActive: Ref<string[]> = ref(['taskInfo'])
const isFormCollapsed = computed(() => !formCollapseActive.value.includes('taskInfo'))
// 列表高度随表单折叠状态切换：折叠时占满弹窗内容区，展开时让出表单空间
const taskListHeight = computed(() => (isFormCollapsed.value ? 'calc(90vh - 200px)' : 'calc(90vh - 80px)'))
// 选中的子任务行（来自 TaskList）；为空时标题栏操作当前任务 formData，非空时批量操作选中子任务
const selectedChildren: Ref<TaskProgressTreeDTO[]> = ref([])
const { buildBatchHandler } = useTaskOperations()
// 标题栏统一控制：一次调用批量接口，完成后刷新子任务列表
const handleBatchOperation = buildBatchHandler({
  onDone: () => taskListRef.value?.doSearch()
})
// 标题栏操作分发：删除需二次确认；删除当前任务（未选中时）确认后关闭弹窗
async function handleTitleOperation(rows: TaskProgressTreeDTO[], code: TaskOperationCodeEnum) {
  const isBatch = selectedChildren.value.length > 0
  if (code === TaskOperationCodeEnum.DELETE) {
    const msg = isBatch ? `确认删除选中的 ${rows.length} 个子任务？` : '确认删除当前任务？'
    try {
      await ElMessageBox.confirm(msg, isBatch ? '批量删除' : '删除任务', { type: 'warning' })
    } catch {
      return
    }
  }
  await handleBatchOperation(rows, code)
  if (code === TaskOperationCodeEnum.DELETE && !isBatch) {
    state.value = false
  }
}

// 方法
// 子任务分页查询：按当前任务 pid 拉取其子任务（query 由 TaskList 构建后传入）
async function taskQueryChildrenTaskPage(p: Page<TaskProgressTreeDTO>, query: TaskQueryDTO): Promise<Page<TaskProgressTreeDTO> | undefined> {
  const pid = formTask.value.id
  if (isNullish(pid)) {
    return undefined
  }
  query.pid = new QueryAttribute({ value: pid })
  try {
    const response = await taskApi.taskQueryChildrenTaskPage(p, query)
    return response.data
  } catch (e: any) {
    ElMessage.error(e.message)
    return undefined
  }
}
// 开关dialog：加载子任务列表
function handleOpen() {
  nextTick(() => {
    taskListRef.value?.doSearch()
  })
}
// 查看子任务：下钻到下一级任务集
function onViewChild(row: TaskProgressTreeDTO) {
  parentCache = formData.value
  formData.value = row
  children.value = []
  nextTick(() => taskListRef.value?.doSearch())
}
// 转到父任务
function toParent() {
  if (notNullish(parentCache)) formData.value = parentCache
  nextTick(() => taskListRef.value?.doSearch())
}
// 表单顶部状态标签渲染
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
</script>

<template>
  <form-dialog
    v-model:form-data="formData"
    v-model:state="state"
    :mode="props.mode"
    @open="handleOpen"
  >
    <template #header>
      <el-button
        v-show="!isParent"
        icon="ArrowLeftBold"
        type="primary"
        @click="toParent"
      >
        查看任务集
      </el-button>
    </template>
    <template #form>
      <el-collapse v-model="formCollapseActive">
        <el-collapse-item
          name="taskInfo"
        >
          <template #title>
            <div class="task-dialog-collapse-title">
              <span class="task-dialog-collapse-title-text">任务信息</span>
              <span
                class="task-dialog-collapse-operations"
                @click.stop
              >
                <!-- 标题栏统一控制：未选中操作当前任务、选中批量操作子任务 -->
                <task-control-bar
                  :rows="selectedChildren.length > 0 ? selectedChildren : [formData]"
                  :button-clicked="handleTitleOperation"
                />
              </span>
            </div>
          </template>
          <div>
            <el-row>
              <el-col>
                <el-form-item label="名称">
                  <el-input v-model="formTask.taskName" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row>
              <el-col>
                <el-form-item label="来源">
                  <el-input v-model="formTask.url" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row>
              <el-col :span="7">
                <el-form-item label="站点">
                  <el-input v-model="formTask.siteId" />
                </el-form-item>
              </el-col>
              <el-col
                v-if="!isParent"
                :span="17"
              >
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
                  <el-date-picker
                    v-model="formTask.createTime"
                    type="datetime"
                  />
                </el-form-item>
              </el-col>
              <el-col :span="7">
                <el-form-item label="修改时间">
                  <el-date-picker
                    v-model="formTask.updateTime"
                    type="datetime"
                  />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row v-if="isNotBlank(formTask.errorMessage)">
              <el-col>
                <el-form-item label="异常信息">
                  <el-input
                    v-model="formTask.errorMessage"
                    type="textarea"
                    autosize
                  />
                </el-form-item>
              </el-col>
            </el-row>
          </div>
        </el-collapse-item>
      </el-collapse>
    </template>
    <template #afterForm>
      <task-list
        v-show="isParent"
        ref="taskListRef"
        v-model:data="children"
        v-model:page="page"
        class="task-dialog-search-table"
        :style="{ height: taskListHeight }"
        :search="taskQueryChildrenTaskPage"
        :selectable="true"
        :multi-select="true"
        @view="onViewChild"
        @selection-change="(rows: TaskProgressTreeDTO[]) => (selectedChildren = rows)"
      />
    </template>
  </form-dialog>
</template>

<style scoped>
.task-dialog-collapse-title {
  display: flex;
  align-items: center;
  width: 100%;
}
.task-dialog-collapse-title-text {
  flex-grow: 1;
}
.task-dialog-collapse-operations {
  flex-shrink: 0;
  margin-right: 20px;
}
</style>
