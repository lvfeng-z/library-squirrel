import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { SearchType } from "@renderer/model/util/SearchCondition.ts";

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
