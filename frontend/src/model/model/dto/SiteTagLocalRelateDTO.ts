import SiteTag from '../entity/SiteTag.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import lodash from 'lodash'
import SiteTagFullDTO from './SiteTagFullDTO.ts'

/**
 * 站点标签与本地标签联系DTO
 */
export default class SiteTagLocalRelateDTO extends SiteTagFullDTO {
  /**
   * 是否有同名本地标签
   */
  hasSameNameLocalTag: boolean | undefined | null

  constructor(siteTag?: SiteTag) {
    super(siteTag)
    if (notNullish(siteTag)) {
      lodash.assign(this, lodash.pick(siteTag, ['hasSameNameLocalTag']))
      this.hasSameNameLocalTag = Boolean(this.hasSameNameLocalTag)
    }
  }
}
