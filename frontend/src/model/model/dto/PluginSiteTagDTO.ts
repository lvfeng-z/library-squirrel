import SiteTag from '../entity/SiteTag.ts'

export default class PluginSiteTagDTO {
  /**
   * 站点标签
   */
  siteTag: SiteTag
  /**
   * 来源站点的名称
   */
  siteName: string | undefined | null

  constructor(siteTag: SiteTag, siteName?: string) {
    this.siteTag = siteTag
    this.siteName = siteName
  }
}
