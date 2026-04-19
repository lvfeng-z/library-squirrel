/**
 * Plugin HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as PluginHandler, PluginQueryDTO, PluginResultDTO } from '@bindings/github.com/library-squirrel/wails/internal/plugin'
import type { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'

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

// ========== 工具函数 ==========

/**
 * 将 PluginResultDTO 转换为 PluginVO
 */
function toPluginVO(dto: PluginResultDTO | null): PluginVO | null {
  if (!dto) return null
  return {
    id: dto.id,
    publicId: dto.publicId ?? '',
    name: dto.name ?? '',
    version: dto.version ?? '',
    author: dto.author ?? '',
    enable: dto.uninstalled === 0,  // 0=未卸载，启用
    createTime: dto.createTime,
    updateTime: dto.updateTime
  }
}

// ========== API 方法 ==========

export async function pluginGetById(id: number): Promise<ApiResponse<PluginVO>> {
  const result = await PluginHandler.GetById(id)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toPluginVO(result.data ?? null) ?? undefined }
}

export async function pluginGetByPublicId(publicId: string): Promise<ApiResponse<PluginVO>> {
  const result = await PluginHandler.GetByPublicId(publicId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toPluginVO(result.data ?? null) ?? undefined }
}

export async function pluginQueryPage(query: {
  page: number
  pageSize: number
  query?: { name?: string; enable?: boolean }
}): Promise<ApiResponse<Page<PluginResultDTO>>> {
  const queryDTO = new PluginQueryDTO({
    nameLike: query.query?.name ?? null
  })
  const result = await PluginHandler.Page(query.page, query.pageSize, queryDTO)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return result
}

export async function pluginCheckInstalled(publicId: string): Promise<ApiResponse<boolean>> {
  const result = await PluginHandler.CheckInstalled(publicId)
  if (!result) {
    return { success: false, msg: '检查失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '检查失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data }
}

/**
 * 保存插件
 */
export async function pluginSave(plugin: {
  publicId?: string
  name?: string
  version?: string
  author?: string
  entryPath?: string
  rootPath?: string
  activationType?: string
}): Promise<ApiResponse<PluginVO>> {
  const result = await PluginHandler.Save({
    publicId: plugin.publicId,
    author: plugin.author,
    name: plugin.name,
    version: plugin.version,
    entryPath: plugin.entryPath,
    rootPath: plugin.rootPath,
    activationType: plugin.activationType
  })
  if (!result) {
    return { success: false, msg: '保存失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '保存失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toPluginVO(result.data ?? null) ?? undefined }
}

/**
 * 更新插件
 */
export async function pluginUpdate(plugin: {
  id: number
  publicId?: string
  name?: string
  version?: string
  author?: string
  entryPath?: string
  rootPath?: string
  activationType?: string
}): Promise<ApiResponse<PluginVO>> {
  const result = await PluginHandler.Update({
    id: plugin.id,
    publicId: plugin.publicId,
    author: plugin.author,
    name: plugin.name,
    version: plugin.version,
    entryPath: plugin.entryPath,
    rootPath: plugin.rootPath,
    activationType: plugin.activationType
  })
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 删除插件
 * 注意：bindings 中使用 SetUninstalled 而非直接删除
 */
export async function pluginDelete(id: number): Promise<ApiResponse<null>> {
  const result = await PluginHandler.SetUninstalled(id)
  if (!result) {
    return { success: false, msg: '删除失败：接口返回为空' }
  }
  return result
}

export async function pluginInstallFromPath(packagePath: string, installType?: number): Promise<ApiResponse<PluginVO>> {
  const result = await PluginHandler.InstallFromPath(packagePath, installType ?? 0)
  if (!result) {
    return { success: false, msg: '安装失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '安装失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toPluginVO(result.data ?? null) ?? undefined }
}

export async function pluginReinstall(publicId: string, installType?: number): Promise<ApiResponse<PluginVO>> {
  const result = await PluginHandler.Reinstall(publicId, installType ?? 0)
  if (!result) {
    return { success: false, msg: '重新安装失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '重新安装失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toPluginVO(result.data ?? null) ?? undefined }
}

/**
 * 从路径重新安装插件
 */
export async function pluginReinstallFromPath(
  publicId: string,
  packagePath: string,
  installType?: number
): Promise<ApiResponse<PluginVO>> {
  const result = await PluginHandler.ReinstallFromPath(publicId, packagePath, installType ?? 0)
  if (!result) {
    return { success: false, msg: '重新安装失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '重新安装失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toPluginVO(result.data ?? null) ?? undefined }
}

export async function pluginUnInstall(publicId: string): Promise<ApiResponse<null>> {
  const result = await PluginHandler.Uninstall(publicId)
  if (!result) {
    return { success: false, msg: '卸载失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}