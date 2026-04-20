/**
 * SiteAuthor HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as SiteAuthorHandler, SiteAuthorDTO, SiteAuthorQueryDTO, SiteAuthorResultDTO, SiteAuthorFullDTO, SiteAuthorLocalRelateDTO } from '@bindings/github.com/library-squirrel/wails/internal/siteAuthor'
import type { QueryAttribute } from '@bindings/github.com/library-squirrel/wails/pkg/query/models'
import { RankedSiteAuthor, Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'

export interface SiteAuthorVO {
  id: number
  authorName: string
  introduce: string
  localAuthorId: number
  lastUse: number
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: SiteAuthorVO[]
  total: number
  page: number
  pageSize: number
}

// ========== 工具函数 ==========

/**
 * 将 SiteAuthorResultDTO 转换为 SiteAuthorVO
 */
function toSiteAuthorVO(dto: SiteAuthorResultDTO | null): SiteAuthorVO | null {
  if (!dto) return null
  return {
    id: dto.id,
    authorName: dto.authorName ?? '',
    introduce: dto.introduce ?? '',
    localAuthorId: dto.localAuthorId ?? 0,
    lastUse: dto.lastUse ?? 0,
    createTime: dto.createTime,
    updateTime: dto.updateTime
  }
}

/**
 * 将 RankedSiteAuthor 转换为 SiteAuthorVO
 */
function rankedToSiteAuthorVO(dto: RankedSiteAuthor | null): SiteAuthorVO | null {
  if (!dto) return null
  return {
    id: dto.id,
    authorName: dto.authorName ?? '',
    introduce: '',
    localAuthorId: dto.localAuthorId ?? 0,
    lastUse: 0,
    createTime: 0,
    updateTime: 0
  }
}

// ========== API 方法 ==========

