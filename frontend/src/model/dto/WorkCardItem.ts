import {
  WorkFullDTO,
  LocalAuthorDTO,
  SiteAuthorFullDTO,
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
          workFullInfo.localAuthors.forEach((localAuthor) => {
            if (notNullish(localAuthor?.id)) {
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
      source.works?.forEach((workFullInfo) => {
        if (arrayNotEmpty(workFullInfo?.siteAuthors)) {
          workFullInfo.siteAuthors.forEach((siteAuthor) => {
            const tempId = siteAuthor?.siteAuthor?.id
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
      this.localAuthors = toRankedLocalAuthors(source.localAuthors?.filter(notNullish))
      this.siteAuthors = toRankedSiteAuthors(source.siteAuthors?.filter(notNullish))
    }
  }
}

function toRankedLocalAuthors(dtoList: (LocalAuthorDTO | null)[] | undefined | null): RankedLocalAuthor[] | undefined {
  if (!arrayNotEmpty(dtoList)) return undefined
  return dtoList!.filter(notNullish).map(dto => {
    const author = new RankedLocalAuthor()
    author.id = dto.id
    author.authorName = dto.authorName ?? ''
    author.introduce = dto.introduce ?? ''
    author.lastUse = dto.lastUse ?? -1
    author.sortOrder = -1
    return author
  })
}

function toRankedSiteAuthors(dtoList: (SiteAuthorFullDTO | null)[] | undefined | null): RankedSiteAuthor[] | undefined {
  if (!arrayNotEmpty(dtoList)) return undefined
  return dtoList!.filter(notNullish).map(dto => {
    const author = new RankedSiteAuthor()
    if (dto.siteAuthor) {
      author.id = dto.siteAuthor.id
      author.authorName = dto.siteAuthor.authorName ?? ''
      author.introduce = dto.siteAuthor.introduce ?? ''
      author.localAuthorId = dto.siteAuthor.localAuthorId ?? -1
    }
    author.sortOrder = -1
    return author
  })
}
