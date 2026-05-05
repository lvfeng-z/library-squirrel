/**
 * FileSysUtil HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as FileSysUtilHandler } from '@bindings/github.com/library-squirrel/wails/backend/fileSysUtil'
import type { OpenDialogResult as BindingOpenDialogResult } from '@bindings/github.com/library-squirrel/wails/backend/fileSysUtil/models'

// ========== 类型定义 ==========

/**
 * 打开对话框结果
 */
export interface OpenDialogResult {
  canceled: boolean
  filePaths: string[]
}

// ========== 工具函数 ==========

/**
 * 将 Binding 的 OpenDialogResult 转换为我们的格式
 */
function toOpenDialogResult(dto: BindingOpenDialogResult | null): OpenDialogResult | null {
  if (!dto) return null
  return {
    canceled: dto.canceled ?? false,
    filePaths: dto.filePaths ?? []
  }
}

// ========== API 方法 ==========

/**
 * 打开目录/文件选择对话框
 * @param openFile true=选择文件, false=选择目录
 * @param isModal 是否模态对话框
 */
export async function fileSysUtilDirSelect(openFile: boolean, isModal: boolean): Promise<ApiResponse<OpenDialogResult>> {
  const result = await FileSysUtilHandler.DirSelect(openFile, isModal)
  if (!result) {
    return { success: false, msg: '选择失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '选择失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? (toOpenDialogResult(result.data) ?? undefined) : undefined }
}