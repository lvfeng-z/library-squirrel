/**
 * HTTP API 类型定义
 * 用于渲染进程与 Go 后端的 HTTP 通信
 */

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