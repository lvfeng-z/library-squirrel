import { notNullish } from './CommonUtil.ts'
import { getPropByPath } from './ObjectUtil.ts'
import TreeNode from '../model/util/TreeNode.ts'

/**
 * 递归创建树形列表
 * @param data 树节点
 * @param pid 父母节点id
 */
export function buildTree<T extends TreeNode>(data: T[], pid: string | number | null | undefined): T[] {
  // 遍历列表，收集所有子节点
  const tree: T[] = [...data.filter((node) => node.pid === pid)]

  // 递归每一个子节点的子节点
  tree.forEach((node) => {
    node.children = buildTree(data, node.id)
  })

  return tree
}

/**
 * 按照value递归查询节点
 * @param root
 * @param id
 */
export function getNode<T extends TreeNode>(root: T, id: string | number): T | undefined {
  if (root.id === id) {
    return root
  }
  if (notNullish(root.children) && root.children.length > 0) {
    for (let index = root.children.length - 1; index >= 0; index--) {
      let target
      for (let childIndex = root.children.length - 1; childIndex >= 0; childIndex--) {
        target = getNode(root.children[childIndex], id)
        if (target !== undefined) {
          return target
        }
      }
    }
  }
  return undefined
}

/**
 * 按照指定路径递归查询节点
 * @param rows 节点列表
 * @param id 目标 id
 * @param idPath id 属性的路径（如 'taskProgress.task.id'），默认 'id'
 */
export function getNodeByPath<T extends object>(
  rows: T[],
  id: string | number,
  idPath: string = 'id'
): T | undefined {
  for (let i = rows.length - 1; i >= 0; i--) {
    const node = rows[i]
    if (getPropByPath<string | number>(node, idPath) === id) {
      return node
    }
    const children = getPropByPath<object[]>(node, 'children')
    if (notNullish(children) && Array.isArray(children)) {
      const found = getNodeByPath(children as T[], id, idPath)
      if (found !== undefined) return found
    }
  }
  return undefined
}
