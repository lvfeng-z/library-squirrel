import lodash from 'lodash'
import Task from '../entity/Task.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 任务进度 DTO，兼容两种数据格式：
 * - 新 binding 格式：{task: TaskDTO, total, finished, siteName, schedule}
 * - 旧 flat 格式：Task + {total, finished, siteName}
 */
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

  constructor(data?: any) {
    // 新 binding 格式：task 字段包含基础任务数据
    const taskData = data?.task ?? data
    super(taskData)
    if (notNullish(data)) {
      lodash.assign(this, lodash.pick(data, ['total', 'finished', 'siteName']))
    }
  }
}
