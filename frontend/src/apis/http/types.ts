/**
 * HTTP API 类型定义
 * 用于渲染进程与 Go 后端的 HTTP 通信
 */

// Go 后端基础 URL
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

// IPC Channel 到 HTTP Route 的映射类型
export interface IpcToHttpRoute {
  method: HttpMethod
  path: string
}

// 路由映射表类型
export type IpcRouteMapping = Record<string, IpcToHttpRoute>
