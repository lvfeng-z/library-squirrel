/**
 * Site HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import {Handler as SiteHandler, SiteQueryDTO} from '@bindings/github.com/library-squirrel/wails/internal/site'
import {SelectItem, SiteDTO} from '@bindings/github.com/library-squirrel/wails/pkg/model/dto'
import { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'

export interface SiteVO {
  id: number
  name: string
  url: string
  enable: boolean
  createTime: number
  updateTime: number
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

export async function siteQueryPage(page: Page<SiteDTO, SiteQueryDTO>): Promise<ApiResponse<Page<SiteDTO, SiteQueryDTO> | null>> {
  const result = await SiteHandler.QueryPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return result
}

export async function siteQuerySelectItemPage(page: Page<SelectItem, SiteQueryDTO>): Promise<ApiResponse<Page<SelectItem, SiteQueryDTO> | null>> {
  const result = await SiteHandler.QuerySelectItemPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return result
}