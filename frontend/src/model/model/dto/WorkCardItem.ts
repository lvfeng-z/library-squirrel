import Resource from '../entity/Resource.ts'
import RankedLocalAuthor from '../domain/RankedLocalAuthor.ts'
import RankedSiteAuthor from '../domain/RankedSiteAuthor.ts'
import WorkFullDTO from './WorkFullDTO.ts'
import WorkSetWithWorkDTO from './WorkSetWithWorkDTO.ts'
import { arrayNotEmpty, notNullish } from '@renderer/utils/CommonUtil.ts'

export default class WorkCardItem {
  /**
   * 主键
   */
  id: number | undefined | null
  /**
   * 站点作品名称
   */
  siteItemName: string | undefined | null
  /**
   * 别称
   */
  nickName: string | undefined | null
  /**
   * 简介
   */
  description: string | undefined | null
  /**
   * 资源
   */
  resource: Resource | undefined | null
  /**
   * 本地作者列表
   */
  localAuthors: RankedLocalAuthor[] | undefined | null
  /**
   * 站点作者列表
   */
  siteAuthors: RankedSiteAuthor[] | undefined | null

  constructor(source: WorkFullDTO | WorkSetWithWorkDTO) {
    if (source instanceof WorkFullDTO) {
      this.id = source.id
      this.siteItemName = source.siteWorkName
      this.nickName = source.nickName
      this.description = source.siteWorkDescription
      this.resource = source.resource
      this.localAuthors = source.localAuthors
      this.siteAuthors = source.siteAuthors
    } else {
      this.id = source.workSet.id
      this.siteItemName = source.workSet.siteWorkSetName
      this.nickName = source.workSet.nickName
      this.description = source.workSet.siteWorkSetDescription
      this.resource = source.workList[0].resource
      const seenIds = new Set()
      this.localAuthors = []
      // 从作品中提取本地作者并去重
      source.workList.forEach((workFullInfo) => {
        if (arrayNotEmpty(workFullInfo.localAuthors)) {
          workFullInfo.localAuthors.forEach((localAuthor) => {
            if (notNullish(localAuthor.id)) {
              if (!seenIds.has(localAuthor.id)) {
                seenIds.add(localAuthor.id)
                if (notNullish(this.localAuthors)) {
                  this.localAuthors.push(localAuthor)
                }
              }
            }
          })
        }
      })
      // 从作品中提取站点作者并去重
      this.siteAuthors = []
      seenIds.clear()
      source.workList.forEach((workFullInfo) => {
        if (arrayNotEmpty(workFullInfo.siteAuthors)) {
          workFullInfo.siteAuthors.forEach((siteAuthor) => {
            if (notNullish(siteAuthor.id)) {
              if (!seenIds.has(siteAuthor.id)) {
                seenIds.add(siteAuthor.id)
                if (notNullish(this.siteAuthors)) {
                  this.siteAuthors.push(siteAuthor)
                }
              }
            }
          })
        }
      })
    }
  }
}
