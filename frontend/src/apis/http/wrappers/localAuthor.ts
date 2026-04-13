/**
 * LocalAuthor HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export interface LocalAuthorVO {
  id: number
  authorName: string
  introduce: string
  lastUse: number
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: LocalAuthorVO[]
  total: number
  page: number
  pageSize: number
}

export async function localAuthorSave(author: {
  authorName?: string
  introduce?: string
}): Promise<ApiResponse<LocalAuthorVO>> {
  return apiProxy.invoke<LocalAuthorVO>('localAuthor-save', author)
}

export async function localAuthorDeleteById(id: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('localAuthor-deleteById', { id })
}

export async function localAuthorUpdateById(author: {
  id: number
  authorName?: string
  introduce?: string
}): Promise<ApiResponse<LocalAuthorVO>> {
  return apiProxy.invoke<LocalAuthorVO>('localAuthor-updateById', author)
}

export async function localAuthorGetById(id: number): Promise<ApiResponse<LocalAuthorVO>> {
  return apiProxy.invoke<LocalAuthorVO>('localAuthor-getById', id)
}

export async function localAuthorQueryPage(query: {
  page: number
  pageSize: number
  query?: { authorName?: string }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('localAuthor-queryPage', query)
}

export async function localAuthorListSelectItems(
  query?: Record<string, unknown>
): Promise<ApiResponse<LocalAuthorVO[]>> {
  return apiProxy.invoke<LocalAuthorVO[]>('localAuthor-listSelectItems', query)
}

export async function localAuthorQuerySelectItemPage(query: {
  page: number
  pageSize: number
  query?: Record<string, unknown>
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('localAuthor-querySelectItemPage', query)
}
