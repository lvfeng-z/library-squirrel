/**
 * 树节点
 */
export default interface TreeNode {
  id: string | number | undefined | null
  pid: string | number | undefined | null
  children: TreeNode[] | undefined | null
  isLeaf: boolean | undefined | null
}
