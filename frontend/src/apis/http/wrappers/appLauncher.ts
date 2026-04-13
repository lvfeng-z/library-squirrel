/**
 * AppLauncher HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export async function appLauncherOpenImage(filePath: string): Promise<ApiResponse<void>> {
  return apiProxy.invoke<void>('appLauncher-openImage', { url: filePath })
}

export async function appLauncherOpen(path: string): Promise<ApiResponse<void>> {
  // ExternalAppEnum = 0 表示使用系统默认应用
  return apiProxy.invoke<void>('appLauncher-open', { app: 0, filePath: path })
}
