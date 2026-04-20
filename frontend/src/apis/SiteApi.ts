import type { ApiResponse } from '@renderer/apis/http/types'
import { siteApi } from '@renderer/apis/http'
import type { SelectItem } from '@bindings/github.com/library-squirrel/wails/internal/model/models'
import type { SiteQueryDTO } from '@bindings/github.com/library-squirrel/wails/internal/site'
import type { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'
import IPage from '@renderer/model/util/IPage.ts'
import PageModel from '@renderer/model/util/Page.ts'

/**
 * 分页查询站点选择列表（适配器版本，供 AutoLoadSelect 使用）
 * @param page 分页信息
 * @param _input 搜索关键字（站点名称过滤在 bindings 中未实现）
 */
export async function siteQuerySelectItemPageBySiteName(
  page: IPage<unknown, SelectItem>,
  _input: string
): Promise<IPage<unknown, SelectItem>> {
  const response = await siteApi.siteQuerySelectItemPage({
    page: page.pageNumber,
    pageSize: page.pageSize
  })
  if (!response.success || !response.data) {
    return new PageModel<unknown, SelectItem>()
  }
  return {
    pageNumber: response.data.pageNumber,
    pageSize: response.data.pageSize,
    pageCount: response.data.pageCount,
    dataCount: response.data.dataCount,
    currentCount: response.data.currentCount,
    query: response.data.query,
    data: response.data.data?.filter((item) => item !== null) as SelectItem[] ?? []
  }
}

/**
 * 分页查询站点选择列表
 * @param query
 */
export async function siteQuerySelectItemPage(query: {
  page: number
  pageSize: number
}): Promise<ApiResponse<Page<SelectItem, SiteQueryDTO> | null>> {
  return siteApi.siteQuerySelectItemPage(query)
}
