/**
 * 任务状态枚举，与后端 TaskState 保持一致
 */
export enum TaskStatusEnum {
  CREATED = 0,
  WAITING = 1,
  PROCESSING = 2,
  PAUSING = 3,
  PAUSED = 4,
  STOPPING = 5,
  FINISHED = 6,
  FAILED = 7,
  PARTLY_FINISHED = 8
}
