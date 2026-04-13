import SiteAuthor from '../entity/SiteAuthor.ts'
import { AuthorRank } from '../constant/AuthorRank.ts'

/**
 * 站点作者DTO
 */
export default class PluginSiteAuthorDTO {
  /**
   * 站点作者
   */
  siteAuthor: SiteAuthor

  /**
   * 来源站点的名称
   */
  siteName: string | undefined | null

  /**
   * 作者级别
   */
  authorRank: AuthorRank | undefined | null

  constructor(siteAuthor: SiteAuthor, siteName?: string, authorRank?: AuthorRank) {
    this.siteAuthor = siteAuthor
    this.siteName = siteName
    this.authorRank = authorRank
  }
}
