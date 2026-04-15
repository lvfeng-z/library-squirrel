/**
 * WorkSet Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'

// ========== API 方法 ==========

/**
 * 保存作品集
 */
export async function workSetSave(workSet: any): Promise<ApiResponse<void>> {
  return App.WorkSetSave(workSet)
}

/**
 * 更新作品集
 */
export async function workSetUpdate(workSet: any): Promise<ApiResponse<void>> {
  return App.WorkSetUpdate(workSet)
}

/**
 * 删除作品集
 */
export async function workSetDelete(id: number): Promise<ApiResponse<void>> {
  return App.WorkSetDelete(id)
}

/**
 * 获取单个作品集
 */
export async function workSetGetById(id: number): Promise<ApiResponse<any>> {
  return App.WorkSetGetById(id)
}

/**
 * 分页查询作品集
 */
export async function workSetQueryPage(query: any): Promise<ApiResponse<any>> {
  return App.WorkSetQueryPage(query)
}

/**
 * 分页查询作品集（带封面）
 */
export async function workSetQueryPageWithCover(query: any): Promise<ApiResponse<any>> {
  return App.WorkSetQueryPageWithCover(query)
}

/**
 * 获取作品集中的作品列表
 */
export async function workSetGetWorks(id: number): Promise<ApiResponse<any[]>> {
  return App.WorkSetGetWorks(id)
}

/**
 * 批量关联作品到作品集
 */
export async function workSetLinkBatch(id: number, workIds: number[]): Promise<ApiResponse<void>> {
  return App.WorkSetLinkBatch(id, workIds)
}

/**
 * 批量从作品集移除作品
 */
export async function workSetRemoveBatch(id: number, workIds: number[]): Promise<ApiResponse<void>> {
  return App.WorkSetRemoveBatch(id, workIds)
}

/**
 * 设置作品集封面
 */
export async function workSetSetCover(id: number, workId: number): Promise<ApiResponse<void>> {
  return App.WorkSetSetCover(id, workId)
}

/**
 * 取消设置作品集封面
 */
export async function workSetUnsetCover(id: number, workId: number): Promise<ApiResponse<void>> {
  return App.WorkSetUnsetCover(id, workId)
}

/**
 * 获取作品集封面作品ID
 */
export async function workSetGetCoverWorkId(id: number): Promise<ApiResponse<number>> {
  return App.WorkSetGetCoverWorkId(id)
}

/**
 * 根据ID列表获取作品集及作品
 */
export async function workSetListWorkSetWithWorkByIds(ids: number[]): Promise<ApiResponse<any[]>> {
  return App.WorkSetListWorkSetWithWorkByIds(ids)
}
