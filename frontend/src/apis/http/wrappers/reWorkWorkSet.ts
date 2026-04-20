/**
 * ReWorkWorkSet HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as WorkSetHandler } from '@bindings/github.com/library-squirrel/wails/internal/workSet'

/**
 * 批量关联作品到作品集
 */
export async function reWorkWorkSetLinkBatchToWorkSet(
  workSetId: number,
  workIds: number[]
): Promise<ApiResponse<boolean>> {
  const result = await WorkSetHandler.LinkBatchToWorkSet(workSetId, workIds)
  if (!result) {
    return { success: false, msg: '操作失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 批量取消作品与作品集的关联
 */
export async function reWorkWorkSetRemoveBatchFromWorkSet(
  workSetId: number,
  workIds: number[]
): Promise<ApiResponse<boolean>> {
  const result = await WorkSetHandler.RemoveBatchFromWorkSet(workSetId, workIds)
  if (!result) {
    return { success: false, msg: '操作失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 更新作品排序顺序
 */
export async function reWorkWorkSetUpdateSortOrders(
  workSetId: number,
  workIds: number[]
): Promise<ApiResponse<boolean>> {
  const result = await WorkSetHandler.UpdateSortOrders(workSetId, workIds)
  if (!result) {
    return { success: false, msg: '操作失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 设置作品集封面
 */
export async function reWorkWorkSetSetCover(
  workSetId: number,
  workId: number
): Promise<ApiResponse<boolean>> {
  const result = await WorkSetHandler.SetCover(workSetId, workId)
  if (!result) {
    return { success: false, msg: '操作失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 取消作品集封面
 */
export async function reWorkWorkSetUnsetCover(
  workSetId: number,
  workId: number
): Promise<ApiResponse<boolean>> {
  const result = await WorkSetHandler.UnsetCover(workSetId, workId)
  if (!result) {
    return { success: false, msg: '操作失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 获取作品集封面作品ID
 */
export async function reWorkWorkSetGetCoverWorkId(
  workSetId: number
): Promise<ApiResponse<number | null>> {
  const result = await WorkSetHandler.GetCoverWorkId(workSetId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? null }
}