import SiteTag from '../entity/SiteTag.ts'

export default class SiteTagWithWorkId extends SiteTag {
  /**
   * 作品id
   */
  workId: number | undefined | null
}
