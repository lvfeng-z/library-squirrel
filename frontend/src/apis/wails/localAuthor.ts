/**
 * LocalAuthor Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/localAuthor'
import type { LocalAuthor } from '@bindings/github.com/library-squirrel/wails/internal/model/models'
import type { ApiResponse } from '@apis/http'

// ========== 类型定义 ==========

export interface LocalAuthorVO {
  id: number
  localAuthorName: string
  createTime: number
  updateTime: number
}

// ========== 工具函数 ==========

function nullLocalAuthorToVO(author: LocalAuthor | null): LocalAuthorVO | null {
  if (!author) return null
  return {
    id: author.id,
    localAuthorName: author.authorName?.String ?? '',
    createTime: author.createTime,
    updateTime: author.updateTime
  }
}

// ========== API 方法 ==========

/**
 * 保存本地作者
 */
export async function localAuthorSave(author: {
  localAuthorName?: string
}): Promise<ApiResponse<number> | null> {
  return Handler.Save(author as any)
}

/**
 * 删除本地作者
 */
export async function localAuthorDeleteById(id: number): Promise<ApiResponse<void> | null> {
  return Handler.Delete(id)
}

/**
 * 更新本地作者
 */
export async function localAuthorUpdateById(author: {
  id: number
  localAuthorName?: string
}): Promise<ApiResponse<void> | null> {
  return Handler.Update(author as any)
}

/**
 * 获取单个本地作者
 */
export async function localAuthorGetById(id: number): Promise<ApiResponse<LocalAuthorVO | null> | null> {
  const result = await Handler.GetById(id)
  if (result.success && result.data) {
    return {
      ...result,
      data: nullLocalAuthorToVO(result.data)!
    }
  }
  return result as unknown as ApiResponse<LocalAuthorVO | null>
}

/**
 * 分页查询本地作者
 */
export async function localAuthorQueryPage(query: any): Promise<ApiResponse<any> | null> {
  return Handler.QueryPage(query.page ?? 1, query.pageSize ?? 10, query)
}

/**
 * 获取选择项列表
 */
export async function localAuthorListSelectItems(query?: any): Promise<ApiResponse<any[]> | null> {
  return Handler.ListSelectItems(query)
}

/**
 * 分页查询选择项
 */
export async function localAuthorQuerySelectItemPage(query: any): Promise<ApiResponse<any> | null> {
  return Handler.QuerySelectItemPage(query.page ?? 1, query.pageSize ?? 10, query.query, '')
}