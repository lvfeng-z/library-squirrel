import BaseEntity from '../base/BaseEntity.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 站点标签
 */
export default class SiteTag extends BaseEntity {
  /**
   * 标签来源站点id
   */
  siteId: number | undefined | null
  /**
   * 站点中标签的id
   */
  siteTagId: string | undefined | null
  /**
   * 站点中标签的名称
   */
  siteTagName: string | undefined | null
  /**
   * 上级标签id
   */
  baseSiteTagId: string | undefined | null
  /**
   * 描述
   */
  description: string | undefined | null
  /**
   * 站点标签对应的本地标签id
   */
  localTagId: number | undefined | null
  /**
   * 最后一次使用的时间
   */
  lastUse: number | null | undefined

  constructor(siteTag?: SiteTag) {
    super(siteTag)
    if (notNullish(siteTag)) {
      this.siteId = siteTag.siteId
      this.siteTagId = siteTag.siteTagId
      this.siteTagName = siteTag.siteTagName
      this.baseSiteTagId = siteTag.baseSiteTagId
      this.description = siteTag.description
      this.localTagId = siteTag.localTagId
      this.lastUse = siteTag.lastUse
    }
  }
}
