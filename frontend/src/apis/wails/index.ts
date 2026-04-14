/**
 * Wails API 入口
 * 直接调用 Wails bindings，绕过 HTTP 代理层
 */

export * from './localTag'
export * from './localAuthor'
export * from './site'
export * from './siteTag'
export * from './siteAuthor'
export * from './work'
export * from './workSet'
export * from './search'
export * from './task'
export * from './taskManager'
export * from './plugin'
export * from './settings'
export * from './secureStorage'
export * from './appLauncher'
export * from './siteBrowser'
export * from './common'

import type { ApiResponse } from '@/apis/http'

/**
 * 将 Wails CancellablePromise 转换为 ApiResponse 格式
 */
export async function toApiResponse<T>(promise: Promise<T>): Promise<ApiResponse<T>> {
  try {
    const data = await promise
    return {
      success: true,
      msg: '',
      data
    }
  } catch (error) {
    return {
      success: false,
      msg: error instanceof Error ? error.message : 'Unknown error',
      data: undefined
    }
  }
}
