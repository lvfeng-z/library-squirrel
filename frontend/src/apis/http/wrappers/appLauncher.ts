/**
 * AppLauncher HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as AppLauncherHandler, ExternalAppEnum } from '@bindings/github.com/library-squirrel/wails/internal/appLauncher'

export async function appLauncherOpenImage(filePath: string): Promise<ApiResponse<void>> {
  // 使用 OpenPath 打开图片文件
  const result = await AppLauncherHandler.OpenPath(filePath)
  if (!result) {
    return { success: false, msg: '打开失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}

export async function appLauncherOpen(path: string): Promise<ApiResponse<void>> {
  // ExternalAppEnum.$zero = 0 表示使用系统默认应用
  const result = await AppLauncherHandler.Open(ExternalAppEnum.$zero, path)
  if (!result) {
    return { success: false, msg: '打开失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '' }
}