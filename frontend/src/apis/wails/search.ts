/**
 * Search Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'
import { toApiResponse } from './index'
import type { SearchType, SearchCondition } from '../../../bindings/github.com/library-squirrel/wails/internal/model/models'
import type { Page } from '../../../bindings/github.com/library-squirrel/wails/pkg/model/models'
import type { SelectItem, WorkFullDTO } from '../../../bindings/github.com/library-squirrel/wails/internal/model/models'

// ========== API 方法 ==========

/**
 * 分页查询搜索条件
 */
export async function searchQuerySearchConditionPage(
  keyword: string,
  types: SearchType[]
): Promise<ApiResponse<Page<SelectItem> | null>> {
  return toApiResponse(App.SearchQuerySearchConditionPage(keyword, types))
}

/**
 * 分页查询作品
 */
export async function searchQueryWorkPage(
  conditions: SearchCondition[]
): Promise<ApiResponse<Page<WorkFullDTO> | null>> {
  return toApiResponse(App.SearchQueryWorkPage(conditions))
}

/**
 * 分页查询作品集
 */
export async function searchQueryWorkSetPage(
  keyword: string,
  siteId: number
): Promise<ApiResponse<Page<SelectItem> | null>> {
  return toApiResponse(App.SearchQueryWorkSetPage(keyword, siteId))
}
