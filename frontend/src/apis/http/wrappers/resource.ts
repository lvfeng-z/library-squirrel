/**
 * Resource HTTP API 包装器
 * 直接调用 bindings 接口
 */

import { Handler as ResourceHandler } from '@bindings/github.com/library-squirrel/backend/resource'
import { requireResponse } from '../types'
import type { MergeResult } from '@bindings/github.com/library-squirrel/backend/resource'

/**
 * 合并指定 Resource 的音视频轨，产出可播放的单文件。
 * 失败（缺轨/ffmpeg 不可用）由 requireResponse 抛出 Error，调用方 try/catch 捕获。
 */
export async function resourceMerge(resourceId: number): Promise<MergeResult> {
  const result = await ResourceHandler.MergeResource(resourceId)
  return requireResponse<MergeResult>(result, '合并音视频', true).data
}
