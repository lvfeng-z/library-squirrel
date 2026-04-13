import Work from '../entity/Work.ts'
import WorkSet from '../entity/WorkSet.ts'

/**
 * 作品与作品集DTO
 */
export default class WorkWithWorkSetDTO {
  /**
   * 作品
   */
  work: Work

  /**
   * 作品集
   */
  workSet: WorkSet

  constructor(work: Work, workSet: WorkSet) {
    this.work = work
    this.workSet = workSet
  }
}
