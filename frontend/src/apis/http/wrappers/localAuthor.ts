/**
 * LocalAuthor HTTP API 包装器
 * 直接调用 bindings 接口
 */

import {
  Handler as LocalAuthorHandler,
  LocalAuthorQueryDTO
} from "@bindings/github.com/library-squirrel/wails/internal/localAuthor";
import type { ApiResponse } from '../types'
import {LocalAuthorDTO, SelectItem} from "@bindings/github.com/library-squirrel/wails/pkg/model/dto";
import {Page} from "@bindings/github.com/library-squirrel/wails/pkg/model";

export interface LocalAuthorVO {
  id: number
  authorName: string
  introduce: string
  lastUse: number
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: LocalAuthorVO[]
  total: number
  page: number
  pageSize: number
}

// ========== 工具函数 ==========

/**
 * 将 LocalAuthorDTO 转换为 LocalAuthorVO
 */
function toLocalAuthorVO(dto: LocalAuthorDTO | null): LocalAuthorVO | null {
  if (!dto) return null
  return {
    id: dto.id,
    authorName: dto.authorName ?? '',
    introduce: dto.introduce ?? '',
    lastUse: dto.lastUse ?? 0,
    createTime: dto.createTime,
    updateTime: dto.updateTime
  }
}

// ========== API 方法 ==========

export async function localAuthorSave(author: {
  authorName?: string
  introduce?: string
}): Promise<ApiResponse<LocalAuthorVO>> {
  const authorDTO = new LocalAuthorDTO({
    authorName: author.authorName ?? null,
    introduce: author.introduce ?? null
  })
  const result = await LocalAuthorHandler.Save(authorDTO)
  if (!result) {
    return { success: false, msg: '保存失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '保存失败' }
  }
  return { success: true, msg: result.msg ?? '', data: { id: result.data ?? 0 } as LocalAuthorVO }
}

export async function localAuthorDeleteById(id: number): Promise<ApiResponse<null>> {
  const result = await LocalAuthorHandler.Delete(id)
  if (!result) {
    return { success: false, msg: '删除失败：接口返回为空' }
  }
  return result
}

export async function localAuthorUpdateById(author: {
  id: number
  authorName?: string
  introduce?: string
}): Promise<ApiResponse<LocalAuthorVO>> {
  const authorDTO = new LocalAuthorDTO({
    id: author.id,
    authorName: author.authorName ?? null,
    introduce: author.introduce ?? null
  })
  const result = await LocalAuthorHandler.Update(authorDTO)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  return result
}

export async function localAuthorGetById(id: number): Promise<ApiResponse<LocalAuthorVO>> {
  const result = await LocalAuthorHandler.GetById(id)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toLocalAuthorVO(result.data ?? null) ?? undefined }
}

export async function localAuthorQueryPage(page: Page<LocalAuthorDTO, LocalAuthorQueryDTO>): Promise<ApiResponse<Page<LocalAuthorDTO, LocalAuthorQueryDTO>>> {
  const result = await LocalAuthorHandler.QueryPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

export async function localAuthorListSelectItems(
  _query?: Record<string, unknown>
): Promise<ApiResponse<SelectItem[]>> {
  const queryDTO = new LocalAuthorQueryDTO({})
  const result = await LocalAuthorHandler.ListSelectItems(queryDTO)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: (result.data ?? []).filter((item): item is SelectItem => item !== null) }
}

export async function localAuthorQuerySelectItemPage(query: {
  page: number
  pageSize: number
  query?: Record<string, unknown>
}): Promise<ApiResponse<Page<SelectItem, LocalAuthorQueryDTO>>> {
  const queryDTO = new LocalAuthorQueryDTO({})
  const page = new Page<SelectItem, LocalAuthorQueryDTO>({
    pageNumber: query.page,
    pageSize: query.pageSize,
    query: queryDTO
  })
  const result = await LocalAuthorHandler.QuerySelectItemPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}