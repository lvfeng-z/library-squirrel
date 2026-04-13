import BaseEntity from '../base/BaseEntity.ts'

/**
 * 兴趣点
 */
export default class Poi extends BaseEntity {
  /**
   * 主键
   */
  id: number | undefined | null
  /**
   * 兴趣点名称
   */
  poiName: string | undefined | null
  constructor(poi: Poi) {
    super(poi)
    this.id = poi.id
    this.poiName = poi.poiName
  }
}
