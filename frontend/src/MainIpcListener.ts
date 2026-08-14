import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { ElMessageBox } from 'element-plus'
import { useParentTaskStore } from '@renderer/store/UseParentTaskStore.ts'
import ConfirmConfig from '@renderer/model/util/ConfirmConfig.ts'
import GotoPageConfig from '@renderer/model/util/GotoPageConfig.ts'
import { askGotoPage } from '@renderer/utils/PageUtil.ts'
import TaskScheduleDTO from '@renderer/model/dto/TaskScheduleDTO.ts'
import { initSlotSyncListener } from '@renderer/composables/useSlotSyncListener'
import { useReplaceConfirmStore } from '@renderer/store/UseReplaceConfirmStore'
import { useChangeConfirmStore, CHANGE_KIND_NAME } from '@renderer/store/UseChangeConfirmStore'
import { onMergeEvent } from '@renderer/composables/useMergeProgress'
import { Events } from '@wailsio/runtime'
import { TaskSnapshotDTO } from '@bindings/github.com/library-squirrel/backend/taskManager/models.js'

export function iniListener() {
  // 子任务事件（统一 topic，同 topic FIFO 保证事件顺序）
  Events.On('task-events', (event: any) => {
    const { type, data } = event.data as { type: string; data: any }
    switch (type) {
      case 'updateTask':
        useTaskStore().updateTask(data as any[])
        break
      case 'updateSchedule':
        useTaskStore().updateTaskSchedule((data as any[]).map((d: any) => new TaskScheduleDTO(d)))
        break
      case 'removeTask':
        useTaskStore().removeTask(data as number[])
        break
    }
  })

  // 父任务事件（统一 topic，同 topic FIFO 保证事件顺序）
  Events.On('parent-events', (event: any) => {
    const { type, data } = event.data as { type: string; data: any }
    switch (type) {
      case 'updateParentTask':
        useParentTaskStore().updateParentTask(data as any[])
        break
      case 'updateParentSchedule':
        useParentTaskStore().updateParentTaskSchedule((data as any[]).map((d: any) => new TaskScheduleDTO(d)))
        break
      case 'removeParentTask':
        useParentTaskStore().removeParentTask(data as number[])
        break
    }
  })

  // 合并事件（独立 topic，与任务面板解耦；阶段1 合并不进 taskManager 控制面）
  Events.On('merge-events', (event: any) => {
    const { type, data } = event.data as { type: string; data: any }
    onMergeEvent(type, data)
  })

  // 兼容：setTask / setParentTask（后端当前未发射，保留监听以备将来使用）
  Events.On('taskStatus-setTask', (event: any) => {
    const taskList = event.data as any[]
    useTaskStore().setTask(taskList)
  })

  Events.On('parentTaskStatus-setParentTask', (event: any) => {
    const taskList = event.data as any[]
    useParentTaskStore().setParentTask(taskList)
  })

  // 任务状态快照（快照模式：后端防抖推送实时快照 + 移除缓冲区，消除 Wails Emit 乱序问题）
  Events.On('task-snapshot', (event: any) => {
    const snapshot = TaskSnapshotDTO.createFrom(event.data)
    useTaskStore().loadSnapshot(snapshot.tasks, snapshot.removedTasks)
    useParentTaskStore().loadSnapshot(snapshot.parentTasks, snapshot.removedParentTasks)
  })

  // 作品重复检测 - Wails Events
  Events.On('taskStatus-duplicateDetected', (event: any) => {
    const data = event.data as { taskId: number; taskName: string; existingWorkId: number; existingWorkName: string }
    useReplaceConfirmStore().add(data)
  })

  // 工作目录外部文件变更 - Wails Events（待用户确认修复：移动/删除）
  Events.On('fsmonitor:change', (event: any) => {
    const data = event.data as { id: number; kind: number; fromPath: string; toPath: string; storeId: number }
    useChangeConfirmStore().add({
      id: data.id,
      kind: data.kind,
      kindName: CHANGE_KIND_NAME[data.kind] ?? '未知',
      fromPath: data.fromPath,
      toPath: data.toPath,
      storeId: data.storeId
    })
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

  // 跳转页面
  Events.On('goto-page', (event: any) => {
    const config = event.data as GotoPageConfig
    askGotoPage(config)
  })

  // 初始化插槽同步监听器
  initSlotSyncListener()
}
