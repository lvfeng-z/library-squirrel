import type { ApiResponse } from '@renderer/apis/http/types'
import { localTagApi } from '@renderer/apis/http'
import { SelectItem } from "@bindings/github.com/library-squirrel/wails/pkg/model/dto";
import type { LocalTagQueryDTO } from '@bindings/github.com/library-squirrel/wails/internal/localTag'
import type { Page } from '@bindings/github.com/library-squirrel/wails/pkg/model/models'
import IPage from '@renderer/model/util/IPage.ts'
import PageModel from '@renderer/model/util/Page.ts'

/**
 * 分页查询本地标签选择列表
 * @param query
 */
export async function localTagQuerySelectItemPage(query: {
  page: number
  pageSize: number
  query?: Record<string, unknown>
}): Promise<ApiResponse<Page<SelectItem, LocalTagQueryDTO>>> {
  return localTagApi.localTagQuerySelectItemPage(query)
}

/**
 * 分页查询本地标签选择列表（适配器版本，供 AutoLoadSelect 使用）
 * @param page 分页信息
 * @param input 搜索关键字
 */
export async function localTagQuerySelectItemPageByName(
  page: IPage<SelectItem, unknown>,
  input: string
): Promise<IPage<SelectItem, unknown>> {
  const response = await localTagApi.localTagQuerySelectItemPage({
    page: page.pageNumber,
    pageSize: page.pageSize,
    query: { localTagName: input }
  })
  if (!response.success || !response.data) {
    return new PageModel<SelectItem, unknown>()
  }
  return response.data
}
