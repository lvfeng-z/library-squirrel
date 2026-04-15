/**
 * LocalTag HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as LocalTagHandler, LocalTagDTO, LocalTagQueryDTO, LocalTagResultDTO } from '@bindings/github.com/library-squirrel/wails/internal/localTag'
import type { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'
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

/**
 * 将 LocalTagResultDTO 转换为 LocalTagVO
 */
function toLocalTagVO(dto: LocalTagResultDTO | null): LocalTagVO | null {
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
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 更新本地标签
 */
export async function localTagUpdateById(tag: {
  id: number
  localTagName?: string
  baseLocalTagId?: number
}): Promise<ApiResponse<LocalTagVO>> {
  const tagDTO = new LocalTagDTO({
    id: tag.id,
    localTagName: tag.localTagName ?? null,
    baseLocalTagId: tag.baseLocalTagId ?? null
  })
  const result = await LocalTagHandler.Update(tagDTO)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
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
 */
export async function localTagQueryPage(query: {
  page: number
  pageSize: number
  query?: {
    localTagName?: string
  }
}): Promise<ApiResponse<PageResult>> {
  const queryDTO = new LocalTagQueryDTO({
    localTagName: query.query?.localTagName ?? null
  })
  const result = await LocalTagHandler.QueryPage(query.page, query.pageSize, queryDTO)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  const page = result.data
  if (!page) {
    return { success: true, msg: '', data: { items: [], total: 0, page: query.page, pageSize: query.pageSize } }
  }
  return {
    success: true,
    msg: result.msg ?? '',
    data: {
      items: page.data ? page.data.map(toLocalTagVO).filter((item): item is LocalTagVO => item !== null) : [],
      total: page.dataCount ?? 0,
      page: page.pageNumber ?? query.page,
      pageSize: page.pageSize ?? query.pageSize
    }
  }
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
}): Promise<ApiResponse<PageResult>> {
  const queryDTO = new LocalTagQueryDTO({})
  const result = await LocalTagHandler.QuerySelectItemPage(query.page, query.pageSize, queryDTO, '')
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  const page = result.data
  if (!page) {
    return { success: true, msg: '', data: { items: [], total: 0, page: query.page, pageSize: query.pageSize } }
  }
  return {
    success: true,
    msg: result.msg ?? '',
    data: {
      items: page.data ? page.data.map(item => ({ value: item?.value, label: item?.label ?? '', lastUse: item?.lastUse ?? 0 })) as unknown as LocalTagVO[] : [],
      total: page.dataCount ?? 0,
      page: page.pageNumber ?? query.page,
      pageSize: page.pageSize ?? query.pageSize
    }
  }
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
 * 注意：此方法在 bindings 中未实现
 */
export async function localTagQuerySelectItemPageByWorkId(
  _workId: number,
  _query: { page: number; pageSize: number; query?: Record<string, unknown> }
): Promise<ApiResponse<PageResult>> {
  // TODO: 此接口在 bindings 中未实现 (QuerySelectItemPageByWorkId)
  return { success: false, msg: '此接口未实现：localTagQuerySelectItemPageByWorkId' }
}