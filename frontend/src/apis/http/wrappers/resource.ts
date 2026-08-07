/**
 * Resource HTTP API 包装器
 * 直接调用 bindings 接口
 */

import { Handler as ResourceHandler } from '@bindings/github.com/library-squirrel/backend/resource'
import { requireResponse } from '../types'

/**
 * 启动指定 Resource 的音视频合并（异步：立即返回，进度与结果经 merge-events 事件推送）。
 * 失败（缺轨/ffmpeg 不可用/已合并/已在合并中）由 requireResponse 抛出 Error，调用方 try/catch 捕获。
 */
export async function resourceMerge(resourceId: number): Promise<void> {
  const result = await ResourceHandler.MergeResource(resourceId)
  requireResponse(result, '合并音视频', false)
}

/**
 * 取消指定 Resource 的进行中合并（无进行中合并则 no-op）。
 */
export async function resourceMergeCancel(resourceId: number): Promise<void> {
  const result = await ResourceHandler.MergeCancel(resourceId)
  requireResponse(result, '取消合并', false)
}
