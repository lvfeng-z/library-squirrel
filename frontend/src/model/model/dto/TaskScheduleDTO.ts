import { TaskStatusEnum } from '../constant/TaskStatusEnum.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 任务进度适配器，兼容两种数据格式：
 * - 新 binding 格式：TaskProgressDTO = {task: {id, status, ...}, total, finished, ...}
 * - 旧 flat 格式（IPC 事件）：{id, pid, status, total, finished}
 */
export default class TaskScheduleDTO {
  /**
   * 主键
   */
  id: number | undefined | null

  /**
   * 上级任务id
   */
  pid: number | undefined | null

  /**
   * 状态
   */
  status: TaskStatusEnum | undefined | null

  /**
   * 总量
   */
  total: number | undefined | null

  /**
   * 已完成的量
   */
  finished: number | undefined | null

  constructor(data?: any) {
    if (notNullish(data)) {
      if (notNullish(data.task)) {
        // 新 binding 格式：TaskProgressDTO
        this.id = data.task.id
        this.pid = data.task.pid
        this.status = data.task.status
      } else {
        // 旧 flat 格式（IPC 事件）
        this.id = data.id
        this.pid = data.pid
        this.status = data.status
      }
      this.total = data.total
      this.finished = data.finished
    }
  }
}
