/**
 * PluginTaskUrlListener HTTP API 包装器
 * 封装 Wails 绑定层响应校验，校验失败时抛出异常，调用方通过 try/catch 捕获
 */

import { Handler as PluginTaskUrlListenerHandler } from '@bindings/github.com/library-squirrel/backend/pluginTaskUrlListener'
import type { ApiResult } from '@renderer/apis/http/types'
import { requireResponse } from '@renderer/apis/http/types'

export interface PluginWithContributionVO {
  id: number
  publicId: string
  name: string
  version: string
  author: string
  enable: boolean
  createTime: number
  updateTime: number
  contributeKey: string
  contributionID: string
}

// ========== API 方法 ==========

/** 根据URL获取监听此链接的插件列表 */
export async function listListener(url: string): Promise<ApiResult<PluginWithContributionVO[]>> {
  const result = requireResponse(
    await PluginTaskUrlListenerHandler.ListListener(url),
    '查询插件监听'
  )
  const data = result.data?.map((item) => {
    if (!item) return null
    return {
      id: item.id,
      publicId: item.publicId?.String ?? '',
      name: item.name?.String ?? '',
      version: item.version?.String ?? '',
      author: item.author?.String ?? '',
      enable: item.uninstalled?.Bool,
      createTime: item.createTime,
      updateTime: item.updateTime,
      contributeKey: item.ContributeKey,
      contributionID: item.ContributionID
    } as PluginWithContributionVO
  }).filter((item): item is PluginWithContributionVO => item !== null) ?? []
  return { success: true as const, msg: result.msg, data }
}
