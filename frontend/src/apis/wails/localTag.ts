/**
 * LocalTag Wails 绑定包装器
 * 直接调用 Wails bindings，绕过 HTTP 代理层
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/localTag'
import { LocalTag } from '@bindings/github.com/library-squirrel/wails/internal/model/models'
import { LocalTagQueryDTO } from '@bindings/github.com/library-squirrel/wails/internal/localTag/models'
import type { Page, ApiResponse } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'
import type { SelectItem } from '@bindings/github.com/library-squirrel/wails/internal/model/models'

// ========== 类型定义 ==========

export interface LocalTagVO {
  id: number
  localTagName: string
  baseLocalTagId: number
  lastUse: number
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: LocalTagVO[]
  total: number
  page: number
  pageSize: number
}

// ========== 工具函数 ==========

function nullLocalTagToVO(tag: LocalTag | null): LocalTagVO | null {
  if (!tag) return null
  return {
    id: tag.id,
    localTagName: tag.localTagName?.String ?? '',
    baseLocalTagId: tag.baseLocalTagId?.Int64 ?? 0,
    lastUse: tag.lastUse?.Int64 ?? 0,
    createTime: tag.createTime,
    updateTime: tag.updateTime
  }
}

// ========== API 方法 ==========

/**
 * 保存本地标签
 */
export async function localTagSave(tag: {
  localTagName?: string
  baseLocalTagId?: number
}): Promise<ApiResponse<number>> {
  const localTag = new LocalTag({
    localTagName: { String: tag.localTagName ?? '', Valid: true },
    baseLocalTagId: { Int64: tag.baseLocalTagId ?? 0, Valid: true }
  })
  try {
    const result = await Handler.Save(localTag)
    if (result && result.success) {
      return { success: true, msg: 'success', data: result.data }
    }
    return { success: false, msg: result?.msg ?? 'Unknown error', data: undefined }
  } catch (error) {
    return {
      success: false,
      msg: error instanceof Error ? error.message : 'Unknown error',
      data: undefined
    }
  }
}

/**
 * 删除本地标签
 */
export async function localTagDeleteById(id: number): Promise<ApiResponse<void>> {
  try {
    const result = await Handler.Delete(id)
    if (result && result.success) {
      return { success: true, msg: 'success', data: undefined }
    }
    return { success: false, msg: result?.msg ?? 'Unknown error', data: undefined }
  } catch (error) {
    return {
      success: false,
      msg: error instanceof Error ? error.message : 'Unknown error',
      data: undefined
    }
  }
}

/**
 * 更新本地标签
 */
export async function localTagUpdateById(tag: {
  id: number
  localTagName?: string
  baseLocalTagId?: number
}): Promise<ApiResponse<void>> {
  const localTag = new LocalTag({
    id: tag.id,
    localTagName: tag.localTagName ? { String: tag.localTagName, Valid: true } : { String: '', Valid: false },
    baseLocalTagId: tag.baseLocalTagId !== undefined ? { Int64: tag.baseLocalTagId, Valid: true } : { Int64: 0, Valid: false }
  })
  try {
    const result = await Handler.Update(localTag)
    if (result && result.success) {
      return { success: true, msg: 'success', data: undefined }
    }
    return { success: false, msg: result?.msg ?? 'Unknown error', data: undefined }
  } catch (error) {
    return {
      success: false,
      msg: error instanceof Error ? error.message : 'Unknown error',
      data: undefined
    }
  }
}

/**
 * 获取单个本地标签
 */
export async function localTagGetById(id: number): Promise<ApiResponse<LocalTagVO>> {
  const result = await Handler.GetById(id)
  if (result && result.success && result.data) {
    return {
      ...result,
      data: nullLocalTagToVO(result.data)
    }
  }
  return result as unknown as ApiResponse<LocalTagVO>
}

/**
 * 分页查询本地标签
 */
