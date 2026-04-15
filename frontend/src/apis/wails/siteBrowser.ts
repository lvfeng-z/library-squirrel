/**
 * SiteBrowser Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'

// ========== API 方法 ==========

/**
 * 分页查询站点浏览器
 */
export async function siteBrowserQueryPage(): Promise<ApiResponse<any[]>> {
  return App.SiteBrowserQueryPage()
}

/**
 * 获取站点浏览器列表
 */
export async function siteBrowserList(): Promise<ApiResponse<any[]>> {
  return App.SiteBrowserList()
}

/**
 * 获取单个站点浏览器
 */
export async function siteBrowserGetById(pluginPublicId: string, contributionId: string): Promise<ApiResponse<any>> {
  return App.SiteBrowserGetById(pluginPublicId, contributionId)
}

/**
 * 根据插件公开ID获取站点浏览器
 */
export async function siteBrowserGetByPluginId(pluginId: number): Promise<ApiResponse<any[]>> {
  return App.SiteBrowserGetByPluginId(pluginId)
}

/**
 * 打开站点浏览器
 */
export async function siteBrowserOpen(pluginPublicId: string, contributionId: string): Promise<ApiResponse<any>> {
  return App.SiteBrowserOpen(pluginPublicId, contributionId)
}
