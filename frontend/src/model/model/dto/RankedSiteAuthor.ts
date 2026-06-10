import SiteAuthor from '../entity/SiteAuthor.ts'
import { AuthorRank } from '../constant/AuthorRank.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import RankAuthor from '../interface/RankAuthor.ts'

/**
 * 站点作者DTO
 */
export default class RankedSiteAuthor extends SiteAuthor implements RankAuthor {
  /**
   * 作者级别
   */
  authorRank: AuthorRank | undefined | null

  constructor(siteAuthorDTO?: SiteAuthor) {
    super(siteAuthorDTO)
    if (notNullish(siteAuthorDTO)) {
      this.authorRank = siteAuthorDTO['authorRank']
    }
  }
}
