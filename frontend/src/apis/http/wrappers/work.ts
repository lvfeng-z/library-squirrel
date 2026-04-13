/**
 * Work HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export interface WorkVO {
  id: number
  title: string
  siteId: number
  siteWorkId: string
  coverUrl: string
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: WorkVO[]
  total: number
  page: number
  pageSize: number
}

export async function workGetFullWorkInfoById(id: number): Promise<ApiResponse<WorkVO>> {
  return apiProxy.invoke<WorkVO>('work-getFullWorkInfoById', id)
}

export async function workQueryPage(query: {
  page: number
  pageSize: number
  query?: { siteId?: number; title?: string }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('work-queryPage', query)
}

export async function workDeleteWorkAndSurroundingData(id: number): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('work-deleteWorkAndSurroundingData', { id })
}

export async function workListRankedLocalAuthorWithWorkIdByWorkIds(
  workIds: number[]
): Promise<ApiResponse<WorkVO[]>> {
  return apiProxy.invoke<WorkVO[]>('work-listRankedLocalAuthorWithWorkIdByWorkIds', { workIds })
}

export async function workListRankedSiteAuthorWithWorkIdByWorkIds(
  workIds: number[]
): Promise<ApiResponse<WorkVO[]>> {
  return apiProxy.invoke<WorkVO[]>('work-listRankedSiteAuthorWithWorkIdByWorkIds', { workIds })
}

export async function workListReWorkAuthor(workId: number): Promise<ApiResponse<WorkVO[]>> {
  return apiProxy.invoke<WorkVO[]>('work-listReWorkAuthor', { workId })
}

export async function workUpdateLastUsed(ids: number[]): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('work-updateLastUsed', { ids })
}
