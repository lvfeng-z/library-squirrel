import SegmentedTagItem from '@renderer/model/util/SegmentedTagItem.ts'
import { arrayNotEmpty, isNullish } from '@renderer/utils/CommonUtil.ts'
import { isNotBlank } from '@renderer/utils/StringUtil.ts'

/** 搜索标签分类色 tone，对应分类色令牌 --app-tag-{tone}-* */
type SearchTagTone = 'green' | 'blue' | 'red' | 'purple'

/**
 * 按「标签/作者 × 本地/站点」四分类为搜索标签着色，复用分类色令牌（--app-tag-{tone}-*）。
 * 四色与 WorkQueryView/MainView 的类型筛选 checkbox 一致，使标签与其筛选项视觉对应、彼此可区分：
 * 本地标签=绿、站点标签=蓝、本地作者=红、站点作者=紫。
 * hover 底色由文字色 color-mix 派生，与 AutoLoadTagSelect 自定义标签同模式。
 * 写入 SegmentedTag 的显式色字段（mainBackGround 等），优先级低于 status 令牌、高于 variant 默认色。
 * @param segmentedTagItem 标签项
 */
export function setSearchTagColor(segmentedTagItem: SegmentedTagItem): void {
  appendNamespaceSegment(segmentedTagItem)
  const subLabels = segmentedTagItem.subLabels
  if (!arrayNotEmpty(subLabels)) {
    return
  }
  const tone = resolveTone(subLabels[0], subLabels[1])
  if (isNullish(tone)) {
    return
  }
  segmentedTagItem.mainBackGround = `var(--app-tag-${tone}-bg)`
  segmentedTagItem.mainBackGroundHover = `color-mix(in srgb, var(--app-tag-${tone}-text) 15%, transparent)`
  segmentedTagItem.mainTextColor = `var(--app-tag-${tone}-text)`
}

/** namespace 文本展示：site_tag 候选 extraData.namespace 非空时追加为 subLabels 的一节，SegmentedTag 按 subLabels 渲染 N 段 sub（组件零改动） */
function appendNamespaceSegment(item: SegmentedTagItem): void {
  const namespace: string | undefined = item.extraData?.namespace
  if (!isNotBlank(namespace)) {
    return
  }
  if (!arrayNotEmpty(item.subLabels)) {
    item.subLabels = []
  }
  item.subLabels.push(namespace)
}

/** 由 subLabels 的「类目/来源」解析分类色 tone：tag→绿(本地)/蓝(站点)，author→红(本地)/紫(站点) */
function resolveTone(category?: string, source?: string): SearchTagTone | undefined {
  const isLocal = source === 'local'
  switch (category) {
    case 'tag':
      return isLocal ? 'green' : 'blue'
    case 'author':
      return isLocal ? 'red' : 'purple'
    default:
      return undefined
  }
}
