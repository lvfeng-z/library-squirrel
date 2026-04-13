import Task from '../entity/Task.ts'
import TreeNode from '../util/TreeNode.ts'
import lodash from 'lodash'

/**
 * 任务
 */
export default class TaskTreeDTO extends Task implements TreeNode {
  /**
   * 子任务（用于el-table的树形数据回显）
   */
  children: TaskTreeDTO[] | undefined | null

  /**
   * 是否有子任务（用于el-table的树形数据回显）
   */
  hasChildren: boolean | undefined | null

  /**
   * 是否为叶子节点
   */
  isLeaf: boolean | undefined | null

  constructor(taskDTO?: Task) {
    super(taskDTO)
    lodash.assign(this, lodash.pick(taskDTO, ['children', 'hasChildren', 'isLeaf']))
  }
}
