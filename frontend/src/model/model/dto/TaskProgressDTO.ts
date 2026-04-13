import lodash from 'lodash'
import Task from '../entity/Task.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

export default class TaskProgressDTO extends Task {
  /**
   * 总量
   */
  total: number | undefined | null

  /**
   * 已完成的量
   */
  finished: number | undefined | null

  /**
   * 站点名称
   */
  siteName: string | undefined | null

  constructor(taskProcessingDTO?: Task) {
    super(taskProcessingDTO)
    if (notNullish(taskProcessingDTO)) {
      lodash.assign(this, lodash.pick(taskProcessingDTO, ['total', 'finished', 'siteName']))
    }
  }
}
