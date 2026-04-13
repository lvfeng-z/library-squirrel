import { CommonInputConfig, ICommonInputConfig } from './CommonInputConfig.ts'

export class InputBox extends CommonInputConfig {
  name: string // 字段名
  placeholder?: string // 占位符
  show?: boolean // 是否展示本InputBox
  inputSpan?: number // 输入组件宽度
  disabled?: boolean // 是否禁用
  label?: string // 标题名称
  labelSpan?: number // 标题宽度
  showLabel?: boolean // 是否展示标题

  constructor(inputBox: IInputBox) {
    super(inputBox)
    this.name = inputBox.name
    this.placeholder = inputBox.placeholder
    this.show = inputBox.show
    this.inputSpan = inputBox.inputSpan
    this.disabled = inputBox.disabled
    this.label = inputBox.label
    this.labelSpan = inputBox.labelSpan
    this.showLabel = inputBox.showLabel
  }
}

export interface IInputBox extends ICommonInputConfig {
  name: string // 字段名
  placeholder?: string // 占位符
  show?: boolean // 是否展示本InputBox
  inputSpan?: number // 输入组件宽度
  disabled?: boolean // 是否禁用
  label?: string // 标题名称
  labelSpan?: number // 标题宽度
  showLabel?: boolean // 是否展示标题
}
