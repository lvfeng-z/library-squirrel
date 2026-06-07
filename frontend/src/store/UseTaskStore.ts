import { defineStore } from 'pinia'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { useNotificationStore } from '@renderer/store/UseNotificationStore.ts'
import NotificationItem from '@renderer/model/util/NotificationItem.ts'
import { h } from 'vue'
import { TaskStatusEnum } from '@renderer/constants/TaskStatusEnum.ts'
import TaskProgressDTO from '@renderer/model/model/dto/TaskProgressDTO.ts'
import TaskScheduleDTO from '@renderer/model/model/dto/TaskScheduleDTO.ts'
import { copyIgnoreUndefined } from '@renderer/utils/ObjectUtil.ts'

/** 最近移除的任务 ID 缓存有效期（毫秒） */
const RECENTLY_REMOVED_TTL = 2000
/** removeTask 延迟移除的等待时间（毫秒），确保 Vue watcher 有时间将 store 中的终态同步到行数据 */
const REMOVE_DELAY = 300

export const useTaskStore = defineStore('task', {
  state: (): {
    tasks: Map<number, TaskStoreObj>
    /** 最近被 removeTask 移除的任务 ID，防止过时的 updateTask 事件重新创建幽灵条目 */
    recentlyRemovedIds: Set<number>
    /** 延迟移除的定时器，key 为任务 ID，收到该 ID 的 setTask 时取消定时器以防止误删 */
    pendingRemoveTimers: Map<number, ReturnType<typeof setTimeout>>
  } => {
    return {
      tasks: new Map<number, TaskStoreObj>(),
      recentlyRemovedIds: new Set<number>(),
      pendingRemoveTimers: new Map<number, ReturnType<typeof setTimeout>>()
    }
  },
  actions: {
    getTask(taskId: number): TaskProgressDTO | undefined {
      return this.tasks.get(taskId)?.task
    },
    setTask(taskList: TaskProgressDTO[]): void {
      const taskStatus: Map<number, TaskStoreObj> = this.tasks
      taskList.forEach((task) => {
        if (isNullish(task.id)) {
          throw new Error('UseTaskStore: 赋值任务失败，任务id为空')
        }
        // 取消待执行的延迟移除，防止误删重新创建的任务
        this.cancelPendingRemove(task.id)
        let notificationId: string | undefined
        // 只有进行中、等待中两种状态才推送到通知Store中
        if (TaskStatusEnum.PROCESSING === task.status || TaskStatusEnum.WAITING === task.status) {
          const notificationItem = createNotificationItem(task)
          notificationId = useNotificationStore().add(notificationItem)
        }
        taskStatus.set(task.id, { task, notificationId })
      })
    },
    hasTask(taskId: number): boolean {
      return this.tasks.has(taskId)
    },
    updateTask(taskList: TaskProgressDTO[]): void {
      taskList.forEach((task) => {
        if (isNullish(task.id)) {
          throw new Error('UseTaskStore: 更新任务失败，任务id为空')
        }
        // 取消待执行的延迟移除，防止误删重新创建的任务
        this.cancelPendingRemove(task.id)
        let taskStoreObj = this.tasks.get(task.id)
        // store 中不存在时，若该 ID 最近被移除过则跳过自动创建，防止幽灵条目
        if (isNullish(taskStoreObj)) {
          if (this.recentlyRemovedIds.has(task.id)) {
            return
          }
          taskStoreObj = { task: new TaskProgressDTO(), notificationId: undefined }
          this.tasks.set(task.id, taskStoreObj)
        }
        if (task.status !== taskStoreObj.task.status) {
          // 任务状态变化为完成或失败，解决通知Store中该任务的Promise
          if (notNullish(taskStoreObj.notificationId)) {
            if (task.status === TaskStatusEnum.FINISHED) {
              useNotificationStore().remove(taskStoreObj.notificationId, {
                type: 'success',
                msg: `任务【${taskStoreObj.task.taskName}】完成`
              })
              taskStoreObj.notificationId = undefined
            } else if (task.status === TaskStatusEnum.FAILED) {
              useNotificationStore().remove(taskStoreObj.notificationId, {
                type: 'error',
                msg: `任务【${taskStoreObj.task.taskName ?? taskStoreObj.task.id}】失败`
              })
              taskStoreObj.notificationId = undefined
            } else if (task.status === TaskStatusEnum.PARTLY_FINISHED) {
              useNotificationStore().remove(taskStoreObj.notificationId, {
                type: 'warning',
                msg: `任务【${taskStoreObj.task.taskName ?? taskStoreObj.task.id}】部分完成`
              })
              taskStoreObj.notificationId = undefined
            }
          }
          // 如果状态为进行中、等待中，就推送到通知Store中
          if (
            isNullish(taskStoreObj.notificationId) &&
            (TaskStatusEnum.PROCESSING === task.status || TaskStatusEnum.WAITING === task.status)
          ) {
            copyIgnoreUndefined(taskStoreObj.task, task)
            const notificationItem = createNotificationItem(taskStoreObj.task)
            taskStoreObj.notificationId = useNotificationStore().add(notificationItem)
            return
          }
          copyIgnoreUndefined(taskStoreObj.task, task)
        }
      })
    },
    updateTaskSchedule(scheduleDTOList: TaskScheduleDTO[]): void {
      scheduleDTOList.forEach((rawData) => {
        const scheduleDTO = rawData instanceof TaskScheduleDTO ? rawData : new TaskScheduleDTO(rawData)
        if (isNullish(scheduleDTO.id)) {
          throw new Error('UseTaskStore: 更新任务进度失败，任务id为空')
        }
        const task = this.getTask(scheduleDTO.id)
        if (notNullish(task)) {
          if (notNullish(scheduleDTO.status)) {
            task.status = scheduleDTO.status
          }
          if (notNullish(scheduleDTO.total)) {
            task.total = scheduleDTO.total
          }
          if (notNullish(scheduleDTO.finished)) {
            task.finished = scheduleDTO.finished
          }
        }
      })
    },
    removeTask(ids: number[]) {
      if (arrayNotEmpty(ids)) {
        ids.forEach((id) => {
          // 若已有待执行的延迟移除，先清除（避免重复定时器）
          this.cancelPendingRemove(id)
          // 不立即删除，而是设置延迟定时器，让 Vue watcher 有时间将 store 中的终态同步到行数据
          const timer = setTimeout(() => {
            this.pendingRemoveTimers.delete(id)
            const taskStoreObj = this.tasks.get(id)
            if (notNullish(taskStoreObj?.notificationId)) {
              useNotificationStore().remove(taskStoreObj.notificationId)
            }
            this.tasks.delete(id)
            // 记录到最近移除集合，防止并发事件重新创建幽灵条目
            this.recentlyRemovedIds.add(id)
            setTimeout(() => this.recentlyRemovedIds.delete(id), RECENTLY_REMOVED_TTL)
          }, REMOVE_DELAY)
          this.pendingRemoveTimers.set(id, timer)
        })
      }
    },
    /** 取消指定任务的延迟移除定时器 */
    cancelPendingRemove(taskId: number) {
      const timer = this.pendingRemoveTimers.get(taskId)
      if (notNullish(timer)) {
        clearTimeout(timer)
        this.pendingRemoveTimers.delete(taskId)
      }
    }
  }
})

export type TaskStoreObj = {
  /**
   * 任务进度DTO
   */
  task: TaskProgressDTO
  /**
   * 通知id
   */
  notificationId: string | undefined
}

function createNotificationItem(task: TaskProgressDTO): NotificationItem {
  const notificationItem = new NotificationItem()
  notificationItem.title = `任务【${task.taskName}】`
  notificationItem.render = () => h('div', {}, '下载中')
  return notificationItem
}
