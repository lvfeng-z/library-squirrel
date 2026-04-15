/**
 * Settings Wails 绑定包装器
 */

import { Handler } from "@bindings/github.com/library-squirrel/wails/internal/settings";
import type { ApiResponse } from '@apis/http'

// ========== API 方法 ==========

/**
 * 获取设置
 */
export async function settingsGetSettings(): Promise<ApiResponse<any>> {
  return Handler.Get()
}

/**
 * 保存设置
 */
export async function settingsSaveSettings(settings: any): Promise<ApiResponse<void>> {
  return Handler.Save(settings)
}

/**
 * 重置设置
 */
export async function settingsResetSettings(): Promise<ApiResponse<void>> {
  return Handler.Reset()
}
