import type { ApiResponse } from '@renderer/apis/http/types'
import { siteApi } from '@renderer/apis/http'
import type { SelectItem } from '@bindings/github.com/library-squirrel/wails/pkg/model/dto'
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
  page: IPage<SelectItem, SiteQueryDTO>,
  _input: string
): Promise<IPage<SelectItem, SiteQueryDTO>> {
  const response = await siteApi.siteQuerySelectItemPage(page)
  if (!response.success || !response.data) {
    return new PageModel<SelectItem, SiteQueryDTO>()
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
 * @param page
 */
export async function siteQuerySelectItemPage(page: Page<SelectItem, SiteQueryDTO>): Promise<ApiResponse<Page<SelectItem, SiteQueryDTO> | null>> {
  return siteApi.siteQuerySelectItemPage(page)
}
