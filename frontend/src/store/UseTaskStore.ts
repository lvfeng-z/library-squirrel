import { defineStore } from 'pinia'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import { useNotificationStore } from '@renderer/store/UseNotificationStore.ts'
import { useReminderStore } from '@renderer/store/UseReminderStore.ts'
import { type NewNotificationItem } from '@renderer/model/util/NotificationItem.ts'
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
          const notificationItem = buildTaskNotification(task)
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
          // 任务进入终态：通知原地更新为终态并脱离任务 store（清空 notificationId），提醒经聚合通道弹出
          if (notNullish(taskStoreObj.notificationId)) {
            const notifyId = taskStoreObj.notificationId
            const taskName = taskStoreObj.task.task?.taskName ?? taskStoreObj.task.task?.id
            const outcome = terminalOutcome(task.task.status)
            if (notNullish(outcome)) {
              useNotificationStore().update(notifyId, { terminal: true, level: outcome.level, statusText: outcome.text })
              useReminderStore().announce({
                level: outcome.level,
                title: `任务${outcome.text}`,
                message: `任务【${taskName}】${outcome.text}`,
                category: 'task'
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
            const notificationItem = buildTaskNotification(taskStoreObj.task)
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
        const taskStoreObj = this.tasks.get(scheduleDTO.id)
        const task = taskStoreObj?.task
        if (notNullish(task)) {
          if (notNullish(scheduleDTO.total)) {
            task.total = scheduleDTO.total
          }
          if (notNullish(scheduleDTO.finished)) {
            task.finished = scheduleDTO.finished
          }
          // 同步通知中心的进度（仅活跃通知；终态通知已脱离、notificationId 为空故跳过）
          if (notNullish(taskStoreObj?.notificationId)) {
            useNotificationStore().update(taskStoreObj.notificationId, {
              progress: { current: task.finished, total: task.total }
            })
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
     * 从快照加载任务状态（快照模式使用）。带迁移检测：对比替换前状态决定通知去留与终态提醒。
     * 1. 记录替换前各任务状态与通知引用（diff 基线），清空定时器
     * 2. 写入实时快照：活跃→活跃原地刷新进度；活跃→终态转终态保留并提醒
     * 3. 补充移除缓冲区：同样检测活跃→终态迁移；设置延迟移除定时器
     * 4. 收尾：撤下既未续用也未转终态保留的旧活跃通知（暂停/中间态/快照缺员）
     */
    loadSnapshot(liveItems: (TaskSnapshotItem | null)[], removedItems: (TaskSnapshotItem | null)[]): void {
      // 1. diff 基线与定时器清理
      this.pendingRemoveTimers.forEach((timer) => clearTimeout(timer))
      this.pendingRemoveTimers.clear()
      const previous = new Map<number, { status: number | undefined; notificationId: string | undefined }>()
      this.tasks.forEach((obj, id) => previous.set(id, { status: obj.task.task?.status, notificationId: obj.notificationId }))
      this.tasks.clear()

      const notificationStore = useNotificationStore()
      // 本轮转终态而保留的通知 id（收尾撤下判定时豁免）
      const retainedIds = new Set<string>()
      // 活跃→终态迁移：通知转终态保留（或漏建时补一条终态条目）+ 聚合提醒
      const handleTerminalTransition = (
        taskDTO: TaskProgressDTO,
        prevNotificationId: string | undefined,
        outcome: TerminalOutcome
      ): void => {
        const taskName = taskDTO.task?.taskName ?? taskDTO.task?.id
        if (notNullish(prevNotificationId)) {
          notificationStore.update(prevNotificationId, { terminal: true, level: outcome.level, statusText: outcome.text })
          retainedIds.add(prevNotificationId)
        } else {
          notificationStore.add({
            level: outcome.level,
            category: 'task',
            title: `任务【${taskName}】`,
            statusText: outcome.text,
            terminal: true,
            progress: { current: taskDTO.finished, total: taskDTO.total },
            route: { name: 'taskManage' }
          })
        }
        useReminderStore().announce({
          level: outcome.level,
          title: `任务${outcome.text}`,
          message: `任务【${taskName}】${outcome.text}`,
          category: 'task'
        })
      }

      // 2. 实时快照
      liveItems.filter(notNullish).forEach((item) => {
        const taskDTO = buildTaskProgressDTO(item)
        const prev = previous.get(item.id)
        const outcome = terminalOutcome(taskDTO.task?.status)
        let notificationId: string | undefined
        if (isActiveStatus(taskDTO.task?.status)) {
          if (notNullish(prev?.notificationId)) {
            // 活跃→活跃：通知原地刷新进度，id 保持稳定
            notificationStore.update(prev.notificationId, {
              progress: { current: taskDTO.finished, total: taskDTO.total }
            })
            notificationId = prev.notificationId
          } else {
            // 新出现的活跃任务：建通知
            notificationId = notificationStore.add(buildTaskNotification(taskDTO))
          }
        } else if (notNullish(outcome) && notNullish(prev) && isActiveStatus(prev.status)) {
          // 活跃→终态：转终态保留并脱离 + 提醒
          handleTerminalTransition(taskDTO, prev.notificationId, outcome)
        }
        // 其余（未见/已终态/暂停等中间态）：不新建通知，既有活跃通知由第 4 步收尾统一撤下
        this.tasks.set(item.id, { task: taskDTO, notificationId })
      })

      // 3. 移除缓冲区
      removedItems.filter(notNullish).forEach((item) => {
        const taskDTO = buildTaskProgressDTO(item)
        const prev = previous.get(item.id)
        const outcome = terminalOutcome(taskDTO.task?.status)
        if (notNullish(outcome) && notNullish(prev) && isActiveStatus(prev.status)) {
          // 活跃→终态（经移除缓冲到达）：同样迁移处理
          handleTerminalTransition(taskDTO, prev.notificationId, outcome)
        }
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

      // 4. 收尾撤下：未被续用（活跃→活跃）也未转终态保留的旧活跃通知
      previous.forEach((prev, id) => {
        if (
          notNullish(prev.notificationId) &&
          !retainedIds.has(prev.notificationId) &&
          this.tasks.get(id)?.notificationId !== prev.notificationId
        ) {
          notificationStore.remove(prev.notificationId)
        }
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

function buildTaskNotification(task: TaskProgressDTO): NewNotificationItem {
  return {
    level: 'info',
    category: 'task',
    title: `任务【${task.task?.taskName ?? task.task?.id}】`,
    statusText: '下载中',
    progress: { current: task.finished, total: task.total },
    route: { name: 'taskManage' }
  }
}

/** 终态结局：严重度 + 状态文案 */
interface TerminalOutcome {
  level: 'success' | 'warning' | 'error'
  text: string
}

/** 活跃态（进行中/等待中）判定 */
function isActiveStatus(status: number | undefined): boolean {
  return status === TaskStatusEnum.PROCESSING || status === TaskStatusEnum.WAITING
}

/** 终态结局映射；非终态返回 undefined */
function terminalOutcome(status: number | undefined): TerminalOutcome | undefined {
  if (status === TaskStatusEnum.FINISHED) {
    return { level: 'success', text: '完成' }
  }
  if (status === TaskStatusEnum.FAILED) {
    return { level: 'error', text: '失败' }
  }
  if (status === TaskStatusEnum.PARTLY_FINISHED) {
    return { level: 'warning', text: '部分完成' }
  }
  return undefined
}
