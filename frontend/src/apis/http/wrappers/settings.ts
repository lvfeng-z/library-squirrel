/**
 * Settings HTTP API 包装器
 */

import { apiProxy } from '../proxy'
import type { ApiResponse } from '../types'

export interface SettingsVO {
  // 根据实际 settings 结构定义
  [key: string]: unknown
}

export async function settingsGetSettings(): Promise<ApiResponse<SettingsVO>> {
  return apiProxy.invoke<SettingsVO>('settings-getSettings')
}

export async function settingsSaveSettings(settings: SettingsVO): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('settings-saveSettings', settings)
}

export async function settingsResetSettings(): Promise<ApiResponse<boolean>> {
  return apiProxy.invoke<boolean>('settings-resetSettings')
}
