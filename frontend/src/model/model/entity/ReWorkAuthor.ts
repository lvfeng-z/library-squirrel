import BaseEntity from '../base/BaseEntity.ts'
import { AuthorRank } from '../constant/AuthorRank.ts'

export default class ReWorkAuthor extends BaseEntity {
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

  constructor(reWorkAuthor?: ReWorkAuthor) {
    if (reWorkAuthor === undefined) {
      super()
      this.authorType = undefined
      this.workId = undefined
      this.localAuthorId = undefined
      this.siteAuthorId = undefined
      this.authorRank = undefined
    } else {
      super(reWorkAuthor)
      this.authorType = reWorkAuthor.authorType
      this.workId = reWorkAuthor.workId
      this.localAuthorId = reWorkAuthor.localAuthorId
      this.siteAuthorId = reWorkAuthor.siteAuthorId
      this.authorRank = reWorkAuthor.authorRank
    }
  }
}
