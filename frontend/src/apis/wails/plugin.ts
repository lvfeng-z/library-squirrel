/**
 * Plugin Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/plugin'
import type { ApiResponse } from '@apis/http'

// ========== API 方法 ==========

/**
 * 获取插件
 */
export async function pluginGetById(id: number): Promise<ApiResponse<any>> {
  return Handler.GetById(id)
}

/**
 * 根据公开ID获取插件
 */
export async function pluginGetByPublicId(publicId: string): Promise<ApiResponse<any>> {
  return Handler.GetByPublicId(publicId)
}

/**
 * 分页查询插件
 */
export async function pluginQueryPage(query: any): Promise<ApiResponse<any>> {
  return Handler.Page(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 检查插件是否已安装
 */
export async function pluginCheckInstalled(publicId: string): Promise<ApiResponse<boolean>> {
  return Handler.CheckInstalled(publicId)
}

/**
 * 从路径安装插件
 */
export async function pluginInstallFromPath(path: string): Promise<ApiResponse<any>> {
  return Handler.InstallFromPath(path, 0)
}

/**
 * 重新安装插件
 */
export async function pluginReinstall(publicId: string): Promise<ApiResponse<any>> {
  return Handler.Reinstall(publicId, 0)
}

/**
 * 卸载插件
 */
export async function pluginUninstall(publicId: string): Promise<ApiResponse<void>> {
  return Handler.Uninstall(publicId)
}

/**
 * 获取插件的Vue文件内容
 */
export async function pluginGetPluginVueFile(publicId: string, filePath: string): Promise<ApiResponse<string>> {
  return Handler.ReadVueFile(publicId, filePath)
}
