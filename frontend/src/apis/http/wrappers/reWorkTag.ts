/**
 * ReWorkTag HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as ReWorkTagHandler } from '@bindings/github.com/library-squirrel/backend/reWorkTag'
import type { ReWorkTag } from '@bindings/github.com/library-squirrel/backend/model/models'

export interface ReWorkTagVO {
  id: number
  workId: number
  tagType: number
  localTagId: number
  siteTagId: number
}

// ========== 工具函数 ==========

/**
 * 将 ReWorkTag 转换为 ReWorkTagVO
 * 注意：ReWorkTag 中的 workId 等字段是 sql.NullInt64 类型
 */
function toReWorkTagVO(dto: ReWorkTag | null): ReWorkTagVO | null {
  if (!dto) return null
  return {
    id: dto.id,
    workId: dto.workId?.Valid ? Number(dto.workId.Int64) : 0,
    tagType: dto.tagType?.Valid ? Number(dto.tagType.Int64) : 0,
    localTagId: dto.localTagId?.Valid ? Number(dto.localTagId.Int64) : 0,
    siteTagId: dto.siteTagId?.Valid ? Number(dto.siteTagId.Int64) : 0
  }
}

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

export async function reWorkTagLinkBatch(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  const result = await ReWorkTagHandler.Link(tagType, tagIds, workId)
  if (!result) {
    return { success: false, msg: '批量关联失败：接口返回为空' }
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

export async function reWorkTagRemoveBatch(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  const result = await ReWorkTagHandler.Unlink(tagType, tagIds, workId)
  if (!result) {
    return { success: false, msg: '批量取消关联失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function reWorkTagList(workId: number): Promise<ApiResponse<ReWorkTagVO[]>> {
  const result = await ReWorkTagHandler.ListByWorkId(workId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? result.data.map(toReWorkTagVO).filter((item): item is ReWorkTagVO => item !== null) : [] }
}