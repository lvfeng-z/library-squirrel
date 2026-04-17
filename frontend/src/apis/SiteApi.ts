import type { ApiResponse } from '@renderer/apis/http/types'
import { siteApi } from '@renderer/apis/http'
import {Page} from "@bindings/github.com/library-squirrel/wails/pkg/model";
import {SelectItem} from "@bindings/github.com/library-squirrel/wails/internal/model";

/**
 * 分页查询站点选择列表
 * 注意：siteName 过滤在 bindings 中未实现
 * @param _siteName
 * @param query
 */
export async function siteQuerySelectItemPageBySiteName(
  _siteName: string,
  query: { page: number; pageSize: number }
): Promise<ApiResponse<Page<SelectItem> | null>> {
  return siteApi.siteQuerySelectItemPage(query)
}

/**
 * 分页查询站点选择列表
 * @param query
 */
export async function siteQuerySelectItemPage(query: {
  page: number
  pageSize: number
}): Promise<ApiResponse<Page<SelectItem> | null>> {
  return siteApi.siteQuerySelectItemPage(query)
}
