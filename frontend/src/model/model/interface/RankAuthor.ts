import BaseAuthor from './BaseAuthor.ts'
import { AuthorRank } from '../constant/AuthorRank.ts'

export default interface RankAuthor extends BaseAuthor {
  /**
   * 作者级别
   */
  authorRank: AuthorRank | undefined | null
}
