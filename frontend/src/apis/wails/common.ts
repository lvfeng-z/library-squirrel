/**
 * Common Wails 绑定包装器
 * 通用方法：DirSelect, OpenPath, OpenExternal, GetVersion, Greet
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'
import { toApiResponse } from './index'
import type { OpenDialogResult } from '../../../bindings/github.com/library-squirrel/wails/internal/fileSysUtil/models'

/**
 * 打开目录/文件选择对话框
 * @param openFile true=选择文件, false=选择目录
 */
export async function dirSelect(openFile: boolean): Promise<ApiResponse<OpenDialogResult | null>> {
  return toApiResponse(App.DirSelect(openFile))
}

/**
 * 使用系统默认应用打开文件
 */
export async function openPath(path: string): Promise<ApiResponse<void>> {
  return toApiResponse(App.OpenPath(path))
}

/**
 * 在默认浏览器中打开 URL
 */
export async function openExternal(url: string): Promise<ApiResponse<void>> {
  return toApiResponse(App.OpenExternal(url))
}

/**
 * 获取应用版本
 */
export async function getVersion(): Promise<ApiResponse<string>> {
  return toApiResponse(App.GetVersion())
}

/**
 * 测试方法 - Greet
 */
export async function greet(name: string): Promise<ApiResponse<string>> {
  return toApiResponse(App.Greet(name))
}

// ========== HTTP API 兼容别名 ==========

/**
 * 打开目录/文件选择对话框 (兼容 HTTP API 版本)
 * @deprecated 使用 dirSelect 替代
 */
export async function fileSysUtilDirSelect(openFile: boolean, isModal: boolean): Promise<ApiResponse<OpenDialogResult | null>> {
  // Wails 版本没有 isModal 参数，直接调用
  return dirSelect(openFile)
}
