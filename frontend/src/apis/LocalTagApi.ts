import type { ApiResponse } from '@renderer/apis/http/types'
import type { PageResult } from '@renderer/apis/http/wrappers/localTag'
import { localTagApi } from '@renderer/apis/http'

/**
 * 分页查询本地标签选择列表
 * @param query
 */
export async function localTagQuerySelectItemPage(query: {
  page: number
  pageSize: number
  query?: Record<string, unknown>
}): Promise<ApiResponse<PageResult>> {
  return localTagApi.localTagQuerySelectItemPage(query)
}

/**
 * 分页查询本地标签选择列表
 * @param localTagName
 * @param query
 */
export async function localTagQuerySelectItemPageByName(
  localTagName: string,
  query: { page: number; pageSize: number }
): Promise<ApiResponse<PageResult>> {
  return localTagApi.localTagQuerySelectItemPage({
    ...query,
    query: { localTagName }
  })
}
