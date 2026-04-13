import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

export default class WorkSetQueryDTO extends BaseQueryDTO {
  /**
   * 集合来源站点id
   */
  siteId: number | undefined | null
  /**
   * 集合在站点的id
   */
  siteWorkSetId: string | string[] | undefined | null
  /**
   * 集合在站点的名称
   */
  siteWorkSetName: string | undefined | null
  /**
   * 集合在站点的作者id
   */
  siteAuthorId: string | undefined | null
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

  constructor(workSetQueryDTO?: WorkSetQueryDTO) {
    super(workSetQueryDTO)
    if (notNullish(workSetQueryDTO)) {
      this.id = workSetQueryDTO.id
      this.siteId = workSetQueryDTO.siteId
      this.siteWorkSetId = workSetQueryDTO.siteWorkSetId
      this.siteWorkSetName = workSetQueryDTO.siteWorkSetName
      this.siteAuthorId = workSetQueryDTO.siteAuthorId
      this.siteUploadTime = workSetQueryDTO.siteUploadTime
      this.siteUpdateTime = workSetQueryDTO.siteUpdateTime
      this.nickName = workSetQueryDTO.nickName
      this.lastView = workSetQueryDTO.lastView
    }
  }
}
