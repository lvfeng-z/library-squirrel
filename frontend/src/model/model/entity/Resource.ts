import BaseEntity from '../base/BaseEntity.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

export default class Resource extends BaseEntity {
  /**
   * 作品id
   */
  workId: number | undefined | null

  /**
   * 任务id
   */
  taskId: number | undefined | null

  /**
   * 状态（0：停用，1：启用）
   */
  state: number | undefined | null

  /**
   * 建议名称
   */
  suggestedName: string | undefined | null

  /**
   * 资源是否保存完成
   */
  resourceComplete: number | undefined | null

  /**
   * 导入方式（0：本地导入，1：站点下载）
   */
  importMethod: number | undefined | null

  constructor(resource?: Resource) {
    super(resource)
    if (notNullish(resource)) {
      this.workId = resource.workId
      this.taskId = resource.taskId
      this.state = resource.state
      this.suggestedName = resource.suggestedName
      this.resourceComplete = resource.resourceComplete
      this.importMethod = resource.importMethod
    }
  }
}
