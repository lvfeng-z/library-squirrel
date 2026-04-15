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
}): Promise<ApiResponse<Page<WorkResultDTO>>> {
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
 * 根据作品ID列表获取关联的本地作者信息
 * 注意：此方法在 bindings 中未实现
 */
export async function workListRankedLocalAuthorWithWorkIdByWorkIds(
  _workIds: number[]
): Promise<ApiResponse<WorkVO[]>> {
  // TODO: 此接口在 bindings 中未实现 (ListRankedLocalAuthorWithWorkIdByWorkIds)
  return { success: false, msg: '此接口未实现：workListRankedLocalAuthorWithWorkIdByWorkIds' }
}

/**
 * 根据作品ID列表获取关联的站点作者信息
 * 注意：此方法在 bindings 中未实现
 */
export async function workListRankedSiteAuthorWithWorkIdByWorkIds(
  _workIds: number[]
): Promise<ApiResponse<WorkVO[]>> {
  // TODO: 此接口在 bindings 中未实现 (ListRankedSiteAuthorWithWorkIdByWorkIds)
  return { success: false, msg: '此接口未实现：workListRankedSiteAuthorWithWorkIdByWorkIds' }
}

/**
 * 获取作品的重新整理的作者信息
 * 注意：此方法在 bindings 中未实现
 */
export async function workListReWorkAuthor(_workId: number): Promise<ApiResponse<WorkVO[]>> {
  // TODO: 此接口在 bindings 中未实现 (ListReWorkAuthor)
  return { success: false, msg: '此接口未实现：workListReWorkAuthor' }
}

/**
 * 更新作品最后使用时间
 * 注意：此方法在 bindings 中未实现
 */
export async function workUpdateLastUsed(_ids: number[]): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (UpdateLastUsed)
  return { success: false, msg: '此接口未实现：workUpdateLastUsed' }
}