/**
 * API 代理核心类
 * 将 IPC channel 调用转换为 HTTP 请求到 Go 后端
 */

import { httpClient } from './client'
import { routeMapping, hasChannel } from './routes'
import type { ApiResponse } from './types'

/**
 * API 代理类
 */
class ApiProxy {
  /**
   * 调用 API 方法
   * @param channel IPC channel 名称
   * @param args 参数
   * @returns API 响应
   */
  async invoke<T = unknown>(channel: string, ...args: unknown[]): Promise<ApiResponse<T>> {
    const route = routeMapping[channel]

    if (!route) {
      return {
        success: false,
        msg: `Unknown IPC channel: ${channel}`,
        data: undefined
      }
    }

    const { method, path } = route

    try {
      switch (method) {
        case 'GET':
          return httpClient.get<T>(this.buildPath(path, args), this.buildQueryParams(args))
        case 'POST':
          return httpClient.post<T>(this.buildPath(path, args), this.buildBody(args))
        case 'PUT':
          return httpClient.put<T>(this.buildPath(path, args), this.buildBody(args))
        case 'DELETE':
          return httpClient.delete<T>(this.buildPath(path, args))
        default:
          return {
            success: false,
            msg: `Unsupported HTTP method: ${method}`,
            data: undefined
          }
      }
    } catch (error) {
      return {
        success: false,
        msg: error instanceof Error ? error.message : 'Unknown error',
        data: undefined
      }
    }
  }

  /**
   * 检查 channel 是否已映射
   */
  isMapped(channel: string): boolean {
    return hasChannel(channel)
  }

  /**
   * 获取所有已映射的 channel
   */
  getMappedChannels(): string[] {
    return Object.keys(routeMapping)
  }

  /**
   * 构建路径
   * 处理路径参数，如 /api/localTag/:id
   */
  private buildPath(basePath: string, args: unknown[]): string {
    let path = basePath

    // 处理 :id 路径参数
    if (path.includes(':id') && args.length > 0) {
      const id = this.extractId(args[0])
      if (id !== null) {
        path = path.replace(':id', String(id))
      }
    }

    // 处理 :workId 路径参数
    if (path.includes(':workId') && args.length > 0) {
      const workId = this.extractId(args[0])
      if (workId !== null) {
        path = path.replace(':workId', String(workId))
      }
    }

    // 处理 :workSetId 路径参数
    if (path.includes(':workSetId') && args.length > 0) {
      const workSetId = this.extractId(args[0])
      if (workSetId !== null) {
        path = path.replace(':workSetId', String(workSetId))
      }
    }

    // 处理 :pluginPublicId 路径参数（字符串类型）
    if (path.includes(':pluginPublicId') && args.length > 0) {
      const pluginPublicId = args[0]
      if (typeof pluginPublicId === 'string') {
        path = path.replace(':pluginPublicId', encodeURIComponent(pluginPublicId))
      }
    }

    // 处理 :contributionId 路径参数（字符串类型）
    if (path.includes(':contributionId') && args.length > 0) {
      // contributionId 在第二个参数位置
      const contributionId = args.length > 1 ? args[1] : null
      if (typeof contributionId === 'string') {
        path = path.replace(':contributionId', encodeURIComponent(contributionId))
      }
    }

    // 处理 :publicId 路径参数（字符串类型）
    if (path.includes(':publicId') && args.length > 0) {
      const publicId = args[0]
      if (typeof publicId === 'string') {
        path = path.replace(':publicId', encodeURIComponent(publicId))
      }
    }

    return path
  }

  /**
   * 从参数中提取 ID
   */
  private extractId(arg: unknown): number | null {
    if (typeof arg === 'number') {
      return arg
    }
    if (typeof arg === 'string') {
      const parsed = parseInt(arg, 10)
      return isNaN(parsed) ? null : parsed
    }
    if (typeof arg === 'object' && arg !== null && 'id' in arg) {
      const id = (arg as Record<string, unknown>).id
      if (typeof id === 'number') {
        return id
      }
      if (typeof id === 'string') {
        const parsed = parseInt(id, 10)
        return isNaN(parsed) ? null : parsed
      }
    }
    return null
  }

  /**
   * 构建查询参数
   */
  private buildQueryParams(args: unknown[]): Record<string, unknown> | undefined {
    if (args.length === 0) return undefined

    const firstArg = args[0]
    if (typeof firstArg === 'object' && firstArg !== null) {
      // 分页查询参数
      if ('pageNumber' in (firstArg as Record<string, unknown>) || 'pageSize' in (firstArg as Record<string, unknown>)) {
        const page = firstArg as Record<string, unknown>
        const params: Record<string, unknown> = {
          pageNumber: page.pageNumber,
          pageSize: page.pageSize
        }
        // 添加其他查询参数（keyword, siteId, tagType, tagId 等）
        for (const key of Object.keys(page)) {
          if (key !== 'pageNumber' && key !== 'pageSize' && key !== 'query') {
            params[key] = page[key]
          }
        }
        if (page.query) {
          params.query = JSON.stringify(page.query)
        }
        return params
      }
      // 普通对象参数
      return firstArg as Record<string, unknown>
    }

    return undefined
  }

  /**
   * 构建请求体
   * 对于路径参数（:id, :workId, :workSetId），如果第一个参数是数字，则跳过它
   */
  private buildBody(args: unknown[]): unknown {
    if (args.length === 0) return undefined

    // 如果第一个参数是数字（路径参数），则跳过它
    if (args.length === 1) {
      return args[0]
    }

    // 多个参数时，第一个可能是路径参数（数字）
    if (typeof args[0] === 'number') {
      // 后续参数作为 body（如果是对象则直接返回，否则包装）
      const bodyArg = args[1]
      if (typeof bodyArg === 'object' && bodyArg !== null) {
        return bodyArg
      }
      return { args: args.slice(1) }
    }

    // 多参数包装为对象
    return { args }
  }
}

// 导出单例
export const apiProxy = new ApiProxy()
export default ApiProxy
