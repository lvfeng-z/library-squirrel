import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 作品与标签关联查询DTO
 */
export class ReWorkTagQueryDTO extends BaseQueryDTO {
  /**
   * 作品id
   */
  workId: number | undefined | null
  /**
   * 标签类型（0：本地，1：站点）
   */
  tagType: number | undefined | null
  /**
   * 本地标签id
   */
  localTagId: number | undefined | null
  /**
   * 站点标签id
   */
  siteTagId: number | undefined | null

  constructor(reWorkTagQueryDTO?: ReWorkTagQueryDTO) {
    super(reWorkTagQueryDTO)
    if (notNullish(reWorkTagQueryDTO)) {
      this.workId = reWorkTagQueryDTO.workId
      this.tagType = reWorkTagQueryDTO.tagType
      this.localTagId = reWorkTagQueryDTO.localTagId
      this.siteTagId = reWorkTagQueryDTO.siteTagId
    }
  }
}
