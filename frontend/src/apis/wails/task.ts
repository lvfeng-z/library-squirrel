/**
 * Task Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/task'
import { Handler as TaskManagerHandler } from '@bindings/github.com/library-squirrel/wails/internal/taskManager'
import type { ApiResponse } from '@apis/http'

// ========== API 方法 ==========

/**
 * 获取任务
 */
export async function taskGetById(id: number): Promise<ApiResponse<any> | null> {
  return Handler.GetById(id)
}

/**
 * 分页查询任务
 */
export async function taskQueryPage(query: any): Promise<ApiResponse<any> | null> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 分页查询父任务
 */
export async function taskQueryParentPage(query: any): Promise<ApiResponse<any> | null> {
  return Handler.QueryParentPage(query.page ?? 1, query.pageSize ?? 10, query.query)
}

/**
 * 保存任务
 */
export async function taskSave(task: any): Promise<ApiResponse<void> | null> {
  return Handler.CreateTask(task)
}

/**
 * 更新任务
 */
export async function taskUpdate(task: any): Promise<ApiResponse<void> | null> {
  return Handler.CreateTask(task) // Update 和 Create 使用相同的 request 结构
}

/**
 * 删除任务
 */
export async function taskDelete(id: number): Promise<ApiResponse<void> | null> {
  return Handler.DeleteTask(id)
}

/**
 * 刷新任务状态
 */
export async function taskRefreshStatus(id: number): Promise<ApiResponse<any> | null> {
  return TaskManagerHandler.GetTaskState(id)
}

/**
 * 获取任务树
 */
export async function taskListTree(taskIds: number[]): Promise<ApiResponse<any[]> | null> {
  return Handler.ListTaskTree(taskIds)
}

/**
 * 获取任务状态列表
 */
export async function taskListStatus(ids: number[]): Promise<ApiResponse<any[]> | null> {
  return Handler.ListStatus(ids)
}

/**
 * 获取任务进度列表
 */
export async function taskListSchedule(ids: number[]): Promise<ApiResponse<any[]> | null> {
  return Handler.ListSchedule(ids)
}

/**
 * 创建任务
 */
export async function taskCreate(task: any): Promise<ApiResponse<any> | null> {
  return Handler.CreateTask(task)
}

/**
 * 通过URL创建任务
 */
export async function taskCreateByUrl(url: string): Promise<ApiResponse<any> | null> {
  return Handler.CreateTaskByURL(url)
}

/**
 * 分页查询子任务
 */
export async function taskQueryChildrenTaskPage(
  pid: number,
  pageNumber: number,
  pageSize: number,
  query?: Record<string, unknown>
): Promise<ApiResponse<any> | null> {
  return Handler.QueryChildrenTaskPage(pid, pageNumber, pageSize, query)
}

// ========== TaskManager 方法别名 (保持与 HTTP API 兼容) ==========

/**
 * 启动任务树
 */
export async function taskStartTree(taskId: number): Promise<ApiResponse<void> | null> {
  return TaskManagerHandler.StartTaskTree(taskId)
}

/**
 * 暂停任务树
 */
export async function taskPauseTree(taskId: number): Promise<ApiResponse<void> | null> {
  return TaskManagerHandler.PauseTaskTree(taskId)
}

/**
 * 恢复任务树
 */
export async function taskResumeTree(taskId: number): Promise<ApiResponse<void> | null> {
  return TaskManagerHandler.ResumeTaskTree(taskId)
}

/**
 * 停止任务树
 */
export async function taskStopTree(taskId: number): Promise<ApiResponse<void> | null> {
  return TaskManagerHandler.StopTaskTree(taskId)
}

/**
 * 重试任务树
 */
export async function taskRetryTree(taskId: number): Promise<ApiResponse<void> | null> {
  return TaskManagerHandler.RetryTaskTree(taskId)
}
