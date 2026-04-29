/**
 * LocalTag HTTP API 包装器
 * 封装 Wails 绑定层响应校验，将 ApiResponse<T | null> | null 转换为 ApiResult<T>（data 保证非空）
 * 校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import {
  Handler as LocalTagHandler,
  LocalTagQueryDTO
} from '@bindings/github.com/library-squirrel/wails/internal/localTag'
import { LocalTagDTO, SelectItem, LocalTagWithBaseTagDTO } from "@bindings/github.com/library-squirrel/wails/pkg/model/dto"
import { Page, ApiResponse } from "@bindings/github.com/library-squirrel/wails/pkg/model"
import { QueryAttribute } from "@bindings/github.com/library-squirrel/wails/pkg/query/models"
import type { ApiResult } from '@renderer/apis/http/types'
import IPage from '@renderer/model/util/IPage.ts'
import { isBlank } from '@renderer/utils/StringUtil.ts'
import { isNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 校验 Wails 绑定层响应
 * 1. 外层 null 检查（响应为空）
 * 2. success 检查（后端返回失败）
 * 3. data 非空检查
 * 校验通过返回 ApiResult<T>（data 保证非空）
 * 校验失败抛出 Error，调用方通过 try/catch 捕获
 */
function requireResponse<T>(
  response: ApiResponse<T | null> | null,
  operation: string
): ApiResult<T> {
  if (!response) throw new Error(`${operation}：接口返回为空`)
  if (!response.success) throw new Error(response.msg || `${operation}：操作失败`)
  if (isNullish(response.data)) throw new Error(`${operation}：未返回数据`)
  return response as unknown as ApiResult<T>
}

// ========== API 方法 ==========

/** 保存本地标签 */
export async function localTagSave(tag: LocalTagDTO): Promise<ApiResult<number>> {
  return requireResponse(await LocalTagHandler.Save(tag), '保存本地标签')
}

/** 删除本地标签 */
export async function localTagDeleteById(id: number): Promise<ApiResult<any>> {
  return requireResponse(await LocalTagHandler.Delete(id), '删除本地标签')
}

/** 更新本地标签 */
export async function localTagUpdateById(tag: LocalTagDTO): Promise<ApiResult<any>> {
  return requireResponse(await LocalTagHandler.Update(tag), '更新本地标签')
}

/** 获取单个本地标签 */
export async function localTagGetById(id: number): Promise<ApiResult<LocalTagDTO>> {
  return requireResponse(await LocalTagHandler.GetById(id), '获取本地标签')
}

/** 分页查询本地标签 */
export async function localTagQueryPage(page: Page<LocalTagDTO>, query: LocalTagQueryDTO): Promise<ApiResult<Page<LocalTagDTO>>> {
  return requireResponse(await LocalTagHandler.QueryPage(page, query), '查询本地标签')
}

/** 获取本地标签树 */
export async function localTagGetTree(rootId?: number, depth?: number): Promise<ApiResult<(LocalTagDTO | null)[]>> {
  return requireResponse(await LocalTagHandler.GetTree(rootId ?? 0, depth ?? 10), '获取标签树')
}

/** 获取选择项列表 */
export async function localTagListSelectItems(queryDTO?: LocalTagQueryDTO): Promise<ApiResult<(SelectItem | null)[]>> {
  return requireResponse(await LocalTagHandler.ListSelectItems(queryDTO ?? new LocalTagQueryDTO({})), '获取标签选择列表')
}

/** 分页查询选择项 */
export async function localTagQuerySelectItemPage(page: Page<SelectItem>, query: LocalTagQueryDTO, secondaryLabel: string): Promise<ApiResult<Page<SelectItem>>> {
  return requireResponse(await LocalTagHandler.QuerySelectItemPage(page, query, secondaryLabel), '查询标签选择列表')
}

/** 根据作品ID获取标签列表 */
export async function localTagListByWorkId(workId: number): Promise<ApiResult<(LocalTagDTO | null)[]>> {
  return requireResponse(await LocalTagHandler.ListByWorkId(workId), '获取作品标签')
}

/** 根据作品ID分页查询选择项 */
export async function localTagQuerySelectItemPageByWorkId(page: Page<SelectItem>, query: LocalTagQueryDTO): Promise<ApiResult<Page<SelectItem>>> {
  return requireResponse(await LocalTagHandler.QuerySelectItemPageByWorkId(page, query), '查询作品标签选择列表')
}

/** 分页查询包含基础标签信息的本地标签 */
export async function localTagQueryWithBaseTagPage(page: Page<LocalTagWithBaseTagDTO>, query: LocalTagQueryDTO): Promise<ApiResult<Page<LocalTagWithBaseTagDTO>>> {
  return requireResponse(await LocalTagHandler.QueryWithBaseTagPage(page, query), '查询本地标签')
}

// ========== 适配器方法（供 AutoLoadSelect 等组件使用） ==========

/**
 * 分页查询本地标签选择列表（供 AutoLoadSelect 使用）
 * 将输入关键词映射为 QueryAttribute 传递给后端
 */
export async function localTagQuerySelectItemPageByName(
  page: IPage<SelectItem>,
  input: string
): Promise<IPage<SelectItem>> {
  const queryDTO = new LocalTagQueryDTO({
    localTagName: isBlank(input) ? undefined : new QueryAttribute({ value: input })
  })
  const pageObj = new Page<SelectItem>({
    pageNumber: page.pageNumber,
    pageSize: page.pageSize
  })
  const response = await localTagQuerySelectItemPage(pageObj, queryDTO, '')
  return response.data
}
