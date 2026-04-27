import type { ApiResponse } from '@renderer/apis/http/types'
import { localAuthorApi } from '@renderer/apis/http'
import { SelectItem } from "@bindings/github.com/library-squirrel/wails/pkg/model/dto";
import type { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'
import IPage from '@renderer/model/util/IPage.ts'
import PageModel from '@renderer/model/util/Page.ts'

/**
 * 分页查询本地作者选择列表（适配器版本，供 AutoLoadSelect 使用）
 * @param page 分页信息
 * @param input 搜索关键字
 */
export async function localAuthorQuerySelectItemPageByName(
  page: IPage<SelectItem>,
  input: string
): Promise<IPage<SelectItem>> {
  const response = await localAuthorApi.localAuthorQuerySelectItemPage({
    page: page.pageNumber,
    pageSize: page.pageSize,
    query: { authorName: input }
  })
  if (!response.success || !response.data) {
    return new PageModel<SelectItem>()
  }
  return {
    pageNumber: response.data.pageNumber,
    pageSize: response.data.pageSize,
    pageCount: response.data.pageCount,
    dataCount: response.data.dataCount,
    currentCount: response.data.currentCount,
    data: response.data.data?.filter((item) => item !== null) as SelectItem[] ?? []
  }
}

/**
 * 分页查询本地作者选择列表
 * @param query
 */
export async function localAuthorQuerySelectItemPage(query: {
  page: number
  pageSize: number
  query?: Record<string, unknown>
}): Promise<ApiResponse<Page<SelectItem>>> {
  return localAuthorApi.localAuthorQuerySelectItemPage(query)
}
