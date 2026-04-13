import { TaskStatusEnum } from '../constant/TaskStatusEnum.ts'

export default class TaskProcessResponseDTO {
  /**
   * 状态
   */
  taskStatus: TaskStatusEnum

  /**
   * 错误
   */
  error: Error | undefined

  constructor(status: TaskStatusEnum, error?: Error) {
    this.taskStatus = status
    this.error = error
  }
}
