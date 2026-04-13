import { isNullish } from '@renderer/utils/CommonUtil.ts'
import RankedLocalAuthor from './RankedLocalAuthor.ts'

export default class RankedLocalAuthorWithWorkId extends RankedLocalAuthor {
  /**
   * 作品id
   */
  workId: number | undefined | null

  constructor(rankedLocalAuthorWithWorkId?: RankedLocalAuthorWithWorkId) {
    if (isNullish(rankedLocalAuthorWithWorkId)) {
      super()
      this.workId = undefined
    } else {
      super(rankedLocalAuthorWithWorkId)
      this.workId = rankedLocalAuthorWithWorkId.workId
    }
  }
}
