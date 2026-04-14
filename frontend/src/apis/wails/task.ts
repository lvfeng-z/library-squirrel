/**
 * Task Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'
import { toApiResponse } from './index'

// ========== API 方法 ==========

/**
 * 获取任务
 */
export async function taskGetById(id: number): Promise<ApiResponse<any>> {
  return toApiResponse(App.TaskGetById(id))
}

/**
 * 分页查询任务
 */
export async function taskQueryPage(query: any): Promise<ApiResponse<any>> {
  return toApiResponse(App.TaskQueryPage(query))
}

/**
 * 分页查询父任务
 */
export async function taskQueryParentPage(query: any): Promise<ApiResponse<any>> {
  return toApiResponse(App.TaskQueryParentPage(query))
}

/**
 * 保存任务
 */
export async function taskSave(task: any): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskSave(task))
}

/**
 * 更新任务
 */
export async function taskUpdate(task: any): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskUpdate(task))
}

/**
 * 删除任务
 */
export async function taskDelete(id: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskDelete(id))
}

/**
 * 刷新任务状态
 */
export async function taskRefreshStatus(id: number): Promise<ApiResponse<any>> {
  return toApiResponse(App.TaskRefreshStatus(id))
}

/**
 * 获取任务树
 */
export async function taskListTree(taskIds: number[]): Promise<ApiResponse<any[]>> {
  return toApiResponse(App.TaskListTree(taskIds))
}

/**
 * 获取任务状态列表
 */
export async function taskListStatus(ids: number[]): Promise<ApiResponse<any[]>> {
  return toApiResponse(App.TaskListStatus(ids))
}

/**
 * 获取任务进度列表
 */
export async function taskListSchedule(ids: number[]): Promise<ApiResponse<any[]>> {
  return toApiResponse(App.TaskListSchedule(ids))
}

/**
 * 创建任务
 */
export async function taskCreate(task: any): Promise<ApiResponse<any>> {
  return toApiResponse(App.TaskCreate(task))
}

/**
 * 通过URL创建任务
 */
export async function taskCreateByUrl(url: string): Promise<ApiResponse<any>> {
  return toApiResponse(App.TaskCreateByURL(url))
}

/**
 * 分页查询子任务
 * @deprecated 使用 Wails 原生签名: taskQueryChildrenTaskPage(pid, query)
 */
export async function taskQueryChildrenTaskPage(
  pid: number,
  pageNumber: number,
  pageSize: number,
  query?: Record<string, unknown>
): Promise<ApiResponse<any>> {
  // 适配 HTTP API 签名，转换为 Wails 签名
  const wailsQuery: any = { pageNumber, pageSize, ...query }
  return toApiResponse(App.TaskQueryChildrenTaskPage(pid, wailsQuery))
}

// ========== TaskManager 方法别名 (保持与 HTTP API 兼容) ==========

/**
 * 启动任务树
 */
export async function taskStartTree(taskId: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskManagerStartTree(taskId))
}

/**
 * 暂停任务树
 */
export async function taskPauseTree(taskId: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskManagerPauseTree(taskId))
}

/**
 * 恢复任务树
 */
export async function taskResumeTree(taskId: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskManagerResumeTree(taskId))
}

/**
 * 停止任务树
 */
export async function taskStopTree(taskId: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskManagerStopTree(taskId))
}

/**
 * 重试任务树
 */
export async function taskRetryTree(taskId: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskManagerRetryTree(taskId))
}
