import BaseEntity from '../base/BaseEntity.ts'
import lodash from 'lodash'

/**
 * 本地标签
 */
export default class LocalTag extends BaseEntity {
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

  constructor(localTag?: LocalTag) {
    super(localTag)
    lodash.assign(this, lodash.pick(localTag, ['localTagName', 'baseLocalTagId', 'lastUse']))
  }
}
