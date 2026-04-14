/**
 * TaskManager Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'
import { toApiResponse } from './index'

// ========== API 方法 ==========

/**
 * 启动任务树
 */
export async function taskManagerStartTree(taskId: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskManagerStartTree(taskId))
}

/**
 * 暂停任务树
 */
export async function taskManagerPauseTree(taskId: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskManagerPauseTree(taskId))
}

/**
 * 恢复任务树
 */
export async function taskManagerResumeTree(taskId: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskManagerResumeTree(taskId))
}

/**
 * 停止任务树
 */
export async function taskManagerStopTree(taskId: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskManagerStopTree(taskId))
}

/**
 * 重试任务树
 */
export async function taskManagerRetryTree(taskId: number): Promise<ApiResponse<void>> {
  return toApiResponse(App.TaskManagerRetryTree(taskId))
}
