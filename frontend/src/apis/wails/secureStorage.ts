/**
 * SecureStorage Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'
import { toApiResponse } from './index'

// ========== API 方法 ==========

/**
 * 设置安全存储值
 */
export async function secureStorageSet(key: string, value: string): Promise<ApiResponse<number>> {
  return toApiResponse(App.SecureStorageSet(key, value, ''))
}

/**
 * 获取安全存储值
 */
export async function secureStorageGet(key: string): Promise<ApiResponse<string | null>> {
  return toApiResponse(App.SecureStorageGetValue(key))
}

/**
 * 删除安全存储值
 */
export async function secureStorageRemove(key: string): Promise<ApiResponse<number>> {
  return toApiResponse(App.SecureStorageRemove(key))
}

/**
 * 检查安全存储键是否存在
 */
export async function secureStorageHasKey(key: string): Promise<ApiResponse<boolean>> {
  return toApiResponse(App.SecureStorageHasKey(key))
}

/**
 * 列出所有安全存储键
 */
export async function secureStorageListKeys(): Promise<ApiResponse<string[]>> {
  return toApiResponse(App.SecureStorageListKeys())
}
