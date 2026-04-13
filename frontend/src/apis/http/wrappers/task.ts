/**
 * Task HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export interface TaskVO {
  id: number
  pid: number
  name: string
  status: number
  schedule: number
  createTime: number
  updateTime: number
}

export interface TaskScheduleDTO {
  id: number
  pid: number
  status: number
  schedule: number
}

export interface PageResult {
  items: TaskVO[]
  total: number
  page: number
  pageSize: number
}

export async function taskGetById(id: number): Promise<ApiResponse<TaskVO>> {
  return apiProxy.invoke<TaskVO>('task-getById', id)
}

export async function taskQueryPage(query: {
  page: number
  pageSize: number
  query?: { status?: number; pid?: number }
}): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('task-queryPage', query)
}

export async function taskQueryParentPage(query: { page: number; pageSize: number }): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('task-queryParentPage', query)
}

export async function taskSave(task: { pid?: number; name?: string; status?: number }): Promise<ApiResponse<TaskVO>> {
  return apiProxy.invoke<TaskVO>('task-save', task)
}

export async function taskUpdate(task: {
  id: number
  name?: string
  status?: number
  schedule?: number
}): Promise<ApiResponse<TaskVO>> {
  return apiProxy.invoke<TaskVO>('task-update', task)
}

export async function taskDelete(taskId: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('task-delete', { taskId })
}

export async function taskRefreshStatus(id: number): Promise<ApiResponse<number>> {
  return apiProxy.invoke<number>('task-refreshStatus', { id })
}

export async function taskSetTreeStatus(taskIds: number[], status: number, includeStatus?: number[]): Promise<ApiResponse<number>> {
  return apiProxy.invoke<number>('task-setTreeStatus', { taskIds, status, includeStatus })
}

export async function taskListTree(taskIds: number[], includeStatus?: number[]): Promise<ApiResponse<TaskVO[]>> {
  return apiProxy.invoke<TaskVO[]>('task-listTree', { taskIds, includeStatus })
}

export async function taskListStatus(ids: number[]): Promise<ApiResponse<TaskScheduleDTO[]>> {
  return apiProxy.invoke<TaskScheduleDTO[]>('task-listStatus', { ids })
}

export async function taskQueryChildrenTaskPage(
  pid: number,
  pageNumber: number,
  pageSize: number,
  query?: Record<string, unknown>
): Promise<ApiResponse<PageResult>> {
  return apiProxy.invoke<PageResult>('task-queryChildrenTaskPage', { pid, pageNumber, pageSize, query })
}

// ========== TaskManager ==========

export interface TaskCreateRequest {
  pid?: number
  taskName?: string
  siteId?: number
  siteWorkId?: string
  url?: string
  isCollection?: number
  pluginPublicId?: string
  pluginContributionId?: string
  pluginData?: string
}

export interface TaskCreateResponse {
  id: number
}

export interface CreateTaskByUrlRequest {
  url: string
}

export interface CreateTaskByUrlResponse {
  succeed: boolean
  addedQuantity: number
  msg: string
}

export async function taskCreate(request: TaskCreateRequest): Promise<ApiResponse<TaskCreateResponse>> {
  return apiProxy.invoke<TaskCreateResponse>('task-create', request)
}

export async function taskCreateByUrl(url: string): Promise<ApiResponse<CreateTaskByUrlResponse>> {
  return apiProxy.invoke<CreateTaskByUrlResponse>('task-createByUrl', { url })
}

export async function taskStartTree(taskId: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('taskManager-startTree', { taskId })
}

export async function taskPauseTree(taskId: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('taskManager-pauseTree', { taskId })
}

export async function taskResumeTree(taskId: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('taskManager-resumeTree', { taskId })
}

export async function taskStopTree(taskId: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('taskManager-stopTree', { taskId })
}

export async function taskRetryTree(taskId: number): Promise<ApiResponse<null>> {
  return apiProxy.invoke<null>('taskManager-retryTree', { taskId })
}
