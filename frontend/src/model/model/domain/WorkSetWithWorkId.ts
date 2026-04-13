import WorkSet from '../entity/WorkSet.ts'

/**
 * 作品集+作品id
 */
export default class WorkSetWithWorkId extends WorkSet {
  /**
   * 作品id
   */
  workId: number | undefined | null
}
