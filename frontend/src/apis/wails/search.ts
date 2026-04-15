/**
 * Search Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/search'
import type { ApiResponse } from '@apis/http'
import type { SearchType, SearchCondition } from '@bindings/github.com/library-squirrel/wails/internal/model/models'
import type { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'
import type { SelectItem, WorkFullDTO } from '@bindings/github.com/library-squirrel/wails/internal/model/models'

// ========== API 方法 ==========

/**
 * 分页查询搜索条件
 */
export async function searchQuerySearchConditionPage(
  keyword: string,
  types: SearchType[]
): Promise<ApiResponse<Page<SelectItem> | null>> {
  return Handler.QuerySearchConditionPage(1, 10, { keyword, types })
}

/**
 * 分页查询作品
 */
export async function searchQueryWorkPage(
  conditions: SearchCondition[]
): Promise<ApiResponse<Page<WorkFullDTO> | null>> {
  return Handler.QueryWorkPage(1, 10, conditions)
}

/**
 * 分页查询作品集
 */
export async function searchQueryWorkSetPage(
  keyword: string,
  siteId: number
): Promise<ApiResponse<Page<SelectItem> | null>> {
  return Handler.QueryWorkSetPage(1, 10, keyword, siteId)
}