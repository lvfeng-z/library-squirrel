import { Id } from './BaseEntity.ts'
import { Operator } from '@renderer/model/util/Operator.ts'
import { QuerySortOption } from '@renderer/model/util/QuerySortOption.ts'

export default class BaseQueryDTO {
    /**
     * 主键
     */
    id?: Id | Id[] | null | undefined

    /**
     * 创建时间
     */
    createTime?: number | null | undefined

    /**
     * 更新时间
     */
    updateTime?: number | null | undefined

    /**
     * 关键字
     */
    nonFieldKeyword?: string | undefined | null

    /**
     * 指定比较符
     */
    operators?: { [key: string]: Operator } | undefined

    /**
     * 排序配置
     */
    sort?: QuerySortOption[] | undefined

    constructor(baseQueryDTO?: BaseQueryDTO) {
        if (baseQueryDTO === undefined) {
            this.id = undefined
            this.createTime = undefined
            this.updateTime = undefined
            this.nonFieldKeyword = undefined
            this.operators = undefined
            this.sort = undefined
        } else {
            this.id = baseQueryDTO.id
            this.createTime = baseQueryDTO.createTime
            this.updateTime = baseQueryDTO.updateTime
            this.nonFieldKeyword = baseQueryDTO.nonFieldKeyword
            this.operators = baseQueryDTO.operators
            this.sort = baseQueryDTO.sort
        }
    }

    /**
     * 获取非字段属性的名称
     */
    public static nonFieldProperties(): string[] {
        return ['nonFieldKeyword', 'operators', 'sort']
    }
}
