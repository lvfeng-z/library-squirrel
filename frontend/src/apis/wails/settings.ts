/**
 * Settings Wails 绑定包装器
 */

import { App } from '../../../bindings/github.com/library-squirrel/wails'
import type { ApiResponse } from '@/apis/http'

// ========== API 方法 ==========

/**
 * 获取设置
 */
export async function settingsGetSettings(): Promise<ApiResponse<any>> {
  return App.SettingsGetSettings()
}

/**
 * 保存设置
 */
export async function settingsSaveSettings(settings: any): Promise<ApiResponse<void>> {
  return App.SettingsSaveSettings(settings)
}

/**
 * 重置设置
 */
export async function settingsResetSettings(): Promise<ApiResponse<void>> {
  return App.SettingsResetSettings()
}
