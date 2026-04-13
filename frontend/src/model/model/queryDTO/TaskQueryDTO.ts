import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 任务-QueryDTO
 */
export default class TaskQueryDTO extends BaseQueryDTO {
  /**
   * 是否是父任务
   */
  isCollection: boolean | undefined | null

  /**
   * 上级任务id
   */
  pid: number | number[] | undefined | null

  /**
   * 任务名称
   */
  taskName: string | undefined | null

  /**
   * 任务的站点id
   */
  siteId: number | undefined | null

  /**
   * 站点作品id
   */
  siteWorkId: string | undefined | null

  /**
   * 当任务是父任务时，url存储创建此父任务所使用的url，否则存储的是下载资源的链接（由于存在临时的下载链接，此字段可能没有作用）
   */
  url: string | undefined | null

  /**
   * 状态（0：未开始，1：进行中，2：暂停，3：已完成，4：失败）
   */
  status: number | undefined | null

  /**
   * 保存中的资源id
   */
  pendingResourceId: number | undefined | null

  /**
   * 资源是否支持续传
   */
  continuable: boolean | undefined | null

  /**
   * 插件作者
   */
  pluginAuthor: string | undefined | null

  /**
   * 插件名称
   */
  pluginName: string | undefined | null

  /**
   * 插件版本
   */
  pluginVersion: string | undefined | null

  /**
   * 插件数据
   */
  pluginData: { [key: string]: unknown } | string | undefined | null

  /**
   * 错误信息
   */
  errorMessage: string | undefined | null

  constructor(task?: TaskQueryDTO) {
    super(task)
    if (notNullish(task)) {
      this.isCollection = task.isCollection
      this.pid = task.pid
      this.taskName = task.taskName
      this.siteId = task.siteId
      this.siteWorkId = task.siteWorkId
      this.url = task.url
      this.status = task.status
      this.pendingResourceId = task.pendingResourceId
      this.continuable = task.continuable
      this.pluginAuthor = task.pluginAuthor
      this.pluginName = task.pluginName
      this.pluginVersion = task.pluginVersion
      if (typeof task.pluginData === 'string') {
        this.pluginData = JSON.parse(task.pluginData)
      } else {
        this.pluginData = task.pluginData
      }
      this.errorMessage = task.errorMessage
    }
  }
}
