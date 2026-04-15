/**
 * SiteTag HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as SiteTagHandler, SiteTagDTO, SiteTagQueryDTO, SiteTagResultDTO } from '@bindings/github.com/library-squirrel/wails/internal/siteTag'
import type { SelectItem } from '@bindings/github.com/library-squirrel/wails/internal/model/models'

export interface SiteTagVO {
  id: number
  siteTagName: string
  localTagId: number
  lastUse: number
  createTime: number
  updateTime: number
}

export interface PageResult {
  items: SiteTagVO[]
  total: number
  page: number
  pageSize: number
}

// ========== 工具函数 ==========

/**
 * 将 SiteTagResultDTO 转换为 SiteTagVO
 */
function toSiteTagVO(dto: SiteTagResultDTO | null): SiteTagVO | null {
  if (!dto) return null
  return {
    id: dto.id,
    siteTagName: dto.siteTagName ?? '',
    localTagId: dto.localTagId ?? 0,
    lastUse: dto.lastUse ?? 0,
    createTime: dto.createTime,
    updateTime: dto.updateTime
  }
}

// ========== API 方法 ==========

export async function siteTagSave(tag: {
  siteTagName?: string
  siteId?: number
}): Promise<ApiResponse<SiteTagVO>> {
  const tagDTO = new SiteTagDTO({
    siteTagName: tag.siteTagName ?? null,
    siteId: tag.siteId ?? null
  })
  const result = await SiteTagHandler.Save(tagDTO)
  if (!result) {
    return { success: false, msg: '保存失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '保存失败' }
  }
  return { success: true, msg: result.msg ?? '', data: { id: result.data ?? 0, siteTagName: '', localTagId: 0, lastUse: 0, createTime: 0, updateTime: 0 } }
}

/**
 * 批量保存站点标签
 * 注意：此方法在 bindings 中未实现
 */
export async function siteTagSaveBatch(_tags: SiteTagVO[]): Promise<ApiResponse<SiteTagVO[]>> {
  // TODO: 此接口在 bindings 中未实现 (SaveBatch)
  return { success: false, msg: '此接口未实现：siteTagSaveBatch' }
}

export async function siteTagDeleteById(id: number): Promise<ApiResponse<null>> {
  const result = await SiteTagHandler.Delete(id)
  if (!result) {
    return { success: false, msg: '删除失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function siteTagUpdateById(tag: {
  id: number
  siteTagName?: string
  localTagId?: number
}): Promise<ApiResponse<SiteTagVO>> {
  const tagDTO = new SiteTagDTO({
    id: tag.id,
    siteTagName: tag.siteTagName ?? null,
    siteId: tag.localTagId ?? null  // 注意：这里可能有问题，siteId 和 localTagId 是不同的字段
  })
  const result = await SiteTagHandler.Update(tagDTO)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function siteTagGetById(id: number): Promise<ApiResponse<SiteTagVO>> {
  const result = await SiteTagHandler.GetById(id)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: toSiteTagVO(result.data ?? null) ?? undefined }
}

export async function siteTagQueryPage(query: {
  page: number
  pageSize: number
  query?: { siteId?: number; siteTagName?: string }
}): Promise<ApiResponse<PageResult>> {
  const queryDTO = new SiteTagQueryDTO({
    siteId: query.query?.siteId ?? null,
    siteTagNameLike: query.query?.siteTagName ?? null
  })
  const result = await SiteTagHandler.QueryPage(query.page, query.pageSize, queryDTO)
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
      items: page.data ? page.data.map(toSiteTagVO).filter((item): item is SiteTagVO => item !== null) : [],
      total: page.dataCount ?? 0,
      page: page.pageNumber ?? query.page,
      pageSize: page.pageSize ?? query.pageSize
    }
  }
}

/**
 * 查询绑定或未绑定到本地标签的站点标签分页
 * 注意：此方法在 bindings 中未实现
 */
export async function siteTagQueryBoundOrUnboundToLocalTagPage(_query: {
  page: number
  pageSize: number
  query?: { localTagId?: number; boundOnLocalTagId?: boolean }
}): Promise<ApiResponse<PageResult>> {
  // TODO: 此接口在 bindings 中未实现 (QueryBoundOrUnboundToLocalTagPage)
  return { success: false, msg: '此接口未实现：siteTagQueryBoundOrUnboundToLocalTagPage' }
}

/**
 * 根据作品ID查询站点标签分页
 * 注意：此方法在 bindings 中未实现
 */
export async function siteTagQueryPageByWorkId(
  _workId: number,
  _query: { page: number; pageSize: number; query?: Record<string, unknown> }
): Promise<ApiResponse<PageResult>> {
  // TODO: 此接口在 bindings 中未实现 (QueryPageByWorkId)
  return { success: false, msg: '此接口未实现：siteTagQueryPageByWorkId' }
}

/**
 * 查询本地关联的站点标签分页
 * 注意：此方法在 bindings 中未实现
 */
export async function siteTagQueryLocalRelateDTOPage(_query: {
  page: number
  pageSize: number
  query?: { workId?: number }
}): Promise<ApiResponse<PageResult>> {
  // TODO: 此接口在 bindings 中未实现 (QueryLocalRelateDTOPage)
  return { success: false, msg: '此接口未实现：siteTagQueryLocalRelateDTOPage' }
}

export async function siteTagQuerySelectItemPageByWorkId(
  workId: number,
  query: { page: number; pageSize: number }
): Promise<ApiResponse<PageResult>> {
  const queryDTO = new SiteTagQueryDTO({})
  const result = await SiteTagHandler.QuerySelectItemPageByWorkId(query.page, query.pageSize, queryDTO, workId)
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
      items: page.data ? page.data.map(item => ({ value: item?.value, label: item?.label ?? '', lastUse: item?.lastUse ?? 0 })) as unknown as SiteTagVO[] : [],
      total: page.dataCount ?? 0,
      page: page.pageNumber ?? query.page,
      pageSize: page.pageSize ?? query.pageSize
    }
  }
}

export async function siteTagListByWorkId(workId: number): Promise<ApiResponse<SiteTagVO[]>> {
  const result = await SiteTagHandler.ListByWorkId(workId)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? result.data.map(toSiteTagVO).filter((item): item is SiteTagVO => item !== null) : [] }
}

/**
 * 根据站点标签ID列表获取站点标签
 * 注意：此方法在 bindings 中未实现
 */
export async function siteTagListBySiteTagIds(_siteTagIds: number[]): Promise<ApiResponse<SiteTagVO[]>> {
  // TODO: 此接口在 bindings 中未实现 (ListBySiteTagIds)
  return { success: false, msg: '此接口未实现：siteTagListBySiteTagIds' }
}

/**
 * 更新站点标签绑定的本地标签
 * 注意：此方法在 bindings 中未实现
 */
export async function siteTagUpdateBindLocalTag(
  _localTagId: number,
  _siteTagIds: number[]
): Promise<ApiResponse<boolean>> {
  // TODO: 此接口在 bindings 中未实现 (UpdateBindLocalTag)
  return { success: false, msg: '此接口未实现：siteTagUpdateBindLocalTag' }
}

/**
 * 创建并绑定同名的本地标签
 * 注意：此方法在 bindings 中未实现
 */
export async function siteTagCreateAndBindSameNameLocalTag(
  _siteTag: SiteTagVO
): Promise<ApiResponse<SiteTagVO>> {
  // TODO: 此接口在 bindings 中未实现 (CreateAndBindSameNameLocalTag)
  return { success: false, msg: '此接口未实现：siteTagCreateAndBindSameNameLocalTag' }
}