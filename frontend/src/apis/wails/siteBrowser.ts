/**
 * SiteBrowser Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/siteBrowser'
import type { ApiResponse } from '@apis/http'

// ========== API 方法 ==========

/**
 * 分页查询站点浏览器
 */
export async function siteBrowserQueryPage(): Promise<ApiResponse<any[]>> {
  return Handler.List()
}

/**
 * 获取站点浏览器列表
 */
export async function siteBrowserList(): Promise<ApiResponse<any[]>> {
  return Handler.List()
}

/**
 * 获取单个站点浏览器
 */
export async function siteBrowserGetById(pluginPublicId: string, contributionId: string): Promise<ApiResponse<any>> {
  return Handler.GetByID(pluginPublicId, contributionId)
}

/**
 * 根据插件公开ID获取站点浏览器
 */
export async function siteBrowserGetByPluginId(pluginId: number): Promise<ApiResponse<any[]>> {
  return Handler.GetByPluginID(pluginId)
}

/**
 * 打开站点浏览器
 */
export async function siteBrowserOpen(pluginPublicId: string, contributionId: string): Promise<ApiResponse<any>> {
  return Handler.Open(pluginPublicId, contributionId)
}
