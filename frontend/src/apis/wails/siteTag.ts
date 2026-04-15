/**
 * SiteTag Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/siteTag'
import type { ApiResponse } from '@apis/http'

// ========== API 方法 ==========

/**
 * 保存站点标签
 */
export async function siteTagSave(tag: any): Promise<ApiResponse<void>> {
  return Handler.Save(tag)
}

/**
 * 批量保存站点标签
 */
export async function siteTagSaveBatch(tags: any[]): Promise<ApiResponse<void>> {
  return Handler.Save(tags)
}

/**
 * 删除站点标签
 */
export async function siteTagDeleteById(id: number): Promise<ApiResponse<void>> {
  return Handler.Delete(id)
}

/**
 * 更新站点标签
 */
export async function siteTagUpdateById(tag: any): Promise<ApiResponse<void>> {
  return Handler.Update(tag)
}

/**
 * 获取单个站点标签
 */
export async function siteTagGetById(id: number): Promise<ApiResponse<any>> {
  return Handler.GetById(id)
}

/**
 * 分页查询站点标签
 */
export async function siteTagQueryPage(query: any): Promise<ApiResponse<any>> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 查询已绑定或未绑定到本地标签的站点标签分页
 */
export async function siteTagQueryBoundOrUnboundToLocalTagPage(query: any): Promise<ApiResponse<any>> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 根据作品ID查询站点标签分页
 */
export async function siteTagQueryPageByWorkId(query: any, workId: number, boundOnWorkId: boolean | null): Promise<ApiResponse<any>> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 查询本地关联DTO分页
 */
export async function siteTagQueryLocalRelateDTOPage(query: any, workId: number, boundOnWorkId: boolean | null): Promise<ApiResponse<any>> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 根据作品ID分页查询选择项
 */
export async function siteTagQuerySelectItemPageByWorkId(
  workId: number,
  query: any
): Promise<ApiResponse<any>> {
  return Handler.QuerySelectItemPageByWorkId(query.page ?? 1, query.pageSize ?? 10, query.query, workId)
}

/**
 * 根据作品ID获取站点标签列表
 */
export async function siteTagListByWorkId(workId: number): Promise<ApiResponse<any[]>> {
  return Handler.ListByWorkId(workId)
}

/**
 * 根据站点标签ID列表获取站点标签
 */
export async function siteTagListBySiteTagIds(ids: number[]): Promise<ApiResponse<any[]>> {
  const promises = ids.map(id => Handler.GetById(id))
  const results = await Promise.all(promises)
  return results as ApiResponse<any[]>
}

/**
 * 更新绑定本地标签
 */
export async function siteTagUpdateBindLocalTag(localTagId: number, siteTagIds: number[]): Promise<ApiResponse<boolean>> {
  return Handler.Update({ localTagId, siteTagIds }) as ApiResponse<boolean>
}

/**
 * 创建并绑定同名的本地标签
 */
export async function siteTagCreateAndBindSameNameLocalTag(tag: any): Promise<ApiResponse<any>> {
  return Handler.Save(tag)
}
