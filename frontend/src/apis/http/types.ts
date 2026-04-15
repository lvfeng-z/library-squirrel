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

// 请求配置接口
export interface RequestConfig {
  method: HttpMethod
  path: string
  params?: Record<string, unknown> // URL query 参数
  body?: unknown // JSON body
  headers?: Record<string, string>
}