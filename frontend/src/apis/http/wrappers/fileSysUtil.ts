/**
 * FileSysUtil HTTP API 包装器
 * 提供目录/文件选择功能的 HTTP API 调用
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

// ========== 类型定义 ==========

/**
 * 打开对话框结果
 */
export interface OpenDialogResult {
  canceled: boolean
  filePaths: string[]
}

// ========== API 方法 ==========

/**
 * 打开目录/文件选择对话框
 * @param openFile true=选择文件, false=选择目录
 * @param isModal 是否模态对话框
 */
export async function fileSysUtilDirSelect(openFile: boolean, isModal: boolean): Promise<ApiResponse<OpenDialogResult>> {
  return apiProxy.invoke<OpenDialogResult>('fileSysUtil-dirSelect', { openFile, isModal })
}