export async function siteAuthorSave(author: {
  authorName?: string
  introduce?: string
  siteId?: number
}): Promise<ApiResponse<SiteAuthorVO>> {
  const authorDTO = new SiteAuthorDTO({
    authorName: author.authorName ?? null,
    siteId: author.siteId ?? null
  })
  const result = await SiteAuthorHandler.Save(authorDTO)
  if (!result) {
    return { success: false, msg: '保存失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '保存失败' }
  }
  return { success: true, msg: result.msg ?? '', data: { id: result.data ?? 0, authorName: '', introduce: '', localAuthorId: 0, lastUse: 0, createTime: 0, updateTime: 0 } }
}

/**
 * 批量保存站点作者
 */
export async function siteAuthorSaveBatch(authors: SiteAuthorVO[]): Promise<ApiResponse<SiteAuthorVO[]>> {
  const result = await SiteAuthorHandler.SaveBatch(authors.map(author => ({
    id: author.id,
    siteId: author.id,
    siteAuthorId: author.id.toString(),
    authorName: author.authorName,
    introduce: author.introduce
  })))
  if (!result) {
    return { success: false, msg: '保存失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function siteAuthorDeleteById(id: number): Promise<ApiResponse<null>> {
  const result = await SiteAuthorHandler.Delete(id)
  if (!result) {
    return { success: false, msg: '删除失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function siteAuthorUpdateById(author: {
  id: number
  authorName?: string
  introduce?: string
  localAuthorId?: number
}): Promise<ApiResponse<SiteAuthorVO>> {
  const authorDTO = new SiteAuthorDTO({
    id: author.id,
    authorName: author.authorName ?? null
  })
  const result = await SiteAuthorHandler.Update(authorDTO)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function siteAuthorGetById(id: number): Promise<ApiResponse<SiteAuthorVO>> {
  const result = await SiteAuthorHandler.GetById(id)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toSiteAuthorVO(result.data ?? null) ?? undefined }
}

export async function siteAuthorQueryPage(query: {
  page: number
  pageSize: number
  query?: { siteId?: number; authorName?: string }
}): Promise<ApiResponse<Page<SiteAuthorResultDTO, SiteAuthorQueryDTO>>> {
  const queryDTO = new SiteAuthorQueryDTO({
    siteId: { value: query.query?.siteId } as QueryAttribute,
    authorName: { value: query.query?.authorName, operator: "like" } as QueryAttribute
  })
  const page = new Page<SiteAuthorQueryDTO, SiteAuthorQueryDTO>({
    pageNumber: query.page,
    pageSize: query.pageSize,
    query: queryDTO
  })
  const result = await SiteAuthorHandler.QueryPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 查询绑定或未绑定到本地作者的站点作者分页
 */
export async function siteAuthorQueryBoundOrUnboundInLocalAuthorPage(query: {
  page: number
  pageSize: number
  query?: { localAuthorId?: number; boundOnLocalAuthorId?: boolean }
}): Promise<ApiResponse<Page<SiteAuthorFullDTO, SiteAuthorQueryDTO>>> {
  const queryDTO = new SiteAuthorQueryDTO({
    localAuthorId: { value: query.query?.localAuthorId } as QueryAttribute,
    boundOnLocalAuthorId: { value: query.query?.boundOnLocalAuthorId } as QueryAttribute
  })
  const page = new Page<SiteAuthorQueryDTO, SiteAuthorQueryDTO>({
    pageNumber: query.page,
    pageSize: query.pageSize,
    query: queryDTO
  })
  const result = await SiteAuthorHandler.QueryBoundOrUnboundToLocalAuthorPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 查询本地关联的站点作者分页
 */
export async function siteAuthorQueryLocalRelateDTOPage(query: {
  page: number
  pageSize: number
}): Promise<ApiResponse<Page<SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO>>> {
  const queryDTO = new SiteAuthorQueryDTO({})
  const page = new Page<SiteAuthorQueryDTO, SiteAuthorQueryDTO>({
    pageNumber: query.page,
    pageSize: query.pageSize,
    query: queryDTO
  })
  const result = await SiteAuthorHandler.QueryLocalRelateDTOPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

export async function siteAuthorListByWorkId(workId: number): Promise<ApiResponse<SiteAuthorVO[]>> {
  const result = await SiteAuthorHandler.ListByWorkId(workId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? result.data.map(rankedToSiteAuthorVO).filter((item): item is SiteAuthorVO => item !== null) : [] }
}

/**
 * 根据站点作者ID列表获取站点作者
 */
export async function siteAuthorListBySiteAuthorIds(siteAuthorIds: number[]): Promise<ApiResponse<SiteAuthorVO[]>> {
  const result = await SiteAuthorHandler.ListBySiteAuthorIds(siteAuthorIds)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? result.data.map(toSiteAuthorVO).filter((item): item is SiteAuthorVO => item !== null) : [] }
}

/**
 * 根据作品ID列表获取关联的站点作者信息
 */
export async function siteAuthorListRankedSiteAuthorWithWorkIdByWorkIds(
  workIds: number[]
): Promise<ApiResponse<SiteAuthorVO[]>> {
  const result = await SiteAuthorHandler.ListRankedSiteAuthorWithWorkIdByWorkIds(workIds)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? result.data.filter((item): item is RankedSiteAuthor => item !== null) : [] }
}

/**
 * 更新站点作者绑定的本地作者
 */
export async function siteAuthorUpdateBindLocalAuthor(
  localAuthorId: number,
  siteAuthorIds: number[]
): Promise<ApiResponse<boolean>> {
  const result = await SiteAuthorHandler.UpdateBindLocalAuthor(localAuthorId, siteAuthorIds)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '更新失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data }
}

/**
 * 创建并绑定同名的本地作者
 */
export async function siteAuthorCreateAndBindSameNameLocalAuthor(
  siteAuthor: SiteAuthorVO
): Promise<ApiResponse<boolean>> {
  const result = await SiteAuthorHandler.CreateAndBindSameNameLocalAuthor({
    id: siteAuthor.id,
    siteId: siteAuthor.localAuthorId > 0 ? siteAuthor.localAuthorId : null,
    siteAuthorId: siteAuthor.id.toString(),
    authorName: siteAuthor.authorName,
    introduce: siteAuthor.introduce
  })
  if (!result) {
    return { success: false, msg: '创建失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '创建失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data }
}