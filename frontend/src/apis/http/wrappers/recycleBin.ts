/**
 * RecycleBin HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import {
	Handler as RecycleBinHandler,
	RecyclePageQuery
} from '@bindings/github.com/library-squirrel/backend/recycleBin'
import { RecycleWorkDTO } from '@bindings/github.com/library-squirrel/backend/recycleBin/models'
import { RecycleStoreDTO, RecycleStorePageQuery } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model/models'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

// ========== API 方法 ==========

/**
 * 分页查询回收站作品条目（条件体系复用作品搜索 SearchCondition + 排序）
 */
export async function recycleBinPageWorks(page: Page<RecycleWorkDTO>, query: RecyclePageQuery): Promise<ApiResult<Page<RecycleWorkDTO>>> {
  return requireResponse(await RecycleBinHandler.PageWorks(page, query), '查询回收站作品')
}

/**
 * 分页查询回收站文件条目（persistent_store 已删行；文件域条件体系见 RecycleStorePageQuery）
 */
export async function recycleBinPageStores(page: Page<RecycleStoreDTO>, query: RecycleStorePageQuery): Promise<ApiResult<Page<RecycleStoreDTO>>> {
  return requireResponse(await RecycleBinHandler.PageStores(page, query), '查询回收站文件')
}

/**
 * 从回收站复原作品条目
 * overwrite: 冲突时是否将占位作品转入回收站
 */
export async function recycleBinRestoreWork(workId: number, overwrite: boolean): Promise<ApiResult<number>> {
  return requireResponse(await RecycleBinHandler.RestoreWork(workId, overwrite), '复原作品', false)
}

/**
 * 彻底删除回收站作品条目（不可恢复，级联清从属行与备份）
 */
export async function recycleBinPurgeWork(workId: number): Promise<ApiResult<any>> {
  return requireResponse(await RecycleBinHandler.PurgeWork(workId), '彻底删除作品', false)
}

/**
 * 彻底删除回收站文件条目（不可恢复，条目单位=store 行）
 */
export async function recycleBinPurgeStore(storeId: number): Promise<ApiResult<any>> {
  return requireResponse(await RecycleBinHandler.PurgeStore(storeId), '彻底删除文件条目', false)
}
