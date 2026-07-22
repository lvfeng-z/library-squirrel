/**
 * AppLauncher HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as AppLauncherHandler } from '@bindings/github.com/library-squirrel/backend/appLauncher'

export async function appLauncherOpenImage(filePath: string): Promise<ApiResponse<void>> {
  // 使用 OpenImage 打开图片资源（filePath 为相对路径）
  const result = await AppLauncherHandler.OpenImage(filePath)
  if (!result) {
    return { success: false, msg: '打开失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function appLauncherOpen(path: string): Promise<ApiResponse<void>> {
  // 系统默认应用打开(OpenPath:Windows cmd /c start / macOS open / Linux xdg-open)
  const result = await AppLauncherHandler.OpenPath(path)
  if (!result) {
    return { success: false, msg: '打开失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function appLauncherOpenExternal(url: string): Promise<ApiResponse<void>> {
  const result = await AppLauncherHandler.OpenExternal(url)
  if (!result) {
    return { success: false, msg: '打开失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}