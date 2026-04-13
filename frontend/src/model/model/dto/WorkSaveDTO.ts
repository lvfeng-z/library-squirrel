import WorkFullDTO from './WorkFullDTO.ts'

/**
 * 用于保存作品信息的DTO
 */
export default class WorkSaveDTO extends WorkFullDTO {
  /**
   * 任务id
   */
  taskId: number

  constructor(taskId: number, workFullDTO?: WorkFullDTO) {
    super(workFullDTO)
    this.taskId = taskId
  }
}
