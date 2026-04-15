/**
 * ReWorkTag Wails 绑定包装器
 * 注意：这些方法在 Go 后端尚未实现，使用 stub 返回错误
 */

import type { ApiResponse } from '@apis/http'

// ========== 类型定义 ==========

export interface ReWorkTagVO {
  id: number
  workId: number
  tagType: number
  localTagId: number
  siteTagId: number
}

// ========== 错误响应辅助函数 ==========

function notImplemented(): ApiResponse<never> {
  return {
    success: false,
    msg: 'ReWorkTag methods not implemented in Wails backend',
    data: undefined
  }
}

// ========== API 方法 (Stub 实现) ==========

/**
 * 链接作品标签
 * @deprecated 使用 localTagApi.reWorkTagLink 替代（如果已实现）
 */
export async function reWorkTagLink(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  console.error('reWorkTagLink is not implemented in Wails backend')
  return notImplemented()
}

/**
 * 批量链接作品标签
 */
export async function reWorkTagLinkBatch(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  console.error('reWorkTagLinkBatch is not implemented in Wails backend')
  return notImplemented()
}

/**
 * 取消链接作品标签
 */
export async function reWorkTagUnlink(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  console.error('reWorkTagUnlink is not implemented in Wails backend')
  return notImplemented()
}

/**
 * 批量移除作品标签
 */
export async function reWorkTagRemoveBatch(
  workId: number,
  tagType: number,
  tagIds: number[]
): Promise<ApiResponse<boolean>> {
  console.error('reWorkTagRemoveBatch is not implemented in Wails backend')
  return notImplemented()
}

/**
 * 获取作品标签列表
 */
export async function reWorkTagList(workId: number): Promise<ApiResponse<ReWorkTagVO[]>> {
  console.error('reWorkTagList is not implemented in Wails backend')
  return notImplemented()
}
