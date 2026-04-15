/**
 * Site Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/site/index.ts'
import type { ApiResponse } from '@apis/http'

// ========== API 方法 ==========

/**
 * 保存站点
 */
export async function siteSave(site: any): Promise<ApiResponse<number> | null> {
  return Handler.Save(site)
}

/**
 * 删除站点
 */
export async function siteDeleteById(id: number): Promise<ApiResponse<void> | null> {
  return Handler.Delete(id)
}

/**
 * 更新站点
 */
export async function siteUpdateById(site: any): Promise<ApiResponse<void> | null> {
  return Handler.Update(site)
}

/**
 * 获取单个站点
 */
export async function siteGetById(id: number): Promise<ApiResponse<any> | null> {
  return Handler.GetById(id)
}

/**
 * 分页查询站点
 */
export async function siteQueryPage(query: any): Promise<ApiResponse<any> | null> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 分页查询选择项
 */
export async function siteQuerySelectItemPage(query: any): Promise<ApiResponse<any> | null> {
  return Handler.QuerySelectItemPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}
