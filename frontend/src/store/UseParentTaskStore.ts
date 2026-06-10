import { defineStore } from 'pinia'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import TaskScheduleDTO from '@renderer/model/dto/TaskScheduleDTO.ts'
import { copyIgnoreUndefined } from '@renderer/utils/ObjectUtil.ts'
import { TaskSnapshotItem } from '@bindings/github.com/library-squirrel/backend/taskManager/models.js'
import { TaskProgressDTO, TaskDTO } from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto'

/** 最近移除的父任务 ID 缓存有效期（毫秒） */
const RECENTLY_REMOVED_TTL = 2000
/** removeParentTask 延迟移除的等待时间（毫秒），确保 Vue watcher 有时间将 store 中的终态同步到行数据 */
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

export const useParentTaskStore = defineStore('parentTask', {
  state: (): {
    parentTasks: Map<number, TaskProgressDTO>
    /** 最近被 removeParentTask 移除的任务 ID，防止过时的 updateParentTask 事件重新创建幽灵条目 */
    recentlyRemovedIds: Set<number>
    /** 延迟移除的定时器，key 为任务 ID，收到该 ID 的 setParentTask 时取消定时器以防止误删 */
    pendingRemoveTimers: Map<number, ReturnType<typeof setTimeout>>
  } => {
    return {
      parentTasks: new Map<number, TaskProgressDTO>(),
      recentlyRemovedIds: new Set<number>(),
      pendingRemoveTimers: new Map<number, ReturnType<typeof setTimeout>>()
    }
  },
  actions: {
    getTask(taskId: number): TaskProgressDTO | undefined {
      return this.parentTasks.get(taskId)
    },
    hasTask(taskId: number): boolean {
      return this.parentTasks.has(taskId)
    },
    setParentTask(taskList: any[]): void {
      const taskStatus = this.parentTasks
      taskList.forEach((raw) => {
        const task = adaptStateEvent(raw)
        if (isNullish(task.task?.id)) {
          throw new Error('UseTaskStatusStore: 赋值父任务失败，任务id为空')
        }
        // 取消待执行的延迟移除，防止误删重新创建的任务
        this.cancelPendingRemove(task.task.id)
        taskStatus.set(task.task.id, task)
      })
    },
    updateParentTask(taskList: any[]): void {
      const taskStatus = this.parentTasks
      taskList.forEach((raw) => {
        const task = adaptStateEvent(raw)
        if (isNullish(task.task?.id)) {
          throw new Error('UseTaskStatusStore: 更新父任务失败，任务id为空')
        }
        const id = task.task.id
        // 取消待执行的延迟移除，防止误删重新创建的任务
        this.cancelPendingRemove(id)
        let oldTask = taskStatus.get(id)
        // store 中不存在时，若该 ID 最近被移除过则跳过自动创建，防止幽灵条目
        if (isNullish(oldTask)) {
          if (this.recentlyRemovedIds.has(id)) {
            return
          }
          oldTask = new TaskProgressDTO()
          oldTask.task = new TaskDTO()
          taskStatus.set(id, oldTask)
        }
        copyIgnoreUndefined(oldTask, task)
        console.log('update P task', id, oldTask.task?.status)
      })
    },
    updateParentTaskSchedule(scheduleDTOList: TaskScheduleDTO[]): void {
      const taskStatus = this.parentTasks
      scheduleDTOList.forEach((rawData) => {
        const scheduleDTO = rawData instanceof TaskScheduleDTO ? rawData : new TaskScheduleDTO(rawData)
        if (isNullish(scheduleDTO.id)) {
          throw new Error('UseTaskStatusStore: 更新父任务进度失败，任务id为空')
        }
        const task = taskStatus.get(scheduleDTO.id)
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
    removeParentTask(ids: number[]) {
      if (arrayNotEmpty(ids)) {
        ids.forEach((id) => {
          // 若已有待执行的延迟移除，先清除（避免重复定时器）
          this.cancelPendingRemove(id)
          console.log('setTimer P task', id)
          // 不立即删除，而是设置延迟定时器，让 Vue watcher 有时间将 store 中的终态同步到行数据
          const timer = setTimeout(() => {
            this.pendingRemoveTimers.delete(id)
            this.parentTasks.delete(id)
            console.log('remove P task', id)
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
     * 从快照加载父任务状态（快照模式使用）。
     * 1. 用实时快照全量替换 store
     * 2. 用移除缓冲区补充被移除任务的终态信息
     * 3. 为缓冲区中的任务设置延迟移除定时器
     */
    loadSnapshot(liveItems: (TaskSnapshotItem | null)[], removedItems: (TaskSnapshotItem | null)[]): void {
      // 1. 全量替换
      this.pendingRemoveTimers.forEach((timer) => clearTimeout(timer))
      this.pendingRemoveTimers.clear()
      this.parentTasks.clear()

      // 写入实时快照
      liveItems.filter(notNullish).forEach((item) => {
        const taskDTO = buildTaskProgressDTO(item)
        this.parentTasks.set(item.id, taskDTO)
      })

      // 2. 补充移除缓冲区
      removedItems.filter(notNullish).forEach((item) => {
        const taskDTO = buildTaskProgressDTO(item)
        this.parentTasks.set(item.id, taskDTO)
        const timer = setTimeout(() => {
          this.pendingRemoveTimers.delete(item.id)
          console.log('remove P task', item.id)
          this.parentTasks.delete(item.id)
          this.recentlyRemovedIds.add(item.id)
          setTimeout(() => this.recentlyRemovedIds.delete(item.id), RECENTLY_REMOVED_TTL)
        }, REMOVE_DELAY)
        this.pendingRemoveTimers.set(item.id, timer)
      })
    }
  }
})
