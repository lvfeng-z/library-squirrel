/**
 * Task HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as TaskHandler } from '@bindings/github.com/library-squirrel/wails/internal/task'
import { Handler as TaskManagerHandler } from '@bindings/github.com/library-squirrel/wails/internal/taskManager'
import { CreateTaskRequest, TaskQueryDTO, TaskResultDTO, TaskScheduleDTO } from '@bindings/github.com/library-squirrel/wails/internal/task/models'
import type { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'

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
}): Promise<ApiResponse<Page<TaskResultDTO>>> {
  const queryDTO = new TaskQueryDTO({
    pid: query.query?.pid ?? null,
    status: query.query?.status ?? null
  })
  const result = await TaskHandler.QueryPage(query.page, query.pageSize, queryDTO)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return result
}

export async function taskQueryParentPage(query: { page: number; pageSize: number }): Promise<ApiResponse<Page<TaskResultDTO>>> {
  const result = await TaskHandler.QueryParentPage(query.page, query.pageSize, null)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return result
}

/**
 * 保存任务
 * 注意：此方法在 bindings 中未实现
 */
export async function taskSave(_task: { pid?: number; name?: string; status?: number }): Promise<ApiResponse<TaskVO>> {
  // TODO: 此接口在 bindings 中未实现 (Save)
  return { success: false, msg: '此接口未实现：taskSave' }
}

/**
 * 更新任务
 * 注意：此方法在 bindings 中未实现
 */
export async function taskUpdate(_task: {
  id: number
  name?: string
  status?: number
  schedule?: number
}): Promise<ApiResponse<TaskVO>> {
  // TODO: 此接口在 bindings 中未实现 (Update)
  return { success: false, msg: '此接口未实现：taskUpdate' }
}

export async function taskDelete(taskId: number): Promise<ApiResponse<null>> {
  const result = await TaskHandler.DeleteTask(taskId)
  if (!result) {
    return { success: false, msg: '删除失败：接口返回为空' }
  }
  return result
}

/**
 * 刷新任务状态
 * 注意：此方法在 bindings 中未实现
 */
export async function taskRefreshStatus(_id: number): Promise<ApiResponse<number>> {
  // TODO: 此接口在 bindings 中未实现 (RefreshStatus)
  return { success: false, msg: '此接口未实现：taskRefreshStatus' }
}

/**
 * 设置任务树状态
 * 注意：此方法在 bindings 中未实现
 */
export async function taskSetTreeStatus(
  _taskIds: number[],
  _status: number,
  _includeStatus?: number[]
): Promise<ApiResponse<number>> {
  // TODO: 此接口在 bindings 中未实现 (SetTreeStatus)
  return { success: false, msg: '此接口未实现：taskSetTreeStatus' }
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
): Promise<ApiResponse<Page<TaskResultDTO>>> {
  const result = await TaskHandler.QueryChildrenTaskPage(pid, pageNumber, pageSize, null)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return result
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