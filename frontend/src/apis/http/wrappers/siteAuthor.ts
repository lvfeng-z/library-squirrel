/**
 * SiteAuthor HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as SiteAuthorHandler, SiteAuthorDTO, SiteAuthorQueryDTO, SiteAuthorResultDTO } from '@bindings/github.com/library-squirrel/wails/internal/siteAuthor'
import type { RankedSiteAuthor } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'

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
 * 注意：此方法在 bindings 中未实现
 */
export async function siteAuthorSaveBatch(_authors: SiteAuthorVO[]): Promise<ApiResponse<SiteAuthorVO[]>> {
  // TODO: 此接口在 bindings 中未实现 (SaveBatch)
  return { success: false, msg: '此接口未实现：siteAuthorSaveBatch' }
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
}): Promise<ApiResponse<PageResult>> {
  const queryDTO = new SiteAuthorQueryDTO({
    siteId: query.query?.siteId ?? null,
    authorNameLike: query.query?.authorName ?? null
  })
  const result = await SiteAuthorHandler.QueryPage(query.page, query.pageSize, queryDTO)
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
      items: page.data ? page.data.map(toSiteAuthorVO).filter((item): item is SiteAuthorVO => item !== null) : [],
      total: page.dataCount ?? 0,
      page: page.pageNumber ?? query.page,
      pageSize: page.pageSize ?? query.pageSize
    }
  }
}

/**
 * 查询绑定或未绑定到本地作者的站点作者分页
 * 注意：此方法在 bindings 中未实现
 */
export async function siteAuthorQueryBoundOrUnboundInLocalAuthorPage(_query: {
  page: number
  pageSize: number
  query?: { localAuthorId?: number; boundOnLocalAuthorId?: boolean }
}): Promise<ApiResponse<PageResult>> {
  // TODO: 此接口在 bindings 中未实现 (QueryBoundOrUnboundInLocalAuthorPage)
  return { success: false, msg: '此接口未实现：siteAuthorQueryBoundOrUnboundInLocalAuthorPage' }
}

/**
 * 查询本地关联的站点作者分页
 * 注意：此方法在 bindings 中未实现
 */
export async function siteAuthorQueryLocalRelateDTOPage(_query: {
  page: number
  pageSize: number
}): Promise<ApiResponse<PageResult>> {
  // TODO: 此接口在 bindings 中未实现 (QueryLocalRelateDTOPage)
  return { success: false, msg: '此接口未实现：siteAuthorQueryLocalRelateDTOPage' }
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
 * 注意：此方法在 bindings 中未实现
 */
export async function siteAuthorListBySiteAuthorIds(_siteAuthorIds: number[]): Promise<ApiResponse<SiteAuthorVO[]>> {
  // TODO: 此接口在 bindings 中未实现 (ListBySiteAuthorIds)
  return { success: false, msg: '此接口未实现：siteAuthorListBySiteAuthorIds' }
}

/**
 * 根据作品ID列表获取关联的站点作者信息
 * 注意：此方法在 bindings 中未实现
 */
export async function siteAuthorListRankedSiteAuthorWithWorkIdByWorkIds(
  _workIds: number[]
): Promise<ApiResponse<SiteAuthorVO[]>> {
  // TODO: 此接口在 bindings 中未实现 (ListRankedSiteAuthorWithWorkIdByWorkIds)
  return { success: false, msg: '此接口未实现：siteAuthorListRankedSiteAuthorWithWorkIdByWorkIds' }
}

/**
 * 更新站点作者绑定的本地作者
 * 注意：此方法在 bindings 中未实现
 */
export async function siteAuthorUpdateBindLocalAuthor(
  _localAuthorId: number,
  _siteAuthorIds: number[]
): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (UpdateBindLocalAuthor)
  return { success: false, msg: '此接口未实现：siteAuthorUpdateBindLocalAuthor' }
}

/**
 * 创建并绑定同名的本地作者
 * 注意：此方法在 bindings 中未实现
 */
export async function siteAuthorCreateAndBindSameNameLocalAuthor(
  _siteAuthor: SiteAuthorVO
): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (CreateAndBindSameNameLocalAuthor)
  return { success: false, msg: '此接口未实现：siteAuthorCreateAndBindSameNameLocalAuthor' }
}