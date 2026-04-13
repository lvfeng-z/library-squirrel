/**
 * Plugin HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export interface PluginVO {
  id: number
  publicId: string
  name: string
  version: string
  author: string
  enable: boolean
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: PluginVO[]
  total: number
  page: number
  pageSize: number
}

export async function pluginGetById(id: number): Promise<ApiResponse<PluginVO>> {
  return apiProxy.invoke<PluginVO>('plugin-getById', id)
}

export async function pluginGetByPublicId(publicId: string): Promise<ApiResponse<PluginVO>> {
  return apiProxy.invoke<PluginVO>('plugin-getByPublicId', publicId)
}

export async function pluginQueryPage(query: {
  page: number
  pageSize: number
  query?: { name?: string; enable?: boolean }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('plugin-queryPage', query)
}

export async function pluginCheckInstalled(publicId: string): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('plugin-checkInstalled', publicId)
}

export async function pluginSave(plugin: {
  publicId?: string
  name?: string
  version?: string
  author?: string
  enable?: boolean
}): Promise<ApiResponse<PluginVO>> {
  return apiProxy.invoke<PluginVO>('plugin-save', plugin)
}

export async function pluginUpdate(plugin: {
  id: number
  name?: string
  version?: string
  enable?: boolean
}): Promise<ApiResponse<PluginVO>> {
  return apiProxy.invoke<PluginVO>('plugin-update', plugin)
}

export async function pluginDelete(id: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('plugin-delete', { id })
}

export async function pluginInstallFromPath(packagePath: string, installType?: number): Promise<ApiResponse<PluginVO>> {
  return apiProxy.invoke<PluginVO>('plugin-installFromPath', { packagePath, installType })
}

export async function pluginReinstall(publicId: string, installType?: number): Promise<ApiResponse<PluginVO>> {
  return apiProxy.invoke<PluginVO>('plugin-reinstall', publicId, { installType })
}

export async function pluginReinstallFromPath(
  publicId: string,
  packagePath: string,
  installType?: number
): Promise<ApiResponse<PluginVO>> {
  return apiProxy.invoke<PluginVO>('plugin-reinstallFromPath', publicId, { packagePath, installType })
}

export async function pluginUnInstall(publicId: string): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('plugin-uninstall', publicId)
}
