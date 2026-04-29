/**
 * HTTP API 类型定义与响应校验工具
 * 用于渲染进程与 Go 后端的通信
 */

import type { ApiResponse as WailsApiResponse } from '@bindings/github.com/library-squirrel/wails/pkg/model'
import { isNullish } from '@renderer/utils/CommonUtil.ts'

// Go 后端基础 URL（保留但不再用于 HTTP 请求）
export const GO_BACKEND_URL = 'http://localhost:8080'

// HTTP 方法类型
export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE'

// API 响应格式（与前端 ApiResponse.ts 一致）
export interface ApiResponse<T = unknown> {
  success: boolean
  msg: string
  data?: T
}

// Wrapper 校验后的响应类型，data 保证非空
// requireResponse 校验通过后返回此类型，调用方无需再做 data 非空判断
export interface ApiResult<T = unknown> {
  readonly success: true
  readonly msg: string
  readonly data: T
}

// 请求配置接口
export interface RequestConfig {
  method: HttpMethod
  path: string
  params?: Record<string, unknown> // URL query 参数
  body?: unknown // JSON body
  headers?: Record<string, string>
}

// ========== 响应校验工具 ==========

/**
 * 校验 Wails 绑定层响应，将 ApiResponse<T | null> | null 转换为 ApiResult<T>
 *
 * 校验步骤：
 * 1. 外层 null 检查（响应为空）
 * 2. success 检查（后端返回失败）
 * 3. data 非空检查（当 requireData=true 时）
 *
 * @param response 原始响应
 * @param operation 操作描述，用于错误信息提示，例如 "查询用户"、"保存数据" 等
 * @param requireData 是否校验 data 非空。查询类接口传 true，变更类接口（Save/Update/Delete）传 false
 * 校验失败抛出 Error，调用方通过 try/catch 捕获
 */
export function requireResponse<T>(
  response: WailsApiResponse<T | null> | null,
  operation: string,
  requireData = true
): ApiResult<T> {
  if (!response) throw new Error(`${operation}：接口返回为空`)
  if (!response.success) throw new Error(response.msg || `${operation}：操作失败`)
  if (requireData && isNullish(response.data)) throw new Error(`${operation}：未返回数据`)
  return response as unknown as ApiResult<T>
}