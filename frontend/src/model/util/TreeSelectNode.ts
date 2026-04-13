import TreeNode from './TreeNode.ts'

/**
 * 树形选择框选项
 */
export default class TreeSelectNode implements TreeNode {
  id: string | number | undefined | null
  pid: string | number | undefined | null
  children: TreeSelectNode[] | undefined | null
  isLeaf: boolean | undefined | null
  value: string | number | null | undefined
  label: string | null | undefined
  secondaryLabel: string | null | undefined
  extraData: object | null | undefined
  disabled: boolean | null | undefined

  constructor(treeSelectNode?: TreeSelectNode) {
    if (treeSelectNode === undefined) {
      this.id = undefined
      this.pid = undefined
      this.children = undefined
      this.isLeaf = undefined
      this.value = undefined
      this.label = undefined
      this.secondaryLabel = undefined
      this.extraData = undefined
      this.disabled = undefined
    } else {
      this.id = treeSelectNode.id
      this.pid = treeSelectNode.pid
      this.children = treeSelectNode.children
      this.isLeaf = treeSelectNode.isLeaf
      this.value = treeSelectNode.value
      this.label = treeSelectNode.label
      this.secondaryLabel = treeSelectNode.secondaryLabel
      this.extraData = treeSelectNode.extraData
      this.disabled = treeSelectNode.disabled
    }
  }
}
