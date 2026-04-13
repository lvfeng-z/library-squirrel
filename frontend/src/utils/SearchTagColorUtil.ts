import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import { arrayNotEmpty } from '@renderer/utils/CommonUtil.ts'

/**
 * 设置搜索标签颜色
 * @param segmentedTagItem 标签项
 */
export function setSearchTagColor(segmentedTagItem: SegmentedTagItem): void {
  const subLabels = segmentedTagItem.subLabels
  if (arrayNotEmpty(subLabels)) {
    switch (subLabels[0]) {
      case 'author':
        switch (subLabels[1]) {
          case 'local':
            segmentedTagItem.mainBackGround = 'rgb(245, 108, 108, 25%)'
            segmentedTagItem.mainBackGroundHover = 'rgb(245, 108, 108, 10%)'
            segmentedTagItem.mainTextColor = 'rgb(245, 108, 108, 75%)'
            break
          default:
            segmentedTagItem.mainBackGround = 'rgb(164, 158, 255, 25%)'
            segmentedTagItem.mainBackGroundHover = 'rgb(164, 158, 255, 10%)'
            segmentedTagItem.mainTextColor = 'rgb(164, 158, 255, 95%)'
            break
        }
        break
      case 'tag':
        if (subLabels[1] !== 'local') {
          segmentedTagItem.mainBackGround = 'rgb(64, 158, 255, 25%)'
          segmentedTagItem.mainBackGroundHover = 'rgb(64, 158, 255, 10%)'
          segmentedTagItem.mainTextColor = 'rgb(64, 158, 255, 85%)'
        }
        break
    }
  }
}
