/**
 * Plugin Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'

// ========== API 方法 ==========

/**
 * 获取插件
 */
export async function pluginGetById(id: number): Promise<ApiResponse<any>> {
  return App.PluginGetById(id)
}

/**
 * 根据公开ID获取插件
 */
export async function pluginGetByPublicId(publicId: string): Promise<ApiResponse<any>> {
  return App.PluginGetByPublicId(publicId)
}

/**
 * 分页查询插件
 */
export async function pluginQueryPage(query: any): Promise<ApiResponse<any>> {
  return App.PluginQueryPage(query)
}

/**
 * 检查插件是否已安装
 */
export async function pluginCheckInstalled(publicId: string): Promise<ApiResponse<boolean>> {
  return App.PluginCheckInstalled(publicId)
}

/**
 * 保存插件
 */
export async function pluginSave(plugin: any): Promise<ApiResponse<void>> {
  return App.PluginSave(plugin)
}

/**
 * 更新插件
 */
export async function pluginUpdate(plugin: any): Promise<ApiResponse<void>> {
  return App.PluginUpdate(plugin)
}

/**
 * 删除插件
 */
export async function pluginDelete(id: number): Promise<ApiResponse<void>> {
  return App.PluginDelete(id)
}

/**
 * 从路径安装插件
 */
export async function pluginInstallFromPath(path: string): Promise<ApiResponse<any>> {
  return App.PluginInstallFromPath(path)
}

/**
 * 重新安装插件
 */
export async function pluginReinstall(publicId: string): Promise<ApiResponse<any>> {
  return App.PluginReinstall(publicId)
}

/**
 * 从路径重新安装插件
 */
export async function pluginReinstallFromPath(publicId: string, path: string): Promise<ApiResponse<any>> {
  return App.PluginReinstallFromPath(publicId, path)
}

/**
 * 卸载插件
 */
export async function pluginUninstall(publicId: string): Promise<ApiResponse<void>> {
  return App.PluginUninstall(publicId)
}

/**
 * 获取插件的Vue文件内容
 */
export async function pluginGetPluginVueFile(publicId: string, filePath: string): Promise<ApiResponse<string>> {
  return App.GetPluginVueFile(publicId, filePath)
}
