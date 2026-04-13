import BaseEntity from '../base/BaseEntity.ts'
import { BackupSourceTypeEnum } from '../constant/BackupSourceTypeEnum.ts'
import lodash from 'lodash'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

export default class Backup extends BaseEntity {
  /**
   * 来源类型
   */
  sourceType: BackupSourceTypeEnum | undefined | null

  /**
   * 源id
   */
  sourceId: number | undefined | null

  /**
   * 文件名
   */
  fileName: string | undefined | null

  /**
   * 文件路径
   */
  filePath: string | undefined | null

  /**
   * 工作目录
   */
  workdir: string | undefined | null

  constructor(backup?: Backup) {
    super()
    if (notNullish(backup)) {
      lodash.assign(this, lodash.pick(backup, ['sourceType', 'sourceId', 'fileName', 'filePath']))
    }
  }
}
