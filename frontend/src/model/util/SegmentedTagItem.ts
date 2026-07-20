import { SelectItem } from "@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto"
import { isNullish } from '@renderer/utils/CommonUtil.ts'

export default class SegmentedTagItem extends SelectItem {
  disabled: boolean
  /** 状态别名 key（如 source-local/source-site）。设置后 main 段颜色由 --app-status-{status}-* 令牌驱动，优先级高于下方显式色字段 */
  status?: string
  mainBackGround?: string
  mainBackGroundHover?: string
  mainTextColor?: string
  sub1BackGround?: string
  sub1BackGroundHover?: string
  sub1TextColor?: string
  sub2BackGround?: string
  sub2BackGroundHover?: string
  sub2TextColor?: string

  constructor(item: Partial<SelectItem> & CSegmentedTagItem) {
    super(item)
    this.disabled = isNullish(item.disabled) ? false : item.disabled
    this.status = item.status
    this.mainBackGround = item.mainBackGround
    this.mainBackGroundHover = item.mainBackGroundHover
    this.mainTextColor = item.mainTextColor
    this.sub1BackGround = item.sub1BackGround
    this.sub1BackGroundHover = item.sub1BackGroundHover
    this.sub1TextColor = item.sub1TextColor
    this.sub2BackGround = item.sub2BackGround
    this.sub2BackGroundHover = item.sub2BackGroundHover
    this.sub2TextColor = item.sub2TextColor
  }
}

interface CSegmentedTagItem {
  disabled?: boolean
  status?: string
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
