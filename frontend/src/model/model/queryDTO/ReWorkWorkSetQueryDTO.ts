import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 作品-作品集关联表QueryDTO
 */
export default class ReWorkWorkSetQueryDTO extends BaseQueryDTO {
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

  constructor(reWorkWorkSetQueryDTO?: ReWorkWorkSetQueryDTO) {
    super(reWorkWorkSetQueryDTO)
    if (notNullish(reWorkWorkSetQueryDTO)) {
      this.workId = reWorkWorkSetQueryDTO.workId
      this.workSetId = reWorkWorkSetQueryDTO.workSetId
      this.isCover = reWorkWorkSetQueryDTO.isCover
      this.sortOrder = reWorkWorkSetQueryDTO.sortOrder
    }
    // 默认按 sortOrder 升序排序
    if (!this.sort) {
      this.sort = [{ key: 'sort_order', asc: true }]
    }
  }
}
