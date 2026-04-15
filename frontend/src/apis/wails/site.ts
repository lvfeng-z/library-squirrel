/**
 * Site Wails 绑定包装器
 */

import type { ApiResponse } from '@/apis/http'

// ========== API 方法 ==========

/**
 * 保存站点
 */
export async function siteSave(site: any): Promise<ApiResponse<number>> {
  return App.SiteSave(site)
}

/**
 * 删除站点
 */
export async function siteDeleteById(id: number): Promise<ApiResponse<void>> {
  return App.SiteDeleteById(id)
}

/**
 * 更新站点
 */
export async function siteUpdateById(site: any): Promise<ApiResponse<void>> {
  return App.SiteUpdateById(site)
}

/**
 * 获取单个站点
 */
export async function siteGetById(id: number): Promise<ApiResponse<any>> {
  return App.SiteGetById(id)
}

/**
 * 分页查询站点
 */
export async function siteQueryPage(query: any): Promise<ApiResponse<any>> {
  return App.SiteQueryPage(query)
}

/**
 * 分页查询选择项
 */
export async function siteQuerySelectItemPage(query: any): Promise<ApiResponse<any>> {
  return App.SiteQuerySelectItemPage(query)
}
