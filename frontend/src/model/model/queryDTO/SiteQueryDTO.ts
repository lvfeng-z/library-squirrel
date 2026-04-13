import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * QueryDTO
 * 站点标签
 */

export default class SiteQueryDTO extends BaseQueryDTO {
  /**
   * 站点名称
   */
  siteName?: string | undefined | null

  /**
   * 站点描述
   */
  siteDescription?: string | undefined | null

  /**
   * 主页
   */
  homepage?: string | undefined | null

  constructor(siteQueryDTO?: SiteQueryDTO) {
    super(siteQueryDTO)
    if (notNullish(siteQueryDTO)) {
      this.siteName = siteQueryDTO.siteName
      this.siteDescription = siteQueryDTO.siteDescription
      this.homepage = siteQueryDTO.homepage
    }
  }
}
