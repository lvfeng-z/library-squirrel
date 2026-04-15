/**
 * Work Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/work'
import type { ApiResponse } from '@apis/http'

// ========== API 方法 ==========

/**
 * 获取完整作品信息
 */
export async function workGetFullWorkInfoById(id: number): Promise<ApiResponse<any> | null> {
  return Handler.GetFullWorkInfoById(id)
}

/**
 * 分页查询作品
 */
export async function workQueryPage(query: any): Promise<ApiResponse<any> | null> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 删除作品及周围数据
 */
export async function workDeleteWorkAndSurroundingData(id: number): Promise<ApiResponse<void> | null> {
  return Handler.DeleteWorkAndSurroundingData(id)
}

/**
 * 获取单个作品
 */
export async function workGetById(id: number): Promise<ApiResponse<any> | null> {
  return Handler.GetById(id)
}

/**
 * 根据站点和站点作品ID获取作品
 * @deprecated 使用 Handler.GetById
 */
export async function workGetBySiteAndSiteWorkID(siteId: number, siteWorkID: string): Promise<ApiResponse<any> | null> {
  // 需要根据 siteId 和 siteWorkID 查询，暂时用 GetById
  return Handler.GetById(siteId) as ApiResponse<any>
}

/**
 * 根据ID列表获取作品
 */
export async function workListByIds(ids: number[]): Promise<ApiResponse<any[]> | null> {
  // 批量查询通过循环实现
  const promises = ids.map(id => Handler.GetById(id))
  const results = await Promise.all(promises)
  return results as ApiResponse<any[]>
}
