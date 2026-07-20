import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import { arrayNotEmpty } from '@renderer/utils/CommonUtil.ts'

/**
 * 根据搜索标签的来源类型（作者/标签 × 本地/站点）设置其状态别名 key，
 * 由 SegmentedTag 据 --app-status-{status}-* 令牌渲染颜色，自动跟随主题。
 * @param segmentedTagItem 标签项
 */
export function setSearchTagStatus(segmentedTagItem: SegmentedTagItem): void {
  const subLabels = segmentedTagItem.subLabels
  if (arrayNotEmpty(subLabels)) {
    switch (subLabels[0]) {
      case 'author':
        // 作者：本地→source-local(红)，站点→source-site(蓝)
        segmentedTagItem.status = subLabels[1] === 'local' ? 'source-local' : 'source-site'
        break
      case 'tag':
        // 标签：站点→source-site(蓝)；本地不设，保持 neutral 默认
        if (subLabels[1] !== 'local') {
          segmentedTagItem.status = 'source-site'
        }
        break
    }
  }
}
