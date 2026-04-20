/**
 * Task HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as TaskHandler } from '@bindings/github.com/library-squirrel/wails/internal/task'
import { Handler as TaskManagerHandler } from '@bindings/github.com/library-squirrel/wails/internal/taskManager'
import { CreateTaskRequest, TaskQueryDTO, TaskResultDTO, TaskScheduleDTO } from '@bindings/github.com/library-squirrel/wails/internal/task/models'
import type { QueryAttribute } from '@bindings/github.com/library-squirrel/wails/pkg/query/models'
import { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'

export interface TaskVO {
  id: number
  pid: number
  name: string
  status: number
  schedule: number
  createTime: number
  updateTime: number
}

export interface TaskScheduleVO {
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

// ========== 工具函数 ==========

/**
 * 将 TaskResultDTO 转换为 TaskVO
 */
function toTaskVO(dto: TaskResultDTO | null): TaskVO | null {
  if (!dto) return null
  return {
    id: dto.id,
    pid: dto.pid ?? 0,
    name: dto.taskName ?? '',
    status: dto.status ?? 0,
    schedule: 0,
    createTime: dto.createTime,
    updateTime: dto.updateTime
  }
}

// ========== API 方法 ==========

export async function taskGetById(id: number): Promise<ApiResponse<TaskVO>> {
  const result = await TaskHandler.GetById(id)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toTaskVO(result.data ?? null) ?? undefined }
}

