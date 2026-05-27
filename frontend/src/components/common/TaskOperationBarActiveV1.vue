<script setup lang="ts">
import { TaskStatusEnum } from '@renderer/constants/TaskStatusEnum.ts'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { TaskOperationCodeEnum } from '@renderer/constants/TaskOperationCodeEnum.ts'
import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { useParentTaskStore } from '@renderer/store/UseParentTaskStore.ts'
import { computed, Ref } from 'vue'
import { TaskProgressTreeDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'

// props
const props = defineProps<{
  row: TaskProgressTreeDTO
  buttonClicked: (row: TaskProgressTreeDTO, code: TaskOperationCodeEnum) => void
}>()

// 变量
// 任务状态与操作按钮状态的对应关系
const taskStatusMapping: {
  [K in TaskStatusEnum]: {
    tooltip: string
    icon: string
    operation: TaskOperationCodeEnum
    processing: boolean
  }
} = {
  [TaskStatusEnum.CREATED]: {
    tooltip: '开始',
    icon: 'VideoPlay',
    operation: TaskOperationCodeEnum.START,
    processing: false
  },
  [TaskStatusEnum.PROCESSING]: {
    tooltip: '暂停',
    icon: 'VideoPause',
    operation: TaskOperationCodeEnum.PAUSE,
    processing: true
  },
  [TaskStatusEnum.WAITING]: {
    tooltip: '等待中',
    icon: 'Loading',
    operation: TaskOperationCodeEnum.PAUSE,
    processing: true
  },
  [TaskStatusEnum.PAUSING]: {
    tooltip: '暂停中',
    icon: 'Loading',
    operation: TaskOperationCodeEnum.PAUSE,
    processing: true
  },
  [TaskStatusEnum.PAUSED]: {
    tooltip: '继续',
    icon: 'RefreshRight',
    operation: TaskOperationCodeEnum.RESUME,
    processing: false
  },
  [TaskStatusEnum.STOPPING]: {
    tooltip: '停止中',
    icon: 'Loading',
    operation: TaskOperationCodeEnum.CANCEL,
    processing: true
  },
  [TaskStatusEnum.FINISHED]: {
    tooltip: '再次下载',
    icon: 'RefreshRight',
    operation: TaskOperationCodeEnum.RETRY,
    processing: false
  },
  [TaskStatusEnum.PARTLY_FINISHED]: {
    tooltip: '开始',
    icon: 'VideoPlay',
    operation: TaskOperationCodeEnum.START,
    processing: false
  },
  [TaskStatusEnum.FAILED]: {
    tooltip: '重试',
    icon: 'RefreshRight',
    operation: TaskOperationCodeEnum.RETRY,
    processing: false
  },
  [TaskStatusEnum.WAITING_FOR_INPUT]: {
    tooltip: '等待确认',
    icon: 'Loading',
    operation: TaskOperationCodeEnum.VIEW,
    processing: true
  }
}
// 任务进度信息Store
const taskStore = useTaskStore()
// 父任务进度信息Store
const parentTaskStore = useParentTaskStore()
// 任务状态
const status: Ref<number | undefined | null> = computed(() => {
  const taskId = props.row.taskProgress?.task?.id
  if (isNullish(taskId)) return props.row.taskProgress?.task?.status
  let tempStatus: number | undefined | null
  if (props.row.hasChildren) {
    tempStatus = parentTaskStore.getTask(taskId)?.status
  } else {
    tempStatus = taskStore.getTask(taskId)?.status
  }
  return isNullish(tempStatus) ? props.row.taskProgress?.task?.status : tempStatus
})
// 进度（百分比）
const schedule: Ref<number> = computed<number>((oldValue) => {
  const taskId = props.row.taskProgress?.task?.id
  if (isNullish(taskId)) return isNullish(oldValue) ? 0 : oldValue
  const tempStatus = props.row.hasChildren
    ? parentTaskStore.getTask(taskId)
    : taskStore.getTask(taskId)
  if (notNullish(tempStatus)) {
    const finished = tempStatus.finished
    const total = tempStatus.total
    if (isNullish(finished) || isNullish(total) || total === 0) {
      return 0
    }
    return Math.round((finished / total) * 100)
  } else {
    return 0
  }
})
// 进度（数据量）
const scheduleByte: Ref<string> = computed(() => {
  const taskId = props.row.taskProgress?.task?.id
  if (isNullish(taskId)) return '...'
  const tempStatus = taskStore.getTask(taskId)
  if (notNullish(tempStatus)) {
    const finishedBytes = tempStatus.finished
    let finished: string | undefined
    if (notNullish(finishedBytes)) {
      finished = formatBytes(finishedBytes)
    }
    const totalBytes = tempStatus.total
    let total: string | undefined
    if (notNullish(totalBytes)) {
      total = formatBytes(totalBytes)
    }
    if (isNullish(total)) {
      return isNullish(finished) ? '...' : finished
    } else {
      return finished + ' / ' + total
    }
  } else {
    return '...'
  }
})
const fractions: Ref<string> = computed(() => {
  if (props.row.hasChildren) {
    const taskId = props.row.taskProgress?.task?.id
    if (isNullish(taskId)) return ''
    const parentTask = parentTaskStore.getTask(taskId)
    if (isNullish(parentTask?.total)) {
      return ''
    }
    return (isNullish(parentTask?.finished) ? 0 : parentTask.finished) + '/' + parentTask.total
  } else {
    return ''
  }
})
// 方法
// 任务状态映射为按钮状态
function mapToButtonStatus(): {
  tooltip: string
  icon: string
  operation: TaskOperationCodeEnum
  processing: boolean
} {
  if (notNullish(status.value)) {
    return taskStatusMapping[status.value]
  } else {
    return taskStatusMapping['0']
  }
}
// 字节数转换为可读的数据量数值
function formatBytes(bytes: number) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'] // 单位数组
  let size = bytes
  let unitIndex = 0

  // 将字节数逐步除以 1024，直到找到合适的单位
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex++
  }

  // 返回格式化的字符串，保留两位小数
  return `${size.toFixed(2)} ${units[unitIndex]}`
}
</script>

