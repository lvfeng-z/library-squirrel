import BaseEntity from '../base/BaseEntity.ts'
import { PathTypeEnum } from '../constant/PathTypeEnum.ts'
import { isNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 自动解释路径含义
 */
export default class AutoExplainPath extends BaseEntity {
  /**
   * 主键
   */
  id: number | undefined | null

  /**
   * 名称
   */
  name: string | undefined | null

  /**
   * 类型
   */
  type: PathTypeEnum | undefined | null

  /**
   * 正则表达式
   */
  regularExpression: string | undefined | null

  constructor(autoPathExplain?: AutoExplainPath) {
    if (isNullish(autoPathExplain)) {
      super()
      this.id = undefined
      this.name = undefined
      this.type = undefined
      this.regularExpression = undefined
    } else {
      super(autoPathExplain)
      this.id = autoPathExplain.id
      this.name = autoPathExplain.name
      this.type = autoPathExplain.type
      this.regularExpression = autoPathExplain.regularExpression
    }
  }
}
