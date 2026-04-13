import WorkSet from '../entity/WorkSet.ts'
import Resource from '../entity/Resource.ts'

/**
 * 作品集封面 DTO
 * 用于展示作品集及其封面信息
 */
export default class WorkSetCoverDTO {
  /**
   * 作品集主键
   */
  id: number | undefined | null
  /**
   * 作品集名称
   */
  siteWorkSetName: string | undefined | null
  /**
   * 作品集别名
   */
  nickName: string | undefined | null
  /**
   * 作品集描述
   */
  siteWorkSetDescription: string | undefined | null
  /**
   * 站点id
   */
  siteId: number | undefined | null
  /**
   * 站点作品集id
   */
  siteWorkSetId: string | undefined | null
  /**
   * 站点作者id
   */
  siteAuthorId: string | undefined | null
  /**
   * 站点上传时间
   */
  siteUploadTime: string | undefined | null
  /**
   * 站点最后更新时间
   */
  siteUpdateTime: string | undefined | null
  /**
   * 最后查看时间
   */
  lastView: number | undefined | null
  /**
   * 创建时间
   */
  createTime: number | undefined | null
  /**
   * 更新时间
   */
  updateTime: number | undefined | null
  /**
   * 封面资源
   */
  coverResource: Resource | undefined | null

  constructor(workSet: WorkSet, coverResource?: Resource) {
    this.id = workSet.id
    this.siteWorkSetName = workSet.siteWorkSetName
    this.nickName = workSet.nickName
    this.siteWorkSetDescription = workSet.siteWorkSetDescription
    this.siteId = workSet.siteId
    this.siteWorkSetId = workSet.siteWorkSetId
    this.siteAuthorId = workSet.siteAuthorId
    this.siteUploadTime = workSet.siteUploadTime
    this.siteUpdateTime = workSet.siteUpdateTime
    this.lastView = workSet.lastView
    this.createTime = workSet.createTime
    this.updateTime = workSet.updateTime
    this.coverResource = coverResource
  }
}
