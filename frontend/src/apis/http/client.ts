/**
 * HTTP 客户端封装
 * 基于 LocalTagHttpApi.ts 中的 httpGet/httpPost 模式重构
 */

import { GO_BACKEND_URL, type ApiResponse, type RequestConfig } from './types'

/**
 * 编码嵌套对象为 URLSearchParams
 * 用于处理 where, orderBy 等复杂查询条件
 */
function encodeNestedParams(params: Record<string, unknown>): URLSearchParams {
  const searchParams = new URLSearchParams()

  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null) {
      continue
    }

    if (Array.isArray(value) || typeof value === 'object') {
      // 数组和对象 JSON 序列化
      searchParams.append(key, JSON.stringify(value))
    } else {
      searchParams.append(key, String(value))
    }
  }

  return searchParams
}

/**
 * 构建完整 URL
 */
function buildUrl(baseUrl: string, path: string, params?: Record<string, unknown>): string {
  let url = `${baseUrl}${path}`

  if (params && Object.keys(params).length > 0) {
    const searchParams = encodeNestedParams(params)
    const query = searchParams.toString()
    if (query) {
      url += `?${query}`
    }
  }

  return url
}

/**
 * HTTP 客户端类
 */
class HttpClient {
  private baseUrl: string
  private defaultHeaders: Record<string, string>

  constructor(baseUrl: string = GO_BACKEND_URL) {
    this.baseUrl = baseUrl
    this.defaultHeaders = {
      'Content-Type': 'application/json'
    }
  }

  /**
   * 通用请求方法
   */
  async request<T>(config: RequestConfig): Promise<ApiResponse<T>> {
    const { method, path, params, body, headers } = config
    const url = buildUrl(this.baseUrl, path, method === 'GET' ? params : undefined)

    const requestOptions: RequestInit = {
      method,
      headers: {
        ...this.defaultHeaders,
        ...headers
      }
    }

    if (body && method !== 'GET') {
      requestOptions.body = JSON.stringify(body)
    }

    try {
      const response = await fetch(url, requestOptions)
      const data: ApiResponse<T> = await response.json()
      return data
    } catch (error) {
      // 网络错误处理
      return {
        success: false,
        msg: error instanceof Error ? error.message : 'Network error',
        data: undefined
      }
    }
  }

  /**
   * GET 请求
   */
  async get<T>(path: string, params?: Record<string, unknown>): Promise<ApiResponse<T>> {
    return this.request<T>({ method: 'GET', path, params })
  }

  /**
   * POST 请求
   */
  async post<T>(path: string, body?: unknown): Promise<ApiResponse<T>> {
    return this.request<T>({ method: 'POST', path, body })
  }

  /**
   * PUT 请求
   */
  async put<T>(path: string, body?: unknown): Promise<ApiResponse<T>> {
    return this.request<T>({ method: 'PUT', path, body })
  }

  /**
   * DELETE 请求
   */
  async delete<T>(path: string, params?: Record<string, unknown>): Promise<ApiResponse<T>> {
    return this.request<T>({ method: 'DELETE', path, params })
  }

  /**
   * 设置基础 URL
   */
  setBaseUrl(baseUrl: string): void {
    this.baseUrl = baseUrl
  }

  /**
   * 获取基础 URL
   */
  getBaseUrl(): string {
    return this.baseUrl
  }
}

// 导出单例
export const httpClient = new HttpClient()
export default HttpClient
