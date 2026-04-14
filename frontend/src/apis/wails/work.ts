/**
 * Work Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'
import { toApiResponse } from './index'

// ========== API 方法 ==========

/**
 * 获取完整作品信息
 */
export async function workGetFullWorkInfoById(id: number): Promise<ApiResponse<any>> {
  return toApiResponse(App.WorkGetFullWorkInfoById(id))
}

/**
 * 分页查询作品
 */
export async function workQueryPage(query: any): Promise<ApiResponse<any>> {
  return toApiResponse(App.WorkQueryPage(query))
}

/**
 * 删除作品及周围数据
 */
export async function workDeleteWorkAndSurroundingData(id: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.WorkDeleteWorkAndSurroundingData(id))
}

/**
 * 获取单个作品
 */
export async function workGetById(id: number): Promise<ApiResponse<any>> {
  return toApiResponse(App.WorkGetById(id))
}

/**
 * 根据站点和站点作品ID获取作品
 */
export async function workGetBySiteAndSiteWorkID(siteId: number, siteWorkID: string): Promise<ApiResponse<any>> {
  return toApiResponse(App.WorkGetBySiteAndSiteWorkID(siteId, siteWorkID))
}

/**
 * 根据ID列表获取作品
 */
export async function workListByIds(ids: number[]): Promise<ApiResponse<any[]>> {
  return toApiResponse(App.WorkListByIds(ids))
}
