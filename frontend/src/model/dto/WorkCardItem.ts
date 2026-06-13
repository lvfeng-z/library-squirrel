import {
  WorkFullDTO,
  ResourceFullDTO, RankedLocalAuthor, RankedSiteAuthor, WorkSetWithWorksResultDTO
} from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto'
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
   * 资源（filePath 可能来自旧的 resource 表或新的 workStore）
   */
  resource: ResourceFullDTO | undefined | null
  /**
   * 本地作者列表
   */
  localAuthors: RankedLocalAuthor[] | undefined | null
  /**
   * 站点作者列表
   */
  siteAuthors: RankedSiteAuthor[] | undefined | null

  constructor(source: WorkFullDTO | WorkSetWithWorksResultDTO) {
    if (source instanceof WorkSetWithWorksResultDTO) {
      this.id = source.workSet?.id
      this.siteItemName = source.workSet?.siteWorkSetName
      this.nickName = source.workSet?.nickName
      this.description = source.workSet?.siteWorkSetDescription
      this.resource = arrayNotEmpty(source.works) ? source.works[0]?.resource : null
      const seenIds = new Set()
      this.localAuthors = []
      // 从作品中提取本地作者并去重
      source.works?.forEach((workFullInfo) => {
        if (arrayNotEmpty(workFullInfo?.localAuthors)) {
          workFullInfo.localAuthors.filter(notNullish).forEach((localAuthor) => {
            const tempId = localAuthor?.author?.id
            if (notNullish(tempId)) {
              if (!seenIds.has(tempId)) {
                seenIds.add(tempId)
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
      source.works?.forEach((workFullInfo) => {
        if (arrayNotEmpty(workFullInfo?.siteAuthors)) {
          workFullInfo.siteAuthors.filter(notNullish).forEach((siteAuthor) => {
            const tempId = siteAuthor?.author?.id
            if (notNullish(tempId)) {
              if (!seenIds.has(tempId)) {
                seenIds.add(tempId)
                if (notNullish(this.siteAuthors)) {
                  this.siteAuthors.push(siteAuthor)
                }
              }
            }
          })
        }
      })
    } else if (source instanceof WorkFullDTO) {
      // bindings WorkFullDTO：嵌套结构
      this.id = source.work?.id
      this.siteItemName = source.work?.siteWorkName
      this.nickName = source.work?.nickName
      this.description = source.work?.siteWorkDescription
      this.resource = source.resource
      this.localAuthors = arrayNotEmpty(source.localAuthors) ? source.localAuthors!.filter(notNullish) : undefined
      this.siteAuthors = arrayNotEmpty(source.siteAuthors) ? source.siteAuthors!.filter(notNullish) : undefined
    }
  }
}
