/**
 * Search HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as SearchHandler } from '@bindings/github.com/library-squirrel/wails/internal/search'
import type { SearchCondition as BindingSearchCondition, SelectItem, WorkFullDTO } from '@bindings/github.com/library-squirrel/wails/internal/model/models'
import { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'

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

// SearchCondition 类型（与后端 domain.SearchCondition 对应）
export interface SearchCondition {
  type: number
  value: unknown
  operator?: string
}

// ========== 工具函数 ==========

/**
 * 将 SelectItem 转换为 SearchConditionItem
 */
function toSearchConditionItem(item: SelectItem | null): SearchConditionItem | null {
  if (!item) return null
  return {
    siteId: 0,
    siteName: item.label ?? '',
    tagIds: [],
    tagNames: []
  }
}

// ========== API 方法 ==========

export async function searchQuerySearchConditionPage(query: {
  pageNumber: number
  pageSize: number
}): Promise<ApiResponse<SearchConditionItem[]>> {
  const result = await SearchHandler.QuerySearchConditionPage(query.pageNumber, query.pageSize, null)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  const page = result.data
  if (!page) {
    return { success: true, msg: '', data: [] }
  }
  return {
    success: true,
    msg: result.msg ?? '',
    data: page.data ? page.data.map(toSearchConditionItem).filter((item): item is SearchConditionItem => item !== null) : []
  }
}

export async function searchQueryWorkPage(page: {
  pageNumber: number
  pageSize: number
  query?: SearchCondition[]
}): Promise<ApiResponse<Page<WorkFullDTO, BindingSearchCondition>>> {
  const conditions: BindingSearchCondition[] = (page.query ?? []).map(c => ({
    type: c.type as any,
    value: c.value
  }))
  const pageObj = new Page<BindingSearchCondition, BindingSearchCondition>({
    pageNumber: page.pageNumber,
    pageSize: page.pageSize,
    query: conditions as any
  })
  const result = await SearchHandler.QueryWorkPage(pageObj)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

export async function searchQueryWorkSetPage(query: {
  pageNumber: number
  pageSize: number
  keyword?: string
  siteId?: number
}): Promise<ApiResponse<SearchWorkSetItem[]>> {
  const { WorkSetQueryDTO } = await import('@bindings/github.com/library-squirrel/wails/internal/workSet/models')
  const queryDTO = new WorkSetQueryDTO({})
  const pageObj = new Page<WorkSetQueryDTO, WorkSetQueryDTO>({
    pageNumber: query.pageNumber,
    pageSize: query.pageSize,
    query: queryDTO
  })
  const result = await SearchHandler.QueryWorkSetPage(pageObj, query.keyword ?? '', query.siteId ?? 0)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  const page = result.data
  if (!page) {
    return { success: true, msg: '', data: [] }
  }
  return {
    success: true,
    msg: result.msg ?? '',
    data: page.data ? page.data.map(item => ({
      id: parseInt(item?.value?.toString() ?? '0'),
      name: item?.label ?? '',
      coverId: 0
    })) : []
  }
}