export async function localTagQueryPage(query: {
  page: number
  pageSize: number
  query?: {
    localTagName?: string
    localTagNameLike?: string
    baseLocalTagId?: number
  }
}): Promise<ApiResponse<PageResult>> {
  const queryDTO = new LocalTagQueryDTO({
    localTagName: query.query?.localTagName ?? null,
    localTagNameLike: query.query?.localTagNameLike ?? null,
    baseLocalTagId: query.query?.baseLocalTagId ?? null
  })
  const result = await Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, queryDTO)
  if (result && result.success) {
    return result
  }
  return result as unknown as ApiResponse<PageResult>
}

/**
 * DTO分页查询本地标签
 */
export async function localTagQueryDTOPage(query: {
  page: number
  pageSize: number
  query?: {
    localTagName?: string
    localTagNameLike?: string
    baseLocalTagId?: number
  }
}): Promise<ApiResponse<PageResult>> {
  const queryDTO = new LocalTagQueryDTO({
    localTagName: query.query?.localTagName ?? null,
    localTagNameLike: query.query?.localTagNameLike ?? null,
    baseLocalTagId: query.query?.baseLocalTagId ?? null
  })
  const result = await Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, queryDTO)
  if (result && result.success) {
    return result
  }
  return result as unknown as ApiResponse<PageResult>
}

/**
 * 获取本地标签树
 */
export async function localTagGetTree(rootId?: number, depth?: number): Promise<ApiResponse<LocalTagVO[]>> {
  const result = await Handler.GetTree(rootId ?? 0, depth ?? -1)
  if (result && result.success) {
    return {
      ...result,
      data: (result.data ?? []).map(nullLocalTagToVO).filter((t): t is LocalTagVO => t !== null)
    }
  }
  return result as unknown as ApiResponse<LocalTagVO[]>
}

/**
 * 获取选择项列表
 */
export async function localTagListSelectItems(
  query?: Record<string, unknown>
): Promise<ApiResponse<(SelectItem | null)[]>> {
  const queryDTO = new LocalTagQueryDTO({})
  return await Handler.ListSelectItems(queryDTO)
}

/**
 * 分页查询选择项
 */
export async function localTagQuerySelectItemPage(query: {
  page: number
  pageSize: number
  query?: Record<string, unknown>
}): Promise<ApiResponse<PageResult>> {
  const queryDTO = new LocalTagQueryDTO({})
  const result = await Handler.QuerySelectItemPage(query.page ?? 1, query.pageSize ?? 10, queryDTO, '')
  if (result && result.success) {
    const page = result.data ?? null
    return {
      ...result,
      data: page ? {
        items: (page.data ?? []) as unknown as LocalTagVO[],
        total: page.dataCount,
        page: page.pageNumber,
        pageSize: page.pageSize
      } : undefined
    }
  }
  return result as unknown as ApiResponse<PageResult>
}

/**
 * 根据作品ID获取标签列表
 */
export async function localTagListByWorkId(workId: number): Promise<ApiResponse<LocalTagVO[]>> {
  const result = await Handler.ListByWorkId(workId)
  if (result && result.success) {
    return {
      ...result,
      data: (result.data ?? []).map(nullLocalTagToVO).filter((t): t is LocalTagVO => t !== null)
    }
  }
  return result as unknown as ApiResponse<LocalTagVO[]>
}

/**
 * 根据作品ID分页查询选择项
 */
export async function localTagQuerySelectItemPageByWorkId(
  workId: number,
  query: { page: number; pageSize: number; query?: Record<string, unknown> }
): Promise<ApiResponse<PageResult>> {
  const queryDTO = new LocalTagQueryDTO({})
  const result = await Handler.QuerySelectItemPageByWorkId(query.page ?? 1, query.pageSize ?? 10, queryDTO, workId)
  if (result && result.success) {
    const page = result.data ?? null
    return {
      ...result,
      data: page ? {
        items: (page.data ?? []) as unknown as LocalTagVO[],
        total: page.dataCount,
        page: page.pageNumber,
        pageSize: page.pageSize
      } : undefined
    }
  }
  return result as unknown as ApiResponse<PageResult>
}