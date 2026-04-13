import Work from '../entity/Work.ts'

/**
 * 作品+作品集id
 */
export default class WorkWithWorkSetId extends Work {
  /**
   * 作品集id
   */
  workSetId: number | undefined | null
  /**
   * 是否为封面作品
   */
  isCover: boolean | undefined | null
  /**
   * 排序顺序
   */
  sortOrder: number | undefined | null
}
