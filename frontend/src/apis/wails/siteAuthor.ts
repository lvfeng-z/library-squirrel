/**
 * SiteAuthor Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'
import { toApiResponse } from './index'

// ========== API 方法 ==========

/**
 * 保存站点作者
 */
export async function siteAuthorSave(author: any): Promise<ApiResponse<void>> {
  return toApiResponse(App.SiteAuthorSave(author))
}

/**
 * 批量保存站点作者
 */
export async function siteAuthorSaveBatch(authors: any[]): Promise<ApiResponse<void>> {
  return toApiResponse(App.SiteAuthorSaveBatch(authors))
}

/**
 * 删除站点作者
 */
export async function siteAuthorDeleteById(id: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.SiteAuthorDeleteById(id))
}

/**
 * 更新站点作者
 */
export async function siteAuthorUpdateById(author: any): Promise<ApiResponse<void>> {
  return toApiResponse(App.SiteAuthorUpdateById(author))
}

/**
 * 获取单个站点作者
 */
export async function siteAuthorGetById(id: number): Promise<ApiResponse<any>> {
  return toApiResponse(App.SiteAuthorGetById(id))
}

/**
 * 分页查询站点作者
 */
export async function siteAuthorQueryPage(query: any): Promise<ApiResponse<any>> {
  return toApiResponse(App.SiteAuthorQueryPage(query))
}

/**
 * 查询已绑定或未绑定到本地作者的站点作者分页
 */
export async function siteAuthorQueryBoundOrUnboundInLocalAuthorPage(query: any): Promise<ApiResponse<any>> {
  return toApiResponse(App.SiteAuthorQueryBoundOrUnboundInLocalAuthorPage(query))
}

/**
 * 查询本地关联DTO分页
 */
export async function siteAuthorQueryLocalRelateDTOPage(query: any): Promise<ApiResponse<any>> {
  return toApiResponse(App.SiteAuthorQueryLocalRelateDTOPage(query))
}

/**
 * 根据作品ID获取站点作者列表
 */
export async function siteAuthorListByWorkId(workId: number): Promise<ApiResponse<any[]>> {
  return toApiResponse(App.SiteAuthorListByWorkId(workId))
}

/**
 * 根据站点作者ID列表获取站点作者
 */
export async function siteAuthorListBySiteAuthorIds(ids: number[]): Promise<ApiResponse<any[]>> {
  return toApiResponse(App.SiteAuthorListBySiteAuthorIds(ids))
}

/**
 * 根据作品ID列表获取排序后的站点作者
 */
export async function siteAuthorListRankedSiteAuthorWithWorkIdByWorkIds(workIds: number[]): Promise<ApiResponse<any[]>> {
  return toApiResponse(App.SiteAuthorListRankedSiteAuthorWithWorkIdByWorkIds(workIds))
}

/**
 * 更新绑定本地作者
 */
export async function siteAuthorUpdateBindLocalAuthor(localAuthorId: number, siteAuthorIds: number[]): Promise<ApiResponse<boolean>> {
  return toApiResponse(App.SiteAuthorUpdateBindLocalAuthor(localAuthorId, siteAuthorIds))
}

/**
 * 创建并绑定同名的本地作者
 */
export async function siteAuthorCreateAndBindSameNameLocalAuthor(author: any): Promise<ApiResponse<boolean>> {
  return toApiResponse(App.SiteAuthorCreateAndBindSameNameLocalAuthor(author))
}
