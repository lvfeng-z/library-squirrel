import SiteAuthor from '../entity/SiteAuthor.ts'
import LocalAuthor from '../entity/LocalAuthor.ts'
import Site from '../entity/Site.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import lodash from 'lodash'
import { parsePropertyFromJson } from '@renderer/utils/ObjectUtil.ts'

/**
 * 站点作者DTO
 */
export default class SiteAuthorFullDTO extends SiteAuthor {
  /**
   * 本地作者
   */
  localAuthor: LocalAuthor | undefined | null

  /**
   * 来源站点的实例
   */
  site: Site | undefined | null

  constructor(siteAuthorDTO?: SiteAuthor) {
    super(siteAuthorDTO)
    if (notNullish(siteAuthorDTO)) {
      lodash.assign(this, lodash.pick(siteAuthorDTO, ['localAuthor', 'site']))
      parsePropertyFromJson(this, [
        { property: 'localAuthor', builder: (src) => new LocalAuthor(src) },
        { property: 'site', builder: (src) => new Site(src) }
      ])
    }
  }
}
