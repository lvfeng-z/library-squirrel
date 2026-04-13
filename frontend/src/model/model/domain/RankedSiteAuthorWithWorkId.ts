import { notNullish } from '@renderer/utils/CommonUtil.ts'
import RankedSiteAuthor from './RankedSiteAuthor.ts'

/**
 * 站点作者DTO
 */
export default class RankedSiteAuthorWithWorkId extends RankedSiteAuthor {
  /**
   * 作品id
   */
  workId: number | undefined | null

  constructor(rankedSiteAuthorWithWorkId?: RankedSiteAuthorWithWorkId) {
    super(rankedSiteAuthorWithWorkId)
    if (notNullish(rankedSiteAuthorWithWorkId)) {
      this.workId = rankedSiteAuthorWithWorkId['workId']
    }
  }
}
