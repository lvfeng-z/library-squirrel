/**
 * ReWorkAuthor HTTP API 包装器
 * 作品与作者关联关系的 API 封装
 */

import type { ApiResponse } from '../types'
import { requireResponse } from '../types'
import type { RankedLocalAuthor, RankedSiteAuthor } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { Handler as ReWorkAuthorHandler } from '@bindings/github.com/library-squirrel/backend/reWorkAuthor'

// ========== API 方法 ==========

export async function reWorkAuthorLink(
  workId: number,
  authorType: number,
  authorIds: number[],
  roleNames: string[]
): Promise<ApiResponse<boolean>> {
  if (authorIds.length === 0) {
    return { success: false, msg: 'authorIds 不能为空' }
  }
  // roleNames 与 authorIds 等长配对（local/site 关联均为用户自设 role，空串=无 role；同作者不同 role 落独立关联行）
  const result = await ReWorkAuthorHandler.Link(authorType, authorIds, roleNames, workId)
  if (!result) {
    return { success: false, msg: '关联失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

// 精确摘除维度关联行：roleNames 与 authorIds 等长配对，只删 (work, author, role) 命中行。
// 改 role 场景由调用方 diff 组合：新值走 reWorkAuthorLink、旧值行走本方法（维度盲删 reWorkAuthorUnlink
// 会连同其他 role 行一起删，多维度值场景禁止用于改值）
export async function reWorkAuthorUnlinkDimension(
  workId: number,
  authorType: number,
  authorIds: number[],
  roleNames: string[]
): Promise<ApiResponse<boolean>> {
  if (authorIds.length === 0) {
    return { success: false, msg: 'authorIds 不能为空' }
  }
  if (authorIds.length !== roleNames.length) {
    return { success: false, msg: 'authorIds 与 roleNames 须等长配对' }
  }
  const result = await ReWorkAuthorHandler.UnlinkDimension(authorType, authorIds, roleNames, workId)
  if (!result) {
    return { success: false, msg: '精确取消关联失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function reWorkAuthorUnlink(
  workId: number,
  authorType: number,
  authorIds: number[]
): Promise<ApiResponse<boolean>> {
  if (authorIds.length === 0) {
    return { success: false, msg: 'authorIds 不能为空' }
  }
  const result = await ReWorkAuthorHandler.Unlink(authorType, authorIds, workId)
  if (!result) {
    return { success: false, msg: '取消关联失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

// 查询作品关联的本地作者（含关联级 roleName），供已绑定候选区与作品详情作者分段 tag 展示
export async function reWorkAuthorListLocalAuthorsByWorkId(workId: number) {
  return requireResponse(await ReWorkAuthorHandler.ListLocalAuthorsByWorkId(workId), '查询作品本地作者')
}

// 查询作品关联的站点作者（含关联级 roleName），供已绑定候选区与作品详情作者分段 tag 展示
export async function reWorkAuthorListSiteAuthorsByWorkId(workId: number) {
  return requireResponse(await ReWorkAuthorHandler.ListSiteAuthorsByWorkId(workId), '查询作品站点作者')
}
