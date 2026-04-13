import { BackupSourceTypeEnum } from '../constant/BackupSourceTypeEnum.ts'
import BaseQueryDTO from '../base/BaseQueryDTO.ts'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import lodash from 'lodash'

export default class BackupQueryDTO extends BaseQueryDTO {
  /**
   * 来源类型
   */
  sourceType?: BackupSourceTypeEnum | undefined | null

  /**
   * 原id
   */
  sourceId?: number | undefined | null

  /**
   * 文件名
   */
  fileName?: string | undefined | null

  /**
   * 文件路径
   */
  filePath?: string | undefined | null

  /**
   * 工作目录
   */
  workdir?: string | undefined | null

  constructor(baseQueryDTO?: BaseQueryDTO) {
    super()
    if (notNullish(baseQueryDTO)) {
      lodash.assign(this, lodash.pick(baseQueryDTO, ['sourceType', 'sourceId', 'fileName', 'filePath']))
    }
  }
}
