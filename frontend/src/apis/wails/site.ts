/**
 * Site Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'
import { toApiResponse } from './index'

// ========== API 方法 ==========

/**
 * 保存站点
 */
export async function siteSave(site: any): Promise<ApiResponse<number>> {
  return toApiResponse(App.SiteSave(site))
}

/**
 * 删除站点
 */
export async function siteDeleteById(id: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.SiteDeleteById(id))
}

/**
 * 更新站点
 */
export async function siteUpdateById(site: any): Promise<ApiResponse<void>> {
  return toApiResponse(App.SiteUpdateById(site))
}

/**
 * 获取单个站点
 */
export async function siteGetById(id: number): Promise<ApiResponse<any>> {
  return toApiResponse(App.SiteGetById(id))
}

/**
 * 分页查询站点
 */
export async function siteQueryPage(query: any): Promise<ApiResponse<any>> {
  return toApiResponse(App.SiteQueryPage(query))
}

/**
 * 分页查询选择项
 */
export async function siteQuerySelectItemPage(query: any): Promise<ApiResponse<any>> {
  return toApiResponse(App.SiteQuerySelectItemPage(query))
}
