/**
 * SiteAuthor HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import {
  Handler as SiteAuthorHandler,
  SiteAuthorQueryDTO
} from '@bindings/github.com/library-squirrel/wails/internal/siteAuthor'
import {SiteAuthorDTO, SiteAuthorLocalRelateDTO} from "@bindings/github.com/library-squirrel/wails/pkg/model/dto";
import {Page} from "@bindings/github.com/library-squirrel/wails/pkg/model";

// ========== API 方法 ==========

export async function siteAuthorSave(author: SiteAuthorDTO): Promise<ApiResponse<SiteAuthorDTO>> {
  const result = await SiteAuthorHandler.Save(author)
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

export async function siteAuthorUpdateById(author: SiteAuthorDTO): Promise<ApiResponse<SiteAuthorDTO>> {
  const result = await SiteAuthorHandler.Update(author)
  if (!result) {
    return { success: false, msg: '更新失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function siteAuthorQueryPage(page: Page<SiteAuthorDTO>, query: SiteAuthorQueryDTO): Promise<ApiResponse<Page<SiteAuthorDTO>>> {
  const result = await SiteAuthorHandler.QueryPage(page, query)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 查询绑定或未绑定到本地作者的站点作者分页
 */
export async function siteAuthorQueryBoundOrUnboundInLocalAuthorPage(page: Page<SiteAuthorLocalRelateDTO>, query: SiteAuthorQueryDTO): Promise<ApiResponse<Page<SiteAuthorLocalRelateDTO>>> {
  const result = await SiteAuthorHandler.QueryBoundOrUnboundToLocalAuthorPage(page, query)
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
export async function siteAuthorQueryLocalRelateDTOPage(page: Page<SiteAuthorLocalRelateDTO>, query: SiteAuthorQueryDTO): Promise<ApiResponse<Page<SiteAuthorLocalRelateDTO>>> {
  const result = await SiteAuthorHandler.QueryLocalRelateDTOPage(page, query)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ?? undefined }
}

/**
 * 更新站点作者绑定的本地作者
 */
export async function siteAuthorUpdateBindLocalAuthor(
  localAuthorId: number | null,
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
  siteAuthor: SiteAuthorDTO
): Promise<ApiResponse<boolean>> {
  const result = await SiteAuthorHandler.CreateAndBindSameNameLocalAuthor(siteAuthor)
  if (!result) {
    return { success: false, msg: '创建失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '创建失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data }
}
