/**
 * ReWorkAuthor HTTP API 包装器
 * 作品与作者关联关系的 API 封装
 */

import type { ApiResponse } from '../types'
import { Handler as ReWorkAuthorHandler } from '@bindings/github.com/library-squirrel/wails/backend/reWorkAuthor'
import type { WorkAuthorsResultDTO } from '@bindings/github.com/library-squirrel/wails/backend/reWorkAuthor/models'
import type { RankedLocalAuthor, RankedSiteAuthor, RankedLocalAuthorWithWorkId, RankedSiteAuthorWithWorkId } from '@bindings/github.com/library-squirrel/wails/backend/base/model/models'

// ========== VO 定义 ==========

/**
 * 本地作者 VO
 */
export interface LocalAuthorVO {
  id: number
  authorName: string
  introduce: string
  lastUse: number
  createTime: number
  updateTime: number
  authorRank?: number
}

/**
 * 站点作者 VO
 */
export interface SiteAuthorVO {
  id: number
  siteId?: number
  siteAuthorId?: string
  authorName: string
  fixedAuthorName?: string
  siteAuthorNameBefore?: string
  introduce: string
  localAuthorId?: number
  lastUse?: number
  createTime: number
  updateTime: number
  authorRank?: number
}

/**
 * 作品作者关联信息 VO
 */
export interface WorkAuthorVO {
  localAuthors: LocalAuthorVO[]
  siteAuthors: SiteAuthorVO[]
}

/**
 * 批量作品作者关联信息结果 VO
 */
export interface WorkAuthorsResultVO {
  workId: number
  localAuthors: LocalAuthorVO[]
  siteAuthors: SiteAuthorVO[]
}

// ========== 转换函数 ==========

function toLocalAuthorVO(dto: RankedLocalAuthor | null): LocalAuthorVO | null {
  if (!dto) return null
  return {
    id: dto.id,
    authorName: dto.authorName ?? '',
    introduce: dto.introduce ?? '',
    lastUse: dto.lastUse ?? 0,
    createTime: dto.createTime,
    updateTime: dto.updateTime,
    authorRank: dto.authorRank
  }
}

function toSiteAuthorVO(dto: RankedSiteAuthor | null): SiteAuthorVO | null {
  if (!dto) return null
  return {
    id: dto.id,
    siteId: dto.siteId ?? undefined,
    siteAuthorId: dto.siteAuthorId ?? undefined,
    authorName: dto.authorName ?? '',
    fixedAuthorName: dto.fixedAuthorName ?? undefined,
    siteAuthorNameBefore: dto.siteAuthorNameBefore ?? undefined,
    introduce: dto.introduce ?? '',
    localAuthorId: dto.localAuthorId ?? undefined,
    lastUse: dto.lastUse ?? undefined,
    createTime: dto.createTime,
    updateTime: dto.updateTime,
    authorRank: dto.authorRank
  }
}

function toWorkAuthorVO(localAuthors: RankedLocalAuthor[], siteAuthors: RankedSiteAuthor[]): WorkAuthorVO {
  return {
    localAuthors: localAuthors.map(toLocalAuthorVO).filter((item): item is LocalAuthorVO => item !== null),
    siteAuthors: siteAuthors.map(toSiteAuthorVO).filter((item): item is SiteAuthorVO => item !== null)
  }
}

// ========== API 方法 ==========

/**
 * 获取单个作品的作者关联信息（包含本地作者和站点作者）
 */
export async function reWorkAuthorListByWorkId(workId: number): Promise<ApiResponse<WorkAuthorVO>> {
  const result = await ReWorkAuthorHandler.ListByWorkId(workId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  const data = result.data
  if (!data) {
    return { success: true, msg: result.msg ?? '', data: { localAuthors: [], siteAuthors: [] } }
  }
  return {
    success: true,
    msg: result.msg ?? '',
    data: toWorkAuthorVO(
      (data.localAuthors ?? []).filter((item): item is RankedLocalAuthor => item !== null),
      (data.siteAuthors ?? []).filter((item): item is RankedSiteAuthor => item !== null)
    )
  }
}

/**
 * 批量获取多个作品的作者关联信息
 */
export async function reWorkAuthorListByWorkIds(workIds: number[]): Promise<ApiResponse<WorkAuthorsResultVO[]>> {
  const result = await ReWorkAuthorHandler.ListByWorkIds(workIds)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  const data = (result.data ?? []).filter((item): item is WorkAuthorsResultDTO => item !== null)
  return {
    success: true,
    msg: result.msg ?? '',
    data: data.map(item => ({
      workId: item.workId,
      localAuthors: (item.localAuthors ?? []).filter((a) => a !== null).map(a => toLocalAuthorVO(a as RankedLocalAuthor)).filter((a): a is LocalAuthorVO => a !== null),
      siteAuthors: (item.siteAuthors ?? []).filter((a) => a !== null).map(a => toSiteAuthorVO(a as RankedSiteAuthor)).filter((a): a is SiteAuthorVO => a !== null)
    }))
  }
}

/**
 * 查询作品关联的本地作者列表
 */
export async function reWorkAuthorListLocalAuthorsByWorkId(workId: number): Promise<ApiResponse<LocalAuthorVO[]>> {
  const result = await ReWorkAuthorHandler.ListLocalAuthorsByWorkId(workId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return {
    success: true,
    msg: result.msg ?? '',
    data: (result.data ?? []).map(toLocalAuthorVO).filter((item): item is LocalAuthorVO => item !== null)
  }
}

/**
 * 查询作品关联的站点作者列表
 */
export async function reWorkAuthorListSiteAuthorsByWorkId(workId: number): Promise<ApiResponse<SiteAuthorVO[]>> {
  const result = await ReWorkAuthorHandler.ListSiteAuthorsByWorkId(workId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return {
    success: true,
    msg: result.msg ?? '',
    data: (result.data ?? []).map(toSiteAuthorVO).filter((item): item is SiteAuthorVO => item !== null)
  }
}

/**
 * 查询多个作品的本地作者列表（带作品ID）
 */
export async function reWorkAuthorListRankedLocalAuthorWithWorkIdByWorkIds(
  workIds: number[]
): Promise<ApiResponse<Array<RankedLocalAuthorWithWorkId & { workId: number }>>> {
  const result = await ReWorkAuthorHandler.ListRankedLocalAuthorWithWorkIdByWorkIds(workIds)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: (result.data ?? []).filter((item): item is RankedLocalAuthorWithWorkId => item !== null) }
}

/**
 * 查询多个作品的站点作者列表（带作品ID）
 */
export async function reWorkAuthorListRankedSiteAuthorWithWorkIdByWorkIds(
  workIds: number[]
): Promise<ApiResponse<Array<RankedSiteAuthorWithWorkId & { workId: number }>>> {
  const result = await ReWorkAuthorHandler.ListRankedSiteAuthorWithWorkIdByWorkIds(workIds)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: (result.data ?? []).filter((item): item is RankedSiteAuthorWithWorkId => item !== null) }
}