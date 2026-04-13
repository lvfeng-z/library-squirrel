import Plugin from '../model/entity/Plugin.ts'
import { isNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 创建任务的响应
 */
export default class TaskCreateResponse {
  /**
   * 是否成功
   */
  succeed: boolean | undefined | null
  /**
   * 新增的数量
   */
  addedQuantity: number | undefined | null

  /**
   * 信息
   */
  msg: string | undefined | null

  /**
   * 插件
   */
  plugin: Plugin | undefined | null

  constructor(taskCreateResponse?: TaskCreateResponse) {
    if (isNullish(taskCreateResponse)) {
      this.succeed = undefined
      this.addedQuantity = undefined
      this.msg = undefined
      this.plugin = undefined
    } else {
      this.succeed = taskCreateResponse.succeed
      this.addedQuantity = taskCreateResponse.addedQuantity
      this.msg = taskCreateResponse.msg
      this.plugin = taskCreateResponse.plugin
    }
  }
}
