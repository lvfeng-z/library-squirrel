import Resource from '../entity/Resource.ts'
import { Readable } from 'node:stream'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import lodash from 'lodash'

export default class ResourceSaveDTO extends Resource {
  /**
   * 保存路径
   */
  fullSavePath: string | undefined | null

  /**
   * 作品资源的数据流
   */
  resourceStream: Readable | undefined | null

  constructor(resource?: Resource) {
    super(resource)
    if (notNullish(resource)) {
      lodash.assign(this, lodash.pick(resource, ['fullSavePath', 'resourceStream', 'resourceSize']))
    }
  }
}
