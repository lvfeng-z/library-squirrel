/**
 * SiteAuthor Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/siteAuthor'
import type { ApiResponse } from '@apis/http'

// ========== API 方法 ==========

/**
 * 保存站点作者
 */
export async function siteAuthorSave(author: any): Promise<ApiResponse<void>> {
  return Handler.Save(author)
}

/**
 * 批量保存站点作者
 */
export async function siteAuthorSaveBatch(authors: any[]): Promise<ApiResponse<void>> {
  return Handler.Save(authors)
}

/**
 * 删除站点作者
 */
export async function siteAuthorDeleteById(id: number): Promise<ApiResponse<void>> {
  return Handler.Delete(id)
}

/**
 * 更新站点作者
 */
export async function siteAuthorUpdateById(author: any): Promise<ApiResponse<void>> {
  return Handler.Update(author)
}

/**
 * 获取单个站点作者
 */
export async function siteAuthorGetById(id: number): Promise<ApiResponse<any>> {
  return Handler.GetById(id)
}

/**
 * 分页查询站点作者
 */
export async function siteAuthorQueryPage(query: any): Promise<ApiResponse<any>> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 查询已绑定或未绑定到本地作者的站点作者分页
 */
export async function siteAuthorQueryBoundOrUnboundInLocalAuthorPage(query: any): Promise<ApiResponse<any>> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 查询本地关联DTO分页
 */
export async function siteAuthorQueryLocalRelateDTOPage(query: any): Promise<ApiResponse<any>> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 根据作品ID获取站点作者列表
 */
export async function siteAuthorListByWorkId(workId: number): Promise<ApiResponse<any[]>> {
  return Handler.ListByWorkId(workId)
}

/**
 * 根据站点作者ID列表获取站点作者
 */
export async function siteAuthorListBySiteAuthorIds(ids: number[]): Promise<ApiResponse<any[]>> {
  const promises = ids.map(id => Handler.GetById(id))
  const results = await Promise.all(promises)
  return results as ApiResponse<any[]>
}

/**
 * 根据作品ID列表获取排序后的站点作者
 */
export async function siteAuthorListRankedSiteAuthorWithWorkIdByWorkIds(workIds: number[]): Promise<ApiResponse<any[]>> {
  const promises = workIds.map(workId => Handler.ListByWorkId(workId))
  const results = await Promise.all(promises)
  return results as ApiResponse<any[]>
}

/**
 * 更新绑定本地作者
 */
export async function siteAuthorUpdateBindLocalAuthor(localAuthorId: number, siteAuthorIds: number[]): Promise<ApiResponse<boolean>> {
  return Handler.Update({ localAuthorId, siteAuthorIds }) as ApiResponse<boolean>
}

/**
 * 创建并绑定同名的本地作者
 */
export async function siteAuthorCreateAndBindSameNameLocalAuthor(author: any): Promise<ApiResponse<boolean>> {
  return Handler.Save(author) as ApiResponse<boolean>
}
