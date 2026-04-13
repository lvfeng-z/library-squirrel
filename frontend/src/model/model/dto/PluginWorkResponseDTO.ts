import Work from '../entity/Work.ts'
import Site from '../entity/Site.ts'
import LocalTag from '../entity/LocalTag.ts'
import RankedLocalAuthor from '../domain/RankedLocalAuthor.ts'
import PluginSiteAuthorDTO from './PluginSiteAuthorDTO.ts'
import PluginWorkSetDTO from './PluginWorkSetDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import PluginResourceDTO from './PluginResourceDTO.ts'
import PluginSiteTagDTO from './PluginSiteTagDTO.ts'

/**
 * 作品
 */
export default class PluginWorkResponseDTO {
  /**
   * 作品信息
   */
  work: Work

  /**
   * 站点
   */
  site: Site | undefined | null

  /**
   * 本地作者
   */
  localAuthors: RankedLocalAuthor[] | undefined | null

  /**
   * 本地标签数组
   */
  localTags: LocalTag[] | undefined | null

  /**
   * 站点作者
   */
  siteAuthors: PluginSiteAuthorDTO[] | undefined | null

  /**
   * 站点标签数组
   */
  siteTags: PluginSiteTagDTO[] | undefined | null

  /**
   * 作品所属作品集
   */
  workSets: PluginWorkSetDTO[] | undefined | null

  /**
   * 资源
   */
  resource: PluginResourceDTO | undefined | null

  constructor(pluginWorkResponseDTO?: PluginWorkResponseDTO) {
    if (notNullish(pluginWorkResponseDTO)) {
      this.work = pluginWorkResponseDTO.work
      this.site = pluginWorkResponseDTO.site
      this.localAuthors = pluginWorkResponseDTO.localAuthors
      this.localTags = pluginWorkResponseDTO.localTags
      this.siteAuthors = pluginWorkResponseDTO.siteAuthors
      this.siteTags = pluginWorkResponseDTO.siteTags
      this.workSets = pluginWorkResponseDTO.workSets
      this.resource = pluginWorkResponseDTO.resource
    } else {
      this.work = new Work()
    }
  }
}
