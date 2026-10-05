/**
 * 插件偏好 HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import { Handler as PluginPreferenceHandler } from '@bindings/github.com/library-squirrel/backend/pluginpreference'
import { PluginPreferenceEntryDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

/** 获取全量插件偏好条目（含归属插件公开 ID/显示名，记忆管理页「插件偏好」分区按插件分组消费） */
export async function pluginPreferenceListAll(): Promise<ApiResult<PluginPreferenceEntryDTO[]>> {
  const result = requireResponse(await PluginPreferenceHandler.ListAllPreferences(), '获取插件偏好')
  const data = result.data?.filter((item): item is PluginPreferenceEntryDTO => item !== null) ?? []
  return { success: true as const, msg: result.msg, data }
}

/** 按插件公开 ID 获取该插件偏好条目（插件设置区只读列表消费） */
export async function pluginPreferenceListByPlugin(pluginPublicId: string): Promise<ApiResult<PluginPreferenceEntryDTO[]>> {
  const result = requireResponse(await PluginPreferenceHandler.ListPreferencesByPlugin(pluginPublicId), '获取插件偏好')
  const data = result.data?.filter((item): item is PluginPreferenceEntryDTO => item !== null) ?? []
  return { success: true as const, msg: result.msg, data }
}

/** 删除一条插件偏好（管理面唯一删除入口；删除即忘掉，插件下次问答重新发起） */
export async function pluginPreferenceDelete(id: number): Promise<ApiResult<void>> {
  return requireResponse(await PluginPreferenceHandler.DeletePreference(id), '删除插件偏好', false)
}
