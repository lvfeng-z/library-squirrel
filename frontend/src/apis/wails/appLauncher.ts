/**
 * AppLauncher Wails 绑定包装器
 */

import { Handler } from '@bindings/github.com/library-squirrel/wails/internal/appLauncher'
import type { ApiResponse } from '@apis/http'
import type { ExternalAppEnum } from '@bindings/github.com/library-squirrel/wails/internal/appLauncher/models'

// ========== API 方法 ==========

/**
 * 打开文件或链接
 */
export async function appLauncherOpen(appEnum: ExternalAppEnum, filePath: string): Promise<ApiResponse<void> | null> {
  return Handler.Open(appEnum, filePath)
}