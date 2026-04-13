import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * QueryDTO
 * 站点标签
 */
export default class SiteTagQueryDTO extends BaseQueryDTO {
  /**
   * 标签来源站点id
   */
  siteId?: number | undefined | null
  /**
   * 站点中标签的id
   */
  siteTagId?: string | undefined | null
  /**
   * 站点中标签的名称
   */
  siteTagName?: string | undefined | null
  /**
   * 上级标签id
   */
  baseSiteTagId?: string | undefined | null
  /**
   * 描述
   */
  description?: string | undefined | null
  /**
   * 站点标签对应的本地标签id
   */
  localTagId?: number | undefined | null
  /**
   * 最后一次使用的时间
   */
  lastUse?: number | null | undefined

  /**
   * 标签来源站点id列表
   */
  sites?: string[] | undefined | null

  /**
   * 查询绑定在localTagId上的，还是未绑定的（true：绑定的，false：未绑定的）
   */
  boundOnLocalTagId?: boolean | undefined | null

  /**
   * 作品id
   */
  workId?: number | null | undefined

  /**
   * 查询绑定在workId上的，还是未绑定的（true：绑定的，false：未绑定的）
   */
  boundOnWorkId?: boolean | undefined | null

  constructor(siteTagQueryDTO?: SiteTagQueryDTO) {
    super(siteTagQueryDTO)
    if (notNullish(siteTagQueryDTO)) {
      this.siteId = siteTagQueryDTO.siteId
      this.siteTagId = siteTagQueryDTO.siteTagId
      this.siteTagName = siteTagQueryDTO.siteTagName
      this.baseSiteTagId = siteTagQueryDTO.baseSiteTagId
      this.description = siteTagQueryDTO.description
      this.localTagId = siteTagQueryDTO.localTagId
      this.sites = siteTagQueryDTO.sites
      this.boundOnLocalTagId = siteTagQueryDTO.boundOnLocalTagId
    }
  }

  public static nonFieldProperties(): string[] {
    return [...BaseQueryDTO.nonFieldProperties(), 'sites', 'boundOnLocalTagId', 'workId', 'boundOnWorkId']
  }
}
