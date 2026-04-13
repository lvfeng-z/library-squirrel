import BaseEntity from '../base/BaseEntity.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import lodash from 'lodash'

/**
 * 作品集合
 */
export default class WorkSet extends BaseEntity {
  /**
   * 集合来源站点id
   */
  siteId: number | undefined | null
  /**
   * 集合在站点的id
   */
  siteWorkSetId: string | undefined | null
  /**
   * 集合在站点的名称
   */
  siteWorkSetName: string | undefined | null
  /**
   * 集合在站点的作者id
   */
  siteAuthorId: string | undefined | null
  /**
   * 站点中作品集的描述
   */
  siteWorkSetDescription: string | undefined | null
  /**
   * 集合在站点的上传时间
   */
  siteUploadTime: string | undefined | null
  /**
   * 集合在站点最后更新的时间
   */
  siteUpdateTime: string | undefined | null
  /**
   * 别名
   */
  nickName: string | undefined | null
  /**
   * 最后一次查看的时间
   */
  lastView: number | undefined | null

  constructor(workSet?: WorkSet) {
    super(workSet)
    if (notNullish(workSet)) {
      lodash.assign(
        this,
        lodash.pick(workSet, [
          'siteId',
          'siteWorkSetId',
          'siteWorkSetName',
          'siteAuthorId',
          'siteWorkSetDescription',
          'siteUploadTime',
          'siteUpdateTime',
          'nickName',
          'lastView'
        ])
      )
    }
  }
}
