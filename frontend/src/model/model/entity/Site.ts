import BaseEntity from '../base/BaseEntity.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 站点
 */
export default class Site extends BaseEntity {
  /**
   * 站点名称
   */
  siteName: string | undefined | null

  /**
   * 站点描述
   */
  siteDescription: string | undefined | null

  /**
   * 主页
   */
  homepage: string | undefined | null

  constructor(site?: Site) {
    super(site)
    if (notNullish(site)) {
      this.siteName = site.siteName
      this.siteDescription = site.siteDescription
      this.homepage = site.homepage
    }
  }
}
