import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 资源
 */
export default class ResourceQueryDTO extends BaseQueryDTO {
  /**
   * 作品id
   */
  workId?: number | number[] | undefined | null

  /**
   * 任务id
   */
  taskId?: number | undefined | null

  /**
   * 状态（0：停用，1：启用）
   */
  state?: number | undefined | null

  /**
   * 文件路径
   */
  filePath?: string | undefined | null

  /**
   * 文件名
   */
  fileName?: string | undefined | null

  /**
   * 扩展名
   */
  filenameExtension?: string | undefined | null

  /**
   * 建议名称
   */
  suggestedName?: string | undefined | null

  /**
   * 工作目录
   */
  workdir?: string | undefined | null

  /**
   * 资源是否保存完成
   */
  resourceComplete?: number | undefined | null

  /**
   * 导入方式（0：本地导入，1：站点下载）
   */
  importMethod?: number | undefined | null

  constructor(resourceQueryDTO?: ResourceQueryDTO) {
    super(resourceQueryDTO)
    if (notNullish(resourceQueryDTO)) {
      this.workId = resourceQueryDTO.workId
      this.taskId = resourceQueryDTO.taskId
      this.state = resourceQueryDTO.state
      this.filePath = resourceQueryDTO.filePath
      this.fileName = resourceQueryDTO.fileName
      this.filenameExtension = resourceQueryDTO.filenameExtension
      this.suggestedName = resourceQueryDTO.suggestedName
      this.workdir = resourceQueryDTO.workdir
      this.resourceComplete = resourceQueryDTO.resourceComplete
      this.importMethod = resourceQueryDTO.importMethod
    }
  }
}
