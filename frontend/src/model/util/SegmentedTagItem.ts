import SelectItem, { CSelectItem } from './SelectItem.ts'
import { isNullish } from '@renderer/utils/CommonUtil.ts'

export default class SegmentedTagItem extends SelectItem {
  disabled: boolean
  mainBackGround?: string
  mainBackGroundHover?: string
  mainTextColor?: string
  sub1BackGround?: string
  sub1BackGroundHover?: string
  sub1TextColor?: string
  sub2BackGround?: string
  sub2BackGroundHover?: string
  sub2TextColor?: string

  constructor(cSegmentedTagItem: CSegmentedTagItem) {
    super(cSegmentedTagItem)
    this.disabled = isNullish(cSegmentedTagItem.disabled) ? false : cSegmentedTagItem.disabled
    this.mainBackGround = cSegmentedTagItem.mainBackGround
    this.mainBackGroundHover = cSegmentedTagItem.mainBackGroundHover
    this.mainTextColor = cSegmentedTagItem.mainTextColor
    this.sub1BackGround = cSegmentedTagItem.sub1BackGround
    this.sub1BackGroundHover = cSegmentedTagItem.sub1BackGroundHover
    this.sub1TextColor = cSegmentedTagItem.sub1TextColor
    this.sub2BackGround = cSegmentedTagItem.sub2BackGround
    this.sub2BackGroundHover = cSegmentedTagItem.sub2BackGroundHover
    this.sub2TextColor = cSegmentedTagItem.sub2TextColor
  }
}

interface CSegmentedTagItem extends CSelectItem {
  disabled?: boolean
  mainBackGround?: string
  mainBackGroundHover?: string
  mainTextColor?: string
  sub1BackGround?: string
  sub1BackGroundHover?: string
  sub1TextColor?: string
  sub2BackGround?: string
  sub2BackGroundHover?: string
  sub2TextColor?: string
}
