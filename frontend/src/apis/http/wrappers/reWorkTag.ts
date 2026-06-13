/**
 * ReWorkTag HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as ReWorkTagHandler } from '@bindings/github.com/library-squirrel/backend/reWorkTag'

// ========== API 方法 ==========

export async function reWorkTagLink(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  // 找到第一个 tagId 进行关联
  if (tagIds.length === 0) {
    return { success: false, msg: 'tagIds 不能为空' }
  }
  const result = await ReWorkTagHandler.Link(tagType, [tagIds[0]], workId)
  if (!result) {
    return { success: false, msg: '关联失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function reWorkTagUnlink(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  // 找到第一个 tagId 进行取消关联
  if (tagIds.length === 0) {
    return { success: false, msg: 'tagIds 不能为空' }
  }
  const result = await ReWorkTagHandler.Unlink(tagType, [tagIds[0]], workId)
  if (!result) {
    return { success: false, msg: '取消关联失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}