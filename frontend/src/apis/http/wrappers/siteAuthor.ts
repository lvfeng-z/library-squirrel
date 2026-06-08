/**
 * SiteAuthor HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import {
  Handler as SiteAuthorHandler,
  SiteAuthorQueryDTO
} from '@bindings/github.com/library-squirrel/backend/siteAuthor'
import { SiteAuthorDTO, SiteAuthorLocalRelateDTO } from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

// ========== API 方法 ==========

/** 保存站点作者 */
export async function siteAuthorSave(author: SiteAuthorDTO): Promise<ApiResult<number>> {
  return requireResponse(await SiteAuthorHandler.Save(author), '保存站点作者', false)
}

/** 删除站点作者 */
export async function siteAuthorDeleteById(id: number): Promise<ApiResult<any>> {
  return requireResponse(await SiteAuthorHandler.Delete(id), '删除站点作者', false)
}

/** 更新站点作者 */
export async function siteAuthorUpdateById(author: SiteAuthorDTO): Promise<ApiResult<any>> {
  return requireResponse(await SiteAuthorHandler.Update(author), '更新站点作者', false)
}

/** 分页查询站点作者 */
export async function siteAuthorQueryPage(page: Page<SiteAuthorDTO>, query: SiteAuthorQueryDTO): Promise<ApiResult<Page<SiteAuthorDTO>>> {
  return requireResponse(await SiteAuthorHandler.QueryPage(page, query), '查询站点作者')
}

/** 查询绑定或未绑定到本地作者的站点作者分页 */
export async function siteAuthorQueryBoundOrUnboundInLocalAuthorPage(page: Page<SiteAuthorLocalRelateDTO>, query: SiteAuthorQueryDTO): Promise<ApiResult<Page<SiteAuthorLocalRelateDTO>>> {
  return requireResponse(await SiteAuthorHandler.QueryBoundOrUnboundToLocalAuthorPage(page, query), '查询站点作者')
}

/** 查询本地关联的站点作者分页 */
export async function siteAuthorQueryLocalRelateDTOPage(page: Page<SiteAuthorLocalRelateDTO>, query: SiteAuthorQueryDTO): Promise<ApiResult<Page<SiteAuthorLocalRelateDTO>>> {
  return requireResponse(await SiteAuthorHandler.QueryLocalRelateDTOPage(page, query), '查询站点作者')
}

/** 更新站点作者绑定的本地作者 */
export async function siteAuthorUpdateBindLocalAuthor(
  localAuthorId: number | null,
  siteAuthorIds: number[]
): Promise<ApiResult<boolean>> {
  return requireResponse(await SiteAuthorHandler.UpdateBindLocalAuthor(localAuthorId, siteAuthorIds), '更新本地作者绑定', false)
}

/** 创建并绑定同名的本地作者 */
export async function siteAuthorCreateAndBindSameNameLocalAuthor(
  siteAuthor: SiteAuthorDTO
): Promise<ApiResult<boolean>> {
  return requireResponse(await SiteAuthorHandler.CreateAndBindSameNameLocalAuthor(siteAuthor), '创建同名本地作者', false)
}
