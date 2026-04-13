/**
 * Site HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export interface SiteVO {
  id: number
  name: string
  url: string
  enable: boolean
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: SiteVO[]
  total: number
  page: number
  pageSize: number
}

export async function siteSave(site: { name?: string; url?: string; enable?: boolean }): Promise<ApiResponse<SiteVO>> {
  return apiProxy.invoke<SiteVO>('site-save', site)
}

export async function siteDeleteById(id: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('site-deleteById', { id })
}

export async function siteUpdateById(site: {
  id: number
  name?: string
  url?: string
  enable?: boolean
}): Promise<ApiResponse<SiteVO>> {
  return apiProxy.invoke<SiteVO>('site-updateById', site)
}

export async function siteGetById(id: number): Promise<ApiResponse<SiteVO>> {
  return apiProxy.invoke<SiteVO>('site-getById', id)
}

export async function siteQueryPage(query: {
  page: number
  pageSize: number
  query?: { name?: string; enable?: boolean }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('site-queryPage', query)
}

export async function siteQuerySelectItemPage(query: { page: number; pageSize: number }): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('site-querySelectItemPage', query)
}

export async function siteGetBySiteAndSiteWorkID(siteId: number, siteWorkId: string): Promise<ApiResponse<SiteVO>> {
  return apiProxy.invoke<SiteVO>('site-getBySiteAndSiteWorkID', { siteId, siteWorkId })
}

export async function siteGetBySiteWorkSetIdAndSiteName(siteWorkSetId: string, siteName: string): Promise<ApiResponse<SiteVO>> {
  return apiProxy.invoke<SiteVO>('site-getBySiteWorkSetIdAndSiteName', { siteWorkSetId, siteName })
}
