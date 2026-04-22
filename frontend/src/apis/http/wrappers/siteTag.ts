/**
 * SiteTag HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import IPage from "@renderer/model/util/IPage.ts";
import {
  Handler as SiteTagHandler,
  SiteTagParamDTO,
  SiteTagQueryDTO,
  SiteTagLocalRelateDTO
} from "@bindings/github.com/library-squirrel/wails/internal/siteTag";
import { SiteTagDTO, SiteTagFullDTO } from "@bindings/github.com/library-squirrel/wails/pkg/model/dto";
import { Page } from "@bindings/github.com/library-squirrel/wails/pkg/model/models";
import { SelectItem } from "@bindings/github.com/library-squirrel/wails/pkg/model/dto";

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
function toSiteTagVO(dto: SiteTagDTO | null): SiteTagVO | null {
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
  const tagDTO = new SiteTagParamDTO({
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
 */
export async function siteTagSaveBatch(tags: SiteTagVO[]): Promise<ApiResponse<SiteTagVO[]>> {
  const result = await SiteTagHandler.SaveBatch(tags.map(tag => new SiteTagParamDTO({
    id: tag.id,
    siteTagName: tag.siteTagName,
    siteId: null,
    siteTagId: null,
    description: null
  })))
  if (!result) {
    return { success: false, msg: '保存失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
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
  const tagDTO = new SiteTagParamDTO({
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

export async function siteTagQueryPage(page: Page<SiteTagDTO, SiteTagQueryDTO>): Promise<ApiResponse<Page<SiteTagDTO, SiteTagQueryDTO>>> {
  const result = await SiteTagHandler.QueryPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 查询绑定或未绑定到本地标签的站点标签分页
 */
export async function siteTagQueryBoundOrUnboundToLocalTagPage(page: IPage<SiteTagFullDTO, SiteTagQueryDTO>): Promise<ApiResponse<Page<SiteTagFullDTO, SiteTagQueryDTO>>> {
  const result = await SiteTagHandler.QueryBoundOrUnboundToLocalTagPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 根据作品ID查询站点标签分页
 */
export async function siteTagQueryPageByWorkId(
  workId: number,
  page: Page<SiteTagFullDTO, SiteTagQueryDTO>
): Promise<ApiResponse<Page<SiteTagFullDTO, SiteTagQueryDTO>>> {
  const result = await SiteTagHandler.QueryPageByWorkId(page, workId)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 查询本地关联的站点标签分页
 */
export async function siteTagQueryLocalRelateDTOPage(page: Page<SiteTagLocalRelateDTO, SiteTagQueryDTO>): Promise<ApiResponse<Page<SiteTagLocalRelateDTO, SiteTagQueryDTO>>> {
  const result = await SiteTagHandler.QueryLocalRelateDTOPage(page)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

export async function siteTagQuerySelectItemPageByWorkId(
  workId: number,
  page: Page<SelectItem, SiteTagQueryDTO>
): Promise<ApiResponse<Page<SelectItem, SiteTagQueryDTO>>> {
  const result = await SiteTagHandler.QuerySelectItemPageByWorkId(page, workId)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '', data: result.data ?? undefined }
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
 */
export async function siteTagListBySiteTagIds(siteTagIds: number[]): Promise<ApiResponse<SiteTagVO[]>> {
  const result = await SiteTagHandler.ListBySiteTagIds(siteTagIds)
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? result.data.map(toSiteTagVO).filter((item): item is SiteTagVO => item !== null) : [] }
}

/**
 * 更新站点标签绑定的本地标签
 */
export async function siteTagUpdateBindLocalTag(
  localTagId: number,
  siteTagIds: number[]
): Promise<ApiResponse<boolean>> {
  const result = await SiteTagHandler.UpdateBindLocalTag(localTagId, siteTagIds)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '更新失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data }
}

/**
 * 创建并绑定同名的本地标签
 */
export async function siteTagCreateAndBindSameNameLocalTag(
  siteTag: SiteTagVO
): Promise<ApiResponse<boolean>> {
  const result = await SiteTagHandler.CreateAndBindSameNameLocalTag(new SiteTagParamDTO({
    id: siteTag.id,
    siteTagName: siteTag.siteTagName,
    siteId: null,
    siteTagId: null,
    description: null
  }))
  if (!result) {
    return { success: false, msg: '创建失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '创建失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data !== null }
}