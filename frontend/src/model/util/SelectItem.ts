/**
 * 下拉选择框选项
 */
export default class SelectItem {
  value: number | string | null | undefined
  label: string | null | undefined
  subLabels: string[] | null | undefined
  rootId: string | null | undefined
  extraData: object | string | null | undefined

  constructor(selectItem?: SelectItem) {
    if (selectItem === undefined) {
      this.value = undefined
      this.label = undefined
      this.subLabels = undefined
      this.rootId = undefined
      this.extraData = undefined
    } else {
      this.value = selectItem.value
      this.label = selectItem.label
      this.subLabels = selectItem.subLabels
      this.rootId = selectItem.rootId
      this.extraData = selectItem.extraData
    }
  }
}