export async function taskQueryPage(query: {
  page: number
  pageSize: number
  query?: { status?: number; pid?: number }
}): Promise<ApiResponse<Page<TaskResultDTO, TaskQueryDTO>>> {
  const queryDTO = new TaskQueryDTO({
    pid: { value: query.query?.pid } as QueryAttribute,
    status: { value: query.query?.status } as QueryAttribute
  })
  const page = new Page<TaskResultDTO, TaskQueryDTO>({
    pageNumber: query.page,
    pageSize: query.pageSize,
    query: queryDTO
  })
  const result = await TaskHandler.QueryPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

export async function taskQueryParentPage(query: { page: number; pageSize: number }): Promise<ApiResponse<Page<TaskResultDTO, TaskQueryDTO>>> {
  const page = new Page<TaskResultDTO, TaskQueryDTO>({
    pageNumber: query.page,
    pageSize: query.pageSize
  })
  const result = await TaskHandler.QueryParentPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 保存任务
 */
export async function taskSave(task: { pid?: number; name?: string; status?: number; isCollection?: number }): Promise<ApiResponse<TaskVO>> {
  const taskDTO = new (await import('@bindings/github.com/library-squirrel/wails/internal/task/models')).TaskDTO({
    pid: task.pid ?? null,
    taskName: task.name ?? null,
    status: task.status ?? 0,
    isCollection: task.isCollection ?? null
  })
  const result = await TaskHandler.Save(taskDTO)
  if (!result) {
    return { success: false, msg: '保存失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '保存失败' }
  }
  return { success: true, msg: result.msg ?? '', data: { id: result.data ?? 0, pid: task.pid ?? 0, name: task.name ?? '', status: task.status ?? 0, schedule: 0, createTime: 0, updateTime: 0 } }
}

/**
 * 更新任务
 */
export async function taskUpdate(task: {
  id: number
  name?: string
  status?: number
  schedule?: number
}): Promise<ApiResponse<TaskVO>> {
  const taskDTO = new (await import('@bindings/github.com/library-squirrel/wails/internal/task/models')).TaskDTO({
    id: task.id,
    taskName: task.name ?? null,
    status: task.status ?? 0
  })
  const result = await TaskHandler.Update(taskDTO)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function taskDelete(taskId: number): Promise<ApiResponse<null>> {
  const result = await TaskHandler.DeleteTask([taskId])
  if (!result) {
    return { success: false, msg: '删除失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 刷新任务状态
 */
export async function taskRefreshStatus(id: number): Promise<ApiResponse<number>> {
  const result = await TaskHandler.RefreshStatus(id)
  if (!result) {
    return { success: false, msg: '刷新失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '刷新失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data }
}

/**
 * 设置任务树状态
 */
export async function taskSetTreeStatus(
  taskIds: number[],
  status: number,
  includeStatus?: number[]
): Promise<ApiResponse<number>> {
  const result = await TaskHandler.SetTreeStatus(taskIds, status, includeStatus ?? [])
  if (!result) {
    return { success: false, msg: '设置失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '设置失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data }
}

export async function taskListTree(taskIds: number[], _includeStatus?: number[]): Promise<ApiResponse<TaskVO[]>> {
  const result = await TaskHandler.ListTaskTree(taskIds)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? result.data.map(toTaskVO).filter((item): item is TaskVO => item !== null) : [] }
}

export async function taskListStatus(ids: number[]): Promise<ApiResponse<TaskScheduleVO[]>> {
  const result = await TaskHandler.ListStatus(ids)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? result.data.map(item => item ? {
    id: item.id,
    pid: item.pid ?? 0,
    status: item.status ?? 0,
    schedule: item.schedule ?? 0
  } : null).filter((item): item is TaskScheduleVO => item !== null) : [] }
}

export async function taskQueryChildrenTaskPage(
  pid: number,
  pageNumber: number,
  pageSize: number,
  _query?: Record<string, unknown>
): Promise<ApiResponse<Page<TaskResultDTO, TaskQueryDTO>>> {
  const page = new Page<TaskResultDTO, TaskQueryDTO>({
    pageNumber: pageNumber,
    pageSize: pageSize
  })
  const result = await TaskHandler.QueryChildrenTaskPage(pid, page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
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
  const req = new CreateTaskRequest({
    pid: request.pid ?? 0,
    taskName: request.taskName ?? '',
    siteId: request.siteId ?? 0,
    siteWorkId: request.siteWorkId ?? '',
    url: request.url ?? '',
    isCollection: request.isCollection ?? 0,
    pluginPublicId: request.pluginPublicId ?? '',
    pluginContributionId: request.pluginContributionId ?? '',
    pluginData: request.pluginData ?? ''
  })
  const result = await TaskHandler.CreateTask(req)
  if (!result) {
    return { success: false, msg: '创建失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '创建失败' }
  }
  return { success: true, msg: result.msg ?? '', data: { id: result.data ?? 0 } }
}

export async function taskCreateByUrl(url: string): Promise<ApiResponse<CreateTaskByUrlResponse>> {
  const result = await TaskHandler.CreateTaskByURL(url)
  if (!result) {
    return { success: false, msg: '创建失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '创建失败' }
  }
  return {
    success: true,
    msg: result.msg ?? '',
    data: {
      succeed: result.data?.succeed ?? false,
      addedQuantity: result.data?.addedQuantity ?? 0,
      msg: result.data?.msg ?? ''
    }
  }
}

export async function taskStartTree(taskId: number): Promise<ApiResponse<null>> {
  const result = await TaskManagerHandler.StartTaskTree(taskId)
  if (!result) {
    return { success: false, msg: '启动失败：接口返回为空' }
  }
  return result
}

export async function taskPauseTree(taskId: number): Promise<ApiResponse<null>> {
  const result = await TaskManagerHandler.PauseTaskTree(taskId)
  if (!result) {
    return { success: false, msg: '暂停失败：接口返回为空' }
  }
  return result
}

export async function taskResumeTree(taskId: number): Promise<ApiResponse<null>> {
  const result = await TaskManagerHandler.ResumeTaskTree(taskId)
  if (!result) {
    return { success: false, msg: '恢复失败：接口返回为空' }
  }
  return result
}

export async function taskStopTree(taskId: number): Promise<ApiResponse<null>> {
  const result = await TaskManagerHandler.StopTaskTree(taskId)
  if (!result) {
    return { success: false, msg: '停止失败：接口返回为空' }
  }
  return result
}

export async function taskRetryTree(taskId: number): Promise<ApiResponse<null>> {
  const result = await TaskManagerHandler.RetryTaskTree(taskId)
  if (!result) {
    return { success: false, msg: '重试失败：接口返回为空' }
  }
  return result
}