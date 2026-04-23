/**
 * LocalTag HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import {
  Handler as LocalTagHandler,
  LocalTagQueryDTO
} from '@bindings/github.com/library-squirrel/wails/internal/localTag'
import {LocalTagDTO, SelectItem, LocalTagWithBaseTagDTO} from "@bindings/github.com/library-squirrel/wails/pkg/model/dto";
import {Page} from "@bindings/github.com/library-squirrel/wails/pkg/model";

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

/**
 * 将 LocalTagResultDTO 转换为 LocalTagVO
 */
function toLocalTagVO(dto: LocalTagDTO | null): LocalTagVO | null {
  if (!dto) return null
  return {
    id: dto.id,
    localTagName: dto.localTagName ?? '',
    baseLocalTagId: dto.baseLocalTagId ?? 0,
    lastUse: dto.lastUse ?? 0,
    createTime: dto.createTime,
    updateTime: dto.updateTime
  }
}

// ========== API 方法 ==========

/**
 * 保存本地标签
 */
export async function localTagSave(tag: {
  localTagName?: string
  baseLocalTagId?: number
}): Promise<ApiResponse<LocalTagVO>> {
  const tagDTO = new LocalTagDTO({
    localTagName: tag.localTagName ?? null,
    baseLocalTagId: tag.baseLocalTagId ?? null
  })
  const result = await LocalTagHandler.Save(tagDTO)
  if (!result) {
    return { success: false, msg: '保存失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '保存失败' }
  }
  return { success: true, msg: result.msg ?? '', data: { id: result.data ?? 0 } as LocalTagVO }
}

/**
 * 删除本地标签
 */
export async function localTagDeleteById(id: number): Promise<ApiResponse<null>> {
  const result = await LocalTagHandler.Delete(id)
  if (!result) {
    return { success: false, msg: '删除失败：接口返回为空' }
  }
  return result
}

/**
 * 更新本地标签
 */
export async function localTagUpdateById(tag: LocalTagDTO): Promise<ApiResponse<LocalTagDTO>> {
  const result = await LocalTagHandler.Update(tag)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  return result
}

/**
 * 获取单个本地标签
 */
export async function localTagGetById(id: number): Promise<ApiResponse<LocalTagVO>> {
  const result = await LocalTagHandler.GetById(id)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toLocalTagVO(result.data ?? null) ?? undefined }
}

/**
 * 分页查询本地标签
 * 直接透传 Page 对象给 binding，不做额外解包
 */
export async function localTagQueryPage(page: Page<LocalTagDTO, LocalTagQueryDTO>): Promise<ApiResponse<Page<LocalTagDTO, LocalTagQueryDTO>>> {
  const result = await LocalTagHandler.QueryPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 获取本地标签树
 */
export async function localTagGetTree(rootId?: number, depth?: number): Promise<ApiResponse<LocalTagVO[]>> {
  const result = await LocalTagHandler.GetTree(rootId ?? 0, depth ?? 10)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? result.data.map(toLocalTagVO).filter((item): item is LocalTagVO => item !== null) : [] }
}

/**
 * 获取选择项列表
 */
export async function localTagListSelectItems(
  query?: Record<string, unknown>
): Promise<ApiResponse<SelectItem[]>> {
  const queryDTO = new LocalTagQueryDTO({})
  const result = await LocalTagHandler.ListSelectItems(queryDTO)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: (result.data ?? []).filter((item): item is SelectItem => item !== null) }
}

/**
 * 分页查询选择项
 */
export async function localTagQuerySelectItemPage(query: {
  page: number
  pageSize: number
  query?: Record<string, unknown>
}): Promise<ApiResponse<Page<SelectItem, LocalTagQueryDTO>>> {
  const queryDTO = new LocalTagQueryDTO({})
  const page = new Page<SelectItem, LocalTagQueryDTO>({
    pageNumber: query.page,
    pageSize: query.pageSize,
    query: queryDTO
  })
  const result = await LocalTagHandler.QuerySelectItemPage(page, '')
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 根据作品ID获取标签列表
 */
export async function localTagListByWorkId(workId: number): Promise<ApiResponse<LocalTagVO[]>> {
  const result = await LocalTagHandler.ListByWorkId(workId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? result.data.map(toLocalTagVO).filter((item): item is LocalTagVO => item !== null) : [] }
}

/**
 * 根据作品ID分页查询选择项
 */
export async function localTagQuerySelectItemPageByWorkId(
  workId: number,
  query: { page: number; pageSize: number; query?: Record<string, unknown> }
): Promise<ApiResponse<Page<SelectItem, LocalTagQueryDTO>>> {
  const queryDTO = new LocalTagQueryDTO({})
  const page = new Page<SelectItem, LocalTagQueryDTO>({
    pageNumber: query.page,
    pageSize: query.pageSize,
    query: queryDTO
  })
  const result = await LocalTagHandler.QuerySelectItemPageByWorkId(page, workId)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 分页查询包含基础标签信息的本地标签
 * 直接透传 Page 对象给 binding，不做额外解包
 */
export async function localTagQueryWithBaseTagPage(page: Page<LocalTagWithBaseTagDTO, LocalTagQueryDTO>): Promise<ApiResponse<Page<LocalTagWithBaseTagDTO, LocalTagQueryDTO>>> {
  const result = await LocalTagHandler.QueryWithBaseTagPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}
