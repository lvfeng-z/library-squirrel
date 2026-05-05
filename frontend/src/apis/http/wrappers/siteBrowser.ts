/**
 * SiteBrowser HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as SiteBrowserHandler } from '@bindings/github.com/library-squirrel/wails/backend/siteBrowser'
import type { SiteBrowserDTO as BindingSiteBrowserDTO } from '@bindings/github.com/library-squirrel/wails/backend/siteBrowser/models'

export interface SiteBrowserDTO {
  pluginPublicId: string
  contributionId: string
  name: string
  pluginId: number
}

export interface PageResult {
  data: SiteBrowserDTO[]
  pageNumber: number
  pageSize: number
  total: number
}

// ========== 工具函数 ==========

/**
 * 将 Binding 的 SiteBrowserDTO 转换为我们的格式
 */
function toSiteBrowserDTO(dto: BindingSiteBrowserDTO | null): SiteBrowserDTO | null {
  if (!dto) return null
  return {
    pluginPublicId: dto.pluginPublicId ?? '',
    contributionId: dto.contributionId ?? '',
    name: dto.name ?? '',
    pluginId: dto.pluginId ?? 0
  }
}

// ========== API 方法 ==========

/**
 * 分页查询站点浏览器
 */
export async function siteBrowserQueryPage(query: {
  pageNumber?: number
  pageSize?: number
}): Promise<ApiResponse<PageResult>> {
  const result = await SiteBrowserHandler.QueryPage(query.pageNumber ?? 1, query.pageSize ?? 10)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return {
    success: true,
    msg: result.msg ?? '',
    data: result.data ? {
      data: result.data.data.map(toSiteBrowserDTO).filter((item): item is SiteBrowserDTO => item !== null),
      pageNumber: result.data.pageNumber,
      pageSize: result.data.pageSize,
      total: result.data.total
    } : undefined
  }
}

export async function siteBrowserOpen(pluginPublicId: string, contributionId: string): Promise<ApiResponse<void>> {
  const result = await SiteBrowserHandler.Open(pluginPublicId, contributionId)
  if (!result) {
    return { success: false, msg: '打开失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}