/**
 * WorkSet HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export interface WorkSetVO {
  id: number
  name: string
  coverId: number
  createTime: number
  updateTime: number
}

export interface WorkSetWithWorksVO {
  workSet: WorkSetVO
  works: WorkSetVO[]
}

export interface WorkSetWithCoverVO {
  workSet: WorkSetVO
  coverWork?: WorkSetVO
}

export interface PageResult {
  items: WorkSetVO[]
  total: number
  page: number
  pageSize: number
}

export async function workSetListWorkSetWithWorkByIds(
  workSetIds: number[]
): Promise<ApiResponse<WorkSetWithWorksVO[]>> {
  return apiProxy.invoke<WorkSetWithWorksVO[]>('workSet-listWorkSetWithWorkByIds', { workSetIds })
}

export async function workSetQueryPageWithCover(query: {
  page: number
  pageSize: number
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('workSet-queryPageWithCover', query)
}

export async function workSetGetById(id: number): Promise<ApiResponse<WorkSetVO>> {
  return apiProxy.invoke<WorkSetVO>('workSet-getById', id)
}

export async function workSetQueryPage(query: {
  page: number
  pageSize: number
  query?: { name?: string }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('workSet-queryPage', query)
}

export async function workSetSave(workSet: {
  name?: string
  coverId?: number
}): Promise<ApiResponse<WorkSetVO>> {
  return apiProxy.invoke<WorkSetVO>('workSet-save', workSet)
}

export async function workSetUpdate(workSet: {
  id: number
  name?: string
  coverId?: number
}): Promise<ApiResponse<WorkSetVO>> {
  return apiProxy.invoke<WorkSetVO>('workSet-update', workSet)
}

export async function workSetDelete(id: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('workSet-delete', { id })
}
