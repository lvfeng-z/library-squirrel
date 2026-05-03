/**
 * Settings HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as SettingsHandler } from '@bindings/github.com/library-squirrel/wails/internal/settings'
import type { SettingChange, Settings } from '@bindings/github.com/library-squirrel/wails/internal/settings/models'

export interface SettingsVO {
  [key: string]: unknown
}

// ========== 工具函数 ==========

/**
 * 将 Settings 转换为 SettingsVO
 */
function toSettingsVO(dto: Settings): SettingsVO {
  // Settings 是 SettingsVO 的超集，直接返回
  return dto as unknown as SettingsVO
}

// ========== API 方法 ==========

export async function settingsGetSettings(): Promise<ApiResponse<SettingsVO>> {
  const result = await SettingsHandler.Get()
  if (!result) {
    return { success: false, msg: '获取失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '获取失败' }
  }
  return { success: true, msg: result.msg ?? '', data: result.data ? toSettingsVO(result.data) : undefined }
}

export async function settingsSaveSettings(changes: SettingChange[]): Promise<ApiResponse<boolean>> {
  const result = await SettingsHandler.Save(changes)
  if (!result) {
    return { success: false, msg: '保存失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '', data: result.success }
}

export async function settingsResetSettings(): Promise<ApiResponse<boolean>> {
  const result = await SettingsHandler.Reset()
  if (!result) {
    return { success: false, msg: '重置失败：接口返回为空' }
  }
  return { success: result.success, msg: result.msg ?? '', data: result.success }
}