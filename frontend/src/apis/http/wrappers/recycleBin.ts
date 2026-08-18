/**
 * RecycleBin HTTP API 包装器
 * 直接调用 bindings 接口
 */

import {
	Handler as RecycleBinHandler,
	RecycleQueryDTO
} from '@bindings/github.com/library-squirrel/backend/recycleBin'
import { RecycleItemDTO } from '@bindings/github.com/library-squirrel/backend/recycleBin/models'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

// ========== API 方法 ==========

/**
 * 分页查询回收站列表（支持时间范围/站点/作者/标签筛选与排序）
 */
export async function recycleBinPage(page: Page<RecycleItemDTO>, query: RecycleQueryDTO): Promise<ApiResult<Page<RecycleItemDTO>>> {
  return requireResponse(await RecycleBinHandler.Page(page, query), '查询回收站')
}

/**
 * 从回收站复原作品
 * overwrite: 冲突时是否覆盖已存在的作品
 */
export async function recycleBinRestore(id: number, overwrite: boolean): Promise<ApiResponse<number>> {
  const result = await RecycleBinHandler.Restore(id, overwrite)
  if (!result) {
    return { success: false, msg: '复原失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '复原失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 彻底删除回收站条目（不可恢复）
 */
export async function recycleBinPurge(id: number): Promise<ApiResponse<boolean>> {
  const result = await RecycleBinHandler.Purge(id)
  if (!result) {
    return { success: false, msg: '删除失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}
