/**
 * PluginTaskUrlListener HTTP API 包装器
 * 直接调用 bindings 接口
 */

import type { ApiResponse } from '../types'
import { Handler as PluginTaskUrlListenerHandler } from '@bindings/github.com/library-squirrel/wails/internal/pluginTaskUrlListener'

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
  contributionId: string
}

// ========== API 方法 ==========

/**
 * 根据URL获取监听此链接的插件列表
 */
export async function listListener(url: string): Promise<ApiResponse<PluginWithContributionVO[]>> {
  const result = await PluginTaskUrlListenerHandler.ListListener(url)
  if (!result) {
    return { success: false, msg: '查询失败：接口返回为空' }
  }
  if (!result.success) {
    return { success: false, msg: result.msg ?? '查询失败' }
  }
  // 转换结果
  const data = result.data?.map((item) => {
    if (!item) return null
    return {
      id: item.id,
      publicId: item.publicId?.String ?? '',
      name: item.name?.String ?? '',
      version: item.version?.String ?? '',
      author: item.author?.String ?? '',
      enable: item.uninstalled?.Int64 === 0,
      createTime: item.createTime,
      updateTime: item.updateTime,
      contributeKey: item.ContributeKey,
      contributionId: item.ContributionID
    } as PluginWithContributionVO
  }).filter((item): item is PluginWithContributionVO => item !== null) ?? []
  return { success: true, msg: result.msg ?? '', data }
}
