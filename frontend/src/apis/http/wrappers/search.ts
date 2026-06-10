/**
 * Search HTTP API 包装器
 * 直接调用 bindings 接口
 */

import {
  Handler as SearchHandler
} from '@bindings/github.com/library-squirrel/backend/search'
import {
  Page
} from '@bindings/github.com/library-squirrel/backend/base/model'
import type {
  WorkFullDTO,
  WorkSetWithCoverDTO,
  SelectItem,
  SearchCondition,
  SearchConditionQuery
} from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

// ========== API 方法 ==========

/** 查询搜索条件分页 */
export async function searchQuerySearchConditionPage(
  page: Page<SelectItem>,
  query: SearchConditionQuery | null
): Promise<ApiResult<Page<SelectItem>>> {
  return requireResponse(
    await SearchHandler.QuerySearchConditionPage(page.pageNumber, page.pageSize, query),
    '查询搜索条件'
  )
}

/** 查询作品分页 */
export async function searchQueryWorkPage(
  page: Page<WorkFullDTO>,
  conditions: SearchCondition[]
): Promise<ApiResult<Page<WorkFullDTO>>> {
  const searchConditions: SearchCondition[] = conditions.map(c => ({
    type: c.type as any,
    value: c.value,
    operator: c.operator as any
  }))
  return requireResponse(
    await SearchHandler.QueryWorkPage(page.pageNumber, page.pageSize, searchConditions),
    '查询作品'
  )
}

/** 查询作品集分页（通过搜索条件筛选） */
export async function searchQueryWorkSetPage(
  page: Page<WorkSetWithCoverDTO>,
  conditions: SearchCondition[]
): Promise<ApiResult<Page<WorkSetWithCoverDTO>>> {
  const searchConditions: SearchCondition[] = conditions.map(c => ({
    type: c.type as any,
    value: c.value,
    operator: c.operator as any
  }))
  return requireResponse(
    await SearchHandler.QueryWorkSetPage(page, searchConditions),
    '查询作品集'
  )
}
