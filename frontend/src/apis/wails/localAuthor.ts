/**
 * LocalAuthor Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { LocalAuthor } from '../../../bindings/github.com/library-squirrel/wails/internal/model/models'
import type { ApiResponse } from '@/apis/http'

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
}): Promise<ApiResponse<number>> {
  return App.LocalAuthorSave(author as any)
}

/**
 * 删除本地作者
 */
export async function localAuthorDeleteById(id: number): Promise<ApiResponse<void>> {
  return App.LocalAuthorDeleteById(id)
}

/**
 * 更新本地作者
 */
export async function localAuthorUpdateById(author: {
  id: number
  localAuthorName?: string
}): Promise<ApiResponse<void>> {
  return App.LocalAuthorUpdateById(author as any)
}

/**
 * 获取单个本地作者
 */
export async function localAuthorGetById(id: number): Promise<ApiResponse<LocalAuthorVO | null>> {
  const result = await App.LocalAuthorGetById(id)
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
export async function localAuthorQueryPage(query: any): Promise<ApiResponse<any>> {
  return App.LocalAuthorQueryPage(query)
}

/**
 * 获取选择项列表
 */
export async function localAuthorListSelectItems(query?: any): Promise<ApiResponse<any[]>> {
  return App.LocalAuthorListSelectItems(query)
}

/**
 * 分页查询选择项
 */
export async function localAuthorQuerySelectItemPage(query: any): Promise<ApiResponse<any>> {
  return App.LocalAuthorQuerySelectItemPage(query)
}
