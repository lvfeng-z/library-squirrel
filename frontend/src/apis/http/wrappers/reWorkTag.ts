/**
 * ReWorkTag HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export interface ReWorkTagVO {
  id: number
  workId: number
  tagType: number
  localTagId: number
  siteTagId: number
}

export async function reWorkTagLink(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('reWorkTag-link', { workId, type: tagType, tagIds })
}

export async function reWorkTagLinkBatch(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('reWorkTag-linkBatch', { workId, tagType, tagIds })
}

export async function reWorkTagUnlink(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('reWorkTag-unlink', { workId, type: tagType, tagIds })
}

export async function reWorkTagRemoveBatch(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('reWorkTag-removeBatch', { workId, tagType, tagIds })
}

export async function reWorkTagList(workId: number): Promise<ApiResponse<ReWorkTagVO[]>> {
  return apiProxy.invoke<ReWorkTagVO[]>('reWorkTag-list', workId)
}
