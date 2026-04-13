/**
 * SiteAuthor HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export interface SiteAuthorVO {
  id: number
  authorName: string
  introduce: string
  localAuthorId: number
  lastUse: number
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: SiteAuthorVO[]
  total: number
  page: number
  pageSize: number
}

export async function siteAuthorSave(author: {
  authorName?: string
  introduce?: string
  siteId?: number
}): Promise<ApiResponse<SiteAuthorVO>> {
  return apiProxy.invoke<SiteAuthorVO>('siteAuthor-save', author)
}

export async function siteAuthorSaveBatch(authors: SiteAuthorVO[]): Promise<ApiResponse<SiteAuthorVO[]>> {
  return apiProxy.invoke<SiteAuthorVO[]>('siteAuthor-saveBatch', authors)
}

export async function siteAuthorDeleteById(id: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('siteAuthor-deleteById', { id })
}

export async function siteAuthorUpdateById(author: {
  id: number
  authorName?: string
  introduce?: string
  localAuthorId?: number
}): Promise<ApiResponse<SiteAuthorVO>> {
  return apiProxy.invoke<SiteAuthorVO>('siteAuthor-updateById', author)
}

export async function siteAuthorGetById(id: number): Promise<ApiResponse<SiteAuthorVO>> {
  return apiProxy.invoke<SiteAuthorVO>('siteAuthor-getById', id)
}

export async function siteAuthorQueryPage(query: {
  page: number
  pageSize: number
  query?: { siteId?: number; authorName?: string }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('siteAuthor-queryPage', query)
}

export async function siteAuthorQueryBoundOrUnboundInLocalAuthorPage(query: {
  page: number
  pageSize: number
  query?: { localAuthorId?: number; boundOnLocalAuthorId?: boolean }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('siteAuthor-queryBoundOrUnboundInLocalAuthorPage', query)
}

export async function siteAuthorQueryLocalRelateDTOPage(query: {
  page: number
  pageSize: number
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('siteAuthor-queryLocalRelateDTOPage', query)
}

export async function siteAuthorListByWorkId(workId: number): Promise<ApiResponse<SiteAuthorVO[]>> {
  return apiProxy.invoke<SiteAuthorVO[]>('siteAuthor-listByWorkId', workId)
}

export async function siteAuthorListBySiteAuthorIds(siteAuthorIds: number[]): Promise<ApiResponse<SiteAuthorVO[]>> {
  return apiProxy.invoke<SiteAuthorVO[]>('siteAuthor-listBySiteAuthorIds', { siteAuthorIds })
}

export async function siteAuthorListRankedSiteAuthorWithWorkIdByWorkIds(
  workIds: number[]
): Promise<ApiResponse<SiteAuthorVO[]>> {
  return apiProxy.invoke<SiteAuthorVO[]>('siteAuthor-listRankedSiteAuthorWithWorkIdByWorkIds', { workIds })
}

export async function siteAuthorUpdateBindLocalAuthor(
  localAuthorId: number,
  siteAuthorIds: number[]
): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('siteAuthor-updateBindLocalAuthor', { localAuthorId, siteAuthorIds })
}

export async function siteAuthorCreateAndBindSameNameLocalAuthor(
  siteAuthor: SiteAuthorVO
): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('siteAuthor-createAndBindSameNameLocalAuthor', siteAuthor)
}
