/**
 * HTTP API Adapter for Go Backend
 * 桥接前端 IPC 调用到 Go HTTP Backend
 */

const GO_BACKEND_URL = 'http://localhost:8080'

// API 响应格式（与前端 ApiResponse.ts 一致）
interface ApiResponse<T = unknown> {
  success: boolean
  msg: string
  data?: T
}

// 转换 URLSearchParams 中无法处理的嵌套对象
function encodeNestedParams(params: Record<string, unknown>): URLSearchParams {
  const searchParams = new URLSearchParams()

  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null) {
      continue
    }

    if (Array.isArray(value)) {
      // 处理 where, orderBy 等数组
      searchParams.append(key, JSON.stringify(value))
    } else if (typeof value === 'object') {
      searchParams.append(key, JSON.stringify(value))
    } else {
      searchParams.append(key, String(value))
    }
  }

  return searchParams
}

/**
 * 通用 HTTP GET 请求
 */
async function httpGet<T>(path: string, params?: Record<string, unknown>): Promise<ApiResponse<T>> {
  let url = `${GO_BACKEND_URL}${path}`
  if (params) {
    const searchParams = encodeNestedParams(params)
    const query = searchParams.toString()
    if (query) {
      url += `?${query}`
    }
  }

  const response = await fetch(url, {
    method: 'GET',
    headers: {
      'Content-Type': 'application/json'
    }
  })

  return response.json()
}

/**
 * 通用 HTTP POST 请求
 */
async function httpPost<T>(path: string, body?: unknown): Promise<ApiResponse<T>> {
  const response = await fetch(`${GO_BACKEND_URL}${path}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: body ? JSON.stringify(body) : undefined
  })

  return response.json()
}

// ========== LocalTag API ==========

// 本地标签数据结构
export interface LocalTagVO {
  id: number
  localTagName: string
  baseLocalTagId: number
  lastUse: number
  createTime: number
  updateTime: number
}

// 分页结果
export interface PageResult {
  items: LocalTagVO[]
  total: number
  page: number
  pageSize: number
}

/**
 * 分页查询本地标签
 */
export async function localTagQueryPage(query: {
  page: number
  pageSize: number
  query?: {
    localTagName?: string
    sort?: Array<{ key: string; asc: boolean }>
  }
}): Promise<ApiResponse<PageResult>> {
  const { page, pageSize, query: condition } = query

  // 构建查询条件
  const example: Record<string, unknown> = {}
  if (condition?.localTagName) {
    example.where = [
      { field: 'local_tag_name', op: 'LIKE', value: `%${condition.localTagName}%` }
    ]
  }
  if (condition?.sort && condition.sort.length > 0) {
    example.orderBy = condition.sort.map(s => ({
      field: s.key === 'updateTime' ? 'update_time' : s.key === 'createTime' ? 'create_time' : s.key,
      asc: s.asc
    }))
  }

  return httpGet<PageResult>('/api/localTag/page', {
    page,
    pageSize,
    ...example
  })
}

/**
 * 保存本地标签
 */
export async function localTagSave(tag: {
  localTagName?: string
  baseLocalTagId?: number
}): Promise<ApiResponse<LocalTagVO>> {
  return httpPost<LocalTagVO>('/api/localTag/save', {
    localTagName: tag.localTagName,
    baseLocalTagId: tag.baseLocalTagId || 0
  })
}

/**
 * 更新本地标签
 */
export async function localTagUpdateById(tag: {
  id: number
  localTagName?: string
  baseLocalTagId?: number
}): Promise<ApiResponse<LocalTagVO>> {
  return httpPost<LocalTagVO>('/api/localTag/update', {
    id: tag.id,
    localTagName: tag.localTagName,
    baseLocalTagId: tag.baseLocalTagId
  })
}

/**
 * 删除本地标签
 */
export async function localTagDeleteById(id: number): Promise<ApiResponse<null>> {
  return httpPost<null>(`/api/localTag/delete/${id}`)
}

/**
 * 获取单个本地标签
 */
export async function localTagGetById(id: number): Promise<ApiResponse<LocalTagVO>> {
  return httpGet<LocalTagVO>(`/api/localTag/${id}`)
}

/**
 * 获取本地标签树
 */
export async function localTagGetTree(rootId?: number, depth?: number): Promise<ApiResponse<LocalTagVO[]>> {
  return httpGet<LocalTagVO[]>('/api/localTag/tree', { rootId, depth })
}
