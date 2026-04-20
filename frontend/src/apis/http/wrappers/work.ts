/**
 * Work HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as WorkHandler, WorkDTO, WorkQueryDTO, WorkResultDTO } from '@bindings/github.com/library-squirrel/wails/internal/work'
import type { WorkFullDTO } from '@bindings/github.com/library-squirrel/wails/internal/model/models'
import type { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'

export interface WorkVO {
  id: number
  title: string
  siteId: number
  siteWorkId: string
  coverUrl: string
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: WorkVO[]
  total: number
  page: number
  pageSize: number
}

// ========== 工具函数 ==========

/**
 * 将 WorkResultDTO 转换为 WorkVO
 */
function toWorkVO(dto: WorkResultDTO): WorkVO {
  return {
    id: dto.id,
    title: dto.siteWorkName ?? '',
    siteId: dto.siteId ?? 0,
    siteWorkId: dto.siteWorkId ?? '',
    coverUrl: '',
    createTime: dto.createTime,
    updateTime: dto.updateTime
  }
}

/**
 * 将 WorkFullDTO 转换为 WorkVO
 */
function toWorkVOFromFullDTO(dto: WorkFullDTO): WorkVO {
  return {
    id: dto.id,
    title: dto.siteWorkName ?? '',
    siteId: dto.siteId,
    siteWorkId: dto.siteWorkId ?? '',
    coverUrl: '',
    createTime: dto.createTime,
    updateTime: dto.updateTime
  }
}

// ========== API 方法 ==========

export async function workGetFullWorkInfoById(id: number): Promise<ApiResponse<WorkVO>> {
  const result = await WorkHandler.GetFullWorkInfoById(id)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? toWorkVOFromFullDTO(result.data) : undefined }
}

export async function workQueryPage(query: {
  page: number
  pageSize: number
  query?: { siteId?: number; title?: string }
}): Promise<ApiResponse<Page<WorkResultDTO, WorkQueryDTO>>> {
  const queryDTO = new WorkQueryDTO({
    siteId: query.query?.siteId ?? null,
    siteWorkNameLike: query.query?.title ?? null
  })
  const result = await WorkHandler.QueryPage(query.page, query.pageSize, queryDTO)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return result
}

export async function workDeleteWorkAndSurroundingData(id: number): Promise<ApiResponse<boolean>> {
  const result = await WorkHandler.DeleteWorkAndSurroundingData(id)
  if (!result) {
    return { success: false, msg: '删除失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

/**
 * 根据站点ID和站点作品ID获取作品
 */
export async function workGetBySiteAndSiteWorkID(
  siteId: number,
  siteWorkId: string
): Promise<ApiResponse<WorkVO>> {
  const result = await WorkHandler.GetBySiteAndSiteWorkID(siteId, siteWorkId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? toWorkVO(result.data) : undefined }
}

/**
 * 根据作品ID列表获取关联的本地作者信息
 */
export async function workListRankedLocalAuthorWithWorkIdByWorkIds(
  workIds: number[]
): Promise<ApiResponse<any[]>> {
  const result = await WorkHandler.ListRankedLocalAuthorWithWorkIdByWorkIds(workIds)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 根据作品ID列表获取关联的站点作者信息
 */
export async function workListRankedSiteAuthorWithWorkIdByWorkIds(
  workIds: number[]
): Promise<ApiResponse<any[]>> {
  const result = await WorkHandler.ListRankedSiteAuthorWithWorkIdByWorkIds(workIds)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 获取作品的重新整理的作者信息
 */
export async function workListReWorkAuthor(workId: number): Promise<ApiResponse<any>> {
  const result = await WorkHandler.ListReWorkAuthor(workId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 更新作品最后使用时间
 */
export async function workUpdateLastUsed(ids: number[]): Promise<ApiResponse<boolean>> {
  const result = await WorkHandler.UpdateLastUsed(ids)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}