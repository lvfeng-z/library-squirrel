import Work from '../entity/Work.ts'
import Site from '../entity/Site.ts'
import LocalTag from '../entity/LocalTag.ts'
import RankedLocalAuthor from '../domain/RankedLocalAuthor.ts'
import RankedSiteAuthor from '../domain/RankedSiteAuthor.ts'
import SiteTagFullDTO from './SiteTagFullDTO.ts'
import WorkSet from '../entity/WorkSet.ts'
import Resource from '../entity/Resource.ts'
import lodash from 'lodash'
import { arrayNotEmpty, notNullish } from '@renderer/utils/CommonUtil.ts'

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
      // 后端 WorkFullDTO 是嵌套结构 {work: {...}, resources: [...], localAuthors: [...], ...}
      // 需要展开 work 子对象的字段，并正确映射 resources → resource
      const nested = work as any
      if (notNullish(nested.work)) {
        lodash.assign(this, nested.work)
      }
      // resources → resource（取第一个活跃资源）
      if (arrayNotEmpty(nested.resources)) {
        const activeRes = nested.resources.find((r: any) => r.state === 1)
        this.resource = activeRes ? new Resource(activeRes) : new Resource(nested.resources[0])
      }
      // inactiveResource
      if (arrayNotEmpty(nested.resources)) {
        this.inactiveResource = nested.resources
          .filter((r: any) => r.state !== 1)
          .map((r: any) => new Resource(r))
      }
      if (notNullish(nested.site)) {
        this.site = new Site(nested.site)
      }
      if (arrayNotEmpty(nested.localAuthors)) {
        this.localAuthors = nested.localAuthors
          .filter(notNullish)
          .map((raw: any) => new RankedLocalAuthor(raw))
      }
      if (arrayNotEmpty(nested.localTags)) {
        this.localTags = nested.localTags.filter(notNullish).map((raw: any) => new LocalTag(raw))
      }
      if (arrayNotEmpty(nested.siteAuthors)) {
        this.siteAuthors = nested.siteAuthors
          .filter(notNullish)
          .map((raw: any) => new RankedSiteAuthor(raw))
      }
      if (arrayNotEmpty(nested.siteTags)) {
        this.siteTags = nested.siteTags.filter(notNullish).map((raw: any) => new SiteTagFullDTO(raw))
      }
      if (arrayNotEmpty(nested.workSets)) {
        this.workSets = nested.workSets.filter(notNullish).map((raw: any) => new WorkSet(raw))
      }
    }
  }
}
