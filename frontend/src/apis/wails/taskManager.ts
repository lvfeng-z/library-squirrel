/**
 * TaskManager Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/taskManager'
import type { ApiResponse } from '@apis/http'

// ========== API 方法 ==========

/**
 * 启动任务树
 */
export async function taskManagerStartTree(taskId: number): Promise<ApiResponse<void>> {
  return Handler.StartTaskTree(taskId)
}

/**
 * 暂停任务树
 */
export async function taskManagerPauseTree(taskId: number): Promise<ApiResponse<void>> {
  return Handler.PauseTaskTree(taskId)
}

/**
 * 恢复任务树
 */
export async function taskManagerResumeTree(taskId: number): Promise<ApiResponse<void>> {
  return Handler.ResumeTaskTree(taskId)
}

/**
 * 停止任务树
 */
export async function taskManagerStopTree(taskId: number): Promise<ApiResponse<void>> {
  return Handler.StopTaskTree(taskId)
}

/**
 * 重试任务树
 */
export async function taskManagerRetryTree(taskId: number): Promise<ApiResponse<void>> {
  return Handler.RetryTaskTree(taskId)
}
