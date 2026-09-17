/**
 * ReWorkTag HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { requireResponse } from '../types'
import type { ReWorkTag } from '@bindings/github.com/library-squirrel/backend/base/model/entity/models'
import { Handler as ReWorkTagHandler } from '@bindings/github.com/library-squirrel/backend/reWorkTag'

// ========== API 方法 ==========

export async function reWorkTagLink(
  workId: number,
  tagType: number,
  tagIds: number[],
  namespaces?: string[]
): Promise<ApiResponse<boolean>> {
  if (tagIds.length === 0) {
    return { success: false, msg: 'tagIds 不能为空' }
  }
  // namespaces 与 tagIds 等长配对（local/site 关联均为用户自设 ns，空串=无 ns）
  const result = await ReWorkTagHandler.Link(tagType, tagIds, namespaces ?? [], workId)
  if (!result) {
    return { success: false, msg: '关联失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

// 精确摘除维度关联行：namespaces 与 tagIds 等长配对，只删 (work, tag, ns) 命中行。
// 改 ns 场景由调用方 diff 组合：新值走 reWorkTagLink、旧值行走本方法（维度盲删 reWorkTagUnlink 会
// 连同其他 ns 行一起删，多维度值场景禁止用于改值）
export async function reWorkTagUnlinkDimension(
  workId: number,
  tagType: number,
  tagIds: number[],
  namespaces: string[]
): Promise<ApiResponse<boolean>> {
  if (tagIds.length === 0) {
    return { success: false, msg: 'tagIds 不能为空' }
  }
  if (tagIds.length !== namespaces.length) {
    return { success: false, msg: 'tagIds 与 namespaces 须等长配对' }
  }
  const result = await ReWorkTagHandler.UnlinkDimension(tagType, tagIds, namespaces, workId)
  if (!result) {
    return { success: false, msg: '精确取消关联失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function reWorkTagUnlink(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  if (tagIds.length === 0) {
    return { success: false, msg: 'tagIds 不能为空' }
  }
  const result = await ReWorkTagHandler.Unlink(tagType, tagIds, workId)
  if (!result) {
    return { success: false, msg: '取消关联失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

// 查询作品的所有标签关联（含 namespace），供已绑定候选区/详情区只读展示 ns
export async function reWorkTagListByWorkId(workId: number) {
  return requireResponse(await ReWorkTagHandler.ListByWorkId(workId), '查询作品标签关联')
}