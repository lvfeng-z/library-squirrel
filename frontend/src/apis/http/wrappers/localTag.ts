/**
 * LocalTag HTTP API 包装器
 * 提供与 window.api.localTag* 相同接口的 HTTP 版本
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

// ========== 类型定义 ==========

export interface LocalTagVO {
  id: number
  localTagName: string
  baseLocalTagId: number
  lastUse: number
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: LocalTagVO[]
  total: number
  page: number
  pageSize: number
}

// ========== API 方法 ==========

/**
 * 保存本地标签
 */
export async function localTagSave(tag: {
  localTagName?: string
  baseLocalTagId?: number
}): Promise<ApiResponse<LocalTagVO>> {
  return apiProxy.invoke<LocalTagVO>('localTag-save', tag)
}

/**
 * 删除本地标签
 */
export async function localTagDeleteById(id: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('localTag-deleteById', { id })
}

/**
 * 更新本地标签
 */
export async function localTagUpdateById(tag: {
  id: number
  localTagName?: string
  baseLocalTagId?: number
}): Promise<ApiResponse<LocalTagVO>> {
  return apiProxy.invoke<LocalTagVO>('localTag-updateById', tag)
}

/**
 * 获取单个本地标签
 */
export async function localTagGetById(id: number): Promise<ApiResponse<LocalTagVO>> {
  return apiProxy.invoke<LocalTagVO>('localTag-getById', id)
}

/**
 * 分页查询本地标签
 */
export async function localTagQueryPage(query: {
  page: number
  pageSize: number
  query?: {
    localTagName?: string
  }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('localTag-queryPage', query)
}

/**
 * 获取本地标签树
 */
export async function localTagGetTree(rootId?: number, depth?: number): Promise<ApiResponse<LocalTagVO[]>> {
  return apiProxy.invoke<LocalTagVO[]>('localTag-getTree', { rootId, depth })
}

/**
 * 获取选择项列表
 */
export async function localTagListSelectItems(
  query?: Record<string, unknown>
): Promise<ApiResponse<LocalTagVO[]>> {
  return apiProxy.invoke<LocalTagVO[]>('localTag-listSelectItems', query)
}

/**
 * 分页查询选择项
 */
export async function localTagQuerySelectItemPage(query: {
  page: number
  pageSize: number
  query?: Record<string, unknown>
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('localTag-querySelectItemPage', query)
}

/**
 * 根据作品ID获取标签列表
 */
export async function localTagListByWorkId(workId: number): Promise<ApiResponse<LocalTagVO[]>> {
  return apiProxy.invoke<LocalTagVO[]>('localTag-listByWorkId', workId)
}

/**
 * 根据作品ID分页查询选择项
 */
export async function localTagQuerySelectItemPageByWorkId(
  workId: number,
  query: { page: number; pageSize: number; query?: Record<string, unknown> }
): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('localTag-querySelectItemPageByWorkId', { workId, ...query })
}
