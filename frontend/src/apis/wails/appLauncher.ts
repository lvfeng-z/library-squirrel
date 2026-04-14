/**
 * AppLauncher Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'
import { toApiResponse } from './index'
import type { ExternalAppEnum } from '../../../bindings/github.com/library-squirrel/wails/internal/appLauncher/models'

// ========== API 方法 ==========

/**
 * 打开图片
 */
export async function appLauncherOpenImage(url: string): Promise<ApiResponse<void>> {
  return toApiResponse(App.AppLauncherOpenImage(url))
}

/**
 * 打开文件或链接
 */
export async function appLauncherOpen(appEnum: ExternalAppEnum, filePath: string): Promise<ApiResponse<void>> {
  return toApiResponse(App.AppLauncherOpen(appEnum, filePath))
}
