import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 本地作者
 */
export default class LocalAuthorQueryDTO extends BaseQueryDTO {
  /**
   * 作者名称
   */
  authorName: string | undefined | null
  /**
   * 最后一次使用的时间
   */
  lastUse: number | null | undefined

  constructor(localAuthorQueryDTO?: LocalAuthorQueryDTO) {
    super(localAuthorQueryDTO)
    if (notNullish(localAuthorQueryDTO)) {
      this.authorName = localAuthorQueryDTO.authorName
      this.lastUse = localAuthorQueryDTO.lastUse
    }
  }
}
