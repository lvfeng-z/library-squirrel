import BaseEntity from '../base/BaseEntity.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 作品
 */
export default class Work extends BaseEntity {
  /**
   * 作品来源站点id
   */
  siteId: number | undefined | null
  /**
   * 站点中作品的id
   */
  siteWorkId: string | undefined | null
  /**
   * 站点中作品的名称
   */
  siteWorkName: string | undefined | null
  /**
   * 站点中作品的作者id
   */
  siteAuthorId: string | undefined | null
  /**
   * 站点中作品的描述
   */
  siteWorkDescription: string | undefined | null
  /**
   * 站点中作品的上传时间
   */
  siteUploadTime: number | undefined | null
  /**
   * 站点中作品最后修改的时间
   */
  siteUpdateTime: number | undefined | null
  /**
   * 作品别称
   */
  nickName: string | undefined | null
  /**
   * 最后一次查看的时间
   */
  lastView: number | undefined | null

  constructor(work?: Work) {
    super(work)
    if (notNullish(work)) {
      this.siteId = work.siteId
      this.siteWorkId = work.siteWorkId
      this.siteWorkName = work.siteWorkName
      this.siteWorkDescription = work.siteWorkDescription
      this.siteAuthorId = work.siteAuthorId
      this.siteUploadTime = work.siteUploadTime
      this.siteUpdateTime = work.siteUpdateTime
      this.nickName = work.nickName
      this.lastView = work.lastView
    }
  }
}
