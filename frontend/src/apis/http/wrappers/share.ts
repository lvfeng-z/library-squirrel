/**
 * Share HTTP API 包装器
 * 直接调用 bindings 接口
 */

import { Handler as ShareHandler } from '@bindings/github.com/library-squirrel/backend/share'
import type { SharePublishOptions, ShareSessionDTO } from '@bindings/github.com/library-squirrel/backend/share/models'
import { requireResponse } from '../types'

/**
 * 启动分享发布（异步：立即返回 shareID，进度/完成/会话状态经 share-events 事件推送）。
 * 空选择/中继未配置等前置错误由 requireResponse 抛出 Error，调用方 try/catch 捕获。
 */
export async function sharePublish(
  workIds: number[],
  workSetIds: number[],
  options: SharePublishOptions
): Promise<string> {
  const result = await requireResponse(
    await ShareHandler.SharePublish(workIds, workSetIds, options),
    '启动分享'
  )
  return result.data
}

/**
 * 取消进行中的发布（无进行中发布则 no-op）。
 */
export async function shareCancelPublish(shareId: string): Promise<void> {
  requireResponse(await ShareHandler.ShareCancelPublish(shareId), '取消分享发布', false)
}

/**
 * 撤销分享会话（在线即在中继即时生效，后续拨号被拒）。
 */
export async function shareRevoke(shareId: string): Promise<void> {
  requireResponse(await ShareHandler.ShareRevoke(shareId), '撤销分享', false)
}

/**
 * 查询全部分享会话快照（含终态）。
 */
export async function shareSessions(): Promise<ShareSessionDTO[]> {
  const result = await requireResponse(await ShareHandler.ShareSessions(), '查询分享列表')
  // 绑定层元素类型含 null（[]*T 的 JSON 形态），后端不会产出 null 元素，过滤兜底
  return result.data.filter((s): s is ShareSessionDTO => s !== null)
}
