/**
 * Site HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as SiteHandler, SiteDTO, SiteQueryDTO, SiteResultDTO } from '@bindings/github.com/library-squirrel/wails/internal/site'
import type { SelectItem } from '@bindings/github.com/library-squirrel/wails/internal/model/models'
import type { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'

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

// ========== 工具函数 ==========

/**
 * 将 SiteResultDTO 转换为 SiteVO
 */
function toSiteVO(dto: SiteResultDTO | null): SiteVO | null {
  if (!dto) return null
  return {
    id: dto.id,
    name: dto.siteName ?? '',
    url: dto.homepage ?? '',
    enable: true,  // SiteResultDTO 没有 enable 字段，默认 true
    createTime: dto.createTime,
    updateTime: dto.updateTime
  }
}

// ========== API 方法 ==========

export async function siteSave(site: { name?: string; url?: string; enable?: boolean }): Promise<ApiResponse<SiteVO>> {
  const siteDTO = new SiteDTO({
    siteName: site.name ?? null,
    homepage: site.url ?? null
  })
  const result = await SiteHandler.Save(siteDTO)
  if (!result) {
    return { success: false, msg: '保存失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '保存失败' }
  }
  return { success: true, msg: result.msg ?? '', data: { id: result.data ?? 0, name: '', url: '', enable: true, createTime: 0, updateTime: 0 } }
}

export async function siteDeleteById(id: number): Promise<ApiResponse<null>> {
  const result = await SiteHandler.Delete(id)
  if (!result) {
    return { success: false, msg: '删除失败：接口返回为空' }
  }
  return result
}

export async function siteUpdateById(site: {
  id: number
  name?: string
  url?: string
  enable?: boolean
}): Promise<ApiResponse<SiteVO>> {
  const siteDTO = new SiteDTO({
    id: site.id,
    siteName: site.name ?? null,
    homepage: site.url ?? null
  })
  const result = await SiteHandler.Update(siteDTO)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  return result
}

export async function siteGetById(id: number): Promise<ApiResponse<SiteVO>> {
  const result = await SiteHandler.GetById(id)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toSiteVO(result.data ?? null) ?? undefined }
}

export async function siteQueryPage(query: {
  page: number
  pageSize: number
  query?: { name?: string; enable?: boolean }
}): Promise<ApiResponse<Page<SiteResultDTO>>> {
  const queryDTO = new SiteQueryDTO({
    siteNameLike: query.query?.name ?? null
  })
  const result = await SiteHandler.QueryPage(query.page, query.pageSize, queryDTO)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return result
}

export async function siteQuerySelectItemPage(query: { page: number; pageSize: number }): Promise<ApiResponse<Page<SelectItem>>> {
  const queryDTO = new SiteQueryDTO({})
  const result = await SiteHandler.QuerySelectItemPage(query.page, query.pageSize, queryDTO)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return result
}

/**
 * 根据站点ID和站点作品ID获取站点
 * 注意：此方法在 bindings 中未实现
 */
export async function siteGetBySiteAndSiteWorkID(_siteId: number, _siteWorkId: string): Promise<ApiResponse<SiteVO>> {
  // TODO: 此接口在 bindings 中未实现 (GetBySiteAndSiteWorkID)
  return { success: false, msg: '此接口未实现：siteGetBySiteAndSiteWorkID' }
}

/**
 * 根据作品集ID和站点名称获取站点
 * 注意：此方法在 bindings 中未实现
 */
export async function siteGetBySiteWorkSetIdAndSiteName(_siteWorkSetId: string, _siteName: string): Promise<ApiResponse<SiteVO>> {
  // TODO: 此接口在 bindings 中未实现 (GetBySiteWorkSetIdAndSiteName)
  return { success: false, msg: '此接口未实现：siteGetBySiteWorkSetIdAndSiteName' }
}