import { useTaskStore } from '@renderer/store/UseTaskStore.ts'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useParentTaskStore } from '@renderer/store/UseParentTaskStore.ts'
import ConfirmConfig from '@renderer/model/util/ConfirmConfig.ts'
import GotoPageConfig from '@renderer/model/util/GotoPageConfig.ts'
import { askGotoPage } from '@renderer/utils/PageUtil.ts'
import TaskScheduleDTO from '@renderer/model/dto/TaskScheduleDTO.ts'
import { initSlotSyncListener } from '@renderer/composables/useSlotSyncListener'
import { useReplaceConfirmStore } from '@renderer/store/UseReplaceConfirmStore'
import { useChangeConfirmStore, changeKindName } from '@renderer/store/UseChangeConfirmStore'
import { onMergeEvent } from '@renderer/composables/useMergeProgress'
import { onExportEvent } from '@renderer/composables/useExportProgress'
import { useShareStore } from '@renderer/store/UseShareStore'
import { useShareReceiveStore } from '@renderer/store/UseShareReceiveStore'
import { shareConsumePendingLink } from '@renderer/apis/http/wrappers/share'
import { Events } from '@wailsio/runtime'
import { TaskSnapshotDTO } from '@bindings/github.com/library-squirrel/backend/taskManager/models.js'

// 自动修复聚合提示：短时间内多起自动处理合并为一条提示（避免弹窗刷屏；详见日志为单一真相源）
let autoHandledCount = 0
let autoHandledTimer: ReturnType<typeof setTimeout> | null = null

function pushAutoHandledNotice() {
  autoHandledCount += 1
  if (autoHandledTimer !== null) return
  autoHandledTimer = setTimeout(() => {
    const count = autoHandledCount
    autoHandledCount = 0
    autoHandledTimer = null
    ElMessage({
      message: `已自动处理 ${count} 条外部变更，详见日志`,
      type: 'success'
    })
  }, 800)
}

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

  // 导出事件（独立 topic，异步导出进度/完成）
  Events.On('export-events', (event: any) => {
    const { type, data } = event.data as { type: string; data: any }
    onExportEvent(type, data)
  })

  // 分享事件（独立 topic，分享发布进度/完成/会话状态/深链到达）
  Events.On('share-events', (event: any) => {
    const { type, data } = event.data as { type: string; data: any }
    switch (type) {
      case 'receive-link':
        // 深链到达：打开接收分享对话框（链接预填），并清空后端待处理缓存
        // （事件送达即前端已就绪，缓存使命完成——防下次启动被消费式拉取重复打开）
        useShareReceiveStore().openWith(data as string)
        void shareConsumePendingLink()
        break
      default:
        useShareStore().onShareEvent(type, data)
    }
  })
  // 冷启动深链兜底：深链事件可能先于前端就绪（本监听注册）而丢失，消费式拉取缓存的
  // 待处理链接（空串=无）
  void shareConsumePendingLink().then((link: string) => {
    if (link !== '') useShareReceiveStore().openWith(link)
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
    const data = event.data as {
      taskId: number
      taskName: string
      existingWorkId: number
      existingWorkName: string
      conflictRoles?: string[] | null
    }
    useReplaceConfirmStore().add(data)
  })

  // 工作目录外部文件变更 - Wails Events（待用户确认修复：移动/删除；store 域资源文件与 backup 域保管清单行）
  Events.On('fsmonitor:change', (event: any) => {
    const data = event.data as {
      id: number
      domain: number
      kind: number
      fromPath: string
      toPath: string
      storeId: number
      backupId: number
      /** 是否已由自动修复模式处理（true 时不入队，前端改为聚合提示） */
      autoHandled?: boolean
    }
    // 已自动处理：不弹确认框，聚合提示（计数防抖合并批量）
    if (data.autoHandled === true) {
      pushAutoHandledNotice()
      return
    }
    useChangeConfirmStore().add({
      id: data.id,
      domain: data.domain,
      kind: data.kind,
      kindName: changeKindName(data.domain, data.kind),
      fromPath: data.fromPath,
      toPath: data.toPath,
      storeId: data.storeId,
      backupId: data.backupId
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
