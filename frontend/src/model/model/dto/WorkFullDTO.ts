import Work from '../entity/Work.ts'
import Site from '../entity/Site.ts'
import LocalTag from '../entity/LocalTag.ts'
import RankedLocalAuthor from '../domain/RankedLocalAuthor.ts'
import RankedSiteAuthor from '../domain/RankedSiteAuthor.ts'
import SiteTagFullDTO from './SiteTagFullDTO.ts'
import WorkSet from '../entity/WorkSet.ts'
import Resource from '../entity/Resource.ts'
import lodash from 'lodash'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import { parsePropertyFromJson } from '@renderer/utils/ObjectUtil.ts'

/**
 * 作品
 */
export default class WorkFullDTO extends Work {
  /**
   * 资源
   */
  resource: Resource | undefined | null

  /**
   * 不活跃的资源
   */
  inactiveResource: Resource[] | undefined | null

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
  siteAuthors: RankedSiteAuthor[] | undefined | null

  /**
   * 站点标签数组
   */
  siteTags: SiteTagFullDTO[] | undefined | null

  /**
   * 作品所属作品集
   */
  workSets: WorkSet[] | undefined | null

  constructor(work?: Work) {
    super(work)
    if (notNullish(work)) {
      parsePropertyFromJson(work, [
        {
          property: 'resource',
          builder: (src) => new Resource(src)
        },
        {
          property: 'inactiveResource',
          builder: (raw: []) => raw.map((rawResource) => new Resource(rawResource))
        },
        {
          property: 'site',
          builder: (src) => new Site(src)
        },
        {
          property: 'localAuthors',
          builder: (raw: []) => raw.map((rawLocalAuthor) => new RankedLocalAuthor(rawLocalAuthor))
        },
        {
          property: 'localTags',
          builder: (raw: []) => raw.map((rawLocalTag) => new LocalTag(rawLocalTag))
        },
        {
          property: 'siteAuthors',
          builder: (raw: []) => raw.map((rawSiteAuthor) => new RankedSiteAuthor(rawSiteAuthor))
        },
        {
          property: 'siteTags',
          builder: (raw: []) => raw.map((rawSiteTag) => new SiteTagFullDTO(rawSiteTag))
        },
        {
          property: 'workSets',
          builder: (raw: []) => raw.map((rawWorkSet) => new WorkSet(rawWorkSet))
        }
      ])
      lodash.assign(
        this,
        lodash.pick(work, [
          'resource',
          'inactiveResource',
          'site',
          'localAuthors',
          'localTags',
          'siteAuthors',
          'siteTags',
          'workSets',
          'resourceStream',
          'resourceSize'
        ])
      )
    }
  }
}
