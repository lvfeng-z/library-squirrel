import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * QueryDTO
 * 站点标签
 */
export default class LocalTagQueryDTO extends BaseQueryDTO {
  /**
   * 本地标签名称
   */
  localTagName: string | null | undefined

  /**
   * 上级标签id
   */
  baseLocalTagId: number | null | undefined

  /**
   * 最后一次使用的时间
   */
  lastUse: number | null | undefined

  /**
   * 作品id
   */
  workId: number | number[] | null | undefined

  /**
   * 查询绑定在workId上的还是没绑定的
   */
  boundOnWorkId: boolean | null | undefined

  constructor(localTagQueryDTO?: LocalTagQueryDTO) {
    super(localTagQueryDTO)
    if (notNullish(localTagQueryDTO)) {
      this.localTagName = localTagQueryDTO.localTagName
      this.baseLocalTagId = localTagQueryDTO.baseLocalTagId
      this.lastUse = localTagQueryDTO.lastUse
      this.workId = localTagQueryDTO.workId
      this.boundOnWorkId = localTagQueryDTO.boundOnWorkId
    }
  }

  public static nonFieldProperties(): string[] {
    return [...BaseQueryDTO.nonFieldProperties(), 'workId', 'boundOnWorkId']
  }
}
