/**
 * SiteBrowser HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as SiteBrowserHandler } from '@bindings/github.com/library-squirrel/wails/internal/siteBrowser'
import type { SiteBrowserDTO as BindingSiteBrowserDTO } from '@bindings/github.com/library-squirrel/wails/internal/siteBrowser/models'

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
 * 注意：此方法在 bindings 中未实现
 */
export async function siteBrowserQueryPage(_query: {
  pageNumber?: number
  pageSize?: number
}): Promise<ApiResponse<PageResult>> {
  // TODO: 此接口在 bindings 中未实现 (QueryPage)
  // bindings 中有 List、GetByPluginID、GetByID 等方法，但没有 QueryPage
  return { success: false, msg: '此接口未实现：siteBrowserQueryPage' }
}

export async function siteBrowserOpen(pluginPublicId: string, contributionId: string): Promise<ApiResponse<void>> {
  const result = await SiteBrowserHandler.Open(pluginPublicId, contributionId)
  if (!result) {
    return { success: false, msg: '打开失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}