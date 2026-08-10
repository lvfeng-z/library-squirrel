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
  // namespaces 与 tagIds 等长配对（local=用户自设 ns，空串=无 ns）；site 关联由后端镜像 site_tag.namespace，传空数组
  const result = await ReWorkTagHandler.Link(tagType, tagIds, namespaces ?? [], workId)
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