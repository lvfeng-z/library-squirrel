import LocalTag from '../entity/LocalTag.ts'
import lodash from 'lodash'
import { notNullish } from '@renderer/utils/CommonUtil.ts'
import { parsePropertyFromJson } from '@renderer/utils/ObjectUtil.ts'

export default class LocalTagDTO extends LocalTag {
  /**
   * 是否为叶子节点
   */
  isLeaf: boolean | undefined | null

  /**
   * 基础标签
   */
  baseTag: LocalTag | undefined | null

  constructor(localTagDTO?: LocalTag) {
    super(localTagDTO)
    if (notNullish(localTagDTO)) {
      lodash.assign(this, lodash.pick(localTagDTO, ['isLeaf', 'baseTag']))
      parsePropertyFromJson(this, [{ property: 'baseTag', builder: (src) => new LocalTag(src) }])
    }
  }
}
