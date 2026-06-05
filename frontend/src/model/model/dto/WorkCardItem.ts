import RankedLocalAuthor from '../domain/RankedLocalAuthor.ts'
import RankedSiteAuthor from '../domain/RankedSiteAuthor.ts'
import WorkSetWithWorkDTO from './WorkSetWithWorkDTO.ts'
import {
  WorkFullDTO,
  LocalAuthorDTO,
  SiteAuthorFullDTO,
  ResourceFullDTO
} from '@bindings/github.com/lvfeng-z/library-squirrel-plugin-sdk/dto'
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

  constructor(source: WorkFullDTO | WorkSetWithWorkDTO) {
    if (source instanceof WorkSetWithWorkDTO) {
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
      source.workList.forEach((workFullInfo) => {
        if (arrayNotEmpty(workFullInfo.siteAuthors)) {
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

function toRankedLocalAuthors(dtos: (LocalAuthorDTO | null)[] | undefined | null): RankedLocalAuthor[] | undefined {
  if (!arrayNotEmpty(dtos)) return undefined
  return dtos!.filter(notNullish).map(dto => {
    const author = new RankedLocalAuthor()
    author.id = dto.id
    author.authorName = dto.authorName ?? undefined
    author.introduce = dto.introduce ?? undefined
    author.lastUse = dto.lastUse ?? undefined
    author.authorRank = undefined
    return author
  })
}

function toRankedSiteAuthors(dtos: (SiteAuthorFullDTO | null)[] | undefined | null): RankedSiteAuthor[] | undefined {
  if (!arrayNotEmpty(dtos)) return undefined
  return dtos!.filter(notNullish).map(dto => {
    const author = new RankedSiteAuthor()
    if (dto.siteAuthor) {
      author.id = dto.siteAuthor.id
      author.authorName = dto.siteAuthor.authorName ?? undefined
      author.introduce = dto.siteAuthor.introduce ?? undefined
      author.localAuthorId = dto.siteAuthor.localAuthorId ?? undefined
    }
    author.authorRank = undefined
    return author
  })
}
