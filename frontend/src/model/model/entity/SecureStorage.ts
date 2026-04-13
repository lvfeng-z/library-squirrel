import BaseEntity from '../base/BaseEntity.ts'
import lodash from 'lodash'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

export default class SecureStorage extends BaseEntity {
  /**
   * 存储键
   */
  storageKey: string | undefined | null

  /**
   * 加密后的值
   */
  encryptedValue: string | undefined | null

  /**
   * 描述
   */
  description: string | undefined | null

  constructor(secureStorage?: SecureStorage) {
    super()
    if (notNullish(secureStorage)) {
      lodash.assign(this, lodash.pick(secureStorage, ['storageKey', 'encryptedValue', 'description']))
    }
  }
}
