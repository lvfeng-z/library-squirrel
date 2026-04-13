import { isNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 基础模型
 */
export default class BaseEntity {
  /**
   * 主键
   */
  id: Id | null | undefined

  /**
   * 创建时间
   */
  createTime: number | null | undefined

  /**
   * 更新时间
   */
  updateTime: number | null | undefined

  /**
   * 主键名称
   */
  public static readonly PK = 'id'
  public static readonly CREATE_TIME = 'create_time'
  public static readonly UPDATE_TIME = 'update_time'

  constructor(baseEntity?: BaseEntity) {
    if (isNullish(baseEntity)) {
      this.id = undefined
      this.createTime = undefined
      this.updateTime = undefined
    } else {
      this.id = baseEntity.id
      this.createTime = baseEntity.createTime
      this.updateTime = baseEntity.updateTime
    }
  }
}

export type Id = number
