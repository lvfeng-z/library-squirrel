/**
 * Search HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'
import type WorkFullDTO from '@renderer/model/model/dto/WorkFullDTO'
import Page from '@renderer/model/model/util/Page'

export interface SearchConditionItem {
  siteId: number
  siteName: string
  tagIds: number[]
  tagNames: string[]
}

export interface SearchWorkSetItem {
  id: number
  name: string
  coverId: number
}

export async function searchQuerySearchConditionPage(query: {
  pageNumber: number
  pageSize: number
}): Promise<ApiResponse<SearchConditionItem[]>> {
  return apiProxy.invoke<SearchConditionItem[]>('search-querySearchConditionPage', query)
}

export async function searchQueryWorkPage(page: {
  pageNumber: number
  pageSize: number
  query?: SearchCondition[]
}): Promise<ApiResponse<Page<unknown, WorkFullDTO>>> {
  return apiProxy.invoke<Page<unknown, WorkFullDTO>>('search-queryWorkPage', page)
}

export async function searchQueryWorkSetPage(query: {
  pageNumber: number
  pageSize: number
  keyword?: string
  siteId?: number
}): Promise<ApiResponse<SearchWorkSetItem[]>> {
  return apiProxy.invoke<SearchWorkSetItem[]>('search-queryWorkSetPage', query)
}

// SearchCondition 类型（与后端 domain.SearchCondition 对应）
export interface SearchCondition {
  type: number
  value: unknown
  operator?: string
}
