import { defineStore } from 'pinia'
import { arrayNotEmpty, isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'
import TaskProgressDTO from '@renderer/model/model/dto/TaskProgressDTO.ts'
import TaskScheduleDTO from '@renderer/model/model/dto/TaskScheduleDTO.ts'
import { copyIgnoreUndefined } from '@renderer/utils/ObjectUtil.ts'

/** 最近移除的父任务 ID 缓存有效期（毫秒） */
const RECENTLY_REMOVED_TTL = 2000

export const useParentTaskStore = defineStore('parentTask', {
  state: (): {
    parentTasks: Map<number, TaskProgressDTO>
    /** 最近被 removeParentTask 移除的任务 ID，防止过时的 updateParentTask 事件重新创建幽灵条目 */
    recentlyRemovedIds: Set<number>
  } => {
    return {
      parentTasks: new Map<number, TaskProgressDTO>(),
      recentlyRemovedIds: new Set<number>()
    }
  },
  actions: {
    getTask(taskId: number): TaskProgressDTO | undefined {
      return this.parentTasks.get(taskId)
    },
    hasTask(taskId: number): boolean {
      return this.parentTasks.has(taskId)
    },
    setParentTask(taskList: TaskProgressDTO[]): void {
      const taskStatus = this.parentTasks
      taskList.forEach((task) => {
        if (isNullish(task.id)) {
          throw new Error('UseTaskStatusStore: 赋值父任务失败，任务id为空')
        }
        taskStatus.set(task.id, task)
      })
    },
    updateParentTask(taskList: TaskProgressDTO[]): void {
      const taskStatus = this.parentTasks
      taskList.forEach((task) => {
        if (isNullish(task.id)) {
          throw new Error('UseTaskStatusStore: 更新父任务失败，任务id为空')
        }
        let oldTask = taskStatus.get(task.id)
        // store 中不存在时，若该 ID 最近被移除过则跳过自动创建，防止幽灵条目
        if (isNullish(oldTask)) {
          if (this.recentlyRemovedIds.has(task.id)) {
            return
          }
          oldTask = new TaskProgressDTO()
          taskStatus.set(task.id, oldTask)
        }
        copyIgnoreUndefined(oldTask, task)
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
    removeParentTask(ids: number[]) {
      const taskStatus = this.parentTasks
      if (arrayNotEmpty(ids)) {
        ids.forEach((id) => {
          taskStatus.delete(id)
          // 记录到最近移除集合，防止并发事件重新创建幽灵条目
          this.recentlyRemovedIds.add(id)
          setTimeout(() => this.recentlyRemovedIds.delete(id), RECENTLY_REMOVED_TTL)
        })
      }
    }
  }
})
