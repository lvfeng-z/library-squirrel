import Task from '../entity/Task.ts'
import TreeNode from '../../util/TreeNode.ts'
import lodash from 'lodash'
import { notNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 任务树节点，兼容两种数据格式：
 * - 新 binding 格式：TaskProgressTreeDTO = {taskProgress: {task: {...}, ...}, children, hasChildren, isLeaf}
 * - 旧 flat 格式：Task + {children, hasChildren, isLeaf}
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

  constructor(data?: any) {
    // 新 binding 格式：从 taskProgress.task 提取基础任务数据
    const taskData = data?.taskProgress?.task ?? data
    super(taskData)
    if (notNullish(data)) {
      lodash.assign(this, lodash.pick(data, ['children', 'hasChildren', 'isLeaf']))
    }
  }
}
