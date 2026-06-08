/**
 * LocalAuthor HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import {
  Handler as LocalAuthorHandler,
  LocalAuthorQueryDTO
} from "@bindings/github.com/library-squirrel/backend/localAuthor"
import { LocalAuthorDTO, SelectItem } from "@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto"
import { Page } from "@bindings/github.com/library-squirrel/backend/base/model"
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'
import { isBlank } from '@renderer/utils/StringUtil.ts'
import { QueryAttribute } from "@bindings/github.com/library-squirrel/backend/base/query"
import IPage from '@renderer/model/util/IPage.ts'

// ========== API 方法 ==========

/** 保存本地作者 */
export async function localAuthorSave(author: LocalAuthorDTO): Promise<ApiResult<number>> {
  return requireResponse(await LocalAuthorHandler.Save(author), '保存本地作者', false)
}

/** 删除本地作者 */
export async function localAuthorDeleteById(id: number): Promise<ApiResult<any>> {
  return requireResponse(await LocalAuthorHandler.Delete(id), '删除本地作者', false)
}

/** 更新本地作者 */
export async function localAuthorUpdateById(author: LocalAuthorDTO): Promise<ApiResult<any>> {
  return requireResponse(await LocalAuthorHandler.Update(author), '更新本地作者', false)
}

/** 获取单个本地作者 */
export async function localAuthorGetById(id: number): Promise<ApiResult<LocalAuthorDTO>> {
  return requireResponse(await LocalAuthorHandler.GetById(id), '获取本地作者')
}

/** 分页查询本地作者 */
export async function localAuthorQueryPage(page: Page<LocalAuthorDTO>, query: LocalAuthorQueryDTO): Promise<ApiResult<Page<LocalAuthorDTO>>> {
  return requireResponse(await LocalAuthorHandler.QueryPage(page, query), '查询本地作者')
}

/** 查询选择项列表 */
export async function localAuthorListSelectItems(queryDTO?: LocalAuthorQueryDTO): Promise<ApiResult<(SelectItem | null)[]>> {
  return requireResponse(await LocalAuthorHandler.ListSelectItems(queryDTO ?? new LocalAuthorQueryDTO()), '获取本地作者选择列表')
}

/** 分页查询选择项 */
export async function localAuthorQuerySelectItemPage(page: Page<SelectItem>, query: LocalAuthorQueryDTO): Promise<ApiResult<Page<SelectItem>>> {
  return requireResponse(await LocalAuthorHandler.QuerySelectItemPage(page, query), '查询本地作者选择列表')
}

/** 根据作品ID获取作者列表 */
export async function localAuthorListByWorkId(workId: number): Promise<ApiResult<any>> {
  return requireResponse(await LocalAuthorHandler.ListByWorkId(workId), '获取作品作者')
}

/** 更新最后使用时间 */
export async function localAuthorUpdateLastUse(ids: number[]): Promise<ApiResult<any>> {
  return requireResponse(await LocalAuthorHandler.UpdateLastUse(ids), '更新使用时间', false)
}

// ========== 适配器方法（供 AutoLoadSelect 等组件使用） ==========

/**
 * 分页查询本地作者选择列表（供 AutoLoadSelect 使用）
 * 将输入关键词映射为 QueryAttribute 传递给后端
 */
export async function localAuthorQuerySelectItemPageByName(
  page: IPage<SelectItem>,
  input: string
): Promise<IPage<SelectItem>> {
  const queryDTO = new LocalAuthorQueryDTO({
    authorName: isBlank(input) ? undefined : new QueryAttribute({ value: input })
  })
  const response = await localAuthorQuerySelectItemPage(page, queryDTO)
  return response.data
}
