/**
 * SiteTag HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import {
  Handler as SiteTagHandler,
  SiteTagQueryDTO
} from "@bindings/github.com/library-squirrel/wails/internal/siteTag"
import { LocalTagDTO, SelectItem, SiteTagDTO, SiteTagFullDTO, SiteTagLocalRelateDTO } from "@bindings/github.com/library-squirrel/wails/pkg/model/dto"
import { Page } from "@bindings/github.com/library-squirrel/wails/pkg/model"
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

// ========== API 方法 ==========

/** 保存站点标签 */
export async function siteTagSave(tag: SiteTagDTO): Promise<ApiResult<number>> {
  return requireResponse(await SiteTagHandler.Save(tag), '保存站点标签', false)
}

/** 批量保存站点标签 */
export async function siteTagSaveBatch(tags: SiteTagDTO[]): Promise<ApiResult<any>> {
  return requireResponse(await SiteTagHandler.SaveBatch(tags), '批量保存站点标签', false)
}

/** 删除站点标签 */
export async function siteTagDeleteById(id: number): Promise<ApiResult<any>> {
  return requireResponse(await SiteTagHandler.Delete(id), '删除站点标签', false)
}

/** 更新站点标签 */
export async function siteTagUpdateById(tag: SiteTagDTO): Promise<ApiResult<any>> {
  return requireResponse(await SiteTagHandler.Update(tag), '更新站点标签', false)
}

/** 获取单个站点标签 */
export async function siteTagGetById(id: number): Promise<ApiResult<SiteTagDTO>> {
  return requireResponse(await SiteTagHandler.GetById(id), '获取站点标签')
}

/** 分页查询站点标签 */
export async function siteTagQueryPage(page: Page<SiteTagDTO>, query: SiteTagQueryDTO): Promise<ApiResult<Page<SiteTagDTO>>> {
  return requireResponse(await SiteTagHandler.QueryPage(page, query), '查询站点标签')
}

/** 查询绑定或未绑定到本地标签的站点标签分页 */
export async function siteTagQueryBoundOrUnboundToLocalTagPage(page: Page<SiteTagFullDTO>, query: SiteTagQueryDTO): Promise<ApiResult<Page<SiteTagFullDTO>>> {
  return requireResponse(await SiteTagHandler.QueryBoundOrUnboundToLocalTagPage(page, query), '查询站点标签')
}

/** 根据作品ID查询站点标签分页 */
export async function siteTagQueryPageByWorkId(workId: number, page: Page<SiteTagFullDTO>, query: SiteTagQueryDTO): Promise<ApiResult<Page<SiteTagFullDTO>>> {
  return requireResponse(await SiteTagHandler.QueryPageByWorkId(page, query, workId), '查询作品站点标签')
}

/** 查询本地关联的站点标签分页 */
export async function siteTagQueryLocalRelateDTOPage(page: Page<SiteTagLocalRelateDTO>, query: SiteTagQueryDTO): Promise<ApiResult<Page<SiteTagLocalRelateDTO>>> {
  return requireResponse(await SiteTagHandler.QueryLocalRelateDTOPage(page, query), '查询站点标签')
}

/** 根据作品ID查询选择项分页 */
export async function siteTagQuerySelectItemPageByWorkId(workId: number, page: Page<SelectItem>, query: SiteTagQueryDTO): Promise<ApiResult<Page<SelectItem>>> {
  return requireResponse(await SiteTagHandler.QuerySelectItemPageByWorkId(page, query, workId), '查询作品标签选择列表')
}

/** 根据作品ID获取标签列表 */
export async function siteTagListByWorkId(workId: number): Promise<ApiResult<(SiteTagDTO | null)[]>> {
  return requireResponse(await SiteTagHandler.ListByWorkId(workId), '获取作品标签')
}

/** 根据站点标签ID列表获取站点标签 */
export async function siteTagListBySiteTagIds(siteTagIds: number[]): Promise<ApiResult<(SiteTagDTO | null)[]>> {
  return requireResponse(await SiteTagHandler.ListBySiteTagIds(siteTagIds), '获取站点标签')
}

/** 更新站点标签绑定的本地标签 */
export async function siteTagUpdateBindLocalTag(localTagId: number | null, siteTagIds: number[]): Promise<ApiResult<boolean>> {
  return requireResponse(await SiteTagHandler.UpdateBindLocalTag(localTagId, siteTagIds), '更新本地标签绑定')
}

/** 创建并绑定同名的本地标签 */
export async function siteTagCreateAndBindSameNameLocalTag(siteTag: SiteTagDTO): Promise<ApiResult<LocalTagDTO>> {
  return requireResponse(await SiteTagHandler.CreateAndBindSameNameLocalTag(siteTag), '创建同名本地标签')
}

/** 更新最后使用时间 */
export async function siteTagUpdateLastUse(ids: number[]): Promise<ApiResult<any>> {
  return requireResponse(await SiteTagHandler.UpdateLastUse(ids), '更新使用时间', false)
}
