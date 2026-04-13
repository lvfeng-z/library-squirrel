import WorkSet from '../entity/WorkSet.ts'

export default class PluginWorkSetDTO {
  /**
   * 作品集
   */
  workSet: WorkSet

  /**
   * 站点名称
   */
  siteName: string | undefined | null

  constructor(workSet: WorkSet, siteName?: string) {
    this.workSet = workSet
    this.siteName = siteName
  }
}
