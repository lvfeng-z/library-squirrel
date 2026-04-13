import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { AuthorRank } from '../constant/AuthorRank.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

export default class ReWorkAuthorQueryDTO extends BaseQueryDTO {
  /**
   * 类型(0: 本地作者，1: 站点作者)
   */
  authorType: number | undefined | null

  /**
   * 作品id
   */
  workId: number | undefined | null

  /**
   * 本地作者id
   */
  localAuthorId: number | undefined | null

  /**
   * 站点作者Id
   */
  siteAuthorId: string | undefined | null

  /**
   * 作者类型
   */
  authorRank: AuthorRank | undefined | null

  constructor(reWorkAuthorQueryDTO?: ReWorkAuthorQueryDTO) {
    super(reWorkAuthorQueryDTO)
    if (notNullish(reWorkAuthorQueryDTO)) {
      this.authorType = reWorkAuthorQueryDTO.authorType
      this.workId = reWorkAuthorQueryDTO.workId
      this.localAuthorId = reWorkAuthorQueryDTO.localAuthorId
      this.siteAuthorId = reWorkAuthorQueryDTO.siteAuthorId
      this.authorRank = reWorkAuthorQueryDTO.authorRank
    }
  }
}
