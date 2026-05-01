import { notNullish } from '@renderer/utils/CommonUtil.ts'
import lodash from 'lodash'
import TreeNode from '../../util/TreeNode.ts'
import TaskProgressDTO from './TaskProgressDTO.ts'

/**
 * 任务进度树节点，兼容两种数据格式：
 * - 新 binding 格式：{taskProgress: {task: {...}, total, finished, siteName}, children, hasChildren, isLeaf}
 * - 旧 flat 格式：TaskProgressDTO + {children, hasChildren, isLeaf}
 */
export default class TaskProgressTreeDTO extends TaskProgressDTO implements TreeNode {
  /**
   * 子任务（用于el-table的树形数据回显）
   */
  children: TaskProgressTreeDTO[] | undefined | null

  /**
   * 是否有子任务（用于el-table的树形数据回显）
   */
  hasChildren: boolean | undefined | null

  /**
   * 是否为叶子节点
   */
  isLeaf: boolean | undefined | null

  constructor(data?: any) {
    // 新 binding 格式：将 taskProgress 整体传给 TaskProgressDTO 处理
    const progressData = data?.taskProgress ?? data
    super(progressData)
    if (notNullish(data)) {
      lodash.assign(this, lodash.pick(data, ['children', 'hasChildren', 'isLeaf']))
    }
  }
}
