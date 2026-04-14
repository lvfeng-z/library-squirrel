import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { ElMessageBox, ElNotification } from 'element-plus'
import { useParentTaskStore } from '@renderer/store/UseParentTaskStore.ts'
import ConfirmConfig from '@renderer/model/util/ConfirmConfig.ts'
import GotoPageConfig from '@renderer/model/util/GotoPageConfig.ts'
import { h } from 'vue'
import NotifyConfig from '@renderer/model/util/NotifyConfig.ts'
import { isNullish } from '@renderer/utils/CommonUtil.ts'
import { askGotoPage } from '@renderer/utils/PageUtil.ts'
import TaskProgressDTO from '@renderer/model/model/dto/TaskProgressDTO.ts'
import TaskScheduleDTO from '@renderer/model/model/dto/TaskScheduleDTO.ts'
import TaskProgressMapTreeDTO from '@renderer/model/model/dto/TaskProgressMapTreeDTO.ts'
import { initSlotSyncListener } from '@renderer/composables/useSlotSyncListener'
import { Events } from '@wailsio/runtime'

export function iniListener() {
  // 任务队列 - 使用 Wails Events
  Events.On('taskStatus-setTask', (event: any) => {
    const taskList = event.data as TaskProgressDTO[]
    useTaskStore().setTask(taskList)
  })

  Events.On('taskStatus-updateTask', (event: any) => {
    const taskList = event.data as TaskProgressDTO[]
    useTaskStore().updateTask(taskList)
  })

  Events.On('taskStatus-updateSchedule', (event: any) => {
    const scheduleDTOList = event.data as TaskScheduleDTO[]
    useTaskStore().updateTaskSchedule(scheduleDTOList)
  })

  Events.On('taskStatus-removeTask', (event: any) => {
    const ids = event.data as number[]
    useTaskStore().removeTask(ids)
  })

  Events.On('parentTaskStatus-setParentTask', (event: any) => {
    const taskList = event.data as TaskProgressMapTreeDTO[]
    useParentTaskStore().setParentTask(taskList)
  })

  Events.On('parentTaskStatus-updateParentTask', (event: any) => {
    const taskList = event.data as TaskProgressMapTreeDTO[]
    useParentTaskStore().updateParentTask(taskList)
  })

  Events.On('parentTaskStatus-updateSchedule', (event: any) => {
    const taskList = event.data as TaskScheduleDTO[]
    useParentTaskStore().updateParentTaskSchedule(taskList)
  })

  Events.On('parentTaskStatus-removeParentTask', (event: any) => {
    const ids = event.data as number[]
    useParentTaskStore().removeParentTask(ids)
  })

  // 自定义确认弹窗 - Wails Events
  Events.On('custom-confirm', async (event: any) => {
    const { confirmId, config } = event.data as { confirmId: string; config: ConfirmConfig }
    try {
      await ElMessageBox.confirm(config.msg, config.title, {
        confirmButtonText: config.confirmButtonText,
        cancelButtonText: config.cancelButtonText,
        type: config.type
      })
      await Events.Emit('custom-confirm-echo', { confirmId, result: true })
    } catch {
      await Events.Emit('custom-confirm-echo', { confirmId, result: false })
    }
  })

  // 自定义通知 - Wails Events
  Events.On('custom-notify', (event: any) => {
    const config = event.data as NotifyConfig
    ElNotification({
      type: config.type,
      message: h(
        'span',
        {
          style: {
            display: '-webkit-box',
            '-webkit-box-orient': 'vertical',
            '-webkit-line-clamp': isNullish(config.maxRow) ? 3 : config.maxRow,
            overflow: 'hidden',
            'text-overflow': 'ellipsis'
          }
        },
        config.msg
      ),
      duration: config.duration
    })
  })

  // 跳转页面
  Events.On('goto-page', (event: any) => {
    const config = event.data as GotoPageConfig
    askGotoPage(config)
  })

  // 初始化插槽同步监听器
  initSlotSyncListener()
}
