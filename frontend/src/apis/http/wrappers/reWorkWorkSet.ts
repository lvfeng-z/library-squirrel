/**
 * ReWorkWorkSet HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as WorkSetHandler } from '@bindings/github.com/library-squirrel/wails/internal/workSet'
import { Handler as TaskManagerHandler } from '@bindings/github.com/library-squirrel/wails/internal/taskManager'

/**
 * 批量关联作品到作品集
 * 注意：此方法在 bindings 中未实现
 */
export async function reWorkWorkSetLinkBatchToWorkSet(
  _workSetId: number,
  _workIds: number[]
): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (LinkBatchToWorkSet)
  // WorkSetHandler 有 LinkWorkToWorkSet 但参数是 (workSetId, workId)
  return { success: false, msg: '此接口未实现：reWorkWorkSetLinkBatchToWorkSet' }
}

/**
 * 批量取消作品与作品集的关联
 * 注意：此方法在 bindings 中未实现
 */
export async function reWorkWorkSetRemoveBatchFromWorkSet(
  _workSetId: number,
  _workIds: number[]
): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (RemoveBatchFromWorkSet)
  return { success: false, msg: '此接口未实现：reWorkWorkSetRemoveBatchFromWorkSet' }
}

/**
 * 更新作品排序顺序
 * 注意：此方法在 bindings 中未实现
 */
export async function reWorkWorkSetUpdateSortOrders(
  _workSetId: number,
  _workIds: number[]
): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (UpdateSortOrders)
  return { success: false, msg: '此接口未实现：reWorkWorkSetUpdateSortOrders' }
}

/**
 * 设置作品集封面
 * 注意：此方法在 bindings 中未实现
 */
export async function reWorkWorkSetSetCover(
  _workSetId: number,
  _workId: number
): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (SetCover)
  return { success: false, msg: '此接口未实现：reWorkWorkSetSetCover' }
}

/**
 * 取消作品集封面
 * 注意：此方法在 bindings 中未实现
 */
export async function reWorkWorkSetUnsetCover(
  _workSetId: number,
  _workId: number
): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (UnsetCover)
  return { success: false, msg: '此接口未实现：reWorkWorkSetUnsetCover' }
}

/**
 * 获取作品集封面作品ID
 * 注意：此方法在 bindings 中未实现
 */
export async function reWorkWorkSetGetCoverWorkId(
  _workSetId: number
): Promise<ApiResponse<number | null>> {
  // TODO: 此接口在 bindings 中未实现 (GetCoverWorkId)
  return { success: false, msg: '此接口未实现：reWorkWorkSetGetCoverWorkId' }
}