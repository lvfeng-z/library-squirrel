/**
 * ReWorkWorkSet HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export async function reWorkWorkSetLinkBatchToWorkSet(workSetId: number, workIds: number[]): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('reWorkWorkSet-linkBatchToWorkSet', workSetId, { workIds })
}

export async function reWorkWorkSetRemoveBatchFromWorkSet(workSetId: number, workIds: number[]): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('reWorkWorkSet-removeBatchFromWorkSet', workSetId, { workIds })
}

export async function reWorkWorkSetUpdateSortOrders(workSetId: number, workIds: number[]): Promise<ApiResponse<boolean>> {
  // workIds 是按顺序排列的作品ID数组，需要转换为 sortOrders map
  const sortOrders: Record<number, number> = {}
  workIds.forEach((workId, index) => {
    sortOrders[workId] = index
  })
  return apiProxy.invoke<boolean>('reWorkWorkSet-updateSortOrders', workSetId, { sortOrders })
}

export async function reWorkWorkSetSetCover(workSetId: number, workId: number): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('reWorkWorkSet-setCover', workSetId, workId)
}

export async function reWorkWorkSetUnsetCover(workSetId: number, workId: number): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('reWorkWorkSet-unsetCover', workSetId, workId)
}

export async function reWorkWorkSetGetCoverWorkId(workSetId: number): Promise<ApiResponse<number | null>> {
  return apiProxy.invoke<number | null>('reWorkWorkSet-getCoverWorkId', workSetId)
}
