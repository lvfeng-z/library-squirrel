/**
 * RecycleBin HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import {
	Handler as RecycleBinHandler,
	RecyclePageQuery
} from '@bindings/github.com/library-squirrel/backend/recycleBin'
import { RecycleWorkDTO } from '@bindings/github.com/library-squirrel/backend/recycleBin/models'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

// ========== API 方法 ==========

/**
 * 分页查询回收站列表（条件体系复用作品搜索 SearchCondition + 排序）
 */
export async function recycleBinPage(page: Page<RecycleWorkDTO>, query: RecyclePageQuery): Promise<ApiResult<Page<RecycleWorkDTO>>> {
  return requireResponse(await RecycleBinHandler.Page(page, query), '查询回收站')
}

/**
 * 从回收站复原作品
 * overwrite: 冲突时是否将占位作品转入回收站
 */
export async function recycleBinRestore(workId: number, overwrite: boolean): Promise<ApiResult<number>> {
  return requireResponse(await RecycleBinHandler.Restore(workId, overwrite), '复原作品', false)
}

/**
 * 彻底删除回收站条目（不可恢复）
 */
export async function recycleBinPurge(workId: number): Promise<ApiResult<any>> {
  return requireResponse(await RecycleBinHandler.Purge(workId), '彻底删除作品', false)
}
