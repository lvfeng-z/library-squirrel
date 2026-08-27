/**
 * Share HTTP API 包装器
 * 直接调用 bindings 接口
 */

import { Handler as ShareHandler } from '@bindings/github.com/library-squirrel/backend/share'
import type { SharePublishOptions, ShareProtocolRegStatus, ShareSessionDTO } from '@bindings/github.com/library-squirrel/backend/share/models'
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
  const result = requireResponse(await ShareHandler.ShareSessions(), '查询分享列表')
  // 绑定层元素类型含 null（[]*T 的 JSON 形态），后端不会产出 null 元素，过滤兜底
  return result.data.filter((s): s is ShareSessionDTO => s !== null)
}

/**
 * 启动收件拉取：解析分享链接（深链或 https 分享链接，可含访问密码）→ 创建并启动
 * share-receive 任务，返回任务 ID（进度/终态由任务面板承载）。
 * 链接形态/密钥缺失等前置错误由 requireResponse 抛出 Error，调用方 try/catch 捕获。
 */
export async function shareReceive(link: string, password: string): Promise<number> {
  const result = requireResponse(await ShareHandler.ShareReceive(link, password), '启动拉取分享')
  return result.data
}

/**
 * 取走深链到达时缓存的待处理链接（空串=无待处理；冷启动期深链事件先于前端就绪的兜底）。
 */
export async function shareConsumePendingLink(): Promise<string> {
  const result = requireResponse(await ShareHandler.ShareConsumePendingLink(), '读取待处理分享链接')
  return result.data ?? ''
}

/**
 * 查询深链协议注册状态（Windows 为 HKCU 自注册视图）。
 */
export async function shareProtocolStatus(): Promise<ShareProtocolRegStatus> {
  const result = requireResponse(await ShareHandler.ShareProtocolRegStatus(), '查询深链协议注册状态')
  return result.data
}

/**
 * 取消深链协议注册（便携版无卸载器的清理入口）。
 */
export async function shareUnregisterProtocol(): Promise<void> {
  requireResponse(await ShareHandler.ShareUnregisterProtocol(), '取消深链协议注册', false)
}
