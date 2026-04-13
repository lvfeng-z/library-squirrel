import PluginCreateTaskResponseDTO from './PluginCreateTaskResponseDTO.ts'
import PluginCreateParentTaskResponseDTO from './PluginCreateParentTaskResponseDTO.ts'

/**
 * 插件流式创建任务DTO
 */
export default class PluginStreamCreateTaskResponseDTO {
  /**
   * 任务类型（父任务 | 子任务）
   */
  taskType: 'child' | 'parent'

  /**
   * 插件任务创建DTO
   */
  task: PluginCreateTaskResponseDTO | PluginCreateParentTaskResponseDTO

  constructor(taskType: 'child' | 'parent', task: PluginCreateTaskResponseDTO | PluginCreateParentTaskResponseDTO) {
    this.taskType = taskType
    this.task = task
  }
}
