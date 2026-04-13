import BaseEntity from '../base/BaseEntity.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 作品-作品集关联表
 */
export default class ReWorkWorkSet extends BaseEntity {
  /**
   * 作品id
   */
  workId: number | undefined | null
  /**
   * 作品集id
   */
  workSetId: number | undefined | null
  /**
   * 是否为封面作品
   */
  isCover: boolean | undefined | null
  /**
   * 排序顺序
   */
  sortOrder: number | undefined | null

  constructor(reWorkWorkSet?: ReWorkWorkSet) {
    super(reWorkWorkSet)
    if (notNullish(reWorkWorkSet)) {
      this.workId = reWorkWorkSet.workId
      this.workSetId = reWorkWorkSet.workSetId
      this.isCover = reWorkWorkSet.isCover
      this.sortOrder = reWorkWorkSet.sortOrder
    }
  }
}
