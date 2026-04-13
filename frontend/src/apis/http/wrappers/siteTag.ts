/**
 * SiteTag HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export interface SiteTagVO {
  id: number
  siteTagName: string
  localTagId: number
  lastUse: number
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: SiteTagVO[]
  total: number
  page: number
  pageSize: number
}

export async function siteTagSave(tag: {
  siteTagName?: string
  siteId?: number
}): Promise<ApiResponse<SiteTagVO>> {
  return apiProxy.invoke<SiteTagVO>('siteTag-save', tag)
}

export async function siteTagSaveBatch(tags: SiteTagVO[]): Promise<ApiResponse<SiteTagVO[]>> {
  return apiProxy.invoke<SiteTagVO[]>('siteTag-saveBatch', tags)
}

export async function siteTagDeleteById(id: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('siteTag-deleteById', { id })
}

export async function siteTagUpdateById(tag: {
  id: number
  siteTagName?: string
  localTagId?: number
}): Promise<ApiResponse<SiteTagVO>> {
  return apiProxy.invoke<SiteTagVO>('siteTag-updateById', tag)
}

export async function siteTagGetById(id: number): Promise<ApiResponse<SiteTagVO>> {
  return apiProxy.invoke<SiteTagVO>('siteTag-getById', id)
}

export async function siteTagQueryPage(query: {
  page: number
  pageSize: number
  query?: { siteId?: number; siteTagName?: string }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('siteTag-queryPage', query)
}

export async function siteTagQueryBoundOrUnboundToLocalTagPage(query: {
  page: number
  pageSize: number
  query?: { localTagId?: number; boundOnLocalTagId?: boolean }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('siteTag-queryBoundOrUnboundToLocalTagPage', query)
}

export async function siteTagQueryPageByWorkId(
  workId: number,
  query: { page: number; pageSize: number; query?: Record<string, unknown> }
): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('siteTag-queryPageByWorkId', { workId, ...query })
}

export async function siteTagQueryLocalRelateDTOPage(query: {
  page: number
  pageSize: number
  query?: { workId?: number }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('siteTag-queryLocalRelateDTOPage', query)
}

export async function siteTagQuerySelectItemPageByWorkId(
  workId: number,
  query: { page: number; pageSize: number }
): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('siteTag-querySelectItemPageByWorkId', { workId, ...query })
}

export async function siteTagListByWorkId(workId: number): Promise<ApiResponse<SiteTagVO[]>> {
  return apiProxy.invoke<SiteTagVO[]>('siteTag-listByWorkId', workId)
}

export async function siteTagListBySiteTagIds(siteTagIds: number[]): Promise<ApiResponse<SiteTagVO[]>> {
  return apiProxy.invoke<SiteTagVO[]>('siteTag-listBySiteTagIds', { siteTagIds })
}

export async function siteTagUpdateBindLocalTag(
  localTagId: number,
  siteTagIds: number[]
): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('siteTag-updateBindLocalTag', { localTagId, siteTagIds })
}

export async function siteTagCreateAndBindSameNameLocalTag(
  siteTag: SiteTagVO
): Promise<ApiResponse<SiteTagVO>> {
  return apiProxy.invoke<SiteTagVO>('siteTag-createAndBindSameNameLocalTag', siteTag)
}
