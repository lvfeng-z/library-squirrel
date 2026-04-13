import TaskCreateDTO from './TaskCreateDTO.ts'

/**
 * 创建任务DTO
 */
export default class PluginCreateTaskResponseDTO {
  /**
   * 插件在流式创建时用于标识父任务的id
   */
  pluginPid: string | undefined

  /**
   * 任务名称
   */
  taskName: string | undefined | null

  /**
   * 站点作品id
   */
  siteWorkId: string | undefined | null

  /**
   来源url
   */
  url: string | undefined | null

  /**
   * 插件数据
   */
  pluginData: { [key: string]: unknown } | string | undefined | null

  /**
   * 站点名称
   */
  siteName: string | undefined | null

  /**
   * 资源是否支持续传
   */
  continuable: boolean | undefined | null

  public static toTaskCreateDTO(pluginTaskResponseDTO: PluginCreateTaskResponseDTO): TaskCreateDTO {
    const result = new TaskCreateDTO()
    result.taskName = pluginTaskResponseDTO.taskName
    result.siteWorkId = pluginTaskResponseDTO.siteWorkId
    result.url = pluginTaskResponseDTO.url
    result.pluginData = pluginTaskResponseDTO.pluginData
    result.siteName = pluginTaskResponseDTO.siteName
    result.continuable = pluginTaskResponseDTO.continuable
    return result
  }
}
