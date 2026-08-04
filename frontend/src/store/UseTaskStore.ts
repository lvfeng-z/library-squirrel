import { defineStore } from 'pinia'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { useNotificationStore } from '@renderer/store/UseNotificationStore.ts'
import NotificationItem from '@renderer/model/util/NotificationItem.ts'
import { h } from 'vue'
import { TaskStatusEnum } from '@renderer/constants/TaskStatusEnum.ts'
import TaskScheduleDTO from '@renderer/model/dto/TaskScheduleDTO.ts'
import { copyIgnoreUndefined } from '@renderer/utils/ObjectUtil.ts'
import { TaskSnapshotItem } from '@bindings/github.com/library-squirrel/backend/taskManager/models.js'
import { TaskDTO } from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto'
import { TaskProgressDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'

/** 最近移除的任务 ID 缓存有效期（毫秒） */
const RECENTLY_REMOVED_TTL = 2000
/** removeTask 延迟移除的等待时间（毫秒），确保 Vue watcher 有时间将 store 中的终态同步到行数据 */
const REMOVE_DELAY = 300

/**
 * 从 taskSnapshotItem 构造 binding TaskProgressDTO
 */
function buildTaskProgressDTO(item: { id: number; taskName: string; status: number; total: number; finished: number }): TaskProgressDTO {
  const taskDTO = new TaskProgressDTO()
  taskDTO.task = new TaskDTO()
  taskDTO.task.id = item.id
  taskDTO.task.taskName = item.taskName
  taskDTO.task.status = item.status
  taskDTO.total = item.total
  taskDTO.finished = item.finished
  return taskDTO
}

/**
 * 创建初始化了 task 字段的空 TaskProgressDTO
 */
function createEmptyTaskProgressDTO(): TaskProgressDTO {
  const dto = new TaskProgressDTO()
  dto.task = new TaskDTO()
  return dto
}

/**
 * 将 IPC 事件中的 taskStateDTO（{id, taskName, status}）适配为 binding TaskProgressDTO
 */
function adaptStateEvent(data: any): TaskProgressDTO {
  const dto = new TaskProgressDTO()
  dto.task = new TaskDTO()
  dto.task.id = data.id
  dto.task.taskName = data.taskName
  dto.task.status = data.status
  return dto
}

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
    setTask(taskList: any[]): void {
      const taskStatus: Map<number, TaskStoreObj> = this.tasks
      taskList.forEach((raw) => {
        const task = adaptStateEvent(raw)
        if (isNullish(task.task?.id)) {
          throw new Error('UseTaskStore: 赋值任务失败，任务id为空')
        }
        const id = task.task.id
        // 取消待执行的延迟移除，防止误删重新创建的任务
        this.cancelPendingRemove(id)
        let notificationId: string | undefined
        // 只有进行中、等待中两种状态才推送到通知Store中
        if (TaskStatusEnum.PROCESSING === task.task.status || TaskStatusEnum.WAITING === task.task.status) {
          const notificationItem = createNotificationItem(task)
          notificationId = useNotificationStore().add(notificationItem)
        }
        taskStatus.set(id, { task, notificationId })
      })
    },
    hasTask(taskId: number): boolean {
      return this.tasks.has(taskId)
    },
    updateTask(taskList: any[]): void {
      taskList.forEach((raw) => {
        const task = adaptStateEvent(raw)
        if (isNullish(task.task?.id)) {
          throw new Error('UseTaskStore: 更新任务失败，任务id为空')
        }
        const id = task.task.id
        // 取消待执行的延迟移除，防止误删重新创建的任务
        this.cancelPendingRemove(id)
        let taskStoreObj = this.tasks.get(id)
        // store 中不存在时，若该 ID 最近被移除过则跳过自动创建，防止幽灵条目
        if (isNullish(taskStoreObj)) {
          if (this.recentlyRemovedIds.has(id)) {
            return
          }
          taskStoreObj = { task: createEmptyTaskProgressDTO(), notificationId: undefined }
          this.tasks.set(id, taskStoreObj)
        }
        if (task.task.status !== taskStoreObj.task.task?.status) {
          // 任务状态变化为完成或失败，解决通知Store中该任务的Promise
          if (notNullish(taskStoreObj.notificationId)) {
            if (task.task.status === TaskStatusEnum.FINISHED) {
              useNotificationStore().remove(taskStoreObj.notificationId, {
                type: 'success',
                msg: `任务【${taskStoreObj.task.task?.taskName}】完成`
              })
              taskStoreObj.notificationId = undefined
            } else if (task.task.status === TaskStatusEnum.FAILED) {
              useNotificationStore().remove(taskStoreObj.notificationId, {
                type: 'error',
                msg: `任务【${taskStoreObj.task.task?.taskName ?? taskStoreObj.task.task?.id}】失败`
              })
              taskStoreObj.notificationId = undefined
            } else if (task.task.status === TaskStatusEnum.PARTLY_FINISHED) {
              useNotificationStore().remove(taskStoreObj.notificationId, {
                type: 'warning',
                msg: `任务【${taskStoreObj.task.task?.taskName ?? taskStoreObj.task.task?.id}】部分完成`
              })
              taskStoreObj.notificationId = undefined
            }
          }
          // 如果状态为进行中、等待中，就推送到通知Store中
          if (
            isNullish(taskStoreObj.notificationId) &&
            (TaskStatusEnum.PROCESSING === task.task.status || TaskStatusEnum.WAITING === task.task.status)
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
    },
    /**
     * 从快照加载任务状态（快照模式使用）。
     * 1. 用实时快照全量替换 store
     * 2. 用移除缓冲区补充被移除任务的终态信息
     * 3. 为缓冲区中的任务设置延迟移除定时器
     */
    loadSnapshot(liveItems: (TaskSnapshotItem | null)[], removedItems: (TaskSnapshotItem | null)[]): void {
      // 1. 全量替换：清除所有旧数据和定时器
      this.pendingRemoveTimers.forEach((timer) => clearTimeout(timer))
      this.pendingRemoveTimers.clear()
      this.tasks.forEach((obj) => {
        if (notNullish(obj.notificationId)) {
          useNotificationStore().remove(obj.notificationId)
        }
      })
      this.tasks.clear()

      // 写入实时快照
      liveItems.filter(notNullish).forEach((item) => {
        const taskDTO = buildTaskProgressDTO(item)
        let notificationId: string | undefined
        if (taskDTO.task?.status === TaskStatusEnum.PROCESSING || taskDTO.task?.status === TaskStatusEnum.WAITING) {
          const notificationItem = createNotificationItem(taskDTO)
          notificationId = useNotificationStore().add(notificationItem)
        }
        this.tasks.set(item.id, { task: taskDTO, notificationId })
      })

      // 2. 补充移除缓冲区：写入终态 + 设置延迟移除定时器
      removedItems.filter(notNullish).forEach((item) => {
        const taskDTO = buildTaskProgressDTO(item)
        this.tasks.set(item.id, { task: taskDTO, notificationId: undefined })
        // 延迟移除
        const timer = setTimeout(() => {
          this.pendingRemoveTimers.delete(item.id)
          this.tasks.delete(item.id)
          this.recentlyRemovedIds.add(item.id)
          setTimeout(() => this.recentlyRemovedIds.delete(item.id), RECENTLY_REMOVED_TTL)
        }, REMOVE_DELAY)
        this.pendingRemoveTimers.set(item.id, timer)
      })
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
  notificationItem.title = `任务【${task.task?.taskName}】`
  notificationItem.render = () => h('div', {}, '下载中')
  return notificationItem
}
