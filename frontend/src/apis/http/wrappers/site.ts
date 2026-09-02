/**
 * Site HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import {
  Handler as SiteHandler,
  SiteQueryDTO
} from '@bindings/github.com/library-squirrel/backend/site'
import { SiteDTO } from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto'
import { SelectItem } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { Page } from '@bindings/github.com/library-squirrel/backend/base/model'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'
import IPage from '@renderer/model/util/IPage.ts'

// ========== API 方法 ==========

/** 删除站点 */
export async function siteDeleteById(id: number): Promise<ApiResult<any>> {
  return requireResponse(await SiteHandler.Delete(id), '删除站点', false)
}

/** 更新站点 */
export async function siteUpdateById(site: SiteDTO): Promise<ApiResult<any>> {
  return requireResponse(await SiteHandler.Update(site), '更新站点', false)
}

/** 分页查询站点 */
export async function siteQueryPage(page: Page<SiteDTO>, query: SiteQueryDTO): Promise<ApiResult<Page<SiteDTO>>> {
  return requireResponse(await SiteHandler.QueryPage(page, query), '查询站点')
}

/** 分页查询站点选择列表 */
export async function siteQuerySelectItemPage(page: Page<SelectItem>, query: SiteQueryDTO): Promise<ApiResult<Page<SelectItem>>> {
  return requireResponse(await SiteHandler.QuerySelectItemPage(page, query), '查询站点选择列表')
}

/**
 * 分页查询站点选择列表（适配器版本，供 AutoLoadSelect 使用）
 * @param page 分页信息
 * @param _input 搜索关键字（站点名称过滤在 bindings 中未实现）
 */
export async function siteQuerySelectItemPageBySiteName(
  page: IPage<SelectItem>,
  _input: string
): Promise<IPage<SelectItem>> {
  const pageArg = new Page<SelectItem>({ pageNumber: page.pageNumber, pageSize: page.pageSize })
  const response = await siteQuerySelectItemPage(pageArg, new SiteQueryDTO({}))
  const responseData = response.data
  return {
    pageNumber: responseData.pageNumber,
    pageSize: responseData.pageSize,
    pageCount: responseData.pageCount,
    dataCount: responseData.dataCount,
    currentCount: responseData.currentCount,
    data: responseData.data?.filter((item) => item !== null) as SelectItem[] ?? []
  }
}