<template>
  <div>
    <el-button-group
      v-show="
        (status !== TaskStatusEnum.PROCESSING && status !== TaskStatusEnum.WAITING && status !== TaskStatusEnum.PAUSED && status !== TaskStatusEnum.PAUSING && status !== TaskStatusEnum.STOPPING) ||
        row.hasChildren
      "
      style="margin-left: auto; margin-right: auto; flex-shrink: 0"
    >
      <el-tooltip :enterable="false" :show-after="650" :hide-after="0" content="详情">
        <el-button size="small" icon="View" @click="buttonClicked(row, TaskOperationCodeEnum.VIEW)" />
      </el-tooltip>
      <el-tooltip :content="mapToButtonStatus().tooltip" :enterable="false" :show-after="650" :hide-after="0">
        <el-button
          size="small"
          :icon="mapToButtonStatus().icon"
          :loading="mapToButtonStatus().processing && !row.taskProgress?.task?.continuable && !row.taskProgress?.task?.hasChild"
          @click="buttonClicked(row, mapToButtonStatus().operation)"
        />
      </el-tooltip>
      <el-tooltip content="取消" :enterable="false" :show-after="650" :hide-after="0">
        <el-button size="small" icon="CircleClose" @click="buttonClicked(row, TaskOperationCodeEnum.CANCEL)" />
      </el-tooltip>
      <el-tooltip content="删除" :enterable="false" :show-after="650" :hide-after="0">
        <el-button size="small" icon="Delete" @click="buttonClicked(row, TaskOperationCodeEnum.DELETE)" />
      </el-tooltip>
    </el-button-group>
    <div
      :class="{
        'task-operation-bar-parent-progress': true,
        'task-operation-bar-parent-progress-disappear':
          !(
            status === TaskStatusEnum.PROCESSING ||
            status === TaskStatusEnum.WAITING ||
            status === TaskStatusEnum.PAUSED ||
            status === TaskStatusEnum.PAUSING ||
            status === TaskStatusEnum.STOPPING
          ) || !row.hasChildren
      }"
    >
      <el-progress style="width: 100%" :percentage="schedule" text-inside :stroke-width="15">
        <template #default="{ percentage }">
          <span style="font-size: 14px; width: 100px">
            {{ percentage + '% ' }}
          </span>
          <span>
            {{ fractions }}
          </span>
        </template>
      </el-progress>
    </div>
    <el-progress
      v-show="
        (status === TaskStatusEnum.PROCESSING || status === TaskStatusEnum.WAITING || status === TaskStatusEnum.PAUSED || status === TaskStatusEnum.PAUSING || status === TaskStatusEnum.STOPPING) &&
        !row.hasChildren
      "
      style="width: 100%"
      :percentage="schedule"
      text-inside
      :stroke-width="24"
      @click="buttonClicked(row, mapToButtonStatus().operation)"
    >
      <template #default>
        <span style="font-size: 15px">{{ scheduleByte }}</span>
      </template>
    </el-progress>
  </div>
</template>

<style scoped>
.task-operation-bar-parent-progress {
  overflow: hidden;
  transition: height 0.3s ease;
}
.task-operation-bar-parent-progress-disappear {
  transition-delay: 1.4s;
  height: 0;
}
</style>
