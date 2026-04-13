<script setup lang="ts">
import { TaskStatusEnum } from '@renderer/constants/TaskStatusEnum.ts'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { TaskOperationCodeEnum } from '@renderer/constants/TaskOperationCodeEnum.ts'
import { computed, Ref } from 'vue'
import TaskProgressTreeDTO from '@renderer/model/model/dto/TaskProgressTreeDTO.ts'
import TaskTreeDTO from '@renderer/model/model/dto/TaskTreeDTO.ts'

// props
const props = defineProps<{
  row: TaskProgressTreeDTO
  buttonClicked: (row: TaskTreeDTO, code: TaskOperationCodeEnum) => void
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
  [TaskStatusEnum.WAITING_USER_INPUT]: {
    tooltip: '等待用户操作',
    icon: 'RefreshRight',
    operation: TaskOperationCodeEnum.PAUSE,
    processing: false
  },
  [TaskStatusEnum.PAUSE]: {
    tooltip: '继续',
    icon: 'RefreshRight',
    operation: TaskOperationCodeEnum.RESUME,
    processing: false
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
  }
}
// 进度（百分比）
const schedule: Ref<number> = computed(() => {
  const finished = props.row.finished
  const total = props.row.total
  if (isNullish(finished) || isNullish(total) || total === 0) {
    return 0
  }
  return Math.round((finished / total) * 100)
})
// 进度（数据量）
const scheduleByte: Ref<string> = computed(() => {
  const finishedBytes = props.row.finished
  let finished: string | undefined
  if (notNullish(finishedBytes)) {
    finished = formatBytes(finishedBytes)
  }
  const totalBytes = props.row.total
  let total: string | undefined
  if (notNullish(totalBytes)) {
    total = formatBytes(totalBytes)
  }
  if (isNullish(total)) {
    return isNullish(finished) ? '...' : finished
  } else {
    return finished + ' / ' + total
  }
})

// 方法
// 任务状态映射为按钮状态
function mapToButtonStatus(row: TaskTreeDTO): {
  tooltip: string
  icon: string
  operation: TaskOperationCodeEnum
  processing: boolean
} {
  if (notNullish(row.status)) {
    return taskStatusMapping[row.status]
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
        (row.status !== TaskStatusEnum.PROCESSING && row.status !== TaskStatusEnum.WAITING && row.status !== TaskStatusEnum.PAUSE) ||
        row.hasChildren
      "
      style="margin-left: auto; margin-right: auto; flex-shrink: 0"
    >
      <el-tooltip v-if="props.row.isCollection" :enterable="false" :show-after="650" :hide-after="0" content="详情">
        <el-button size="small" icon="View" @click="buttonClicked(row, TaskOperationCodeEnum.VIEW)" />
      </el-tooltip>
      <el-tooltip :content="mapToButtonStatus(row).tooltip" :enterable="false" :show-after="650" :hide-after="0">
        <el-button
          size="small"
          :icon="mapToButtonStatus(row).icon"
          :loading="mapToButtonStatus(row).processing && !row.continuable && !row.isCollection"
          @click="buttonClicked(row, mapToButtonStatus(row).operation)"
        ></el-button>
      </el-tooltip>
      <el-tooltip content="取消" :enterable="false" :show-after="650" :hide-after="0">
        <el-button size="small" icon="CircleClose" @click="buttonClicked(row, TaskOperationCodeEnum.CANCEL)" />
      </el-tooltip>
      <el-tooltip content="删除" :enterable="false" :show-after="650" :hide-after="0">
        <el-button size="small" icon="Delete" @click="buttonClicked(row, TaskOperationCodeEnum.DELETE)" />
      </el-tooltip>
    </el-button-group>
    <transition name="task-operation-bar-el-progress-fade">
      <div
        v-if="
          (row.status === TaskStatusEnum.PROCESSING || row.status === TaskStatusEnum.WAITING || row.status === TaskStatusEnum.PAUSE) &&
          row.hasChildren
        "
      >
        <el-progress style="width: 100%" :percentage="schedule" text-inside :stroke-width="15" striped striped-flow :duration="5">
          <template #default="{ percentage }">
            <span style="font-size: 14px">
              {{ percentage + '% ' + row.finished + ' / ' + row.total }}
            </span>
          </template>
        </el-progress>
      </div>
    </transition>
    <el-progress
      v-show="
        (row.status === TaskStatusEnum.PROCESSING || row.status === TaskStatusEnum.WAITING || row.status === TaskStatusEnum.PAUSE) &&
        !row.hasChildren
      "
      style="width: 100%"
      :percentage="schedule"
      text-inside
      :stroke-width="24"
      striped
      striped-flow
      :duration="5"
      @click="buttonClicked(row, mapToButtonStatus(row).operation)"
    >
      <template #default>
        <span style="font-size: 15px">{{ scheduleByte }}</span>
      </template>
    </el-progress>
  </div>
</template>

<style scoped>
.task-operation-bar-el-progress-fade-enter-active,
.task-operation-bar-el-progress-fade-leave-active {
  transition: opacity 1.5s;
}
.task-operation-bar-el-progress-fade-enter,
.task-operation-bar-el-progress-fade-leave-to {
  opacity: 0;
}
</style>
