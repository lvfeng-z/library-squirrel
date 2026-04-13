import { SearchType } from '@renderer/utils/SearchCondition.ts'
import BaseQueryDTO from '../base/BaseQueryDTO.ts'

export default class SearchConditionQueryDTO extends BaseQueryDTO {
  /**
   * 类型
   */
  types?: SearchType[]

  keyword?: string

  constructor(types?: SearchType[]) {
    super()
    this.types = types
  }
}
