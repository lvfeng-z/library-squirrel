import type { ApiResponse } from '@renderer/apis/http/types'
import type { PageResult } from '@renderer/apis/http/wrappers/localAuthor'
import { localAuthorApi } from '@renderer/apis/http'

/**
 * 分页查询站点作者选择列表
 * @param authorName
 * @param query
 */
export async function localAuthorQuerySelectItemPageByName(
  authorName: string,
  query: { page: number; pageSize: number }
): Promise<ApiResponse<PageResult>> {
  return localAuthorApi.localAuthorQuerySelectItemPage({
    ...query,
    query: { authorName }
  })
}

/**
 * 分页查询站点作者选择列表
 * @param query
 */
export async function localAuthorQuerySelectItemPage(query: {
  page: number
  pageSize: number
  query?: Record<string, unknown>
}): Promise<ApiResponse<PageResult>> {
  return localAuthorApi.localAuthorQuerySelectItemPage(query)
}
