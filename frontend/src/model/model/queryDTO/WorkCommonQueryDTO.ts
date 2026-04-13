import { notNullish } from '@renderer/utils/CommonUtil.ts'
import WorkQueryDTO from './WorkQueryDTO.ts'
import lodash from 'lodash'
import BaseQueryDTO from '../base/BaseQueryDTO.ts'

/**
 * QueryDTO
 * 作品
 */
export default class WorkCommonQueryDTO extends WorkQueryDTO {
  /**
   * 排除作品ID
   */
  excludeWorkIds?: (string | number)[]
  /**
   * 包含本地标签
   */
  includeLocalTagIds?: (string | number)[]
  /**
   * 排除本地标签
   */
  excludeLocalTagIds?: (string | number)[]
  /**
   * 包含站点标签
   */
  includeSiteTagIds?: (string | number)[]
  /**
   * 排除站点标签
   */
  excludeSiteTagIds?: (string | number)[]
  /**
   * 包含本地作者
   */
  includeLocalAuthorIds?: (string | number)[]
  /**
   * 排除本地作者
   */
  excludeLocalAuthorIds?: (string | number)[]
  /**
   * 包含站点作者
   */
  includeSiteAuthorIds?: (string | number)[]
  /**
   * 排除站点作者
   */
  excludeSiteAuthorIds?: (string | number)[]

  constructor(workQueryDTO?: WorkQueryDTO) {
    super(workQueryDTO)
    if (notNullish(workQueryDTO)) {
      lodash.assign(
        this,
        lodash.pick(workQueryDTO, [
          'excludeWorkIds',
          'includeLocalTagIds',
          'excludeLocalTagIds',
          'includeSiteTagIds',
          'excludeSiteTagIds',
          'includeLocalAuthorIds',
          'excludeLocalAuthorIds',
          'includeSiteAuthorIds',
          'excludeSiteAuthorIds'
        ])
      )
    }
  }

  public static nonFieldProperties(): string[] {
    return [
      ...BaseQueryDTO.nonFieldProperties(),
      'excludeWorkIds',
      'includeLocalTagIds',
      'excludeLocalTagIds',
      'includeSiteTagIds',
      'excludeSiteTagIds',
      'includeLocalAuthorIds',
      'excludeLocalAuthorIds',
      'includeSiteAuthorIds',
      'excludeSiteAuthorIds'
    ]
  }
}
