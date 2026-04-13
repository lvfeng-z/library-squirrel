/**
 * SiteBrowser HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export interface SiteBrowserDTO {
  pluginPublicId: string
  contributionId: string
  name: string
  imagePath: string
  pluginId: number
}

export interface PageResult<T> {
  data: T[]
  pageNumber: number
  pageSize: number
  total: number
}

export async function siteBrowserQueryPage(query: {
  pageNumber?: number
  pageSize?: number
}): Promise<ApiResponse<PageResult<SiteBrowserDTO>>> {
  return apiProxy.invoke<PageResult<SiteBrowserDTO>>('siteBrowser-queryPage', query)
}

export async function siteBrowserOpen(pluginPublicId: string, contributionId: string): Promise<ApiResponse<void>> {
  return apiProxy.invoke<void>('siteBrowser-open', pluginPublicId, contributionId)
}