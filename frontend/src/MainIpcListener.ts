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

export function iniListener() {
  // 任务队列
  window.electron.ipcRenderer.on('taskStatus-setTask', (_event, [taskList]: [TaskProgressDTO[]]) => useTaskStore().setTask(taskList))

  window.electron.ipcRenderer.on('taskStatus-updateTask', (_event, [taskList]: [TaskProgressDTO[]]) =>
    useTaskStore().updateTask(taskList)
  )

  window.electron.ipcRenderer.on('taskStatus-updateSchedule', (_event, [scheduleDTOList]: [TaskScheduleDTO[]]) =>
    useTaskStore().updateTaskSchedule(scheduleDTOList)
  )

  window.electron.ipcRenderer.on('taskStatus-removeTask', (_event, [ids]: [number[]]) => useTaskStore().removeTask(ids))

  window.electron.ipcRenderer.on('parentTaskStatus-setParentTask', (_event, [taskList]: [TaskProgressMapTreeDTO[]]) =>
    useParentTaskStore().setParentTask(taskList)
  )

  window.electron.ipcRenderer.on('parentTaskStatus-updateParentTask', (_event, [taskList]: [TaskProgressMapTreeDTO[]]) =>
    useParentTaskStore().updateParentTask(taskList)
  )

  window.electron.ipcRenderer.on('parentTaskStatus-updateSchedule', (_event, [taskList]: [TaskScheduleDTO[]]) =>
    useParentTaskStore().updateParentTaskSchedule(taskList)
  )

  window.electron.ipcRenderer.on('parentTaskStatus-removeParentTask', (_event, [ids]: [number[]]) =>
    useParentTaskStore().removeParentTask(ids)
  )

  // 自定义确认弹窗
  window.electron.ipcRenderer.on('custom-confirm', (_event: Electron.IpcRendererEvent, confirmId: string, config: ConfirmConfig) => {
    ElMessageBox.confirm(config.msg, config.title, {
      confirmButtonText: config.confirmButtonText,
      cancelButtonText: config.cancelButtonText,
      type: config.type
    })
      .then(() => window.electron.ipcRenderer.send('custom-confirm-echo', confirmId, true))
      .catch(() => window.electron.ipcRenderer.send('custom-confirm-echo', confirmId, false))
  })

  // 自定义通知
  window.electron.ipcRenderer.on('custom-notify', (_event: Electron.IpcRendererEvent, config: NotifyConfig) => {
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

  // // 已有资源替换确认弹窗
  // const resourceReplaceConfirmList: Ref<{ confirmId: string; msg: string }[]> = ref([])
  // let resourceReplaceConfirmState: boolean = false
  // window.electron.ipcRenderer.on(
  //   'task-queue-resource-replace-confirm',
  //   (_event: Electron.IpcRendererEvent, config: { confirmId: string; msg: string }) => {
  //     resourceReplaceConfirmList.value.push(config)
  //     if (!resourceReplaceConfirmState) {
  //       resourceReplaceConfirmState = true
  //       ElMessageBox.confirm(
  //         () => h(TaskQueueResourceReplaceConfirmList, { confirmList: resourceReplaceConfirmList.value }),
  //         `以下任务下载的作品已有可用的资源，是否替换？`,
  //         {
  //           confirmButtonText: '替换原有资源',
  //           cancelButtonText: '保留原有资源',
  //           type: 'warning'
  //         }
  //       )
  //         .then(() => {
  //           const responseConfirmIds = resourceReplaceConfirmList.value.map(
  //             (resourceReplaceConfirm) => resourceReplaceConfirm.confirmId
  //           )
  //           window.electron.ipcRenderer.send('task-queue-resource-replace-confirm-echo', responseConfirmIds, true)
  //           resourceReplaceConfirmList.value.splice(0, responseConfirmIds.length)
  //         })
  //         .catch(() => {
  //           const responseConfirmIds = resourceReplaceConfirmList.value.map(
  //             (resourceReplaceConfirm) => resourceReplaceConfirm.confirmId
  //           )
  //           window.electron.ipcRenderer.send('task-queue-resource-replace-confirm-echo', responseConfirmIds, false)
  //           resourceReplaceConfirmList.value.splice(0, responseConfirmIds.length)
  //         })
  //         .finally(() => (resourceReplaceConfirmState = false))
  //     }
  //   }
  // )

  window.electron.ipcRenderer.on('goto-page', (_event, config: GotoPageConfig) => askGotoPage(config))

  // 初始化插槽同步监听器
  initSlotSyncListener()
}
