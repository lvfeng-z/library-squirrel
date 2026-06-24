/**
 * 插件设置 HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import { SettingHandler } from '@bindings/github.com/library-squirrel/backend/plugin'
import { SettingItem } from '@bindings/github.com/library-squirrel/backend/plugin/models'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

/** 获取插件设置项（声明 + 当前值） */
export async function pluginSettingGetSettings(publicId: string): Promise<ApiResult<SettingItem[]>> {
  return requireResponse(await SettingHandler.GetSettings(publicId), '获取插件设置')
}

/** 保存单个插件设置项 */
export async function pluginSettingSave(publicId: string, key: string, value: string): Promise<ApiResult<any>> {
  return requireResponse(await SettingHandler.SaveSetting(publicId, key, value), '保存插件设置', false)
}

/** 重置插件设置项为默认值 */
export async function pluginSettingReset(publicId: string, key: string): Promise<ApiResult<any>> {
  return requireResponse(await SettingHandler.ResetSetting(publicId, key), '重置插件设置', false)
}
