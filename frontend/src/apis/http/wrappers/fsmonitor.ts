/**
 * fsmonitor 工作目录监控 HTTP API 包装器
 * 使用 requireResponse + ApiResult 统一响应校验
 */

import { requireResponse, type ApiResult } from '../types'
import { Handler as FsmonitorHandler } from '@bindings/github.com/library-squirrel/backend/fsmonitor'
import type { PendingChangeDTO } from '@bindings/github.com/library-squirrel/backend/fsmonitor/models'

// ========== 查询操作 ==========

/** 查询待修复变更列表（供确认弹窗展示） */
export async function fsmonitorListPending(): Promise<ApiResult<PendingChangeDTO[]>> {
  const result = requireResponse(await FsmonitorHandler.ListPendingChanges(), '查询待修复变更')
  const data = result.data?.filter((item): item is PendingChangeDTO => item !== null) ?? []
  return { success: true as const, msg: result.msg, data }
}

// ========== 修复确认 ==========

/**
 * 确认修复动作
 * action: 'sync'(同步DB路径,用于Move) | 'restore'(复原) | 'ack'(确认/标记失效,用于Delete)
 */
export async function fsmonitorConfirmChange(id: number, action: string): Promise<ApiResult<void>> {
  return requireResponse(await FsmonitorHandler.ConfirmChange(id, action), '确认修复', false)
}
