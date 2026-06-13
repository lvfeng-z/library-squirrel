/**
 * FileSysUtil HTTP API 包装器
 * 统一的文件/目录选择对话框接口
 */

import type { ApiResponse } from '../types'
import {
  Handler as FileSysUtilHandler,
  OpenDialogOptions
} from '@bindings/github.com/library-squirrel/backend/fileSysUtil'
import type { OpenDialogResult as BindingOpenDialogResult } from '@bindings/github.com/library-squirrel/backend/fileSysUtil/models'
import { FileFilter } from '@bindings/github.com/wailsapp/wails/v3/pkg/application/models'

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
 * 将 Binding 的 OpenDialogResult 转换为本地格式
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
 * 打开文件/目录选择对话框（通用方法）
 */
export async function fileSysUtilOpenDialog(options: OpenDialogOptions): Promise<ApiResponse<OpenDialogResult>> {
  const result = await FileSysUtilHandler.OpenDialog(options)
  if (!result) {
    return { success: false, msg: '选择失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '选择失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? (toOpenDialogResult(result.data) ?? undefined) : undefined }
}

// ========== 便捷方法 ==========

/**
 * 选择目录
 */
export async function fileSysUtilSelectDirectory(title?: string, defaultPath?: string): Promise<ApiResponse<OpenDialogResult>> {
  return fileSysUtilOpenDialog({
    title: title ?? '选择目录',
    defaultPath: defaultPath ?? '',
    canChooseFiles: false,
    canChooseDirs: true,
    multiSelect: false,
    filters: []
  })
}

/**
 * 选择文件
 */
export async function fileSysUtilSelectFile(title?: string, defaultPath?: string, filters?: FileFilter[]): Promise<ApiResponse<OpenDialogResult>> {
  return fileSysUtilOpenDialog({
    title: title ?? '选择文件',
    defaultPath: defaultPath ?? '',
    canChooseFiles: true,
    canChooseDirs: false,
    multiSelect: false,
    filters: filters ?? []
  })
}

/**
 * 选择文件或目录
 */
export async function fileSysUtilSelectFileOrDirectory(title?: string, defaultPath?: string): Promise<ApiResponse<OpenDialogResult>> {
  return fileSysUtilOpenDialog({
    title: title ?? '选择文件或目录',
    defaultPath: defaultPath ?? '',
    canChooseFiles: true,
    canChooseDirs: true,
    multiSelect: false,
    filters: []
  })
}
